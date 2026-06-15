package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestXAI_ChatCompletion_Success(t *testing.T) {
	wantResp := ChatResponse{
		ID:      "chatcmpl-xai-123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "grok-4",
		Choices: []ChatChoice{
			{Index: 0, Message: ChatMessage{Role: "assistant", Content: "Hello from Grok!"}, FinishReason: "stop"},
		},
		Usage: Usage{PromptTokens: 8, CompletionTokens: 4, TotalTokens: 12},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-xai-key" {
			t.Errorf("expected Bearer test-xai-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}

		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "grok-4" {
			t.Errorf("expected model grok-4, got %s", req.Model)
		}
		if req.Stream {
			t.Error("expected stream=false")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wantResp)
	}))
	defer srv.Close()

	client := NewXAI(srv.URL, "test-xai-key")
	result, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "grok-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "chatcmpl-xai-123" {
		t.Errorf("got ID %s, want chatcmpl-xai-123", result.ID)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}
	if result.Choices[0].Message.Content != "Hello from Grok!" {
		t.Errorf("got content %q, want Hello from Grok!", result.Choices[0].Message.Content)
	}
	if result.Choices[0].Message.Role != "assistant" {
		t.Errorf("got role %s, want assistant", result.Choices[0].Message.Role)
	}
	if result.Choices[0].FinishReason != "stop" {
		t.Errorf("got finish_reason %s, want stop", result.Choices[0].FinishReason)
	}
	if result.Usage.PromptTokens != 8 {
		t.Errorf("got prompt_tokens %d, want 8", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 4 {
		t.Errorf("got completion_tokens %d, want 4", result.Usage.CompletionTokens)
	}
	if result.Usage.TotalTokens != 12 {
		t.Errorf("got total_tokens %d, want 12", result.Usage.TotalTokens)
	}
}

func TestXAI_ChatCompletion_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	client := NewXAI(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "grok-4",
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

func TestXAI_ChatCompletion_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal server error"}}`))
	}))
	defer srv.Close()

	client := NewXAI(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "grok-4",
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

func TestXAI_ChatCompletion_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv.Close()

	client := NewXAI(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "grok-4",
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

func TestXAI_ChatCompletion_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewXAI(srv.URL, "key")
	client.SetHTTPClient(&http.Client{Timeout: 50 * time.Millisecond})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "grok-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !IsRetryable(err) {
		t.Error("timeout should be retryable")
	}
}

func TestXAI_StreamingNotSupported(t *testing.T) {
	client := NewXAI("http://localhost", "key")
	_, err := client.ChatCompletionStream(context.Background(), ChatRequest{
		Model:    "grok-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != ErrStreamingNotSupported {
		t.Errorf("expected ErrStreamingNotSupported, got %v", err)
	}
}
