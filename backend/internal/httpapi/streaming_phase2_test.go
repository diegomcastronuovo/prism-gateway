package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// streamingMeta extracts the stream fields from a RequestLog's Metadata.
func streamingMeta(t *testing.T, rl storage.RequestLog) map[string]interface{} {
	t.Helper()
	if rl.Metadata == nil {
		t.Fatal("request log has no metadata")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rl.Metadata, &m); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	return m
}

// findStreamLog returns the single request log row persisted by a streaming request.
func findStreamLog(t *testing.T, store *fakeStorage) storage.RequestLog {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.requests) == 0 {
		t.Fatal("no request log rows stored")
	}
	return store.requests[len(store.requests)-1]
}

func findStreamUsage(t *testing.T, store *fakeStorage) storage.UsageRecord {
	t.Helper()
	usages := store.Usages()
	if len(usages) == 0 {
		t.Fatal("no usage rows stored")
	}
	return usages[len(usages)-1]
}

// --- Phase 2 tests ---

// TestStreamPhase2_SuccessfulStreamLogsCompleted verifies that a complete stream
// persists stream_completed=true with chunk_count and first_token_latency_ms.
func TestStreamPhase2_SuccessfulStreamLogsCompleted(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello"},
		{Type: "delta", Content: " World"},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	rl := findStreamLog(t, store)
	if rl.Status != "ok" {
		t.Errorf("expected status ok, got %q", rl.Status)
	}

	meta := streamingMeta(t, rl)
	if meta["streaming_enabled"] != true {
		t.Errorf("expected streaming_enabled=true, got %v", meta["streaming_enabled"])
	}
	if meta["stream_completed"] != true {
		t.Errorf("expected stream_completed=true, got %v", meta["stream_completed"])
	}
	// chunk_count is stored as float64 in JSON unmarshaling
	if cc, _ := meta["chunk_count"].(float64); cc != 2 {
		t.Errorf("expected chunk_count=2, got %v", meta["chunk_count"])
	}
	if _, ok := meta["first_token_latency_ms"]; !ok {
		t.Error("expected first_token_latency_ms to be present")
	}
	if _, ok := meta["stream_duration_ms"]; !ok {
		t.Error("expected stream_duration_ms to be present")
	}
	// No error type on success
	if _, ok := meta["stream_error_type"]; ok {
		t.Errorf("expected no stream_error_type on success, got %v", meta["stream_error_type"])
	}
}

// TestStreamPhase2_ClientDisconnectLogsIncomplete verifies that a client disconnect
// persists stream_completed=false, status=cancelled, stream_error_type=client_disconnect.
func TestStreamPhase2_ClientDisconnectLogsIncomplete(t *testing.T) {
	cfg := testConfig()

	// Provider that blocks until the context is cancelled.
	blockingCh := make(chan providers.StreamEvent)
	sp := &controlledStreamProvider{ch: blockingCh}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := newJSONRequest(ctx, `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	req.Header.Set("X-API-Key", "key1")
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(w, req)
	}()

	// Cancel the context to simulate client disconnect.
	// Do NOT close blockingCh — we want only ctx.Done() to become ready so
	// the select in writeSSEStream deterministically picks the disconnect path.
	cancel()
	<-done

	rl := findStreamLog(t, store)
	if rl.Status != "cancelled" {
		t.Errorf("expected status cancelled, got %q", rl.Status)
	}

	meta := streamingMeta(t, rl)
	if meta["stream_completed"] != false {
		t.Errorf("expected stream_completed=false, got %v", meta["stream_completed"])
	}
	if et, _ := meta["stream_error_type"].(string); et != "client_disconnect" {
		t.Errorf("expected stream_error_type=client_disconnect, got %q", et)
	}
}

// TestStreamPhase2_UpstreamErrorLogsIncomplete verifies that a mid-stream upstream
// error persists stream_completed=false, status=error, stream_error_type classified.
func TestStreamPhase2_UpstreamErrorLogsIncomplete(t *testing.T) {
	cfg := testConfig()

	// Provider that sends one chunk then an error.
	ch := make(chan providers.StreamEvent, 2)
	ch <- providers.StreamEvent{Type: "delta", Content: "Hello"}
	ch <- providers.StreamEvent{Type: "error", Error: fmt.Errorf("unexpected EOF")}
	close(ch)
	sp := &controlledStreamProvider{ch: ch}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	// Headers are already written (200) before the error occurs mid-stream.
	_ = w
	rl := findStreamLog(t, store)
	if rl.Status != "error" {
		t.Errorf("expected status error, got %q", rl.Status)
	}

	meta := streamingMeta(t, rl)
	if meta["stream_completed"] != false {
		t.Errorf("expected stream_completed=false, got %v", meta["stream_completed"])
	}
	// "EOF" in error message → upstream_disconnect
	if et, _ := meta["stream_error_type"].(string); et != "upstream_disconnect" {
		t.Errorf("expected stream_error_type=upstream_disconnect, got %q", et)
	}
	// chunk_count = 1 (one chunk was forwarded before error)
	if cc, _ := meta["chunk_count"].(float64); cc != 1 {
		t.Errorf("expected chunk_count=1, got %v", meta["chunk_count"])
	}
}

// TestStreamPhase2_FirstTokenLatencyRecorded verifies first_token_latency_ms is present
// when at least one chunk was delivered.
func TestStreamPhase2_FirstTokenLatencyRecorded(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{{Type: "delta", Content: "Hi"}}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	meta := streamingMeta(t, findStreamLog(t, store))
	if _, ok := meta["first_token_latency_ms"]; !ok {
		t.Error("expected first_token_latency_ms to be present when chunks were sent")
	}
}

// TestStreamPhase2_ClassifyStreamError_Table tests the classifyStreamError helper.
func TestStreamPhase2_ClassifyStreamError_Table(t *testing.T) {
	cases := []struct {
		name      string
		ctxErr    error
		streamErr error
		want      string
	}{
		{"deadline_from_ctx", context.DeadlineExceeded, nil, "timeout"},
		{"deadline_from_err", nil, context.DeadlineExceeded, "timeout"},
		{"cancel_from_ctx", context.Canceled, nil, "client_disconnect"},
		{"cancel_from_err", nil, context.Canceled, "client_disconnect"},
		{"eof_upstream", nil, fmt.Errorf("read tcp: EOF"), "upstream_disconnect"},
		{"reset_upstream", nil, fmt.Errorf("connection reset by peer"), "upstream_disconnect"},
		{"broken_pipe", nil, fmt.Errorf("write: broken pipe"), "upstream_disconnect"},
		{"other_error", nil, fmt.Errorf("some provider error"), "provider_error"},
		{"no_error_no_ctx", nil, nil, "client_disconnect"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyStreamError(tc.ctxErr, tc.streamErr)
			if got != tc.want {
				t.Errorf("classifyStreamError(%v, %v) = %q, want %q", tc.ctxErr, tc.streamErr, got, tc.want)
			}
		})
	}
}

// TestStreamPhase2_NoChunksStillLogs verifies that even a zero-chunk complete stream
// logs correctly (e.g. model returns [DONE] immediately).
func TestStreamPhase2_NoChunksStillLogs(t *testing.T) {
	cfg := testConfig()
	// Provider returns [DONE] with no data chunks.
	sp := &streamingProvider{chunks: []providers.StreamEvent{}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Verify [DONE] was emitted
	if !strings.Contains(w.Body.String(), "[DONE]") {
		t.Error("expected [DONE] in response body")
	}

	meta := streamingMeta(t, findStreamLog(t, store))
	if meta["stream_completed"] != true {
		t.Errorf("expected stream_completed=true, got %v", meta["stream_completed"])
	}
	if cc, _ := meta["chunk_count"].(float64); cc != 0 {
		t.Errorf("expected chunk_count=0, got %v", meta["chunk_count"])
	}
	if _, ok := meta["stream_error_type"]; ok {
		t.Error("expected no stream_error_type on zero-chunk success")
	}
}

func TestStreamPhase2_UsageEstimatedOnSuccess(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello"},
		{Type: "delta", Content: " World"},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi there"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	u := findStreamUsage(t, store)
	if u.PromptTokens <= 0 {
		t.Fatalf("expected prompt tokens > 0, got %d", u.PromptTokens)
	}
	if u.CompletionTokens <= 0 {
		t.Fatalf("expected completion tokens > 0, got %d", u.CompletionTokens)
	}
	if u.TotalTokens != u.PromptTokens+u.CompletionTokens {
		t.Fatalf("total tokens mismatch: got %d want %d", u.TotalTokens, u.PromptTokens+u.CompletionTokens)
	}
	if u.CostUSD <= 0 {
		t.Fatalf("expected cost > 0, got %f", u.CostUSD)
	}
}

func TestStreamPhase2_UsageEstimatedOnPartialCancel(t *testing.T) {
	cfg := testConfig()

	ch := make(chan providers.StreamEvent, 1)
	ch <- providers.StreamEvent{Type: "delta", Content: "partial"}
	sp := &controlledStreamProvider{ch: ch}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := newJSONRequest(ctx, `{"model":"model-a","messages":[{"role":"user","content":"stream me"}],"stream":true}`)
	req.Header.Set("X-API-Key", "key1")
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(w, req)
	}()
	cancel()
	<-done

	u := findStreamUsage(t, store)
	if u.PromptTokens <= 0 {
		t.Fatalf("expected prompt tokens > 0, got %d", u.PromptTokens)
	}
	if u.CompletionTokens <= 0 {
		t.Fatalf("expected completion tokens > 0 on partial output, got %d", u.CompletionTokens)
	}
}

func TestStreamPhase2_UsageEstimatedEmptyOutput(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"input only"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	u := findStreamUsage(t, store)
	if u.PromptTokens <= 0 {
		t.Fatalf("expected prompt tokens > 0, got %d", u.PromptTokens)
	}
	if u.CompletionTokens != 0 {
		t.Fatalf("expected completion tokens = 0, got %d", u.CompletionTokens)
	}
}
