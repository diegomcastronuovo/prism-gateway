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

func TestParseResponsesAPIRequest_FullBody(t *testing.T) {
	maxTok := 100
	temp := 0.7
	body := `{
		"model": "gpt-4o-mini",
		"input": [{"role":"user","content":"Hello"}],
		"instructions": "Be concise.",
		"temperature": 0.7,
		"max_output_tokens": 100,
		"stream": false
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseResponsesAPIRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pr.BodyModel != "gpt-4o-mini" {
		t.Errorf("BodyModel: want gpt-4o-mini, got %q", pr.BodyModel)
	}
	if pr.SystemPrompt != "Be concise." {
		t.Errorf("SystemPrompt: want \"Be concise.\", got %q", pr.SystemPrompt)
	}
	if pr.APIStyle != APIStyleOpenAIResponses {
		t.Errorf("APIStyle: want %q, got %q", APIStyleOpenAIResponses, pr.APIStyle)
	}
	if pr.MaxTokens == nil || *pr.MaxTokens != maxTok {
		t.Errorf("MaxTokens: want %d, got %v", maxTok, pr.MaxTokens)
	}
	if pr.Temperature == nil || *pr.Temperature != temp {
		t.Errorf("Temperature: want %v, got %v", temp, pr.Temperature)
	}
	if len(pr.Messages) != 1 {
		t.Errorf("Messages: want 1, got %d", len(pr.Messages))
	}
}

func TestParseResponsesAPIRequest_PreviousResponseIDIgnored(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"input": [{"role":"user","content":"Hi"}],
		"previous_response_id": "resp_abc123"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseResponsesAPIRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No field in ParsedRequest should contain the previous_response_id value.
	if pr.BodyModel == "resp_abc123" {
		t.Error("previous_response_id leaked into BodyModel")
	}
	if pr.SystemPrompt == "resp_abc123" {
		t.Error("previous_response_id leaked into SystemPrompt")
	}
	// The field is silently dropped — no error returned.
}

func TestParseResponsesAPIRequest_InstructionsAbsent(t *testing.T) {
	body := `{"model":"gpt-4o","input":[{"role":"user","content":"hi"}]}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseResponsesAPIRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.SystemPrompt != "" {
		t.Errorf("SystemPrompt: want empty string, got %q", pr.SystemPrompt)
	}
}

func TestParseResponsesAPIRequest_MalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{invalid"))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	_, err := parseResponsesAPIRequest(req)
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

// ── Serializer tests ──────────────────────────────────────────────────────────

func TestCanonicalToResponsesAPIResponse_StopFinish(t *testing.T) {
	cr := &CanonicalResponse{
		ID:      "chatcmpl-abc",
		Created: 1234567890,
		Model:   "gpt-4o-mini",
		Choices: []providers.ChatChoice{
			{
				Index:        0,
				Message:      providers.ChatMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	resp := canonicalToResponsesAPIResponse(cr)

	if resp.Object != "response" {
		t.Errorf("Object: want \"response\", got %q", resp.Object)
	}
	if resp.Status != "completed" {
		t.Errorf("Status: want \"completed\", got %q", resp.Status)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("Output len: want 1, got %d", len(resp.Output))
	}
	if resp.Output[0].Type != "message" {
		t.Errorf("Output[0].Type: want \"message\", got %q", resp.Output[0].Type)
	}
	if len(resp.Output[0].Content) != 1 {
		t.Fatalf("Output[0].Content len: want 1, got %d", len(resp.Output[0].Content))
	}
	if resp.Output[0].Content[0].Type != "output_text" {
		t.Errorf("Content[0].Type: want \"output_text\", got %q", resp.Output[0].Content[0].Type)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens: want 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("Usage.OutputTokens: want 5, got %d", resp.Usage.OutputTokens)
	}
}

func TestCanonicalToResponsesAPIResponse_LengthFinish(t *testing.T) {
	cr := &CanonicalResponse{
		ID:    "chatcmpl-xyz",
		Model: "gpt-4o",
		Choices: []providers.ChatChoice{
			{
				Message:      providers.ChatMessage{Role: "assistant", Content: "Truncated..."},
				FinishReason: "length",
			},
		},
	}

	resp := canonicalToResponsesAPIResponse(cr)

	if resp.Status != "incomplete" {
		t.Errorf("Status: want \"incomplete\", got %q", resp.Status)
	}
	if resp.Output[0].Status != "incomplete" {
		t.Errorf("Output[0].Status: want \"incomplete\", got %q", resp.Output[0].Status)
	}
}

func TestCanonicalToResponsesAPIResponse_NilRawProvider(t *testing.T) {
	// RawProviderResponse nil + Choices populated → no panic, output built from Choices.
	cr := &CanonicalResponse{
		ID:                  "chatcmpl-cache",
		Model:               "gpt-4o-mini",
		RawProviderResponse: nil,
		Choices: []providers.ChatChoice{
			{
				Message:      providers.ChatMessage{Role: "assistant", Content: "cached"},
				FinishReason: "stop",
			},
		},
	}

	resp := canonicalToResponsesAPIResponse(cr) // must not panic
	if len(resp.Output) != 1 {
		t.Fatalf("Output len: want 1, got %d", len(resp.Output))
	}
	if resp.Output[0].Content[0].Text != "cached" {
		t.Errorf("Text: want \"cached\", got %q", resp.Output[0].Content[0].Text)
	}
}

// ── Streaming writer tests ─────────────────────────────────────────────────────

// parseResponsesSSEEvents extracts event types and data payloads from SSE output.
type responsesSSEEvent struct {
	EventType string
	Data      string
}

func parseResponsesSSEEvents(body string) []responsesSSEEvent {
	var events []responsesSSEEvent
	var current responsesSSEEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			current.EventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.Data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if current.EventType != "" {
				events = append(events, current)
				current = responsesSSEEvent{}
			}
		}
	}
	return events
}

func TestResponsesAPIStreamingWriter_EventSequence(t *testing.T) {
	var buf bytes.Buffer
	// Use a minimal ResponseWriter wrapper.
	rec := httptest.NewRecorder()

	rw := newResponsesAPIWriter(rec, "gpt-4o-mini")

	// Simulate writeSSEStream output: 3 delta chunks + [DONE].
	chunks := []string{
		`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}` + "\n\n",
		`data: {"choices":[{"delta":{"content":" World"},"finish_reason":null}]}` + "\n\n",
		`data: {"choices":[{"delta":{"content":"!"},"finish_reason":null}]}` + "\n\n",
		"data: [DONE]\n\n",
	}

	for _, chunk := range chunks {
		if _, err := rw.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write error: %v", err)
		}
	}

	buf.Write(rec.Body.Bytes())
	events := parseResponsesSSEEvents(buf.String())

	// Verify the required event sequence.
	eventTypes := make([]string, 0, len(events))
	for _, e := range events {
		eventTypes = append(eventTypes, e.EventType)
	}

	// Must contain these event types (in order).
	required := []string{
		"response.created",
		"response.output_item.added",
	}
	for _, r := range required {
		found := false
		for _, et := range eventTypes {
			if et == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required event %q in: %v", r, eventTypes)
		}
	}

	// Count deltas.
	deltaCount := 0
	for _, et := range eventTypes {
		if et == "response.output_text.delta" {
			deltaCount++
		}
	}
	if deltaCount != 3 {
		t.Errorf("expected 3 delta events, got %d", deltaCount)
	}

	// Must end with done + completed.
	doneFound := false
	completedFound := false
	for _, et := range eventTypes {
		if et == "response.output_text.done" {
			doneFound = true
		}
		if et == "response.completed" {
			completedFound = true
		}
	}
	if !doneFound {
		t.Error("missing response.output_text.done event")
	}
	if !completedFound {
		t.Error("missing response.completed event")
	}

	// Verify order: created before output_item.added before first delta.
	createdIdx := -1
	addedIdx := -1
	firstDeltaIdx := -1
	doneIdx := -1
	completedIdx := -1
	for i, et := range eventTypes {
		switch et {
		case "response.created":
			if createdIdx < 0 {
				createdIdx = i
			}
		case "response.output_item.added":
			if addedIdx < 0 {
				addedIdx = i
			}
		case "response.output_text.delta":
			if firstDeltaIdx < 0 {
				firstDeltaIdx = i
			}
		case "response.output_text.done":
			if doneIdx < 0 {
				doneIdx = i
			}
		case "response.completed":
			if completedIdx < 0 {
				completedIdx = i
			}
		}
	}
	if createdIdx >= addedIdx {
		t.Errorf("response.created (%d) must come before response.output_item.added (%d)", createdIdx, addedIdx)
	}
	if addedIdx >= firstDeltaIdx {
		t.Errorf("response.output_item.added (%d) must come before first delta (%d)", addedIdx, firstDeltaIdx)
	}
	if doneIdx >= completedIdx {
		t.Errorf("response.output_text.done (%d) must come before response.completed (%d)", doneIdx, completedIdx)
	}
}

func TestResponsesAPIStreamingWriter_DeltaPayloadFields(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newResponsesAPIWriter(rec, "gpt-4o-mini")

	chunk := `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}` + "\n\n"
	if _, err := rw.Write([]byte(chunk)); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	events := parseResponsesSSEEvents(rec.Body.String())

	// Find the first delta event.
	var deltaData string
	for _, e := range events {
		if e.EventType == "response.output_text.delta" {
			deltaData = e.Data
			break
		}
	}
	if deltaData == "" {
		t.Fatal("no response.output_text.delta event found")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(deltaData), &payload); err != nil {
		t.Fatalf("unmarshal delta payload: %v", err)
	}

	if payload["delta"] != "Hello" {
		t.Errorf("delta: want \"Hello\", got %v", payload["delta"])
	}
	if payload["output_index"] != float64(0) {
		t.Errorf("output_index: want 0, got %v", payload["output_index"])
	}
	if payload["content_index"] != float64(0) {
		t.Errorf("content_index: want 0, got %v", payload["content_index"])
	}
	if _, ok := payload["item_id"]; !ok {
		t.Error("item_id field is missing from delta payload")
	}
}

// ── Integration tests ─────────────────────────────────────────────────────────

func makeResponsesRequest(t *testing.T, handler http.Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestResponsesAPIHandler_NonStreaming(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","input":[{"role":"user","content":"ping"}]}`
	w := makeResponsesRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}

	var resp ResponsesAPIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Object != "response" {
		t.Errorf("object: want \"response\", got %q", resp.Object)
	}
	if len(resp.Output) == 0 {
		t.Error("output must be non-empty")
	}
}

func TestResponsesAPIHandler_InvalidBody(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	handler := setupTestServer(cfg, reg)

	w := makeResponsesRequest(t, handler, "{bad json", map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestResponsesAPIHandler_StreamingEmitsEvents(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello"},
		{Type: "delta", Content: " World"},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","input":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeResponsesRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type: want text/event-stream, got %q", ct)
	}

	events := parseResponsesSSEEvents(w.Body.String())
	eventTypes := make([]string, 0, len(events))
	for _, e := range events {
		eventTypes = append(eventTypes, e.EventType)
	}

	// At minimum response.created and response.completed must be present.
	createdFound := false
	completedFound := false
	for _, et := range eventTypes {
		if et == "response.created" {
			createdFound = true
		}
		if et == "response.completed" {
			completedFound = true
		}
	}
	if !createdFound {
		t.Errorf("missing response.created in events: %v", eventTypes)
	}
	if !completedFound {
		t.Errorf("missing response.completed in events: %v", eventTypes)
	}
}
