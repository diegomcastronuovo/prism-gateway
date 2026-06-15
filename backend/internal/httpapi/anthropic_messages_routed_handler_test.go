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

func TestParseAnthropicMessagesRequest_Valid(t *testing.T) {
	maxTok := 1024
	temp := 0.5
	body := `{
		"model": "claude-3-5-sonnet-20241022",
		"messages": [{"role": "user", "content": "Hello, world!"}],
		"system": "You are a helpful assistant.",
		"max_tokens": 1024,
		"temperature": 0.5,
		"stream": false
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseAnthropicMessagesRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pr.APIStyle != APIStyleAnthropicMessages {
		t.Errorf("APIStyle: want %q, got %q", APIStyleAnthropicMessages, pr.APIStyle)
	}
	if pr.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("SystemPrompt: want \"You are a helpful assistant.\", got %q", pr.SystemPrompt)
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
	if pr.Stream {
		t.Error("Stream: want false, got true")
	}
}

func TestParseAnthropicMessagesRequest_MissingMaxTokens(t *testing.T) {
	body := `{
		"model": "claude-3-5-sonnet-20241022",
		"messages": [{"role": "user", "content": "hi"}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	_, err := parseAnthropicMessagesRequest(req)
	if err == nil {
		t.Fatal("expected error when max_tokens is absent, got nil")
	}
	if !strings.Contains(err.Error(), "max_tokens") {
		t.Errorf("error message should mention max_tokens, got: %v", err)
	}
}

func TestParseAnthropicMessagesRequest_StringContent(t *testing.T) {
	body := `{
		"model": "claude-3-5-sonnet-20241022",
		"messages": [{"role": "user", "content": "Hello"}],
		"max_tokens": 100
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseAnthropicMessagesRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pr.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(pr.Messages))
	}
	if pr.Messages[0].TextContent() != "Hello" {
		t.Errorf("TextContent: want \"Hello\", got %q", pr.Messages[0].TextContent())
	}
}

func TestParseAnthropicMessagesRequest_BlockArrayContent(t *testing.T) {
	// Block array: only type="text" blocks should be concatenated; image block should be dropped.
	body := `{
		"model": "claude-3-5-sonnet-20241022",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Hello"},
				{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "abc"}},
				{"type": "text", "text": " World"}
			]
		}],
		"max_tokens": 100
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseAnthropicMessagesRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pr.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(pr.Messages))
	}
	// extractTextFromContent concatenates text blocks with "\n" between them (for non-text, it adds "[image]").
	// We only assert the two text parts are present.
	content := pr.Messages[0].TextContent()
	if !strings.Contains(content, "Hello") {
		t.Errorf("content should contain \"Hello\", got %q", content)
	}
	if !strings.Contains(content, "World") {
		t.Errorf("content should contain \"World\", got %q", content)
	}
}

func TestParseAnthropicMessagesRequest_StopSequences(t *testing.T) {
	body := `{
		"model": "claude-3-5-sonnet-20241022",
		"messages": [{"role": "user", "content": "hi"}],
		"max_tokens": 100,
		"stop_sequences": ["STOP", "END"]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)

	pr, err := parseAnthropicMessagesRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := pr.ProviderOptions["stop_sequences"]
	if !ok {
		t.Fatal("ProviderOptions[stop_sequences] should be set")
	}
	var seqs []string
	if err := json.Unmarshal(val, &seqs); err != nil {
		t.Fatalf("unmarshal stop_sequences: %v", err)
	}
	if len(seqs) != 2 || seqs[0] != "STOP" || seqs[1] != "END" {
		t.Errorf("stop_sequences: want [\"STOP\",\"END\"], got %v", seqs)
	}
}

// ── Serializer tests ──────────────────────────────────────────────────────────

func TestCanonicalToAnthropicMessagesResponse_StopFinish(t *testing.T) {
	cr := &CanonicalResponse{
		ID:    "chatcmpl-abc",
		Model: "claude-3-5-sonnet-20241022",
		Choices: []providers.ChatChoice{
			{
				Index:        0,
				Message:      providers.ChatMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	resp := canonicalToAnthropicMessagesResponse(cr)

	if resp.Type != "message" {
		t.Errorf("Type: want \"message\", got %q", resp.Type)
	}
	if resp.Role != "assistant" {
		t.Errorf("Role: want \"assistant\", got %q", resp.Role)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason: want \"end_turn\", got %q", resp.StopReason)
	}
	if resp.StopSequence != nil {
		t.Errorf("StopSequence: want nil, got %v", resp.StopSequence)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" {
		t.Errorf("Content: want [{type:text}], got %v", resp.Content)
	}
	if resp.Content[0].Text != "Hello!" {
		t.Errorf("Content[0].Text: want \"Hello!\", got %q", resp.Content[0].Text)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens: want 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("Usage.OutputTokens: want 5, got %d", resp.Usage.OutputTokens)
	}
}

func TestFinishReasonToStopReason_Mapping(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stop", "end_turn"},
		{"length", "max_tokens"},
		{"tool_calls", "tool_use"},
		{"content_filter", "end_turn"},
		{"", "end_turn"},
		{"unknown_reason", "end_turn"},
	}
	for _, tc := range cases {
		got := finishReasonToStopReason(tc.in)
		if got != tc.want {
			t.Errorf("finishReasonToStopReason(%q): want %q, got %q", tc.in, tc.want, got)
		}
	}
}

func TestCanonicalToAnthropicMessagesResponse_LengthFinish(t *testing.T) {
	cr := &CanonicalResponse{
		ID:    "chatcmpl-xyz",
		Model: "claude-3-haiku",
		Choices: []providers.ChatChoice{
			{
				Message:      providers.ChatMessage{Role: "assistant", Content: "Truncated..."},
				FinishReason: "length",
			},
		},
	}

	resp := canonicalToAnthropicMessagesResponse(cr)

	if resp.StopReason != "max_tokens" {
		t.Errorf("StopReason: want \"max_tokens\", got %q", resp.StopReason)
	}
}

func TestCanonicalToAnthropicMessagesResponse_ToolCallsFinish(t *testing.T) {
	cr := &CanonicalResponse{
		ID:    "chatcmpl-tools",
		Model: "claude-3-opus",
		Choices: []providers.ChatChoice{
			{
				Message:      providers.ChatMessage{Role: "assistant", Content: ""},
				FinishReason: "tool_calls",
			},
		},
	}

	resp := canonicalToAnthropicMessagesResponse(cr)

	if resp.StopReason != "tool_use" {
		t.Errorf("StopReason: want \"tool_use\", got %q", resp.StopReason)
	}
}

// ── SSE writer tests ──────────────────────────────────────────────────────────

// anthropicSSEEvent holds one parsed SSE event.
type anthropicSSEEvent struct {
	EventType string
	Data      string
}

func parseAnthropicSSEEvents(body string) []anthropicSSEEvent {
	var events []anthropicSSEEvent
	var current anthropicSSEEvent
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
				current = anthropicSSEEvent{}
			}
		}
	}
	return events
}

func TestAnthropicMessagesWriter_EventSequence(t *testing.T) {
	var buf bytes.Buffer
	rec := httptest.NewRecorder()

	aw := newAnthropicMessagesWriter(rec, "claude-3-5-sonnet-20241022")

	// Simulate writeSSEStream output: 2 delta chunks + [DONE].
	chunks := []string{
		`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}` + "\n\n",
		`data: {"choices":[{"delta":{"content":" World"},"finish_reason":null}]}` + "\n\n",
		"data: [DONE]\n\n",
	}

	for _, chunk := range chunks {
		if _, err := aw.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write error: %v", err)
		}
	}

	buf.Write(rec.Body.Bytes())
	events := parseAnthropicSSEEvents(buf.String())

	eventTypes := make([]string, 0, len(events))
	for _, e := range events {
		eventTypes = append(eventTypes, e.EventType)
	}

	// Required events that must appear.
	required := []string{
		"message_start",
		"content_block_start",
		"ping",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
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
			t.Errorf("missing required event %q; got: %v", r, eventTypes)
		}
	}

	// Ordering: message_start must be first.
	if len(eventTypes) == 0 || eventTypes[0] != "message_start" {
		t.Errorf("first event: want message_start, got %q (all events: %v)", eventTypes[0], eventTypes)
	}
	// message_stop must be last.
	if last := eventTypes[len(eventTypes)-1]; last != "message_stop" {
		t.Errorf("last event: want message_stop, got %q", last)
	}

	// Count deltas: should be 2 (one per chunk).
	deltaCount := 0
	for _, et := range eventTypes {
		if et == "content_block_delta" {
			deltaCount++
		}
	}
	if deltaCount != 2 {
		t.Errorf("expected 2 content_block_delta events, got %d", deltaCount)
	}
}

func TestAnthropicMessagesWriter_NoDeltasThenDone(t *testing.T) {
	rec := httptest.NewRecorder()
	aw := newAnthropicMessagesWriter(rec, "claude-3-haiku")

	// [DONE] with no prior deltas — should still emit preamble events.
	if _, err := aw.Write([]byte("data: [DONE]\n\n")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	events := parseAnthropicSSEEvents(rec.Body.String())
	eventTypes := make([]string, 0, len(events))
	for _, e := range events {
		eventTypes = append(eventTypes, e.EventType)
	}

	// Even with no deltas, preamble + close events must fire.
	for _, required := range []string{"message_start", "content_block_start", "ping", "content_block_stop", "message_delta", "message_stop"} {
		found := false
		for _, et := range eventTypes {
			if et == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing event %q when [DONE] arrives with no prior deltas; got: %v", required, eventTypes)
		}
	}
}

// ── Integration tests ─────────────────────────────────────────────────────────

func makeAnthropicMessagesRequest(t *testing.T, handler http.Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestAnthropicMessagesRoutedHandler_NonStreaming(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"ping"}],"max_tokens":100}`
	w := makeAnthropicMessagesRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}

	var resp AnthropicMessagesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Type != "message" {
		t.Errorf("type: want \"message\", got %q", resp.Type)
	}
	if resp.Role != "assistant" {
		t.Errorf("role: want \"assistant\", got %q", resp.Role)
	}
	if len(resp.Content) == 0 {
		t.Error("content must be non-empty")
	} else if resp.Content[0].Type != "text" {
		t.Errorf("content[0].type: want \"text\", got %q", resp.Content[0].Type)
	}
}

func TestAnthropicMessagesRoutedHandler_MissingMaxTokens_400(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	handler := setupTestServer(cfg, reg)

	// No max_tokens in body.
	body := `{"model":"model-a","messages":[{"role":"user","content":"ping"}]}`
	w := makeAnthropicMessagesRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnthropicMessagesRoutedHandler_StreamingEmitsEvents(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello"},
		{Type: "delta", Content: " World"},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"stream":true}`
	w := makeAnthropicMessagesRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type: want text/event-stream, got %q", ct)
	}

	events := parseAnthropicSSEEvents(w.Body.String())
	eventTypes := make([]string, 0, len(events))
	for _, e := range events {
		eventTypes = append(eventTypes, e.EventType)
	}

	// Must have message_start and message_stop at minimum.
	startFound := false
	stopFound := false
	for _, et := range eventTypes {
		if et == "message_start" {
			startFound = true
		}
		if et == "message_stop" {
			stopFound = true
		}
	}
	if !startFound {
		t.Errorf("missing message_start in events: %v", eventTypes)
	}
	if !stopFound {
		t.Errorf("missing message_stop in events: %v", eventTypes)
	}
}
