package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnthropic_ChatCompletion_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-anthropic-key" {
			t.Errorf("expected x-api-key header, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Errorf("expected anthropic-version %s, got %s", anthropicAPIVersion, r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}

		var req anthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "claude-3-5-sonnet" {
			t.Errorf("expected model claude-3-5-sonnet, got %s", req.Model)
		}
		if req.System != "You are helpful" {
			t.Errorf("expected system prompt, got %q", req.System)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Errorf("expected 1 user message, got %+v", req.Messages)
		}
		if req.MaxTokens != 100 {
			t.Errorf("expected max_tokens 100, got %d", req.MaxTokens)
		}

		resp := anthropicResponse{
			ID:         "msg_test123",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-3-5-sonnet-20241022",
			StopReason: "end_turn",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Hello from Claude!"},
			},
			Usage: anthropicUsage{InputTokens: 12, OutputTokens: 8},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	maxTokens := 100
	client := NewAnthropic(srv.URL, "test-anthropic-key")
	result, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "claude-3-5-sonnet",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hi"},
		},
		MaxTokens: &maxTokens,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "msg_test123" {
		t.Errorf("got ID %s, want msg_test123", result.ID)
	}
	if result.Object != "chat.completion" {
		t.Errorf("got object %s, want chat.completion", result.Object)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}
	if result.Choices[0].Message.Content != "Hello from Claude!" {
		t.Errorf("got content %q", result.Choices[0].Message.Content)
	}
	if result.Choices[0].Message.Role != "assistant" {
		t.Errorf("got role %s, want assistant", result.Choices[0].Message.Role)
	}
	if result.Choices[0].FinishReason != "stop" {
		t.Errorf("got finish_reason %s, want stop", result.Choices[0].FinishReason)
	}
	if result.Usage.PromptTokens != 12 {
		t.Errorf("got prompt_tokens %d, want 12", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 8 {
		t.Errorf("got completion_tokens %d, want 8", result.Usage.CompletionTokens)
	}
	if result.Usage.TotalTokens != 20 {
		t.Errorf("got total_tokens %d, want 20", result.Usage.TotalTokens)
	}
}

func TestAnthropic_ChatCompletion_MaxTokensStopReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			ID: "msg_trunc", Type: "message", Role: "assistant", Model: "claude-3-5-sonnet",
			StopReason: "max_tokens",
			Content:    []anthropicContentBlock{{Type: "text", Text: "truncated..."}},
			Usage:      anthropicUsage{InputTokens: 10, OutputTokens: 100},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	result, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Choices[0].FinishReason != "length" {
		t.Errorf("expected finish_reason=length for max_tokens, got %s", result.Choices[0].FinishReason)
	}
}

func TestAnthropic_ChatCompletionStream_MapsToInternalEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("expected /v1/messages, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"type\":\"message_start\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"Hello\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\" Anthropic\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "k")
	streamResp, err := client.ChatCompletionStream(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []StreamEvent
	for ev := range streamResp.Events {
		got = append(got, ev)
	}
	if len(got) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(got))
	}
	if got[0].Type != "delta" || got[0].Content != "Hello" {
		t.Fatalf("unexpected first event: %+v", got[0])
	}
	if got[1].Type != "delta" || got[1].Content != " Anthropic" {
		t.Fatalf("unexpected second event: %+v", got[1])
	}
	if got[len(got)-1].Type != "done" {
		t.Fatalf("expected last event done, got %+v", got[len(got)-1])
	}
}

func TestAnthropic_ChatCompletionStream_EventHeaderWithDeltaText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\"}\n\n")
		fmt.Fprint(w, "event: content_block_start\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\"}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		fmt.Fprint(w, "event: ping\n")
		fmt.Fprint(w, "data: {\"type\":\"ping\"}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n")
		fmt.Fprint(w, "event: content_block_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_stop\"}\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"stop_reason\":\"end_turn\"}\n\n")
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "k")
	streamResp, err := client.ChatCompletionStream(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []StreamEvent
	for ev := range streamResp.Events {
		got = append(got, ev)
	}
	if len(got) < 3 {
		t.Fatalf("expected two deltas and done, got %d events: %+v", len(got), got)
	}
	if got[0].Type != "delta" || got[0].Content != "Hello" {
		t.Fatalf("unexpected first event: %+v", got[0])
	}
	if got[1].Type != "delta" || got[1].Content != " world" {
		t.Fatalf("unexpected second event: %+v", got[1])
	}
	if got[len(got)-1].Type != "done" {
		t.Fatalf("expected done event, got %+v", got[len(got)-1])
	}
}

func TestAnthropic_ChatCompletion_DefaultMaxTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req anthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.MaxTokens != defaultAnthropicMaxTokens {
			t.Errorf("expected default max_tokens %d, got %d", defaultAnthropicMaxTokens, req.MaxTokens)
		}

		resp := anthropicResponse{
			ID: "msg_def", Type: "message", Role: "assistant", Model: "claude-3-5-sonnet",
			StopReason: "end_turn",
			Content:    []anthropicContentBlock{{Type: "text", Text: "ok"}},
			Usage:      anthropicUsage{InputTokens: 5, OutputTokens: 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnthropic_ChatCompletion_SystemMessageExtracted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req anthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.System != "sys1\n\nsys2" {
			t.Errorf("expected concatenated system prompts, got %q", req.System)
		}
		// System messages should not appear in messages array
		for _, m := range req.Messages {
			if m.Role == "system" {
				t.Error("system message should not be in messages array")
			}
		}

		resp := anthropicResponse{
			ID: "msg_sys", Type: "message", Role: "assistant", Model: "claude-3-5-sonnet",
			StopReason: "end_turn",
			Content:    []anthropicContentBlock{{Type: "text", Text: "ok"}},
			Usage:      anthropicUsage{InputTokens: 5, OutputTokens: 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "claude-3-5-sonnet",
		Messages: []ChatMessage{
			{Role: "system", Content: "sys1"},
			{Role: "system", Content: "sys2"},
			{Role: "user", Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnthropic_ChatCompletion_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"type":"api_error","message":"internal server error"}}`))
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
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

func TestAnthropic_ChatCompletion_529(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(529)
		w.Write([]byte(`{"error":{"type":"overloaded_error","message":"overloaded"}}`))
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 529 response")
	}
	ue := err.(*UpstreamError)
	if ue.StatusCode != 529 {
		t.Errorf("got status %d, want 529", ue.StatusCode)
	}
	if !IsRetryable(err) {
		t.Error("529 should be retryable")
	}
}

func TestAnthropic_ChatCompletion_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	ue := err.(*UpstreamError)
	if ue.StatusCode != 429 {
		t.Errorf("got status %d, want 429", ue.StatusCode)
	}
	if !IsRetryable(err) {
		t.Error("429 should be retryable")
	}
}

func TestAnthropic_ChatCompletion_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"bad request"}}`))
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	ue := err.(*UpstreamError)
	if ue.StatusCode != 400 {
		t.Errorf("got status %d, want 400", ue.StatusCode)
	}
	if IsRetryable(err) {
		t.Error("400 should NOT be retryable")
	}
}

func TestAnthropic_ChatCompletion_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"type":"not_found_error","message":"model not found"}}`))
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "nonexistent-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	ue := err.(*UpstreamError)
	if ue.StatusCode != 404 {
		t.Errorf("got status %d, want 404", ue.StatusCode)
	}
	if !strings.Contains(ue.Error(), "404") {
		t.Errorf("error should contain status code: %s", ue.Error())
	}
	if !IsRetryable(err) {
		t.Error("404 should be retryable (model not found/endpoint mismatch)")
	}
}

func TestAnthropic_ChatCompletion_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "key")
	client.SetHTTPClient(&http.Client{Timeout: 50 * time.Millisecond})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !IsRetryable(err) {
		t.Error("timeout should be retryable")
	}
}

func TestAnthropic_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Error("expected no x-api-key header when key is empty")
		}
		resp := anthropicResponse{
			ID: "msg_nokey", Type: "message", Role: "assistant", Model: "claude-3-5-sonnet",
			StopReason: "end_turn",
			Content:    []anthropicContentBlock{{Type: "text", Text: "ok"}},
			Usage:      anthropicUsage{InputTokens: 5, OutputTokens: 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAnthropic(srv.URL, "")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnthropic_StreamingEndpointErrorBubblesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"bad stream"}}`))
	}))
	defer srv.Close()
	client := NewAnthropic(srv.URL, "key")
	_, err := client.ChatCompletionStream(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected stream error")
	}
}
