package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAI_ChatCompletion_Success(t *testing.T) {
	resp := ChatResponse{
		ID:      "chatcmpl-abc123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-4o-mini",
		Choices: []ChatChoice{
			{Index: 0, Message: ChatMessage{Role: "assistant", Content: "Hello!"}, FinishReason: "stop"},
		},
		Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}

		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %s", req.Model)
		}
		if req.Stream {
			t.Errorf("expected stream=false for non-streaming call")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "test-key")
	result, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "chatcmpl-abc123" {
		t.Errorf("got ID %s, want chatcmpl-abc123", result.ID)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}
	if result.Choices[0].Message.Content != "Hello!" {
		t.Errorf("got content %q, want Hello!", result.Choices[0].Message.Content)
	}
	if result.Usage.TotalTokens != 15 {
		t.Errorf("got total_tokens %d, want 15", result.Usage.TotalTokens)
	}
}

func TestOpenAI_ChatCompletion_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal error"}}`))
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 500 response")
	}

	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("expected UpstreamError, got %T", err)
	}
	if ue.StatusCode != 500 {
		t.Errorf("got status %d, want 500", ue.StatusCode)
	}
	if !IsRetryable(err) {
		t.Error("500 should be retryable")
	}
}

func TestOpenAI_ChatCompletion_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 429 response")
	}

	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("expected UpstreamError, got %T", err)
	}
	if ue.StatusCode != 429 {
		t.Errorf("got status %d, want 429", ue.StatusCode)
	}
	if !IsRetryable(err) {
		t.Error("429 should be retryable")
	}
}

func TestOpenAI_ChatCompletion_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "key")
	client.SetHTTPClient(&http.Client{Timeout: 50 * time.Millisecond})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !IsRetryable(err) {
		t.Error("timeout should be retryable")
	}
}

func TestOpenAI_ChatCompletion_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 400 response")
	}

	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("expected UpstreamError, got %T", err)
	}
	if ue.StatusCode != 400 {
		t.Errorf("got status %d, want 400", ue.StatusCode)
	}
	if IsRetryable(err) {
		t.Error("400 should NOT be retryable")
	}
}

func TestOpenAI_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("expected no Authorization header when key is empty")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatResponse{
			ID:      "test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "local-llama",
			Choices: []ChatChoice{{Index: 0, Message: ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		})
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "local-llama",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAI_ChatCompletionStream_SSE(t *testing.T) {
	// Fake SSE endpoint that emits two chunks and [DONE].
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Error("expected stream=true in request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "key")
	streamResp, err := client.ChatCompletionStream(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []StreamEvent
	for c := range streamResp.Events {
		events = append(events, c)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	last := events[len(events)-1]
	if last.Type != "done" {
		t.Errorf("expected last event to be done, got %+v", last)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"500", &UpstreamError{StatusCode: 500}, true},
		{"502", &UpstreamError{StatusCode: 502}, true},
		{"503", &UpstreamError{StatusCode: 503}, true},
		{"429", &UpstreamError{StatusCode: 429}, true},
		{"404", &UpstreamError{StatusCode: 404}, true}, // Model not found/endpoint mismatch - should fallback
		{"400", &UpstreamError{StatusCode: 400}, false},
		{"401", &UpstreamError{StatusCode: 401}, false},
		{"403", &UpstreamError{StatusCode: 403}, false},
		{"generic", context.DeadlineExceeded, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}
