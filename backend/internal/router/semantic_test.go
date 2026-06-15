package router

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// --- helpers ---

func semanticCond(threshold float64) config.SmartRuleCondition {
	return config.SmartRuleCondition{
		SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: threshold},
	}
}

func semanticStage(name string, threshold float64, useAnchor bool) config.SmartStage {
	return config.SmartStage{
		Name: name,
		Rules: []config.SmartStageRule{
			{
				When:   semanticCond(threshold),
				Action: config.SmartAction{UseAnchor: useAnchor},
			},
		},
	}
}

// stubEmbedding returns a fixed unit vector for any input.
func stubEmbedding(vec []float64) func(context.Context, string) ([]float64, error) {
	return func(_ context.Context, _ string) ([]float64, error) {
		return vec, nil
	}
}

// stubLookup returns a fixed anchor for any query.
func stubLookup(anchor SemanticAnchorResult, found bool, err error) SemanticLookupFunc {
	return func(_ context.Context, _ string, _ []float64) (SemanticAnchorResult, bool, error) {
		return anchor, found, err
	}
}

// --- tests ---

// TestSemantic_Match verifies that when similarity >= threshold, the anchor's
// preferred_models and route_group are applied to the stage result.
func TestSemantic_Match(t *testing.T) {
	anchor := SemanticAnchorResult{
		Name:            "legal_contracts",
		RouteGroup:      "premium",
		PreferredModels: []string{"gemini-2.5-flash"},
		Distance:        0.10, // similarity = 0.90
	}
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
	}

	ev := NewStageEvaluator(cfg, []string{"Analyze this contract clause."})
	ev.SetSemanticDeps(context.Background(), "t1", stubEmbedding([]float64{1, 0}), stubLookup(anchor, true, nil))
	result := ev.Evaluate()

	if result.SemanticAnchor != "legal_contracts" {
		t.Errorf("SemanticAnchor = %q, want %q", result.SemanticAnchor, "legal_contracts")
	}
	wantSim := 1.0 - 0.10
	if math.Abs(result.SemanticSimilarity-wantSim) > 1e-9 {
		t.Errorf("SemanticSimilarity = %v, want %v", result.SemanticSimilarity, wantSim)
	}
	if result.AnchorRouteGroup != "premium" {
		t.Errorf("AnchorRouteGroup = %q, want %q", result.AnchorRouteGroup, "premium")
	}
	if len(result.PreferredModels) == 0 || result.PreferredModels[0] != "gemini-2.5-flash" {
		t.Errorf("PreferredModels = %v, want [gemini-2.5-flash]", result.PreferredModels)
	}
	if len(result.StagesEvaluated) == 0 || result.StagesEvaluated[0] != "semantic_intent" {
		t.Errorf("StagesEvaluated = %v, want [semantic_intent]", result.StagesEvaluated)
	}
}

// TestSemantic_NoMatch verifies that when similarity < threshold, no preferences are set.
func TestSemantic_NoMatch(t *testing.T) {
	anchor := SemanticAnchorResult{
		Name:     "legal_contracts",
		Distance: 0.50, // similarity = 0.50, below threshold 0.80
	}
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
	}

	ev := NewStageEvaluator(cfg, []string{"Hello world"})
	ev.SetSemanticDeps(context.Background(), "t1", stubEmbedding([]float64{1, 0}), stubLookup(anchor, true, nil))
	result := ev.Evaluate()

	if result.SemanticAnchor != "" {
		t.Errorf("expected no anchor match, got SemanticAnchor=%q", result.SemanticAnchor)
	}
	if len(result.PreferredModels) != 0 {
		t.Errorf("expected no PreferredModels, got %v", result.PreferredModels)
	}
	if result.AnchorRouteGroup != "" {
		t.Errorf("expected empty AnchorRouteGroup, got %q", result.AnchorRouteGroup)
	}
}

// TestSemantic_SkippedWhenPreferredAlreadySet verifies that semantic evaluation is
// skipped when a previous rule already set preferred_models (avoids unnecessary
// embedding API calls per the spec).
func TestSemantic_SkippedWhenPreferredAlreadySet(t *testing.T) {
	embeddingCalled := false
	embFn := func(_ context.Context, _ string) ([]float64, error) {
		embeddingCalled = true
		return []float64{1, 0}, nil
	}

	anchor := SemanticAnchorResult{Name: "legal_contracts", Distance: 0.05}
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{
			// Stage 1: sets preferred_models via a simple contains rule
			{
				Name: "guardrails",
				Rules: []config.SmartStageRule{
					{
						When:   config.SmartRuleCondition{Contains: []string{"contract"}},
						Action: config.SmartAction{PreferModels: []string{"gpt-4o"}},
					},
				},
			},
			// Stage 2: semantic rule (should be skipped because preferred_models already set)
			semanticStage("semantic_intent", 0.80, true),
		},
	}

	ev := NewStageEvaluator(cfg, []string{"Sign this contract"})
	ev.SetSemanticDeps(context.Background(), "t1", embFn, stubLookup(anchor, true, nil))
	result := ev.Evaluate()

	if embeddingCalled {
		t.Error("embedding was computed even though preferred_models were already set by a prior rule")
	}
	// Stage 1 preferred model must still be present
	if len(result.PreferredModels) == 0 || result.PreferredModels[0] != "gpt-4o" {
		t.Errorf("PreferredModels = %v, want [gpt-4o]", result.PreferredModels)
	}
	if result.SemanticAnchor != "" {
		t.Errorf("SemanticAnchor should be empty, got %q", result.SemanticAnchor)
	}
}

// TestSemantic_FailOpenOnEmbeddingError verifies that when the embedding call fails,
// the semantic rule is silently ignored and routing continues normally.
func TestSemantic_FailOpenOnEmbeddingError(t *testing.T) {
	errEmb := func(_ context.Context, _ string) ([]float64, error) {
		return nil, errors.New("embedding service unavailable")
	}
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
	}

	ev := NewStageEvaluator(cfg, []string{"some prompt"})
	ev.SetSemanticDeps(context.Background(), "t1", errEmb, stubLookup(SemanticAnchorResult{}, false, nil))
	result := ev.Evaluate()

	if result.SemanticAnchor != "" || result.AnchorRouteGroup != "" || len(result.PreferredModels) != 0 {
		t.Errorf("expected empty result on embedding error, got anchor=%q rg=%q preferred=%v",
			result.SemanticAnchor, result.AnchorRouteGroup, result.PreferredModels)
	}
	if result.Blocked {
		t.Error("request must not be blocked when embedding fails")
	}
}

// TestSemantic_FailOpenWhenNoAnchors verifies that when the lookup returns found=false,
// the semantic rule is silently ignored.
func TestSemantic_FailOpenWhenNoAnchors(t *testing.T) {
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
	}

	ev := NewStageEvaluator(cfg, []string{"some prompt"})
	ev.SetSemanticDeps(
		context.Background(), "t1",
		stubEmbedding([]float64{1, 0}),
		stubLookup(SemanticAnchorResult{}, false, nil), // no anchors
	)
	result := ev.Evaluate()

	if result.SemanticAnchor != "" || result.AnchorRouteGroup != "" {
		t.Errorf("expected empty result when no anchors found, got anchor=%q rg=%q",
			result.SemanticAnchor, result.AnchorRouteGroup)
	}
}

// TestSemantic_FailOpenOnNaNSimilarity verifies that a NaN similarity (e.g. from a
// zero-norm vector) is treated as fail open.
func TestSemantic_FailOpenOnNaNSimilarity(t *testing.T) {
	// distance = NaN → similarity = NaN
	anchor := SemanticAnchorResult{Name: "nan_anchor", Distance: math.NaN()}
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
	}

	ev := NewStageEvaluator(cfg, []string{"some prompt"})
	ev.SetSemanticDeps(context.Background(), "t1", stubEmbedding([]float64{1, 0}), stubLookup(anchor, true, nil))
	result := ev.Evaluate()

	if result.SemanticAnchor != "" {
		t.Errorf("expected fail-open for NaN similarity, got SemanticAnchor=%q", result.SemanticAnchor)
	}
}

// TestSemantic_EmbeddingComputedOnce verifies that the prompt embedding is computed
// at most once even when multiple semantic rules exist.
func TestSemantic_EmbeddingComputedOnce(t *testing.T) {
	calls := 0
	embFn := func(_ context.Context, _ string) ([]float64, error) {
		calls++
		return []float64{1, 0}, nil
	}
	anchor := SemanticAnchorResult{Name: "a", Distance: 0.05}
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{
			semanticStage("stage1", 0.90, false), // won't match (similarity=0.95 >= 0.90 actually matches)
			semanticStage("stage2", 0.50, true),  // both thresholds
		},
	}

	ev := NewStageEvaluator(cfg, []string{"test prompt"})
	ev.SetSemanticDeps(context.Background(), "t1", embFn, stubLookup(anchor, true, nil))
	ev.Evaluate()

	// Stage1 matches (0.95 >= 0.90) → preferred_models set (empty) → stage2 is skipped.
	// Embedding should be computed exactly once.
	if calls != 1 {
		t.Errorf("embedding computed %d times, want 1", calls)
	}
}

// TestSemantic_NoDepsFailOpen verifies fail-open when no semantic deps are configured.
func TestSemantic_NoDepsFailOpen(t *testing.T) {
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{semanticStage("semantic_intent", 0.80, true)},
	}

	ev := NewStageEvaluator(cfg, []string{"some prompt"})
	// No SetSemanticDeps call → deps are nil
	result := ev.Evaluate()

	if result.SemanticAnchor != "" || result.AnchorRouteGroup != "" {
		t.Errorf("expected empty result with no deps, got anchor=%q rg=%q",
			result.SemanticAnchor, result.AnchorRouteGroup)
	}
}

// TestSemantic_ExistingRulesUnaffected verifies that contains/prompt_length rules
// continue to work exactly as before when semantic rules are also present.
func TestSemantic_ExistingRulesUnaffected(t *testing.T) {
	threshold := 10
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{
			{
				Name: "guardrails",
				Rules: []config.SmartStageRule{
					{
						When:   config.SmartRuleCondition{PromptLength: &config.PromptLengthCondition{GT: &threshold}},
						Action: config.SmartAction{Block: true},
					},
				},
			},
			semanticStage("semantic_intent", 0.80, true),
		},
	}

	// Long message that triggers the prompt_length block before semantic runs
	ev := NewStageEvaluator(cfg, []string{"this message is definitely longer than 10 chars"})
	ev.SetSemanticDeps(context.Background(), "t1",
		stubEmbedding([]float64{1, 0}),
		stubLookup(SemanticAnchorResult{Name: "anchor", Distance: 0.05}, true, nil),
	)
	result := ev.Evaluate()

	if !result.Blocked {
		t.Error("expected request to be blocked by prompt_length rule")
	}
	// Semantic anchor must not have been applied (block came first → early return)
	if result.SemanticAnchor != "" {
		t.Errorf("SemanticAnchor should be empty when blocked, got %q", result.SemanticAnchor)
	}
}
