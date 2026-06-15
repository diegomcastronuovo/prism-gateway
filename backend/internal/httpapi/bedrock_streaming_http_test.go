package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// bedrockTestConfig returns a config that includes a Bedrock-backed model.
func bedrockTestConfig() *config.Config {
	cfg := testConfig()
	cfg.Providers["bedrock"] = config.ProviderConfig{
		Type:      "aws_bedrock",
		AwsRegion: "us-east-1",
	}
	cfg.Models = append(cfg.Models, config.ModelConfig{
		Name:            "bedrock-haiku",
		Provider:        "bedrock",
		ProviderModelID: "anthropic.claude-haiku-4-5-20251001-v1:0",
		Pricing:         config.Pricing{PromptPer1M: 0.25, CompletionPer1M: 1.25},
	})
	cfg.Tenants[0].AllowedModels = append(cfg.Tenants[0].AllowedModels, "bedrock-haiku")
	return cfg
}

// streamingProviderWithUsage is a streaming provider that reports exact usage on done.
// Simulates what the real Bedrock provider does when it receives a metadata event.
type streamingProviderWithUsage struct {
	chunks    []providers.StreamEvent
	doneUsage *providers.Usage
}

func (p *streamingProviderWithUsage) ChatCompletion(_ context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	return &providers.ChatResponse{
		ID: "test", Model: req.Model,
		Choices: []providers.ChatChoice{{Message: providers.ChatMessage{Role: "assistant", Content: "ok"}}},
	}, nil
}

func (p *streamingProviderWithUsage) ChatCompletionStream(_ context.Context, _ providers.ChatRequest) (*providers.StreamResponse, error) {
	ch := make(chan providers.StreamEvent, len(p.chunks)+1)
	for _, ev := range p.chunks {
		ch <- ev
	}
	ch <- providers.StreamEvent{Type: "done", Usage: p.doneUsage}
	close(ch)
	return &providers.StreamResponse{Events: ch}, nil
}

// TestBedrockHTTP_StreamReturnsSSE verifies that a Bedrock streaming request returns
// OpenAI-compatible SSE with [DONE] and the correct Content-Type.
func TestBedrockHTTP_StreamReturnsSSE(t *testing.T) {
	cfg := bedrockTestConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "La capital"},
		{Type: "delta", Content: " de España es Madrid."},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	reg.Register("bedrock", sp)

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"bedrock-haiku","messages":[{"role":"user","content":"Dime la capital de España en una frase"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "[DONE]") {
		t.Error("expected [DONE] in SSE output")
	}
}

// TestBedrockHTTP_StreamChunksOpenAICompatible verifies that each SSE data chunk
// is valid OpenAI-shaped JSON and that [DONE] is the final token.
func TestBedrockHTTP_StreamChunksOpenAICompatible(t *testing.T) {
	cfg := bedrockTestConfig()
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "Hello"},
		{Type: "delta", Content: " World"},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	reg.Register("bedrock", sp)

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"bedrock-haiku","messages":[{"role":"user","content":"hi"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	lines := parseSSELines(w.Body.String())
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 SSE lines (chunks + [DONE]), got %d: %v", len(lines), lines)
	}
	if lines[len(lines)-1] != "[DONE]" {
		t.Errorf("last SSE line must be [DONE], got %q", lines[len(lines)-1])
	}
	for _, line := range lines {
		if line == "[DONE]" {
			continue
		}
		if !strings.Contains(line, `"choices"`) {
			t.Errorf("SSE chunk missing choices field: %s", line)
		}
	}
}

// TestBedrockHTTP_StreamUsagePersistsWhenProvided verifies that when the provider
// reports exact usage (as Bedrock does via stream metadata), those values are stored.
func TestBedrockHTTP_StreamUsagePersistsWhenProvided(t *testing.T) {
	cfg := bedrockTestConfig()

	reportedUsage := &providers.Usage{PromptTokens: 42, CompletionTokens: 17, TotalTokens: 59}
	sp := &streamingProviderWithUsage{
		chunks:    []providers.StreamEvent{{Type: "delta", Content: "Madrid."}},
		doneUsage: reportedUsage,
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	reg.Register("bedrock", sp)

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"bedrock-haiku","messages":[{"role":"user","content":"capital?"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	u := findStreamUsage(t, store)
	if u.PromptTokens != 42 {
		t.Errorf("expected prompt_tokens=42, got %d", u.PromptTokens)
	}
	if u.CompletionTokens != 17 {
		t.Errorf("expected completion_tokens=17, got %d", u.CompletionTokens)
	}
	if u.TotalTokens != 59 {
		t.Errorf("expected total_tokens=59, got %d", u.TotalTokens)
	}
}

// TestBedrockHTTP_StreamFallbackEstimationWhenNoUsage verifies that when no provider
// usage is reported on the done event, token counts are estimated from output text.
func TestBedrockHTTP_StreamFallbackEstimationWhenNoUsage(t *testing.T) {
	cfg := bedrockTestConfig()
	// streamingProvider sends done with nil Usage — estimation must kick in.
	sp := &streamingProvider{chunks: []providers.StreamEvent{
		{Type: "delta", Content: "La capital de España es Madrid."},
	}}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	reg.Register("bedrock", sp)

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"bedrock-haiku","messages":[{"role":"user","content":"capital?"}],"stream":true}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	u := findStreamUsage(t, store)
	if u.PromptTokens <= 0 {
		t.Errorf("expected estimated prompt tokens > 0, got %d", u.PromptTokens)
	}
	if u.CompletionTokens <= 0 {
		t.Errorf("expected estimated completion tokens > 0, got %d", u.CompletionTokens)
	}
	if u.TotalTokens != u.PromptTokens+u.CompletionTokens {
		t.Errorf("total mismatch: got %d, want %d+%d=%d", u.TotalTokens, u.PromptTokens, u.CompletionTokens, u.PromptTokens+u.CompletionTokens)
	}
}

// TestBedrockHTTP_NonStreamUnchanged verifies that Bedrock non-stream requests still
// produce a standard JSON chat response, not SSE.
func TestBedrockHTTP_NonStreamUnchanged(t *testing.T) {
	cfg := bedrockTestConfig()

	nonStreamBedrock := &fakeProvider{
		handler: func(_ context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID: "bedrock-test", Object: "chat.completion", Model: req.Model,
				Choices: []providers.ChatChoice{{
					Index:        0,
					Message:      providers.ChatMessage{Role: "assistant", Content: "Madrid"},
					FinishReason: "stop",
				}},
				Usage: providers.Usage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6},
			}, nil
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	reg.Register("bedrock", nonStreamBedrock)

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"bedrock-haiku","messages":[{"role":"user","content":"capital?"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		t.Errorf("non-stream request must not return text/event-stream, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), `"choices"`) {
		t.Errorf("expected JSON chat response with choices, got: %s", w.Body.String())
	}
}
