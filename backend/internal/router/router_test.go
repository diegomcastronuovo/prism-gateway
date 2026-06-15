package router

import (
	"context"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

var testModels = []config.ModelConfig{
	{Name: "gpt-4o-mini", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}},
	{Name: "claude-3-5-sonnet", Provider: "anthropic", Pricing: config.Pricing{PromptPer1M: 3.00, CompletionPer1M: 15.00}},
	{Name: "local-llama", Provider: "local", Pricing: config.Pricing{PromptPer1M: 0.00, CompletionPer1M: 0.00}},
}

func TestRoundRobin(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: testModels,
	}

	r1, _ := rt.Select(req)
	r2, _ := rt.Select(req)
	r3, _ := rt.Select(req)
	r4, _ := rt.Select(req)

	if r1.Selected != "gpt-4o-mini" {
		t.Errorf("round 1: got %s, want gpt-4o-mini", r1.Selected)
	}
	if r2.Selected != "claude-3-5-sonnet" {
		t.Errorf("round 2: got %s, want claude-3-5-sonnet", r2.Selected)
	}
	if r3.Selected != "local-llama" {
		t.Errorf("round 3: got %s, want local-llama", r3.Selected)
	}
	if r4.Selected != "gpt-4o-mini" {
		t.Errorf("round 4 (wrap): got %s, want gpt-4o-mini", r4.Selected)
	}
}

func TestRoundRobinCandidatesOrder(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: testModels,
	}

	r, _ := rt.Select(req)
	// First selection: candidates should start with gpt-4o-mini, then cycle
	if len(r.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(r.Candidates))
	}
	if r.Candidates[0] != "gpt-4o-mini" || r.Candidates[1] != "claude-3-5-sonnet" || r.Candidates[2] != "local-llama" {
		t.Errorf("unexpected order: %v", r.Candidates)
	}
}

func TestForcedModel(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "round_robin",
		Candidates:  testModels,
		ForcedModel: "claude-3-5-sonnet",
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Selected != "claude-3-5-sonnet" {
		t.Errorf("got %s, want claude-3-5-sonnet", r.Selected)
	}
	// Forced model should be first, rest as fallback
	if r.Candidates[0] != "claude-3-5-sonnet" {
		t.Errorf("forced model not first in candidates: %v", r.Candidates)
	}
}

func TestForcedModelNotAllowed(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "round_robin",
		Candidates:  testModels[:2], // only gpt-4o-mini and claude
		ForcedModel: "local-llama",
	}

	_, err := rt.Select(req)
	if err == nil {
		t.Error("expected error for forced model not in candidates")
	}
}

func TestRouteGroupRestriction(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: testModels,
		RouteGroup: "cheap",
		RouteGroups: map[string][]string{
			"cheap":   {"gpt-4o-mini", "local-llama"},
			"premium": {"claude-3-5-sonnet"},
		},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	// Should only have models from "cheap" group
	for _, c := range r.Candidates {
		if c == "claude-3-5-sonnet" {
			t.Error("claude-3-5-sonnet should not be in cheap group candidates")
		}
	}
	if r.Selected != "gpt-4o-mini" && r.Selected != "local-llama" {
		t.Errorf("selected %s, expected a model from cheap group", r.Selected)
	}
}

func TestRouteGroupWithForcedModel(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "round_robin",
		Candidates:  testModels,
		RouteGroup:  "cheap",
		ForcedModel: "local-llama",
		RouteGroups: map[string][]string{
			"cheap": {"gpt-4o-mini", "local-llama"},
		},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Selected != "local-llama" {
		t.Errorf("got %s, want local-llama", r.Selected)
	}
}

func TestRouteGroupForcedModelNotInGroup(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "round_robin",
		Candidates:  testModels,
		RouteGroup:  "cheap",
		ForcedModel: "claude-3-5-sonnet", // not in cheap group
		RouteGroups: map[string][]string{
			"cheap": {"gpt-4o-mini", "local-llama"},
		},
	}

	_, err := rt.Select(req)
	if err == nil {
		t.Error("expected error for forced model not in route group")
	}
}

func TestFallbackWithDebugFail(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: testModels,
		FailModels: map[string]bool{"gpt-4o-mini": true},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Selected == "gpt-4o-mini" {
		t.Error("failed model should not be selected")
	}
	for _, c := range r.Candidates {
		if c == "gpt-4o-mini" {
			t.Error("failed model should not appear in candidates")
		}
	}
}

func TestFallbackAllFailed(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: testModels[:1],
		FailModels: map[string]bool{"gpt-4o-mini": true},
	}

	_, err := rt.Select(req)
	if err == nil {
		t.Error("expected error when all models failed")
	}
}

func TestFallbackForcedModelFailed(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "round_robin",
		Candidates:  testModels[:2],
		ForcedModel: "gpt-4o-mini",
		FailModels:  map[string]bool{"gpt-4o-mini": true},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	// Should fall back to the next candidate
	if r.Selected != "claude-3-5-sonnet" {
		t.Errorf("got %s, want claude-3-5-sonnet as fallback", r.Selected)
	}
}

func TestLatencyStrategy(t *testing.T) {
	rt := New()

	// Seed EWMA latencies
	rt.RecordLatency("t1", "gpt-4o-mini", 200)
	rt.RecordLatency("t1", "claude-3-5-sonnet", 50)
	rt.RecordLatency("t1", "local-llama", 100)

	req := Request{
		TenantID:   "t1",
		Strategy:   "latency",
		Candidates: testModels,
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Selected != "claude-3-5-sonnet" {
		t.Errorf("got %s, want claude-3-5-sonnet (lowest latency)", r.Selected)
	}
	if r.Candidates[0] != "claude-3-5-sonnet" || r.Candidates[1] != "local-llama" || r.Candidates[2] != "gpt-4o-mini" {
		t.Errorf("unexpected latency order: %v", r.Candidates)
	}
}

func TestLatencyEWMAUpdate(t *testing.T) {
	rt := New()

	rt.RecordLatency("t1", "m1", 100)
	v1, _ := rt.GetLatency("t1", "m1")
	if v1 != 100 {
		t.Errorf("initial EWMA should be 100, got %f", v1)
	}

	rt.RecordLatency("t1", "m1", 200)
	v2, _ := rt.GetLatency("t1", "m1")
	// EWMA = 0.3*200 + 0.7*100 = 60 + 70 = 130
	expected := 0.3*200 + 0.7*100
	if v2 != expected {
		t.Errorf("EWMA after update should be %f, got %f", expected, v2)
	}
}

func TestLatencyUnknownModelsGoLast(t *testing.T) {
	rt := New()

	rt.RecordLatency("t1", "gpt-4o-mini", 100)
	// claude-3-5-sonnet has no recorded latency

	req := Request{
		TenantID:   "t1",
		Strategy:   "latency",
		Candidates: testModels[:2],
	}

	r, _ := rt.Select(req)
	if r.Selected != "gpt-4o-mini" {
		t.Errorf("known-latency model should be preferred over unknown, got %s", r.Selected)
	}
}

func TestCostStrategy(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "cost",
		Candidates: testModels,
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	// local-llama is free (0.00), gpt-4o-mini is cheap (0.75), claude is expensive (18.00)
	if r.Selected != "local-llama" {
		t.Errorf("got %s, want local-llama (cheapest)", r.Selected)
	}
	if r.Candidates[0] != "local-llama" || r.Candidates[1] != "gpt-4o-mini" || r.Candidates[2] != "claude-3-5-sonnet" {
		t.Errorf("unexpected cost order: %v", r.Candidates)
	}
}

func TestHeaderStrategyNoForce(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "header",
		Candidates: testModels,
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	// Without forced model, header strategy preserves original order
	if r.Selected != "gpt-4o-mini" {
		t.Errorf("got %s, want gpt-4o-mini (first in list)", r.Selected)
	}
}

func TestNoCandidates(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: nil,
	}

	_, err := rt.Select(req)
	if err == nil {
		t.Error("expected error with no candidates")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hi", 1},
		{"hello world test", 4},
		{"a]longer string with more characters in it for testing", 13},
	}
	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got != tt.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestRouteGroupUnknownGroupKeepsAllCandidates(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: testModels,
		RouteGroup: "nonexistent",
		RouteGroups: map[string][]string{
			"cheap": {"local-llama"},
		},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Candidates) != 3 {
		t.Errorf("unknown group should keep all candidates, got %d", len(r.Candidates))
	}
}

// TestSemanticAnchorRouteGroup_HeaderWins verifies that when both a header route group
// (req.RouteGroup) and a semantic anchor route_group are present, the header wins.
// The anchor route group must NOT further restrict candidates that were already filtered
// by the explicit header group.
func TestSemanticAnchorRouteGroup_HeaderWins(t *testing.T) {
	rt := New()

	candidates := []config.ModelConfig{
		{Name: "premium-gpt4", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 10}},
		{Name: "standard-claude", Provider: "anthropic", Pricing: config.Pricing{PromptPer1M: 3}},
	}

	// Semantic anchor returns route_group="standard" with high similarity (distance=0.05 → sim=0.95)
	anchorLookup := SemanticLookupFunc(func(_ context.Context, _ string, _ []float64) (SemanticAnchorResult, bool, error) {
		return SemanticAnchorResult{
			Name:       "my_anchor",
			RouteGroup: "standard",
			Distance:   0.05,
		}, true, nil
	})
	embFn := func(_ context.Context, _ string) ([]float64, error) {
		return []float64{1, 0}, nil
	}

	req := Request{
		TenantID:   "t1",
		Strategy:   "smart",
		Candidates: candidates,
		Messages:   []string{"test prompt"},
		// Header explicitly sets route group to "premium" — must win over anchor's "standard"
		RouteGroup: "premium",
		RouteGroups: map[string][]string{
			"premium":  {"premium-gpt4"},
			"standard": {"standard-claude"},
		},
		SmartConfig: &config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "semantic_stage",
					Rules: []config.SmartStageRule{
						{
							When:   config.SmartRuleCondition{SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: 0.80}},
							Action: config.SmartAction{UseAnchor: true},
						},
					},
				},
			},
		},
		Ctx:            context.Background(),
		EmbeddingFunc:  embFn,
		SemanticLookup: anchorLookup,
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// Header route group "premium" must win: only premium-gpt4 should remain.
	if result.Selected != "premium-gpt4" {
		t.Errorf("Selected = %q, want %q (header route group must win over anchor route group)", result.Selected, "premium-gpt4")
	}
	if len(result.Candidates) != 1 || result.Candidates[0] != "premium-gpt4" {
		t.Errorf("Candidates = %v, want [premium-gpt4]", result.Candidates)
	}
}

func TestSmartStrategy_Scoring(t *testing.T) {
	rt := New()

	// Use only paid models for this test (exclude local-llama which has 0 cost)
	paidModels := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}},
		{Name: "claude-3-5-sonnet", Provider: "anthropic", Pricing: config.Pricing{PromptPer1M: 3.00, CompletionPer1M: 15.00}},
	}

	// Record latency: gpt-4o-mini is faster
	rt.RecordLatency("t1", "gpt-4o-mini", 100)
	rt.RecordLatency("t1", "claude-3-5-sonnet", 300)

	// Record stats: claude has errors
	rt.UpdateModelStats("t1", "gpt-4o-mini", true)
	rt.UpdateModelStats("t1", "claude-3-5-sonnet", false)

	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.3, Latency: 0.3, Errors: 0.4},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  paidModels,
		SmartConfig: &smartCfg,
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// gpt-4o-mini should win (cheaper, faster, no errors)
	if r.Selected != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini, got %s", r.Selected)
	}
}

func TestSmartStrategy_Rules_ShortPrompt(t *testing.T) {
	rt := New()

	maxTokens := 100
	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.5, Latency: 0.3, Errors: 0.2},
		Rules: []config.SmartRule{
			{
				Name: "cheap_for_short",
				When: config.SmartRuleCondition{MaxPromptTokens: &maxTokens},
				PreferModels: []string{"gpt-4o-mini", "local-llama"},
			},
		},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  testModels,
		SmartConfig: &smartCfg,
		Messages:    []string{"hi"},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// Rule should filter to gpt-4o-mini or local-llama
	if r.Selected != "gpt-4o-mini" && r.Selected != "local-llama" {
		t.Errorf("rule should prefer gpt-4o-mini or local-llama, got %s", r.Selected)
	}
}

func TestSmartStrategy_Rules_ContainsKeyword(t *testing.T) {
	rt := New()

	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.5, Latency: 0.3, Errors: 0.2},
		Rules: []config.SmartRule{
			{
				Name: "prefer_claude_for_json",
				When: config.SmartRuleCondition{Contains: []string{"json", "schema"}},
				PreferModels: []string{"claude-3-5-sonnet"},
			},
		},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  testModels,
		SmartConfig: &smartCfg,
		Messages:    []string{"Please return a JSON schema for user profile"},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// Rule should match and prefer claude
	if r.Selected != "claude-3-5-sonnet" {
		t.Errorf("rule should prefer claude-3-5-sonnet for json keyword, got %s", r.Selected)
	}
}

func TestSmartStrategy_NoRulesMatch(t *testing.T) {
	rt := New()

	// Record latency to make results predictable
	rt.RecordLatency("t1", "local-llama", 50)
	rt.RecordLatency("t1", "gpt-4o-mini", 100)
	rt.RecordLatency("t1", "claude-3-5-sonnet", 300)

	maxTokens := 100
	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.4, Latency: 0.3, Errors: 0.3},
		Rules: []config.SmartRule{
			{
				Name: "short_prompt_rule",
				When: config.SmartRuleCondition{MaxPromptTokens: &maxTokens},
				PreferModels: []string{"local-llama"},
			},
		},
	}

	// Long message that won't match the rule
	longMessage := ""
	for i := 0; i < 200; i++ {
		longMessage += "word "
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  testModels,
		SmartConfig: &smartCfg,
		Messages:    []string{longMessage},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// No rule matches, should use weighted scoring (local-llama should win: free + fast)
	if r.Selected != "local-llama" {
		t.Errorf("expected local-llama to win on cost+latency, got %s", r.Selected)
	}
}

func TestSmartStrategy_DefaultWeights(t *testing.T) {
	rt := New()

	// Test with empty weights (should use defaults if any, or just not crash)
	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0, Latency: 0, Errors: 0},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  testModels,
		SmartConfig: &smartCfg,
		Messages:    []string{"test"},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// Should not crash and return a valid model
	found := false
	for _, m := range testModels {
		if m.Name == r.Selected {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("selected model %s not in candidates", r.Selected)
	}
}

// --- Normalization Safety Tests ---

func TestNormalizeMetrics_AllZeros(t *testing.T) {
	values := []float64{0, 0, 0}
	normalized := normalizeMetrics(values)

	for i, v := range normalized {
		if v != 0 {
			t.Errorf("all zeros: expected 0 at index %d, got %f", i, v)
		}
	}
}

func TestNormalizeMetrics_AllSameNonZero(t *testing.T) {
	values := []float64{5.0, 5.0, 5.0}
	normalized := normalizeMetrics(values)

	for i, v := range normalized {
		if v != 0 {
			t.Errorf("all same: expected 0 at index %d, got %f", i, v)
		}
	}
}

func TestNormalizeMetrics_SingleValue(t *testing.T) {
	values := []float64{10.0}
	normalized := normalizeMetrics(values)

	if len(normalized) != 1 {
		t.Fatalf("expected 1 value, got %d", len(normalized))
	}
	if normalized[0] != 0 {
		t.Errorf("single value: expected 0, got %f", normalized[0])
	}
}

func TestNormalizeMetrics_MixedValues(t *testing.T) {
	values := []float64{0, 5, 10}
	normalized := normalizeMetrics(values)

	if normalized[0] != 0 {
		t.Errorf("min value should normalize to 0, got %f", normalized[0])
	}
	if normalized[2] != 1 {
		t.Errorf("max value should normalize to 1, got %f", normalized[2])
	}
	if normalized[1] != 0.5 {
		t.Errorf("mid value should normalize to 0.5, got %f", normalized[1])
	}
}

func TestNormalizeMetrics_TwoValues(t *testing.T) {
	values := []float64{1, 3}
	normalized := normalizeMetrics(values)

	if normalized[0] != 0 {
		t.Errorf("min should be 0, got %f", normalized[0])
	}
	if normalized[1] != 1 {
		t.Errorf("max should be 1, got %f", normalized[1])
	}
}

func TestSmartStrategy_SingleCandidate(t *testing.T) {
	rt := New()

	singleModel := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}},
	}

	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.5, Latency: 0.3, Errors: 0.2},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  singleModel,
		SmartConfig: &smartCfg,
		Messages:    []string{"test"},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	if r.Selected != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini, got %s", r.Selected)
	}
}

func TestSmartStrategy_AllSameCost(t *testing.T) {
	rt := New()

	// All models with same pricing
	samePriceModels := []config.ModelConfig{
		{Name: "model-a", Provider: "p1", Pricing: config.Pricing{PromptPer1M: 1.0, CompletionPer1M: 2.0}},
		{Name: "model-b", Provider: "p2", Pricing: config.Pricing{PromptPer1M: 1.0, CompletionPer1M: 2.0}},
		{Name: "model-c", Provider: "p3", Pricing: config.Pricing{PromptPer1M: 1.0, CompletionPer1M: 2.0}},
	}

	// Record different latencies to differentiate
	rt.RecordLatency("t1", "model-a", 100)
	rt.RecordLatency("t1", "model-b", 200)
	rt.RecordLatency("t1", "model-c", 300)

	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.5, Latency: 0.5, Errors: 0.0},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  samePriceModels,
		SmartConfig: &smartCfg,
		Messages:    []string{"test"},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// model-a should win (fastest)
	if r.Selected != "model-a" {
		t.Errorf("expected model-a (fastest), got %s", r.Selected)
	}
}

func TestSmartStrategy_AllSameLatency(t *testing.T) {
	rt := New()

	// Record same latency for all
	rt.RecordLatency("t1", "gpt-4o-mini", 100)
	rt.RecordLatency("t1", "claude-3-5-sonnet", 100)
	rt.RecordLatency("t1", "local-llama", 100)

	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.5, Latency: 0.5, Errors: 0.0},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  testModels,
		SmartConfig: &smartCfg,
		Messages:    []string{"test"},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// local-llama should win (free)
	if r.Selected != "local-llama" {
		t.Errorf("expected local-llama (free), got %s", r.Selected)
	}
}

func TestSmartStrategy_AllSameErrorRate(t *testing.T) {
	rt := New()

	// Record same error rate for all
	rt.UpdateModelStats("t1", "gpt-4o-mini", true)
	rt.UpdateModelStats("t1", "claude-3-5-sonnet", true)
	rt.UpdateModelStats("t1", "local-llama", true)

	smartCfg := config.SmartConfig{
		Weights: config.SmartWeights{Cost: 0.5, Latency: 0.0, Errors: 0.5},
	}

	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  testModels,
		SmartConfig: &smartCfg,
		Messages:    []string{"test"},
	}

	r, err := rt.Select(req)
	if err != nil {
		t.Fatal(err)
	}

	// local-llama should win (free)
	if r.Selected != "local-llama" {
		t.Errorf("expected local-llama (free), got %s", r.Selected)
	}
}

// --- Tests for SPEC_smart_content_JSON_Schema ---

var jsonSchemaStageConfig = config.SmartConfig{
	Weights: config.SmartWeights{Cost: 0.4, Latency: 0.3, Errors: 0.3},
	Stages: []config.SmartStage{
		{
			Name: "structured_output",
			Rules: []config.SmartStageRule{
				{
					When:   config.SmartRuleCondition{Contains: []string{"json", "schema"}},
					Action: config.SmartAction{PreferModels: []string{"claude-sonnet-4-6"}},
				},
			},
		},
	},
}

var jsonSchemaCandidates = []config.ModelConfig{
	{Name: "gpt-4o-mini", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}},
	{Name: "claude-sonnet-4-6", Provider: "anthropic", Pricing: config.Pricing{PromptPer1M: 3.00, CompletionPer1M: 15.00}},
}

// Test 1: JSON prompt → rule triggers, preferred model placed first, SmartResult populated.
func TestSmartRouting_JSONPrompt_PreferredModelFirst(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  jsonSchemaCandidates,
		SmartConfig: &jsonSchemaStageConfig,
		Messages:    []string{"Return the output strictly as valid JSON with schema fields: id, name."},
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Preferred model must be first in the plan.
	if result.Selected != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6 first, got %s", result.Selected)
	}
	if len(result.Candidates) < 2 || result.Candidates[0] != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6 first in candidates, got %v", result.Candidates)
	}

	// SmartResult must be populated with stage info.
	if result.SmartResult == nil {
		t.Fatal("expected SmartResult to be non-nil when stage matched")
	}
	if len(result.SmartResult.StagesEvaluated) == 0 {
		t.Error("expected StagesEvaluated to be non-empty")
	}
	if result.SmartResult.StagesEvaluated[0] != "structured_output" {
		t.Errorf("expected stage 'structured_output', got %v", result.SmartResult.StagesEvaluated)
	}
	if len(result.SmartResult.PreferredModels) != 1 || result.SmartResult.PreferredModels[0] != "claude-sonnet-4-6" {
		t.Errorf("expected preferred model claude-sonnet-4-6, got %v", result.SmartResult.PreferredModels)
	}
	if result.SmartResult.Blocked {
		t.Error("expected Blocked=false")
	}
}

// Test 2: Non-JSON prompt → rule not triggered, SmartResult nil.
func TestSmartRouting_NonJSONPrompt_NoSmartResult(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  jsonSchemaCandidates,
		SmartConfig: &jsonSchemaStageConfig,
		Messages:    []string{"Explain quantum computing."},
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All candidates must still be present.
	if len(result.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %v", result.Candidates)
	}

	// No stage matched → StagesEvaluated is empty (cost optimizer may still set SmartResult with pricing).
	if result.SmartResult != nil && len(result.SmartResult.StagesEvaluated) > 0 {
		t.Errorf("expected no smart stages evaluated for non-matching prompt, got %v", result.SmartResult.StagesEvaluated)
	}
}

// Test 3: Preferred model fails → next candidate used (fallback ordering preserved).
func TestSmartRouting_PreferredModelFails_FallbackNext(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  jsonSchemaCandidates,
		SmartConfig: &jsonSchemaStageConfig,
		Messages:    []string{"Return a JSON schema for a user object."},
		FailModels:  map[string]bool{"claude-sonnet-4-6": true}, // simulate preferred model failing
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Preferred model is removed; gpt-4o-mini must be selected.
	if result.Selected != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini as fallback, got %s", result.Selected)
	}
	if len(result.Candidates) != 1 || result.Candidates[0] != "gpt-4o-mini" {
		t.Errorf("unexpected candidates: %v", result.Candidates)
	}
}

// ── prompt_length condition tests ──────────────────────────────────────────────

var promptLengthCandidates = []config.ModelConfig{
	{Name: "cheap-model", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.10}},
	{Name: "premium-model", Provider: "anthropic", Pricing: config.Pricing{PromptPer1M: 5.00}},
}

// TestPromptLength_GT_Match verifies that a gt condition routes to prefer_models when matched.
func TestPromptLength_GT_Match(t *testing.T) {
	gt := 100
	stageConfig := config.SmartConfig{
		Stages: []config.SmartStage{
			{
				Name: "long_prompt_stage",
				Rules: []config.SmartStageRule{
					{
						When: config.SmartRuleCondition{
							PromptLength: &config.PromptLengthCondition{GT: &gt},
						},
						Action: config.SmartAction{PreferModels: []string{"cheap-model"}},
					},
				},
			},
		},
	}

	// 150 chars > 100 → rule should match
	longMsg := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	rt := New()
	result, err := rt.Select(Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  promptLengthCandidates,
		SmartConfig: &stageConfig,
		Messages:    []string{longMsg},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Selected != "cheap-model" {
		t.Errorf("expected cheap-model (preferred), got %s", result.Selected)
	}
	if result.SmartResult == nil {
		t.Fatal("expected SmartResult non-nil")
	}
	if result.SmartResult.PromptLength != len(longMsg) {
		t.Errorf("PromptLength = %d, want %d", result.SmartResult.PromptLength, len(longMsg))
	}
}

// TestPromptLength_LT_Match verifies that a lt condition routes to prefer_models when matched.
func TestPromptLength_LT_Match(t *testing.T) {
	lt := 500
	stageConfig := config.SmartConfig{
		Stages: []config.SmartStage{
			{
				Name: "short_prompt_stage",
				Rules: []config.SmartStageRule{
					{
						When: config.SmartRuleCondition{
							PromptLength: &config.PromptLengthCondition{LT: &lt},
						},
						Action: config.SmartAction{PreferModels: []string{"premium-model"}},
					},
				},
			},
		},
	}

	rt := New()
	result, err := rt.Select(Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  promptLengthCandidates,
		SmartConfig: &stageConfig,
		Messages:    []string{"short prompt"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Selected != "premium-model" {
		t.Errorf("expected premium-model (preferred for short prompts), got %s", result.Selected)
	}
	if result.SmartResult == nil {
		t.Fatal("expected SmartResult non-nil")
	}
	if result.SmartResult.PromptLength != len("short prompt") {
		t.Errorf("PromptLength = %d, want %d", result.SmartResult.PromptLength, len("short prompt"))
	}
}

// TestPromptLength_Block verifies that block:true on a prompt_length rule returns
// ErrBlockedBySmartStage with IsPromptLength=true and the correct auto-generated reason.
func TestPromptLength_Block(t *testing.T) {
	gt := 50
	stageConfig := config.SmartConfig{
		Stages: []config.SmartStage{
			{
				Name: "block_large_prompts",
				Rules: []config.SmartStageRule{
					{
						When: config.SmartRuleCondition{
							PromptLength: &config.PromptLengthCondition{GT: &gt},
						},
						Action: config.SmartAction{Block: true},
					},
				},
			},
		},
	}

	// 60 chars > 50 → block
	bigMsg := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	rt := New()
	_, err := rt.Select(Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  promptLengthCandidates,
		SmartConfig: &stageConfig,
		Messages:    []string{bigMsg},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	blockedErr, ok := err.(*ErrBlockedBySmartStage)
	if !ok {
		t.Fatalf("expected *ErrBlockedBySmartStage, got %T: %v", err, err)
	}
	if !blockedErr.IsPromptLength {
		t.Error("expected IsPromptLength=true")
	}
	if blockedErr.PromptLength != len(bigMsg) {
		t.Errorf("PromptLength = %d, want %d", blockedErr.PromptLength, len(bigMsg))
	}
	wantReason := "prompt_length_gt_50"
	if blockedErr.Reason != wantReason {
		t.Errorf("Reason = %q, want %q", blockedErr.Reason, wantReason)
	}
}

// TestPromptLength_NoMatch verifies that a prompt_length rule that does NOT match
// has no effect and SmartResult is nil (no stage evaluated).
func TestPromptLength_NoMatch(t *testing.T) {
	gt := 10000
	stageConfig := config.SmartConfig{
		Stages: []config.SmartStage{
			{
				Name: "large_prompt_only",
				Rules: []config.SmartStageRule{
					{
						When: config.SmartRuleCondition{
							PromptLength: &config.PromptLengthCondition{GT: &gt},
						},
						Action: config.SmartAction{PreferModels: []string{"cheap-model"}},
					},
				},
			},
		},
	}

	rt := New()
	result, err := rt.Select(Request{
		TenantID:    "t1",
		Strategy:    "smart",
		Candidates:  promptLengthCandidates,
		SmartConfig: &stageConfig,
		Messages:    []string{"tiny"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No stage matched → StagesEvaluated is empty (cost optimizer may still set SmartResult with pricing).
	if result.SmartResult != nil && len(result.SmartResult.StagesEvaluated) > 0 {
		t.Errorf("expected no smart stages evaluated for non-matching prompt, got %v", result.SmartResult.StagesEvaluated)
	}
	// Both candidates available, selection non-empty
	if result.Selected == "" {
		t.Error("expected a model to be selected")
	}
}
