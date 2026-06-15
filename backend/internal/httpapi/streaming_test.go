package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// streamingProvider implements providers.Provider with streaming support.
type streamingProvider struct {
	chunks []providers.StreamEvent
	err    error // if non-nil, returned from ChatCompletionStream
}

func (p *streamingProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	return &providers.ChatResponse{
		ID: "chatcmpl-test", Model: req.Model,
		Choices: []providers.ChatChoice{{Message: providers.ChatMessage{Role: "assistant", Content: "ok"}}},
	}, nil
}

func (p *streamingProvider) ChatCompletionStream(ctx context.Context, req providers.ChatRequest) (*providers.StreamResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	ch := make(chan providers.StreamEvent, len(p.chunks)+1)
	for _, ev := range p.chunks {
		ch <- ev
	}
	ch <- providers.StreamEvent{Type: "done"}
	close(ch)
	return &providers.StreamResponse{Events: ch}, nil
}

// parseSSELines extracts the data payloads from SSE output.
func parseSSELines(body string) []string {
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

// TestStream_NonStreamRequestUnchanged verifies that stream:false continues to work.
func TestStream_NonStreamRequestUnchanged(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":false}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Model != "model-a" {
		t.Errorf("expected model-a, got %q", resp.Model)
	}
}

// TestStream_StreamTrueOpenAIReturnsSSE verifies SSE is returned for stream:true with a streaming-capable provider.
func TestStream_StreamTrueOpenAIReturnsSSE(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello"},
		{Type: "delta", Content: " World"},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}

	lines := parseSSELines(w.Body.String())
	if len(lines) == 0 {
		t.Fatal("expected SSE lines, got none")
	}
	// Last line must be [DONE]
	if lines[len(lines)-1] != "[DONE]" {
		t.Errorf("expected last SSE line to be [DONE], got %q", lines[len(lines)-1])
	}
	// Data lines should include the two chunks
	if len(lines) < 3 {
		t.Errorf("expected at least 3 SSE lines (2 chunks + [DONE]), got %d", len(lines))
	}
}

// TestStream_EndsDONE verifies [DONE] terminator is always emitted.
func TestStream_EndsDONE(t *testing.T) {
	cfg := testConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{{Type: "delta", Content: ""}}}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	lines := parseSSELines(w.Body.String())
	if len(lines) == 0 {
		t.Fatal("no SSE lines")
	}
	last := lines[len(lines)-1]
	if last != "[DONE]" {
		t.Errorf("expected [DONE] as last SSE line, got %q", last)
	}
}

// TestStream_UnsupportedProvider verifies that a provider returning ErrStreamingNotSupported gives a clear error.
func TestStream_UnsupportedProvider(t *testing.T) {
	cfg := testConfig()
	// fakeProvider always returns ErrStreamingNotSupported from ChatCompletionStream
	fp := &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{}, nil
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", fp)
	reg.Register("backup", fp)
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("expected invalid_request_error, got %q", errResp.Error.Type)
	}
	if !strings.Contains(errResp.Error.Message, "streaming not supported") {
		t.Errorf("expected 'streaming not supported' in message, got %q", errResp.Error.Message)
	}
}

// TestStream_ClientDisconnect verifies context cancellation stops the stream gracefully.
func TestStream_ClientDisconnect(t *testing.T) {
	cfg := testConfig()

	// Provider with a slow stream (uses a channel we control)
	ch := make(chan providers.StreamEvent)
	sp := &controlledStreamProvider{ch: ch}

	reg := providers.NewRegistry()
	reg.Register("openai", sp)
	reg.Register("backup", successProvider("model-b"))
	handler := setupTestServer(cfg, reg)

	// Use a cancelable context to simulate client disconnect
	ctx, cancel := context.WithCancel(context.Background())

	req, _ := newJSONRequest(ctx, `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	req.Header.Set("X-API-Key", "key1")
	w := newResponseRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(w, req)
	}()

	// Cancel the context to simulate client disconnect before any chunks are sent
	cancel()
	close(ch)

	select {
	case <-done:
		// Handler returned cleanly — OK
	}
}

// controlledStreamProvider returns chunks from a channel controlled by the test.
type controlledStreamProvider struct {
	ch <-chan providers.StreamEvent
}

func (p *controlledStreamProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	return &providers.ChatResponse{}, nil
}

func (p *controlledStreamProvider) ChatCompletionStream(ctx context.Context, req providers.ChatRequest) (*providers.StreamResponse, error) {
	return &providers.StreamResponse{Events: p.ch}, nil
}

// newJSONRequest creates a POST request with a JSON body and a custom context.
func newJSONRequest(ctx context.Context, body string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, err
}

// TestStreamingFallback_RetryableError_FallsBackToNextCandidate verifies that a retryable
// streaming init error (503) causes the orchestrator to try the next candidate.
func TestStreamingFallback_RetryableError_FallsBackToNextCandidate(t *testing.T) {
	cfg := testConfig()

	primary := &streamingProvider{err: &providers.UpstreamError{StatusCode: 503, Body: "service unavailable"}}
	secondary := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello from fallback"},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", primary)
	reg.Register("backup", secondary)
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after fallback, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
	lines := parseSSELines(w.Body.String())
	if len(lines) == 0 {
		t.Fatal("expected SSE lines, got none")
	}
	if lines[len(lines)-1] != "[DONE]" {
		t.Errorf("expected last SSE line to be [DONE], got %q", lines[len(lines)-1])
	}
}

// TestStreamingFallback_AllCandidatesFail verifies that when every candidate returns a
// retryable 503, the orchestrator exhausts the loop and returns a single upstream error.
func TestStreamingFallback_AllCandidatesFail(t *testing.T) {
	cfg := testConfig()

	errProvider := &streamingProvider{err: &providers.UpstreamError{StatusCode: 503, Body: "service unavailable"}}

	reg := providers.NewRegistry()
	reg.Register("openai", errProvider)
	reg.Register("backup", errProvider)
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code < 500 || w.Code >= 600 {
		t.Fatalf("expected 5xx when all candidates fail, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected JSON error body, not SSE, got Content-Type %q", ct)
	}
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("expected JSON error body, decode failed: %v — body: %s", err, w.Body.String())
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty error message in response")
	}
}

// TestStreamingFallback_NonRetryableError_NoFallback verifies that a non-retryable error (401)
// is returned immediately without attempting the secondary candidate.
func TestStreamingFallback_NonRetryableError_NoFallback(t *testing.T) {
	cfg := testConfig()

	var secondaryCalled bool
	primary := &streamingProvider{err: &providers.UpstreamError{StatusCode: 401, Body: "unauthorized"}}
	secondary := &trackingStreamingProvider{
		inner: &streamingProvider{chunks: []providers.StreamEvent{{Type: "delta", Content: "should not appear"}}},
		called: func() { secondaryCalled = true },
	}

	reg := providers.NewRegistry()
	reg.Register("openai", primary)
	reg.Register("backup", secondary)
	handler := setupTestServer(cfg, reg)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code < 400 || w.Code >= 500 {
		t.Fatalf("expected 4xx for non-retryable error, got %d: %s", w.Code, w.Body.String())
	}
	if secondaryCalled {
		t.Error("secondary candidate must NOT be called after a non-retryable error")
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected JSON error body, not SSE, got Content-Type %q", ct)
	}
}

// trackingStreamingProvider wraps a streamingProvider and records when it is called.
type trackingStreamingProvider struct {
	inner  *streamingProvider
	called func()
}

func (p *trackingStreamingProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	p.called()
	return p.inner.ChatCompletion(ctx, req)
}

func (p *trackingStreamingProvider) ChatCompletionStream(ctx context.Context, req providers.ChatRequest) (*providers.StreamResponse, error) {
	p.called()
	return p.inner.ChatCompletionStream(ctx, req)
}

// newResponseRecorder returns a simple ResponseRecorder (already defined in httptest but kept here for clarity).
// We use httptest.NewRecorder indirectly through the test setup.
// Actually we need a real one from net/http/httptest:
func newResponseRecorder() *nonBlockingRecorder {
	return &nonBlockingRecorder{headers: make(http.Header)}
}

// nonBlockingRecorder is a minimal ResponseWriter that doesn't block on writes.
type nonBlockingRecorder struct {
	headers http.Header
	code    int
	buf     strings.Builder
}

func (r *nonBlockingRecorder) Header() http.Header         { return r.headers }
func (r *nonBlockingRecorder) WriteHeader(code int)        { r.code = code }
func (r *nonBlockingRecorder) Write(b []byte) (int, error) { return r.buf.Write(b) }
