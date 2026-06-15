package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

// fakeEmbeddingProvider implements providers.EmbeddingProvider and returns a fixed vector.
type fakeEmbeddingProvider struct {
	vec []float64
}

func (f fakeEmbeddingProvider) CreateEmbedding(_ context.Context, req providers.EmbeddingRequest) (*providers.EmbeddingResponse, error) {
	return &providers.EmbeddingResponse{
		Object: "list",
		Data: []providers.EmbeddingData{
			{Object: "embedding", Embedding: f.vec, Index: 0},
		},
		Model: req.Model,
		Usage: providers.EmbeddingUsage{PromptTokens: 5, TotalTokens: 5},
	}, nil
}

// semCacheTestConfig returns a testConfig augmented with an embedding model and
// semantic cache enabled on tenant t1.
func semCacheTestConfig() *config.Config {
	cfg := testConfig()
	// Add an embedding model
	cfg.Models = append(cfg.Models, config.ModelConfig{
		Name:     "text-embedding-ada-002",
		Provider: "openai",
		Type:     "embedding",
	})
	// Enable semantic cache on tenant t1
	cfg.Tenants[0].SemanticCache = config.SemanticCacheConfig{
		Enabled:        true,
		Threshold:      0.92,
		TTLSeconds:     86400,
		Scope:          "model",
		EmbeddingModel: "text-embedding-ada-002",
	}
	cfg.Tenants[0].Routing.Semantic.EmbeddingModel = "text-embedding-ada-002"
	cfg.Tenants[0].AllowedModels = append(cfg.Tenants[0].AllowedModels, "text-embedding-ada-002")
	return cfg
}

// semCacheRegistry creates a providers.Registry with chat providers and the given embedding vector.
func semCacheRegistry(vec []float64) *providers.Registry {
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	reg.Register("backup", successProvider(""))
	reg.RegisterEmbedding("openai", fakeEmbeddingProvider{vec: vec})
	return reg
}

// fixedVector returns a simple non-zero 1536-dim embedding vector.
func fixedVector() []float64 {
	v := make([]float64, 1536)
	for i := range v {
		v[i] = 0.01
	}
	return v
}

// cachedEntry returns a SemanticCacheEntry suitable for hit tests.
func cachedEntry() *storage.SemanticCacheEntry {
	body := ChatCompletionResponse{
		ID:      "cached-id",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "model-a",
		Choices: []ChatChoice{
			{Index: 0, Message: newTextMessage("assistant", "cached response"), FinishReason: "stop"},
		},
		Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	b, _ := json.Marshal(body)
	return &storage.SemanticCacheEntry{
		ID:           uuid.New(),
		TenantID:     "t1",
		Model:        "model-a",
		RouteGroup:   "",
		ResponseJSON: json.RawMessage(b),
		Similarity:   0.96,
	}
}

// TestSemanticCache_Miss_FallsThrough verifies that on a cache miss the provider is called
// and the X-Semantic-Cache: MISS header is set.
func TestSemanticCache_Miss_FallsThrough(t *testing.T) {
	cfg := semCacheTestConfig()
	reg := semCacheRegistry(fixedVector())
	store := &fakeStorage{} // semCacheHit = nil → miss

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Semantic-Cache"); got != "MISS" {
		t.Errorf("X-Semantic-Cache: want MISS, got %q", got)
	}
}

// TestSemanticCache_Hit_SkipsProvider verifies that on a cache hit the upstream provider
// is NOT called, X-Semantic-Cache: HIT is set, and the body matches the cached response.
func TestSemanticCache_Hit_SkipsProvider(t *testing.T) {
	cfg := semCacheTestConfig()

	var callCount atomic.Int32
	reg := providers.NewRegistry()
	reg.Register("openai", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			callCount.Add(1)
			return &providers.ChatResponse{
				ID: "live-id", Object: "chat.completion", Created: 1, Model: req.Model,
				Choices: []providers.ChatChoice{{Index: 0, Message: providers.ChatMessage{Role: "assistant", Content: "live"}, FinishReason: "stop"}},
				Usage:   providers.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
			}, nil
		},
	})
	reg.Register("backup", successProvider(""))
	reg.RegisterEmbedding("openai", fakeEmbeddingProvider{vec: fixedVector()})

	store := &fakeStorage{semCacheHit: cachedEntry()}

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if callCount.Load() != 0 {
		t.Errorf("provider should not have been called on cache hit, got %d calls", callCount.Load())
	}
	if got := w.Header().Get("X-Semantic-Cache"); got != "HIT" {
		t.Errorf("X-Semantic-Cache: want HIT, got %q", got)
	}
	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != "cached-id" {
		t.Errorf("expected cached response ID, got %s", resp.ID)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.TextContent() != "cached response" {
		t.Errorf("unexpected response content: %+v", resp.Choices)
	}
}

// TestSemanticCache_Expired_Miss simulates an expired entry (storage returns miss).
// At the handler level this behaves the same as a regular miss.
func TestSemanticCache_Expired_Miss(t *testing.T) {
	cfg := semCacheTestConfig()
	reg := semCacheRegistry(fixedVector())
	store := &fakeStorage{} // nil = storage already filtered out the expired entry

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after expired cache miss, got %d: %s", w.Code, w.Body.String())
	}
	// Provider was called (miss path)
	if got := w.Header().Get("X-Semantic-Cache"); got != "MISS" {
		t.Errorf("X-Semantic-Cache: want MISS, got %q", got)
	}
}

// TestSemanticCache_Threshold_Miss simulates a below-threshold score (storage returns miss).
func TestSemanticCache_Threshold_Miss(t *testing.T) {
	cfg := semCacheTestConfig()
	reg := semCacheRegistry(fixedVector())
	store := &fakeStorage{} // storage simulates threshold filter by returning nil

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after threshold miss, got %d: %s", w.Code, w.Body.String())
	}
	// Provider was called (miss path)
	if got := w.Header().Get("X-Semantic-Cache"); got != "MISS" {
		t.Errorf("X-Semantic-Cache: want MISS, got %q", got)
	}
}

// TestSemanticCache_TenantIsolation verifies that the tenant ID passed to FindNearestSemanticCache
// is the requesting tenant's ID ("t1").
func TestSemanticCache_TenantIsolation(t *testing.T) {
	cfg := semCacheTestConfig()
	reg := semCacheRegistry(fixedVector())
	store := &fakeStorage{}

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	// Wait briefly for any goroutines
	time.Sleep(10 * time.Millisecond)

	store.mu.Lock()
	got := store.lastCacheTenant
	store.mu.Unlock()

	if got != "t1" {
		t.Errorf("lastCacheTenant: want t1, got %q", got)
	}
}

// TestSemanticCache_ScopeModel verifies that scope="model" is used and the candidate model
// name is passed to the storage layer.
func TestSemanticCache_ScopeModel(t *testing.T) {
	cfg := semCacheTestConfig()
	cfg.Tenants[0].SemanticCache.Scope = "model"
	reg := semCacheRegistry(fixedVector())
	store := &fakeStorage{}

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	time.Sleep(10 * time.Millisecond)

	store.mu.Lock()
	scope := store.lastCacheScope
	model := store.lastCacheModel
	store.mu.Unlock()

	if scope != storage.SemanticCacheScopeModel {
		t.Errorf("lastCacheScope: want %q, got %q", storage.SemanticCacheScopeModel, scope)
	}
	if model != "model-a" {
		t.Errorf("lastCacheModel: want model-a, got %q", model)
	}
}

// TestSemanticCache_OnlySuccessIsCached verifies that when all upstream attempts fail,
// no entry is written to the semantic cache.
func TestSemanticCache_OnlySuccessIsCached(t *testing.T) {
	cfg := semCacheTestConfig()

	reg := providers.NewRegistry()
	reg.Register("openai", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.UpstreamError{StatusCode: 500, Body: "internal error"}
		},
	})
	reg.Register("backup", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.UpstreamError{StatusCode: 500, Body: "internal error"}
		},
	})
	reg.RegisterEmbedding("openai", fakeEmbeddingProvider{vec: fixedVector()})

	store := &fakeStorage{}

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	// All attempts failed — expect a non-200 response
	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 when all providers fail, got %d", w.Code)
	}

	// Wait briefly for any goroutines
	time.Sleep(20 * time.Millisecond)

	store.mu.Lock()
	inserts := len(store.semCacheInserts)
	store.mu.Unlock()

	if inserts != 0 {
		t.Errorf("expected 0 cache inserts on failure, got %d", inserts)
	}
}
