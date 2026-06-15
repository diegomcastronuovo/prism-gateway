package router

import (
	"context"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// SPEC_149: Smart routing rule match logging

// Test 1: contains rule match → RulesMatched populated with condition="contains"
func TestSpec149_Contains_RuleMatch(t *testing.T) {
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{{
			Name: "keyword_guard",
			Rules: []config.SmartStageRule{{
				When:   config.SmartRuleCondition{Contains: []string{"ssn", "credit card"}},
				Action: config.SmartAction{Block: true, Reason: "PII detected"},
			}},
		}},
	}

	ev := NewStageEvaluator(cfg, []string{"my ssn is 123-45-6789"})
	result := ev.Evaluate()

	if len(result.RulesMatched) != 1 {
		t.Fatalf("expected 1 rule match, got %d", len(result.RulesMatched))
	}
	rm := result.RulesMatched[0]
	if rm.Stage != "keyword_guard" {
		t.Errorf("Stage = %q, want %q", rm.Stage, "keyword_guard")
	}
	if rm.Condition != "contains" {
		t.Errorf("Condition = %q, want %q", rm.Condition, "contains")
	}
	if rm.Reason != "PII detected" {
		t.Errorf("Reason = %q, want %q", rm.Reason, "PII detected")
	}
	if _, ok := rm.Action["block"]; !ok {
		t.Errorf("Action missing 'block' key; got %v", rm.Action)
	}
}

// Test 2: prompt_length rule match → RulesMatched populated with condition="prompt_length"
func TestSpec149_PromptLength_RuleMatch(t *testing.T) {
	threshold := 5
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{{
			Name: "size_guard",
			Rules: []config.SmartStageRule{{
				When:   config.SmartRuleCondition{PromptLength: &config.PromptLengthCondition{GT: &threshold}},
				Action: config.SmartAction{Block: true, Reason: "too long"},
			}},
		}},
	}

	ev := NewStageEvaluator(cfg, []string{"this is definitely longer than 5 chars"})
	result := ev.Evaluate()

	if len(result.RulesMatched) != 1 {
		t.Fatalf("expected 1 rule match, got %d", len(result.RulesMatched))
	}
	rm := result.RulesMatched[0]
	if rm.Stage != "size_guard" {
		t.Errorf("Stage = %q, want %q", rm.Stage, "size_guard")
	}
	if rm.Condition != "prompt_length" {
		t.Errorf("Condition = %q, want %q", rm.Condition, "prompt_length")
	}
	// Value must be the actual prompt length (int)
	if _, ok := rm.Value.(int); !ok {
		t.Errorf("Value must be int, got %T", rm.Value)
	}
	if rm.Reason != "too long" {
		t.Errorf("Reason = %q, want %q", rm.Reason, "too long")
	}
}

// Test 3: semantic_similarity rule match → RulesMatched populated with condition="semantic_similarity"
func TestSpec149_Semantic_RuleMatch(t *testing.T) {
	threshold := 0.80
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{{
			Name: "math_stage",
			Rules: []config.SmartStageRule{{
				When:   config.SmartRuleCondition{SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: threshold}},
				Action: config.SmartAction{UseAnchor: true},
			}},
		}},
	}

	ev := NewStageEvaluator(cfg, []string{"solve this integral"})
	ev.SetSemanticDeps(
		context.Background(),
		"tenant1",
		func(_ context.Context, _ string) ([]float64, error) {
			return []float64{0.1, 0.2}, nil
		},
		func(_ context.Context, _ string, _ []float64) (SemanticAnchorResult, bool, error) {
			return SemanticAnchorResult{Name: "math_anchor", RouteGroup: "math", Distance: 0.05}, true, nil
		},
	)

	result := ev.Evaluate()

	if len(result.RulesMatched) != 1 {
		t.Fatalf("expected 1 rule match, got %d", len(result.RulesMatched))
	}
	rm := result.RulesMatched[0]
	if rm.Stage != "math_stage" {
		t.Errorf("Stage = %q, want %q", rm.Stage, "math_stage")
	}
	if rm.Condition != "semantic_similarity" {
		t.Errorf("Condition = %q, want %q", rm.Condition, "semantic_similarity")
	}
	// Value must be the similarity score (float64), approximately 0.95 (1 - 0.05)
	simVal, ok := rm.Value.(float64)
	if !ok {
		t.Fatalf("Value must be float64, got %T", rm.Value)
	}
	if simVal < 0.9 || simVal > 1.0 {
		t.Errorf("Value (similarity) = %.3f, expected ~0.95", simVal)
	}
	if useAnchor, _ := rm.Action["use_anchor"].(bool); !useAnchor {
		t.Errorf("Action['use_anchor'] must be true, got %v", rm.Action)
	}
}

// Test 4: no rule matches → RulesMatched is empty / nil
func TestSpec149_NoMatch_NoRulesMatched(t *testing.T) {
	threshold := 5
	cfg := config.SmartConfig{
		Stages: []config.SmartStage{{
			Name: "size_guard",
			Rules: []config.SmartStageRule{{
				// Condition: prompt must be > 1000 chars to match — our message is short
				When:   config.SmartRuleCondition{PromptLength: &config.PromptLengthCondition{GT: &threshold}},
				Action: config.SmartAction{Block: true},
			}},
		}},
	}

	ev := NewStageEvaluator(cfg, []string{"hi"}) // only 2 chars, does NOT exceed 5
	result := ev.Evaluate()

	if len(result.RulesMatched) != 0 {
		t.Errorf("expected no rule matches for non-matching prompt, got %d", len(result.RulesMatched))
	}
}
