package router

import (
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// cheapModel returns a ModelConfig with the given name and pricing.
func cheapModel(name string, promptPer1M, completionPer1M float64) config.ModelConfig {
	return config.ModelConfig{
		Name:     name,
		Provider: "openai",
		Pricing: config.Pricing{
			PromptPer1M:     promptPer1M,
			CompletionPer1M: completionPer1M,
		},
	}
}

// smartReq builds a minimal smart-routing Request with the given candidates and messages.
func smartReq(candidates []config.ModelConfig, messages []string) Request {
	return Request{
		TenantID:   "t1",
		Strategy:   "smart",
		Candidates: candidates,
		Messages:   messages,
		SmartConfig: &config.SmartConfig{
			Weights: config.SmartWeights{
				Cost:    0.5,
				Latency: 0.3,
				Errors:  0.2,
			},
		},
	}
}

// Test 1: cheaper model is preferred when candidate pool is the same.
func TestCostOptimizer_CheaperModelPreferred(t *testing.T) {
	cheap := cheapModel("gpt-4o-mini", 0.15, 0.60)
	expensive := cheapModel("gpt-4o", 5.00, 15.00)

	rt := New()
	req := smartReq([]config.ModelConfig{expensive, cheap}, []string{"hello"})
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if result.Selected != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini (cheaper) to be selected, got %q", result.Selected)
	}
}

// Test 2: larger estimated prompt changes cost ranking.
// With a large prompt, the model with lower prompt_per_1m wins.
func TestCostOptimizer_LargerPromptChangesRanking(t *testing.T) {
	// modelA: cheap prompt, expensive completion
	modelA := cheapModel("model-a", 0.10, 10.00)
	// modelB: expensive prompt, cheap completion
	modelB := cheapModel("model-b", 1.00, 0.50)

	// Short prompt + small completion → completion cost dominates → modelA is more expensive
	// because completion_per_1m=10.00 vs modelB=0.50.
	// With a tiny prompt (5 tokens) and 300 completion tokens:
	//   modelA: 5/1M*0.10 + 300/1M*10.00 = 0 + 0.003 = ~$0.003
	//   modelB: 5/1M*1.00 + 300/1M*0.50  = 0 + 0.00015 = ~$0.00015
	// → modelB should be selected (cheaper total cost).
	rt := New()
	shortPromptReq := smartReq([]config.ModelConfig{modelA, modelB}, []string{"hi"})
	result, err := rt.Select(shortPromptReq)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if result.Selected != "model-b" {
		t.Errorf("short prompt: expected model-b (cheaper total), got %q", result.Selected)
	}

	// Very long prompt (100,000 chars ≈ 25,000 tokens) + small completion → prompt cost dominates.
	// modelA: 25000/1M*0.10 + 300/1M*10.00 = 0.0025 + 0.003 = ~$0.0055
	// modelB: 25000/1M*1.00 + 300/1M*0.50  = 0.025  + 0.00015 = ~$0.0252
	// → modelA should be selected (much cheaper for long prompts).
	longPrompt := make([]byte, 100_000)
	for i := range longPrompt {
		longPrompt[i] = 'a'
	}
	longPromptReq := smartReq([]config.ModelConfig{modelA, modelB}, []string{string(longPrompt)})
	resultLong, err := rt.Select(longPromptReq)
	if err != nil {
		t.Fatalf("Select (long prompt) error: %v", err)
	}
	if resultLong.Selected != "model-a" {
		t.Errorf("long prompt: expected model-a (cheaper for large inputs), got %q", resultLong.Selected)
	}
}

// Test 3: high budget pressure increases cheap-model preference.
// We set up two models with different pricing and verify that under high pressure
// the cheaper model is still selected (cost weight is amplified).
func TestCostOptimizer_HighBudgetPressure(t *testing.T) {
	cheap := cheapModel("cheap-model", 0.15, 0.60)
	expensive := cheapModel("expensive-model", 5.00, 15.00)

	rt := New()

	// High pressure (>= 0.80) → multiplier 2.0 on cost weight.
	req := smartReq([]config.ModelConfig{expensive, cheap}, []string{"analyze this document"})
	req.BudgetPressure = 0.85

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if result.Selected != "cheap-model" {
		t.Errorf("high pressure: expected cheap-model to be selected, got %q", result.Selected)
	}

	// Also verify the cost optimizer metadata was recorded.
	if result.SmartResult == nil {
		t.Fatal("expected SmartResult to be non-nil for smart routing")
	}
	if !result.SmartResult.CostOptimizerApplied {
		t.Error("CostOptimizerApplied should be true when pricing is configured")
	}
	if result.SmartResult.BudgetPressure != 0.85 {
		t.Errorf("BudgetPressure: want 0.85, got %f", result.SmartResult.BudgetPressure)
	}
}

// Test 4: missing pricing fails open (routing still works, model returned).
func TestCostOptimizer_MissingPricing_FailOpen(t *testing.T) {
	// Both models have zero pricing — no estimated cost data.
	modelA := config.ModelConfig{Name: "model-a", Provider: "openai"}
	modelB := config.ModelConfig{Name: "model-b", Provider: "openai"}

	rt := New()
	req := smartReq([]config.ModelConfig{modelA, modelB}, []string{"hello"})
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("Select should not error when pricing is missing (fail-open): %v", err)
	}
	if result.Selected == "" {
		t.Error("expected a model to be selected even when pricing is missing")
	}
	// CostOptimizerApplied should be false (no pricing data available).
	if result.SmartResult != nil && result.SmartResult.CostOptimizerApplied {
		t.Error("CostOptimizerApplied should be false when no pricing is configured")
	}
}

// Test 5: routing snapshot includes cost metadata.
func TestCostOptimizer_SnapshotIncludesCostMetadata(t *testing.T) {
	cheap := cheapModel("gpt-4o-mini", 0.15, 0.60)
	expensive := cheapModel("gpt-4", 30.00, 60.00)

	rt := New()
	req := smartReq([]config.ModelConfig{cheap, expensive}, []string{"summarize this"})
	req.BudgetPressure = 0.55 // medium pressure

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}

	sr := result.SmartResult
	if sr == nil {
		t.Fatal("SmartResult should be non-nil for smart routing with stages evaluated")
	}

	if !sr.CostOptimizerApplied {
		t.Error("CostOptimizerApplied should be true when pricing is configured")
	}
	if sr.BudgetPressure != 0.55 {
		t.Errorf("BudgetPressure: want 0.55, got %f", sr.BudgetPressure)
	}
	if len(sr.EstimatedCostsUSD) != 2 {
		t.Errorf("EstimatedCostsUSD: want 2 entries, got %d", len(sr.EstimatedCostsUSD))
	}
	if _, ok := sr.EstimatedCostsUSD["gpt-4o-mini"]; !ok {
		t.Error("EstimatedCostsUSD should contain gpt-4o-mini")
	}
	if _, ok := sr.EstimatedCostsUSD["gpt-4"]; !ok {
		t.Error("EstimatedCostsUSD should contain gpt-4")
	}
	// gpt-4o-mini should be cheaper than gpt-4 in the estimates.
	if sr.EstimatedCostsUSD["gpt-4o-mini"] >= sr.EstimatedCostsUSD["gpt-4"] {
		t.Errorf("gpt-4o-mini (%f) should cost less than gpt-4 (%f)",
			sr.EstimatedCostsUSD["gpt-4o-mini"], sr.EstimatedCostsUSD["gpt-4"])
	}
}

// ── Unit tests for helpers ────────────────────────────────────────────────────

func TestBudgetPressureMultiplier(t *testing.T) {
	cases := []struct {
		pressure float64
		want     float64
	}{
		{0.00, 1.0},
		{0.49, 1.0},
		{0.50, 1.5},
		{0.79, 1.5},
		{0.80, 2.0},
		{1.00, 2.0},
	}
	for _, tc := range cases {
		got := budgetPressureMultiplier(tc.pressure)
		if got != tc.want {
			t.Errorf("pressure=%.2f: want %.1f, got %.1f", tc.pressure, tc.want, got)
		}
	}
}

func TestEstimateRequestCostUSD(t *testing.T) {
	p := config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}
	// 1000 prompt tokens + 300 completion tokens
	// 1000/1M*0.15 + 300/1M*0.60 = 0.00015 + 0.00018 = 0.00033
	got := estimateRequestCostUSD(p, 1000, 300)
	want := 0.00033
	if diff := got - want; diff > 0.000001 || diff < -0.000001 {
		t.Errorf("want %.6f, got %.6f", want, got)
	}
}

func TestEstimateRequestCostUSD_ZeroPricing(t *testing.T) {
	p := config.Pricing{}
	got := estimateRequestCostUSD(p, 1000, 300)
	if got != 0 {
		t.Errorf("zero pricing should return 0, got %f", got)
	}
}
