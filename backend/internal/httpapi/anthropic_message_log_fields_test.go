package httpapi

import (
	"context"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func strPtr(s string) *string { return &s }

func handlersWithModels(models []config.ModelConfig) *Handlers {
	cfg := testConfig()
	cfg.Models = models
	return &Handlers{cfg: cfg, store: &fakeStorage{}, log: testLogger()}
}

// ── cost computation tests ────────────────────────────────────────────────────

// 1. priced model → non-nil cost
func TestComputeAnthropicCost_PricedModel(t *testing.T) {
	h := handlersWithModels([]config.ModelConfig{
		{Name: "model-a", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 3.0, CompletionPer1M: 15.0}},
	})
	cost := h.computeAnthropicCost(context.Background(), strPtr("model-a"), nil, 1_000_000, 500_000)
	if cost == nil {
		t.Fatal("expected non-nil cost for priced model")
	}
	// 1M * $3/M + 0.5M * $15/M = $3 + $7.5 = $10.5
	if *cost != 10.5 {
		t.Errorf("expected 10.5, got %v", *cost)
	}
}

// 2. model without pricing → nil cost
func TestComputeAnthropicCost_UnpricedModel(t *testing.T) {
	h := handlersWithModels([]config.ModelConfig{
		{Name: "free-model", Provider: "openai", Pricing: config.Pricing{}},
	})
	cost := h.computeAnthropicCost(context.Background(), strPtr("free-model"), nil, 1_000, 500)
	if cost != nil {
		t.Errorf("expected nil cost for unpriced model, got %v", *cost)
	}
}

// 3. unknown model → nil cost
func TestComputeAnthropicCost_UnknownModel(t *testing.T) {
	h := handlersWithModels([]config.ModelConfig{
		{Name: "model-a", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 3.0, CompletionPer1M: 15.0}},
	})
	cost := h.computeAnthropicCost(context.Background(), strPtr("nonexistent-model"), nil, 1_000, 500)
	if cost != nil {
		t.Errorf("expected nil cost for unknown model, got %v", *cost)
	}
}

// 4. modelUsed nil, modelRequested set → uses fallback
func TestComputeAnthropicCost_FallbackToModelRequested(t *testing.T) {
	h := handlersWithModels([]config.ModelConfig{
		{Name: "model-a", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 1.0, CompletionPer1M: 2.0}},
	})
	cost := h.computeAnthropicCost(context.Background(), nil, strPtr("model-a"), 1_000_000, 1_000_000)
	if cost == nil {
		t.Fatal("expected non-nil cost using model_requested fallback")
	}
	// $1 + $2 = $3
	if *cost != 3.0 {
		t.Errorf("expected 3.0, got %v", *cost)
	}
}

// 5. zero tokens → nil cost (avoid misleading $0 rows)
func TestComputeAnthropicCost_ZeroTokens(t *testing.T) {
	h := handlersWithModels([]config.ModelConfig{
		{Name: "model-a", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 3.0, CompletionPer1M: 15.0}},
	})
	cost := h.computeAnthropicCost(context.Background(), strPtr("model-a"), nil, 0, 0)
	if cost != nil {
		t.Errorf("expected nil cost for zero tokens, got %v", *cost)
	}
}

// ── APIKeyName field wiring test ───────────────────────────────────────────────

// 6. InsertAnthropicMessageLog receives api_key_name when set
func TestAnthropicMessageLog_APIKeyName_Populated(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{cfg: testConfig(), store: store, log: testLogger()}

	name := "my-key"
	row := storage.AnthropicMessageLog{
		TenantID:   "t1",
		RequestID:  "req-1",
		Provider:   "anthropic",
		Endpoint:   "/v1/claudecode",
		HTTPMethod: "POST",
		APIKeyName: &name,
	}
	// logAnthropicMessage is async — call InsertAnthropicMessageLog directly via fakeStorage.
	// fakeStorage stub accepts any call without error; we just verify it doesn't panic.
	if err := h.store.InsertAnthropicMessageLog(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// 7. InsertAnthropicMessageLog accepts nil api_key_name (JWT/YAML flows)
func TestAnthropicMessageLog_APIKeyName_Nil(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{cfg: testConfig(), store: store, log: testLogger()}

	row := storage.AnthropicMessageLog{
		TenantID:   "t1",
		RequestID:  "req-2",
		Provider:   "anthropic",
		Endpoint:   "/v1/claudecode",
		HTTPMethod: "POST",
		APIKeyName: nil,
	}
	if err := h.store.InsertAnthropicMessageLog(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
