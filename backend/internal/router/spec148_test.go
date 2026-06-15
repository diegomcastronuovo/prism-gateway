package router

import (
	"context"
	"errors"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// helpers ---------------------------------------------------------------

// makeModels builds a slice of ModelConfig from names using zero pricing.
func makeModels(names ...string) []config.ModelConfig {
	out := make([]config.ModelConfig, 0, len(names))
	for _, n := range names {
		out = append(out, config.ModelConfig{Name: n, Provider: "openai"})
	}
	return out
}

// routeGroups builds a map[string][]string route groups map.
func routeGroups(groups map[string][]string) map[string][]string {
	return groups
}

// baseSmartConfig returns a minimal SmartConfig (no stages) for smoke tests.
func baseSmartConfig() *config.SmartConfig {
	return &config.SmartConfig{
		Weights: config.SmartWeights{
			Cost:    1.0,
			Latency: 0.0,
			Errors:  0.0,
		},
	}
}

// =======================================================================
// SPEC_148 tests
// =======================================================================

// --- Config validation (tests 1-3 exercised via validateTenantConfig; ----
// --- those live in admin_config_unit_test.go; see spec148_config_test.go)
// The router package itself does not host validateTenantConfig.
// Tests 1-3 are therefore covered in the httpapi package.
// We start from test 4 here.

// Test 4: strategy without DefaultRouteGroup → uses all allowed candidates.
func TestSpec148_NoDefaultRouteGroup_UsesAllCandidates(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:  "t1",
		Strategy:  "round_robin",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		// DefaultRouteGroup intentionally empty
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Candidates) != 5 {
		t.Errorf("expected 5 candidates (all allowed), got %d: %v", len(result.Candidates), result.Candidates)
	}
}

// Test 5: strategy with DefaultRouteGroup="cheap" → uses only cheap ∩ allowed.
func TestSpec148_DefaultRouteGroup_FiltersToGroup(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		RouteGroups: routeGroups(map[string][]string{
			"cheap":   {"m1", "m2", "m3"},
			"math":    {"m4"},
			"finance": {"m5"},
		}),
		DefaultRouteGroup: "cheap",
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only m1, m2, m3 should be candidates
	for _, c := range result.Candidates {
		switch c {
		case "m1", "m2", "m3":
			// ok
		default:
			t.Errorf("unexpected candidate %q (outside cheap group)", c)
		}
	}
	if len(result.Candidates) != 3 {
		t.Errorf("expected 3 candidates, got %d: %v", len(result.Candidates), result.Candidates)
	}
}

// Test 6: group contains models outside allowed_models → those are excluded.
// (Security: intersection with allowed_models is enforced by PrecedenceResolver
// before reaching router.Select; here we model it by passing only allowed candidates.)
func TestSpec148_GroupModelsOutsideAllowed_AreExcluded(t *testing.T) {
	rt := New()
	// candidates = allowed models (only m1,m2 — m99 is NOT allowed)
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: makeModels("m1", "m2"), // m99 not in allowed set
		RouteGroups: routeGroups(map[string][]string{
			"cheap": {"m1", "m2", "m99"}, // m99 listed in group but not allowed
		}),
		DefaultRouteGroup: "cheap",
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only m1 and m2 should appear (m99 not in candidates)
	for _, c := range result.Candidates {
		if c == "m99" {
			t.Errorf("m99 appeared in candidates despite not being in allowed set")
		}
	}
	if len(result.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d: %v", len(result.Candidates), result.Candidates)
	}
}

// Test 7: intersection is empty → clear ErrDefaultRouteGroupEmpty error.
func TestSpec148_EmptyIntersection_ReturnsError(t *testing.T) {
	rt := New()
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: makeModels("m4", "m5"), // allowed
		RouteGroups: routeGroups(map[string][]string{
			"cheap": {"m1", "m2", "m3"}, // no overlap with allowed
		}),
		DefaultRouteGroup: "cheap",
	}
	_, err := rt.Select(req)
	if err == nil {
		t.Fatal("expected error for empty intersection, got nil")
	}
	var target *ErrDefaultRouteGroupEmpty
	if !errors.As(err, &target) {
		t.Errorf("expected ErrDefaultRouteGroupEmpty, got %T: %v", err, err)
	}
	if target.RouteGroup != "cheap" {
		t.Errorf("RouteGroup = %q, want %q", target.RouteGroup, "cheap")
	}
}

// Test 8: smart with no semantic match → uses default route group (cheap).
func TestSpec148_Smart_NoSemanticMatch_UsesDefaultGroup(t *testing.T) {
	rt := New()
	req := Request{
		TenantID: "t1",
		Strategy: "smart",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		SmartConfig: baseSmartConfig(),
		Messages:    []string{"hello"},
		RouteGroups: routeGroups(map[string][]string{
			"cheap":   {"m1", "m2", "m3"},
			"math":    {"m4"},
			"finance": {"m5"},
		}),
		DefaultRouteGroup: "cheap",
		// No embedding func / semantic lookup → semantic disabled
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range result.Candidates {
		switch c {
		case "m1", "m2", "m3":
			// ok
		default:
			t.Errorf("unexpected candidate %q — smart should use only cheap pool when no semantic match", c)
		}
	}
}

// Test 9: smart with semantic match → math group overrides default cheap.
func TestSpec148_Smart_SemanticMatchMath_UsesMathGroup(t *testing.T) {
	rt := New()

	mathAnchor := SemanticAnchorResult{
		Name:       "math_anchor",
		RouteGroup: "math",
		Distance:   0.05, // similarity = 0.95
	}

	req := Request{
		TenantID: "t1",
		Strategy: "smart",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		SmartConfig: &config.SmartConfig{
			Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
			Weights: config.SmartWeights{Cost: 1.0},
		},
		Messages: []string{"Solve this differential equation."},
		RouteGroups: routeGroups(map[string][]string{
			"cheap":   {"m1", "m2", "m3"},
			"math":    {"m4"},
			"finance": {"m5"},
		}),
		DefaultRouteGroup: "cheap",
		Ctx:               context.Background(),
		EmbeddingFunc:     stubEmbedding([]float64{1, 0}),
		SemanticLookup:    stubLookup(mathAnchor, true, nil),
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The math anchor should have been applied; only m4 is in math group
	if result.Selected != "m4" {
		t.Errorf("expected m4 (math group via semantic anchor), got %q", result.Selected)
	}
	for _, c := range result.Candidates {
		if c != "m4" {
			t.Errorf("unexpected candidate %q — only math group models expected", c)
		}
	}
}

// Test 10: smart with semantic match finance → finance group overrides cheap.
func TestSpec148_Smart_SemanticMatchFinance_UsesFinanceGroup(t *testing.T) {
	rt := New()

	financeAnchor := SemanticAnchorResult{
		Name:       "finance_anchor",
		RouteGroup: "finance",
		Distance:   0.03, // similarity = 0.97
	}

	req := Request{
		TenantID: "t1",
		Strategy: "smart",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		SmartConfig: &config.SmartConfig{
			Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
			Weights: config.SmartWeights{Cost: 1.0},
		},
		Messages: []string{"Calculate NPV for this portfolio."},
		RouteGroups: routeGroups(map[string][]string{
			"cheap":   {"m1", "m2", "m3"},
			"math":    {"m4"},
			"finance": {"m5"},
		}),
		DefaultRouteGroup: "cheap",
		Ctx:               context.Background(),
		EmbeddingFunc:     stubEmbedding([]float64{1, 0}),
		SemanticLookup:    stubLookup(financeAnchor, true, nil),
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Selected != "m5" {
		t.Errorf("expected m5 (finance group via semantic anchor), got %q", result.Selected)
	}
}

// Test 11: smart does not rank models outside the pool.
func TestSpec148_Smart_DoesNotRankOutsidePool(t *testing.T) {
	rt := New()
	cheapModels := []config.ModelConfig{
		{Name: "cheap-a", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.10, CompletionPer1M: 0.20}},
		{Name: "cheap-b", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.30}},
	}
	allCandidates := append(cheapModels,
		config.ModelConfig{Name: "expensive", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 10.0, CompletionPer1M: 30.0}},
	)

	req := Request{
		TenantID:   "t1",
		Strategy:   "smart",
		Candidates: allCandidates,
		SmartConfig: &config.SmartConfig{
			Weights: config.SmartWeights{Cost: 1.0},
		},
		Messages: []string{"hello"},
		RouteGroups: routeGroups(map[string][]string{
			"cheap": {"cheap-a", "cheap-b"},
		}),
		DefaultRouteGroup: "cheap",
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range result.Candidates {
		if c == "expensive" {
			t.Errorf("expensive model appeared in smart candidates despite being outside pool")
		}
	}
}

// Test 12: cost strategy does not select models outside pool.
func TestSpec148_Cost_DoesNotSelectOutsidePool(t *testing.T) {
	rt := New()
	allCandidates := []config.ModelConfig{
		{Name: "cheap-a", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.10, CompletionPer1M: 0.20}},
		{Name: "expensive", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.01, CompletionPer1M: 0.01}}, // cheapest pricing but outside pool
	}
	req := Request{
		TenantID:   "t1",
		Strategy:   "cost",
		Candidates: allCandidates,
		RouteGroups: routeGroups(map[string][]string{
			"cheap": {"cheap-a"},
		}),
		DefaultRouteGroup: "cheap",
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Selected == "expensive" {
		t.Errorf("cost strategy selected 'expensive' which is outside the default pool")
	}
	if result.Selected != "cheap-a" {
		t.Errorf("expected cheap-a, got %q", result.Selected)
	}
}

// Test 13: latency strategy does not select models outside pool.
func TestSpec148_Latency_DoesNotSelectOutsidePool(t *testing.T) {
	rt := New()
	// Pre-record a very low latency for the model outside the pool
	rt.metricsStore.UpdateLatencyEWMA(context.Background(), "t1", "fast-outside", 1.0)
	rt.metricsStore.UpdateLatencyEWMA(context.Background(), "t1", "slow-inside", 999.0)

	allCandidates := []config.ModelConfig{
		{Name: "slow-inside", Provider: "openai"},
		{Name: "fast-outside", Provider: "openai"},
	}
	req := Request{
		TenantID:   "t1",
		Strategy:   "latency",
		Candidates: allCandidates,
		RouteGroups: routeGroups(map[string][]string{
			"pool": {"slow-inside"},
		}),
		DefaultRouteGroup: "pool",
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Selected == "fast-outside" {
		t.Errorf("latency strategy selected 'fast-outside' which is outside the pool")
	}
	if result.Selected != "slow-inside" {
		t.Errorf("expected slow-inside (only candidate in pool), got %q", result.Selected)
	}
}

// Test 14: round_robin does not rotate outside pool.
func TestSpec148_RoundRobin_DoesNotRotateOutsidePool(t *testing.T) {
	rt := New()
	allCandidates := makeModels("in-a", "in-b", "out-c")
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: allCandidates,
		RouteGroups: routeGroups(map[string][]string{
			"pool": {"in-a", "in-b"},
		}),
		DefaultRouteGroup: "pool",
	}
	seen := map[string]int{}
	for i := 0; i < 10; i++ {
		r, err := rt.Select(req)
		if err != nil {
			t.Fatalf("round %d: unexpected error: %v", i, err)
		}
		seen[r.Selected]++
	}
	if seen["out-c"] > 0 {
		t.Errorf("out-c was selected %d times — should never appear (outside pool)", seen["out-c"])
	}
	if seen["in-a"] == 0 || seen["in-b"] == 0 {
		t.Errorf("not all pool members were selected: in-a=%d in-b=%d", seen["in-a"], seen["in-b"])
	}
}

// Test 16 (regression): semantic matches with use_anchor=false → DefaultRouteGroup must NOT apply.
// Before the fix, AnchorRouteGroup=="" with DefaultRouteGroup set would incorrectly restrict
// to the default pool even when a semantic anchor had already matched. The correct behavior is:
// if any semantic anchor was found (SemanticAnchor != ""), skip DefaultRouteGroup entirely.
func TestSpec148_Smart_SemanticMatchUseAnchorFalse_DoesNotApplyDefaultGroup(t *testing.T) {
	rt := New()

	// Anchor matches but use_anchor=false → route_group is NOT applied; preferred_models could still apply.
	mathAnchor := SemanticAnchorResult{
		Name:       "math_anchor",
		RouteGroup: "math", // This would be set in DB but use_anchor=false means we don't apply it
		Distance:   0.05,   // similarity = 0.95, well above threshold
	}

	req := Request{
		TenantID: "t1",
		Strategy: "smart",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		SmartConfig: &config.SmartConfig{
			// use_anchor=false — anchor matches but route group is not applied
			Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, false)},
			Weights: config.SmartWeights{Cost: 1.0},
		},
		Messages: []string{"Solve this differential equation."},
		RouteGroups: routeGroups(map[string][]string{
			"cheap": {"m1", "m2"},
			"math":  {"m4"},
		}),
		DefaultRouteGroup: "cheap",
		Ctx:               context.Background(),
		EmbeddingFunc:     stubEmbedding([]float64{1, 0}),
		SemanticLookup:    stubLookup(mathAnchor, true, nil),
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// DefaultRouteGroup ("cheap") must NOT be applied because semantic found an anchor.
	// All 5 candidates must be available for scoring.
	if len(result.Candidates) < 3 {
		t.Errorf("expected all candidates (semantic matched, use_anchor=false should not restrict to cheap), got %d: %v",
			len(result.Candidates), result.Candidates)
	}
	// m3, m4, m5 would be missing if DefaultRouteGroup incorrectly applied
	found := map[string]bool{}
	for _, c := range result.Candidates {
		found[c] = true
	}
	for _, expected := range []string{"m3", "m4", "m5"} {
		if !found[expected] {
			t.Errorf("candidate %q missing — DefaultRouteGroup was incorrectly applied (semantic had already matched)", expected)
		}
	}
}

// Test 15: tenants without DefaultRouteGroup keep existing behavior.
func TestSpec148_Regression_NoNewFieldSameAsToday(t *testing.T) {
	rt := New()
	models := makeModels("m1", "m2", "m3")
	req := Request{
		TenantID:          "t1",
		Strategy:          "round_robin",
		Candidates:        models,
		DefaultRouteGroup: "", // not set
	}
	// Should cycle through all 3 models without error
	r1, err := rt.Select(req)
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, _ := rt.Select(req)
	r3, _ := rt.Select(req)
	r4, _ := rt.Select(req)

	all := []string{r1.Selected, r2.Selected, r3.Selected, r4.Selected}
	if all[0] != "m1" || all[1] != "m2" || all[2] != "m3" || all[3] != "m1" {
		t.Errorf("round-robin without route group: unexpected order %v", all)
	}
}

// Test 17: round_robin + semantic anchor → candidate pool restricted to anchor route_group.
// This is the exact production bug: tenant uses round_robin strategy but has semantic stages.
// Before the fix, semantic was only evaluated for strategy=smart, so round_robin ignored anchors.
func TestSpec148_RoundRobin_SemanticAnchor_OverridesPool(t *testing.T) {
	rt := New()

	mathAnchor := SemanticAnchorResult{
		Name:       "math_anchor",
		RouteGroup: "math",
		Distance:   0.38, // similarity = 0.62, above threshold 0.6
	}

	// m4 and m5 are the math group. m1/m2/m3 are cheap. Default pool is cheap.
	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		SmartConfig: &config.SmartConfig{
			Stages: []config.SmartStage{semanticStage("semantic_intent", 0.60, true)},
		},
		Messages: []string{"ayudame con esta ecuacion"},
		RouteGroups: routeGroups(map[string][]string{
			"cheap": {"m1", "m2", "m3"},
			"math":  {"m4", "m5"},
		}),
		DefaultRouteGroup: "cheap",
		Ctx:               context.Background(),
		EmbeddingFunc:     stubEmbedding([]float64{1, 0}),
		SemanticLookup:    stubLookup(mathAnchor, true, nil),
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must select from math group, NOT cheap group.
	for _, c := range result.Candidates {
		switch c {
		case "m4", "m5":
			// ok — math group
		default:
			t.Errorf("candidate %q selected — should only contain math group models (m4, m5)", c)
		}
	}
	if len(result.Candidates) != 2 {
		t.Errorf("expected 2 candidates (math group), got %d: %v", len(result.Candidates), result.Candidates)
	}
}

// Test 18: round_robin + semantic stages + NO match → DefaultRouteGroup applies.
func TestSpec148_RoundRobin_SemanticNoMatch_FallsBackToDefaultGroup(t *testing.T) {
	rt := New()

	req := Request{
		TenantID:   "t1",
		Strategy:   "round_robin",
		Candidates: makeModels("m1", "m2", "m3", "m4", "m5"),
		SmartConfig: &config.SmartConfig{
			Stages: []config.SmartStage{semanticStage("semantic_intent", 0.60, true)},
		},
		Messages: []string{"tell me a joke"},
		RouteGroups: routeGroups(map[string][]string{
			"cheap": {"m1", "m2", "m3"},
			"math":  {"m4", "m5"},
		}),
		DefaultRouteGroup: "cheap",
		Ctx:               context.Background(),
		EmbeddingFunc:     stubEmbedding([]float64{1, 0}),
		// No anchor matches (found=false)
		SemanticLookup: stubLookup(SemanticAnchorResult{}, false, nil),
	}

	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must fall back to cheap group.
	for _, c := range result.Candidates {
		switch c {
		case "m1", "m2", "m3":
			// ok — cheap group
		default:
			t.Errorf("candidate %q selected — should only contain cheap group models when no semantic match", c)
		}
	}
	if len(result.Candidates) != 3 {
		t.Errorf("expected 3 candidates (cheap group), got %d: %v", len(result.Candidates), result.Candidates)
	}
}
