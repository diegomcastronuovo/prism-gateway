package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── Parser tests ──────────────────────────────────────────────────────────────

func TestParseGeminiGenerateContentRequest_HappyPath(t *testing.T) {
	maxTok := 1024
	temp := 0.7
	topP := 0.9
	body := `{
		"contents": [{"role":"user","parts":[{"text":"Hello"}]}],
		"systemInstruction": {"parts":[{"text":"Be helpful."}]},
		"generationConfig": {"maxOutputTokens":1024,"temperature":0.7,"topP":0.9}
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-1.5-pro:generateContent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("model", "gemini-1.5-pro")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseGeminiGenerateContentRequest(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pr.BodyModel != "gemini-1.5-pro" {
		t.Errorf("BodyModel: want gemini-1.5-pro, got %q", pr.BodyModel)
	}
	if pr.APIStyle != APIStyleGemini {
		t.Errorf("APIStyle: want %q, got %q", APIStyleGemini, pr.APIStyle)
	}
	if pr.SystemPrompt != "Be helpful." {
		t.Errorf("SystemPrompt: want \"Be helpful.\", got %q", pr.SystemPrompt)
	}
	if pr.MaxTokens == nil || *pr.MaxTokens != maxTok {
		t.Errorf("MaxTokens: want %d, got %v", maxTok, pr.MaxTokens)
	}
	if pr.Temperature == nil || *pr.Temperature != temp {
		t.Errorf("Temperature: want %v, got %v", temp, pr.Temperature)
	}
	if pr.TopP == nil || *pr.TopP != topP {
		t.Errorf("TopP: want %v, got %v", topP, pr.TopP)
	}
	if len(pr.Messages) != 1 {
		t.Errorf("Messages: want 1, got %d", len(pr.Messages))
	}
	if pr.Stream {
		t.Error("Stream: want false")
	}
}

func TestParseGeminiGenerateContentRequest_ModelFromPath(t *testing.T) {
	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-pro:generateContent", strings.NewReader(body))
	req.SetPathValue("model", "gemini-pro")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseGeminiGenerateContentRequest(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.BodyModel != "gemini-pro" {
		t.Errorf("BodyModel: want gemini-pro, got %q", pr.BodyModel)
	}
}

func TestParseGeminiGenerateContentRequest_SystemInstruction(t *testing.T) {
	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"systemInstruction":{"parts":[{"text":"You are helpful."}]}}`

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-1.5-pro:generateContent", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseGeminiGenerateContentRequest(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.SystemPrompt != "You are helpful." {
		t.Errorf("SystemPrompt: want \"You are helpful.\", got %q", pr.SystemPrompt)
	}
}

func TestParseGeminiGenerateContentRequest_RoleMapping(t *testing.T) {
	body := `{
		"contents": [
			{"role":"user","parts":[{"text":"Hi"}]},
			{"role":"model","parts":[{"text":"Hello"}]}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-1.5-pro:generateContent", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseGeminiGenerateContentRequest(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pr.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(pr.Messages))
	}
	if pr.Messages[0].Role != "user" {
		t.Errorf("Messages[0].Role: want user, got %q", pr.Messages[0].Role)
	}
	if pr.Messages[1].Role != "assistant" {
		t.Errorf("Messages[1].Role: want assistant, got %q", pr.Messages[1].Role)
	}
}

func TestParseGeminiGenerateContentRequest_MissingModel_Error(t *testing.T) {
	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`

	req := httptest.NewRequest(http.MethodPost, "/v1/models/:generateContent", strings.NewReader(body))
	// Intentionally do not set path value — model will be ""
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	_, err := parseGeminiGenerateContentRequest(req, false)
	if err == nil {
		t.Fatal("expected error when model path param is empty, got nil")
	}
}

func TestParseGeminiGenerateContentRequest_AbsentGenerationConfig(t *testing.T) {
	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-1.5-pro:generateContent", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseGeminiGenerateContentRequest(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.MaxTokens != nil || pr.Temperature != nil || pr.TopP != nil {
		t.Error("MaxTokens, Temperature, TopP must be nil when generationConfig is absent")
	}
}

func TestParseGeminiGenerateContentRequest_MultiPartContent(t *testing.T) {
	body := `{
		"contents":[{"role":"user","parts":[{"text":"Hello"},{"text":" world"}]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-1.5-pro:generateContent", strings.NewReader(body))
	req.SetPathValue("model", "gemini-1.5-pro")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseGeminiGenerateContentRequest(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pr.Messages) != 1 {
		t.Fatalf("Messages: want 1, got %d", len(pr.Messages))
	}
	want := `"Hello\n world"`
	if string(pr.Messages[0].Content) != want {
		t.Errorf("multi-part content: want %s, got %s", want, string(pr.Messages[0].Content))
	}
}

func TestParseGeminiGenerateContentRequest_SafetySettingsPassthrough(t *testing.T) {
	safety := `[{"category":"HARM_CATEGORY_HATE_SPEECH","threshold":"BLOCK_LOW_AND_ABOVE"}]`
	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"safetySettings":` + safety + `}`
	req := httptest.NewRequest(http.MethodPost, "/v1/models/gemini-1.5-pro:generateContent", strings.NewReader(body))
	req.SetPathValue("model", "gemini-1.5-pro")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseGeminiGenerateContentRequest(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(pr.SafetySettings) != safety {
		t.Errorf("SafetySettings: want %s, got %s", safety, string(pr.SafetySettings))
	}
}

// ── Serializer tests ──────────────────────────────────────────────────────────

func TestCanonicalToGeminiResponse_HappyPath(t *testing.T) {
	cr := &CanonicalResponse{
		ID:    "chatcmpl-abc",
		Model: "gemini-1.5-pro",
		Choices: []providers.ChatChoice{
			{
				Index:        0,
				Message:      providers.ChatMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	resp := canonicalToGeminiResponse(cr)

	if len(resp.Candidates) != 1 {
		t.Fatalf("Candidates: want 1, got %d", len(resp.Candidates))
	}
	c := resp.Candidates[0]
	if c.Content.Role != "model" {
		t.Errorf("Content.Role: want model, got %q", c.Content.Role)
	}
	if len(c.Content.Parts) != 1 || c.Content.Parts[0].Text != "Hello!" {
		t.Errorf("Content.Parts[0].Text: want \"Hello!\", got %v", c.Content.Parts)
	}
	if c.FinishReason != "STOP" {
		t.Errorf("FinishReason: want STOP, got %q", c.FinishReason)
	}
	if resp.UsageMetadata == nil {
		t.Fatal("UsageMetadata must not be nil")
	}
	if resp.UsageMetadata.PromptTokenCount != 10 {
		t.Errorf("PromptTokenCount: want 10, got %d", resp.UsageMetadata.PromptTokenCount)
	}
	if resp.UsageMetadata.CandidatesTokenCount != 5 {
		t.Errorf("CandidatesTokenCount: want 5, got %d", resp.UsageMetadata.CandidatesTokenCount)
	}
	if resp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("TotalTokenCount: want 15, got %d", resp.UsageMetadata.TotalTokenCount)
	}
}

func TestFinishReasonToGemini_Mapping(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stop", "STOP"},
		{"", "STOP"},
		{"length", "MAX_TOKENS"},
		{"content_filter", "OTHER"},
		{"tool_calls", "OTHER"},
		{"unknown_reason", "OTHER"},
	}
	for _, tc := range cases {
		got := finishReasonToGemini(tc.in)
		if got != tc.want {
			t.Errorf("finishReasonToGemini(%q): want %q, got %q", tc.in, tc.want, got)
		}
	}
}

func TestCanonicalToGeminiResponse_EmptyChoices(t *testing.T) {
	cr := &CanonicalResponse{
		ID:      "chatcmpl-empty",
		Model:   "gemini-1.5-pro",
		Choices: []providers.ChatChoice{},
		Usage:   providers.Usage{PromptTokens: 5, CompletionTokens: 0, TotalTokens: 5},
	}

	resp := canonicalToGeminiResponse(cr)

	if resp.Candidates == nil {
		t.Error("Candidates must be non-nil even when empty")
	}
	if len(resp.Candidates) != 0 {
		t.Errorf("Candidates: want 0, got %d", len(resp.Candidates))
	}
	if resp.UsageMetadata == nil {
		t.Fatal("UsageMetadata must not be nil")
	}
	if resp.UsageMetadata.PromptTokenCount != 5 {
		t.Errorf("PromptTokenCount: want 5, got %d", resp.UsageMetadata.PromptTokenCount)
	}
}

// ── SSE sink tests ────────────────────────────────────────────────────────────

// parseGeminiSSELines returns the data payloads from SSE output (Gemini uses only data: lines).
func parseGeminiSSELines(body string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			lines = append(lines, strings.TrimPrefix(line, "data: "))
		}
	}
	return lines
}

func TestGeminiSSESink_DeltaChunkTranslation(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := newGeminiGenerateContentWriter(rec, "gemini-1.5-pro")

	chunks := []string{
		`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}` + "\n\n",
		`data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}` + "\n\n",
		"data: [DONE]\n\n",
	}

	for _, chunk := range chunks {
		if _, err := gw.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write error: %v", err)
		}
	}

	lines := parseGeminiSSELines(rec.Body.String())

	// Filter out [DONE] — keep only JSON data lines.
	var jsonLines []string
	for _, l := range lines {
		if l != "[DONE]" {
			jsonLines = append(jsonLines, l)
		}
	}

	// Expect: 2 delta chunks + 1 final chunk = 3 JSON data lines.
	if len(jsonLines) < 3 {
		t.Fatalf("expected at least 3 JSON data lines, got %d: %v", len(jsonLines), jsonLines)
	}

	// First delta should carry "Hello".
	var first map[string]interface{}
	if err := json.Unmarshal([]byte(jsonLines[0]), &first); err != nil {
		t.Fatalf("unmarshal first chunk: %v", err)
	}
	candidates, _ := first["candidates"].([]interface{})
	if len(candidates) == 0 {
		t.Fatal("first chunk: candidates must not be empty")
	}
	cand0 := candidates[0].(map[string]interface{})
	content := cand0["content"].(map[string]interface{})
	parts := content["parts"].([]interface{})
	if len(parts) == 0 {
		t.Fatal("first chunk: parts must not be empty")
	}
	text := parts[0].(map[string]interface{})["text"]
	if text != "Hello" {
		t.Errorf("first delta text: want \"Hello\", got %v", text)
	}
}

func TestGeminiSSESink_DoneWithoutPriorDeltas(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := newGeminiGenerateContentWriter(rec, "gemini-1.5-pro")

	if _, err := gw.Write([]byte("data: [DONE]\n\n")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	lines := parseGeminiSSELines(rec.Body.String())
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 data lines (final chunk + [DONE]), got %d: %v", len(lines), lines)
	}

	// Second line must be [DONE].
	if lines[len(lines)-1] != "[DONE]" {
		t.Errorf("last line: want [DONE], got %q", lines[len(lines)-1])
	}

	// First line must be valid JSON with finishReason STOP.
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("unmarshal final chunk: %v", err)
	}
	candidates, _ := payload["candidates"].([]interface{})
	if len(candidates) == 0 {
		t.Fatal("final chunk: candidates must not be empty")
	}
	finishReason := candidates[0].(map[string]interface{})["finishReason"]
	if finishReason != "STOP" {
		t.Errorf("finishReason in final chunk: want STOP, got %v", finishReason)
	}
}

func TestGeminiSSESink_FinalChunkHasUsageMetadata(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := newGeminiGenerateContentWriter(rec, "gemini-1.5-pro")

	if _, err := gw.Write([]byte("data: [DONE]\n\n")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	lines := parseGeminiSSELines(rec.Body.String())
	// First JSON line is the final chunk.
	var finalChunk map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &finalChunk); err != nil {
		t.Fatalf("unmarshal final chunk: %v", err)
	}
	if _, ok := finalChunk["usageMetadata"]; !ok {
		t.Error("final chunk must contain usageMetadata")
	}
}

func TestGeminiSSESink_FlusherCalledOnEveryWrite(t *testing.T) {
	var flushCount int
	flusher := &flushCounterRecorder{ResponseRecorder: httptest.NewRecorder(), count: &flushCount}
	gw := newGeminiGenerateContentWriter(flusher, "gemini-1.5-pro")

	chunks := []string{
		`data: {"choices":[{"delta":{"content":"A"},"finish_reason":null}]}` + "\n\n",
		"data: [DONE]\n\n",
	}
	for _, chunk := range chunks {
		if _, err := gw.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write error: %v", err)
		}
	}

	if flushCount == 0 {
		t.Error("Flusher.Flush() was never called")
	}
}

// flushCounterRecorder wraps httptest.ResponseRecorder and counts Flush() calls.
type flushCounterRecorder struct {
	*httptest.ResponseRecorder
	count *int
}

func (f *flushCounterRecorder) Flush() {
	*f.count++
	f.ResponseRecorder.Flush()
}

// ── Integration tests ─────────────────────────────────────────────────────────

func makeGeminiRequest(t *testing.T, handler http.Handler, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestGeminiGenerateContentHandler_NonStreaming(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"contents":[{"role":"user","parts":[{"text":"ping"}]}]}`
	w := makeGeminiRequest(t, handler, "/v1/models/model-a:generateContent", body,
		map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}

	var resp GeminiGenerateContentResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Candidates) == 0 {
		t.Error("candidates must be non-empty")
	} else {
		if resp.Candidates[0].Content.Role != "model" {
			t.Errorf("candidates[0].content.role: want model, got %q", resp.Candidates[0].Content.Role)
		}
	}
}

func TestGeminiGenerateContentHandler_Streaming(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello"},
		{Type: "delta", Content: " World"},
	}}
	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	w := makeGeminiRequest(t, handler, "/v1/models/model-a:streamGenerateContent", body,
		map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type: want text/event-stream, got %q", ct)
	}

	lines := parseGeminiSSELines(w.Body.String())
	if len(lines) == 0 {
		t.Error("expected at least one SSE data line")
	}
	// Must end with [DONE].
	last := lines[len(lines)-1]
	if last != "[DONE]" {
		t.Errorf("last SSE line: want [DONE], got %q", last)
	}
}

func TestGeminiGenerateContentHandler_CoexistsWithListModels(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	// GET /v1/models must still work alongside the new POST routes.
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-API-Key", "key1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/models: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGeminiGenerateContentHandler_InvalidBody(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("{bad json")

	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	handler := setupTestServer(cfg, reg)

	req := httptest.NewRequest(http.MethodPost, "/v1/models/model-a:generateContent", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "key1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
