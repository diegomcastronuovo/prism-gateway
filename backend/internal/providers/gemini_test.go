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

func TestGemini_ChatCompletion_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/models/gemini-1.5-flash:generateContent") {
			t.Errorf("expected path containing /models/gemini-1.5-flash:generateContent, got %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-gemini-key" {
			t.Errorf("expected x-goog-api-key header, got %q", r.Header.Get("x-goog-api-key"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}

		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Contents) != 1 || req.Contents[0].Role != "user" {
			t.Errorf("expected 1 user content, got %+v", req.Contents)
		}
		if req.SystemInstruction == nil {
			t.Error("expected systemInstruction to be set")
		} else if req.SystemInstruction.Parts[0].Text != "You are helpful" {
			t.Errorf("expected system instruction text, got %q", req.SystemInstruction.Parts[0].Text)
		}
		if req.GenerationConfig.MaxOutputTokens != 100 {
			t.Errorf("expected maxOutputTokens 100, got %d", req.GenerationConfig.MaxOutputTokens)
		}

		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{
					Content: geminiContent{
						Role:  "model",
						Parts: []geminiPart{{Text: "Hello from Gemini!"}},
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: geminiUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 6,
				TotalTokenCount:      16,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	maxTokens := 100
	client := NewGemini(srv.URL, "test-gemini-key")
	result, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "gemini-1.5-flash",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hi"},
		},
		MaxTokens: &maxTokens,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Object != "chat.completion" {
		t.Errorf("got object %s, want chat.completion", result.Object)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}
	if result.Choices[0].Message.Content != "Hello from Gemini!" {
		t.Errorf("got content %q", result.Choices[0].Message.Content)
	}
	if result.Choices[0].Message.Role != "assistant" {
		t.Errorf("got role %s, want assistant", result.Choices[0].Message.Role)
	}
	if result.Choices[0].FinishReason != "stop" {
		t.Errorf("got finish_reason %s, want stop", result.Choices[0].FinishReason)
	}
	if result.Usage.PromptTokens != 10 {
		t.Errorf("got prompt_tokens %d, want 10", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 6 {
		t.Errorf("got completion_tokens %d, want 6", result.Usage.CompletionTokens)
	}
	if result.Usage.TotalTokens != 16 {
		t.Errorf("got total_tokens %d, want 16", result.Usage.TotalTokens)
	}
}

func TestGemini_ChatCompletion_MaxTokensFinishReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{
					Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: "truncated..."}}},
					FinishReason: "MAX_TOKENS",
				},
			},
			UsageMetadata: geminiUsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 50, TotalTokenCount: 60},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	result, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Choices[0].FinishReason != "length" {
		t.Errorf("expected finish_reason=length for MAX_TOKENS, got %s", result.Choices[0].FinishReason)
	}
}

func TestGemini_ChatCompletion_SafetyFinishReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{
					Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: ""}}},
					FinishReason: "SAFETY",
				},
			},
			UsageMetadata: geminiUsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 0, TotalTokenCount: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	result, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Choices[0].FinishReason != "content_filter" {
		t.Errorf("expected finish_reason=content_filter for SAFETY, got %s", result.Choices[0].FinishReason)
	}
}

func TestGemini_ChatCompletionStream_MapsToInternalEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" Gemini\"}]},\"finishReason\":\"STOP\"}]}\n\n")
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "k")
	streamResp, err := client.ChatCompletionStream(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
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
	if len(got) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(got))
	}
	if got[0].Type != "delta" || got[0].Content != "Hello" {
		t.Fatalf("unexpected first event: %+v", got[0])
	}
	if got[len(got)-1].Type != "done" {
		t.Fatalf("expected last event done, got %+v", got[len(got)-1])
	}
}

func TestGemini_ChatCompletion_DefaultMaxTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.GenerationConfig.MaxOutputTokens != defaultGeminiMaxTokens {
			t.Errorf("expected default maxOutputTokens %d, got %d", defaultGeminiMaxTokens, req.GenerationConfig.MaxOutputTokens)
		}

		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{Role: "model", Parts: []geminiPart{{Text: "ok"}}}, FinishReason: "STOP"},
			},
			UsageMetadata: geminiUsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 1, TotalTokenCount: 6},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGemini_ChatCompletion_SystemMessageExtracted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.SystemInstruction == nil || len(req.SystemInstruction.Parts) != 2 {
			t.Errorf("expected 2 system instruction parts, got %+v", req.SystemInstruction)
		} else {
			if req.SystemInstruction.Parts[0].Text != "sys1" || req.SystemInstruction.Parts[1].Text != "sys2" {
				t.Errorf("unexpected system instruction parts: %+v", req.SystemInstruction.Parts)
			}
		}

		// System messages should not appear in contents array
		for _, c := range req.Contents {
			if c.Role == "system" {
				t.Error("system role should not appear in contents")
			}
		}

		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{Role: "model", Parts: []geminiPart{{Text: "ok"}}}, FinishReason: "STOP"},
			},
			UsageMetadata: geminiUsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 1, TotalTokenCount: 6},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "gemini-1.5-flash",
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

func TestGemini_ChatCompletion_AssistantRoleMapsToModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Contents) != 2 {
			t.Fatalf("expected 2 contents, got %d", len(req.Contents))
		}
		if req.Contents[0].Role != "user" {
			t.Errorf("expected first content role=user, got %s", req.Contents[0].Role)
		}
		if req.Contents[1].Role != "model" {
			t.Errorf("expected second content role=model (mapped from assistant), got %s", req.Contents[1].Role)
		}

		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{Role: "model", Parts: []geminiPart{{Text: "ok"}}}, FinishReason: "STOP"},
			},
			UsageMetadata: geminiUsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 1, TotalTokenCount: 6},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "gemini-1.5-flash",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGemini_ChatCompletion_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":429,"message":"rate limited","status":"RESOURCE_EXHAUSTED"}}`))
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
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

func TestGemini_ChatCompletion_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"code":500,"message":"internal error","status":"INTERNAL"}}`))
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	ue := err.(*UpstreamError)
	if ue.StatusCode != 500 {
		t.Errorf("got status %d, want 500", ue.StatusCode)
	}
	if !IsRetryable(err) {
		t.Error("500 should be retryable")
	}
}

func TestGemini_ChatCompletion_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":400,"message":"invalid argument","status":"INVALID_ARGUMENT"}}`))
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
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

func TestGemini_ChatCompletion_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for malformed response")
	}
	if !strings.Contains(err.Error(), "unmarshal gemini response") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestGemini_ChatCompletion_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "key")
	client.SetHTTPClient(&http.Client{Timeout: 50 * time.Millisecond})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !IsRetryable(err) {
		t.Error("timeout should be retryable")
	}
}

func TestGemini_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "" {
			t.Error("expected no x-goog-api-key header when key is empty")
		}
		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{Role: "model", Parts: []geminiPart{{Text: "ok"}}}, FinishReason: "STOP"},
			},
			UsageMetadata: geminiUsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 1, TotalTokenCount: 6},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGemini(srv.URL, "")
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGemini_StreamingEndpointErrorBubblesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad stream"}}`))
	}))
	defer srv.Close()
	client := NewGemini(srv.URL, "key")
	_, err := client.ChatCompletionStream(context.Background(), ChatRequest{
		Model:    "gemini-1.5-flash",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected stream error")
	}
}
