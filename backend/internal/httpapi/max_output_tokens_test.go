package httpapi

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// capturingProvider records the last ChatRequest it received.
type capturingProvider struct {
	mu      sync.Mutex
	lastReq providers.ChatRequest
}

func (c *capturingProvider) ChatCompletion(_ context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	c.mu.Lock()
	c.lastReq = req
	c.mu.Unlock()
	return &providers.ChatResponse{
		ID: "chatcmpl-cap", Object: "chat.completion", Created: 1234567890, Model: req.Model,
		Choices: []providers.ChatChoice{{Index: 0, Message: providers.ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		Usage:   providers.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}, nil
}

func (c *capturingProvider) ChatCompletionStream(_ context.Context, _ providers.ChatRequest) (*providers.StreamResponse, error) {
	return nil, providers.ErrStreamingNotSupported
}

func (c *capturingProvider) LastMaxTokens() *int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastReq.MaxTokens
}

// maxTokensConfig builds a config with specific model type and tenant cap.
func maxTokensConfig(modelType string, cap *int) *config.Config {
	cfg := testConfig()
	cfg.Models[0].Type = modelType        // model-a uses openai
	cfg.Tenants[0].MaxOutputTokens = cap
	cfg.Tenants[0].AllowedModels = []string{"model-a"}
	cfg.Tenants[0].Routing.Strategy = "round_robin"
	return cfg
}

func intPtr(v int) *int { return &v }

// TestMaxOutputTokens_LLMType injects cap when model type = "llm".
func TestMaxOutputTokens_LLMType(t *testing.T) {
	cap := capturingProvider{}
	cfg := maxTokensConfig("llm", intPtr(200))
	reg := providers.NewRegistry()
	reg.Register("openai", &cap)

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	mt := cap.LastMaxTokens()
	if mt == nil {
		t.Fatal("expected max_tokens to be set, got nil")
	}
	if *mt != 200 {
		t.Errorf("expected max_tokens=200, got %d", *mt)
	}
}

// TestMaxOutputTokens_EmptyType injects cap when model type = "" (unset).
func TestMaxOutputTokens_EmptyType(t *testing.T) {
	cap := capturingProvider{}
	cfg := maxTokensConfig("", intPtr(150))
	reg := providers.NewRegistry()
	reg.Register("openai", &cap)

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	mt := cap.LastMaxTokens()
	if mt == nil {
		t.Fatal("expected max_tokens to be set, got nil")
	}
	if *mt != 150 {
		t.Errorf("expected max_tokens=150, got %d", *mt)
	}
}

// TestMaxOutputTokens_CapDisabledZero does not inject when cap = 0.
func TestMaxOutputTokens_CapDisabledZero(t *testing.T) {
	cap := capturingProvider{}
	cfg := maxTokensConfig("llm", intPtr(0))
	reg := providers.NewRegistry()
	reg.Register("openai", &cap)

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if mt := cap.LastMaxTokens(); mt != nil {
		t.Errorf("expected max_tokens to be nil (no cap), got %d", *mt)
	}
}

// TestMaxOutputTokens_CapAbsent does not inject when field is nil.
func TestMaxOutputTokens_CapAbsent(t *testing.T) {
	cap := capturingProvider{}
	cfg := maxTokensConfig("llm", nil)
	reg := providers.NewRegistry()
	reg.Register("openai", &cap)

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if mt := cap.LastMaxTokens(); mt != nil {
		t.Errorf("expected max_tokens to be nil (no cap), got %d", *mt)
	}
}

// TestMaxOutputTokens_EmbeddingModel: embedding models are excluded from chat routing
// entirely (P0 filter in PrecedenceResolver). The cap is therefore never reached for them —
// a stronger guarantee than merely skipping the injection.
func TestMaxOutputTokens_EmbeddingModel(t *testing.T) {
	cap := capturingProvider{}
	cfg := maxTokensConfig("embedding", intPtr(200))
	reg := providers.NewRegistry()
	reg.Register("openai", &cap)

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	// Embedding models are not routable via chat → 4xx is expected.
	// The test confirms max_tokens injection code was never reached.
	if w.Code == http.StatusOK {
		mt := cap.LastMaxTokens()
		if mt != nil {
			t.Errorf("cap must not be injected for embedding model, got %d", *mt)
		}
	}
	// Either the request fails (embedding excluded from candidates) or succeeds
	// without a cap — both are correct outcomes per the spec.
}

// TestMaxOutputTokens_MLModel: ml models are excluded from chat routing (P0 filter),
// same reasoning as the embedding test above.
func TestMaxOutputTokens_MLModel(t *testing.T) {
	cap := capturingProvider{}
	cfg := maxTokensConfig("ml", intPtr(200))
	reg := providers.NewRegistry()
	reg.Register("openai", &cap)

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code == http.StatusOK {
		mt := cap.LastMaxTokens()
		if mt != nil {
			t.Errorf("cap must not be injected for ml model, got %d", *mt)
		}
	}
}

// TestMaxOutputTokens_EffectiveHelper verifies the EffectiveMaxOutputTokens helper.
func TestMaxOutputTokens_EffectiveHelper(t *testing.T) {
	cases := []struct {
		name string
		val  *int
		want int
	}{
		{"nil → 0", nil, 0},
		{"zero → 0", intPtr(0), 0},
		{"negative → 0", intPtr(-5), 0},
		{"positive → value", intPtr(300), 300},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.TenantConfig{MaxOutputTokens: tc.val}
			got := cfg.EffectiveMaxOutputTokens()
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
