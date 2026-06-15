package router

import (
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func TestSmartStageEvaluator(t *testing.T) {
	t.Run("block action on PII detection", func(t *testing.T) {
		smartConfig := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "compliance",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								Contains: []string{"pii", "ssn", "credit card"},
							},
							Action: config.SmartAction{
								Block:  true,
								Reason: "PII risk detected",
							},
						},
					},
				},
			},
		}

		messages := []string{"My SSN is 123-45-6789"}
		evaluator := NewStageEvaluator(smartConfig, messages)
		result := evaluator.Evaluate()

		if !result.Blocked {
			t.Error("expected request to be blocked")
		}
		if result.BlockReason != "PII risk detected" {
			t.Errorf("expected reason 'PII risk detected', got '%s'", result.BlockReason)
		}
	})

	t.Run("prefer models for JSON tasks", func(t *testing.T) {
		smartConfig := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "format_preference",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								Contains: []string{"json", "schema"},
							},
							Action: config.SmartAction{
								PreferModels: []string{"claude-sonnet-4-6"},
							},
						},
					},
				},
			},
		}

		messages := []string{"Generate a JSON schema for user data"}
		evaluator := NewStageEvaluator(smartConfig, messages)
		result := evaluator.Evaluate()

		if result.Blocked {
			t.Error("expected request not to be blocked")
		}
		if len(result.PreferredModels) != 1 || result.PreferredModels[0] != "claude-sonnet-4-6" {
			t.Errorf("expected preferred model 'claude-sonnet-4-6', got %v", result.PreferredModels)
		}
	})

	t.Run("ban expensive models for short prompts", func(t *testing.T) {
		smartConfig := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "size_optimization",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								MaxPromptTokens: intPtr(100),
							},
							Action: config.SmartAction{
								BanModels: []string{"claude-sonnet-4-6"},
							},
						},
					},
				},
			},
		}

		messages := []string{"Hello world"} // Very short (< 100 tokens)
		evaluator := NewStageEvaluator(smartConfig, messages)
		result := evaluator.Evaluate()

		if result.Blocked {
			t.Error("expected request not to be blocked")
		}
		if len(result.BannedModels) != 1 || result.BannedModels[0] != "claude-sonnet-4-6" {
			t.Errorf("expected banned model 'claude-sonnet-4-6', got %v", result.BannedModels)
		}
	})

	t.Run("set cost constraint", func(t *testing.T) {
		maxCost := 1.0
		smartConfig := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "cost_control",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								MaxPromptTokens: intPtr(500),
							},
							Action: config.SmartAction{
								SetConstraints: &config.SmartConstraints{
									MaxCostPer1M: &maxCost,
								},
							},
						},
					},
				},
			},
		}

		messages := []string{"Short prompt"}
		evaluator := NewStageEvaluator(smartConfig, messages)
		result := evaluator.Evaluate()

		if result.Constraints.MaxCostPer1M == nil {
			t.Fatal("expected cost constraint to be set")
		}
		if *result.Constraints.MaxCostPer1M != 1.0 {
			t.Errorf("expected cost constraint 1.0, got %f", *result.Constraints.MaxCostPer1M)
		}
	})

	t.Run("multiple stages evaluated in order", func(t *testing.T) {
		smartConfig := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "format",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								Contains: []string{"json"},
							},
							Action: config.SmartAction{
								PreferModels: []string{"claude-sonnet-4-6"},
							},
						},
					},
				},
				{
					Name: "size",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								MaxPromptTokens: intPtr(100),
							},
							Action: config.SmartAction{
								PreferModels: []string{"gpt-4o-mini"},
							},
						},
					},
				},
			},
		}

		messages := []string{"Generate JSON"} // Matches both stages
		evaluator := NewStageEvaluator(smartConfig, messages)
		result := evaluator.Evaluate()

		if len(result.StagesEvaluated) != 2 {
			t.Errorf("expected 2 stages evaluated, got %d", len(result.StagesEvaluated))
		}
		if len(result.PreferredModels) != 2 {
			t.Errorf("expected 2 preferred models, got %d", len(result.PreferredModels))
		}
	})

	t.Run("backwards compatibility with legacy rules", func(t *testing.T) {
		smartConfig := config.SmartConfig{
			Rules: []config.SmartRule{
				{
					Name: "prefer_cheap_for_short",
					When: config.SmartRuleCondition{
						MaxPromptTokens: intPtr(300),
					},
					PreferModels: []string{"gpt-4o-mini", "gemini-2.5-flash"},
				},
			},
			Stages: []config.SmartStage{}, // No stages, should use legacy rules
		}

		messages := []string{"Hello"}
		evaluator := NewStageEvaluator(smartConfig, messages)
		result := evaluator.Evaluate()

		if len(result.PreferredModels) != 2 {
			t.Errorf("expected 2 preferred models from legacy rules, got %d", len(result.PreferredModels))
		}
		if len(result.StagesEvaluated) != 1 || result.StagesEvaluated[0] != "legacy:prefer_cheap_for_short" {
			t.Errorf("expected legacy stage marker, got %v", result.StagesEvaluated)
		}
	})
}

func TestApplyStageResult(t *testing.T) {
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}},
		{Name: "claude-sonnet-4-6", Pricing: config.Pricing{PromptPer1M: 3.0, CompletionPer1M: 15.0}},
		{Name: "gemini-2.5-flash", Pricing: config.Pricing{PromptPer1M: 0.10, CompletionPer1M: 0.40}},
	}

	t.Run("ban models", func(t *testing.T) {
		result := SmartStageResult{
			BannedModels: []string{"claude-sonnet-4-6"},
		}

		filtered := ApplyStageResult(models, result)
		if len(filtered) != 2 {
			t.Errorf("expected 2 models after ban, got %d", len(filtered))
		}
		for _, m := range filtered {
			if m.Name == "claude-sonnet-4-6" {
				t.Error("banned model should not be in filtered list")
			}
		}
	})

	t.Run("cost constraint", func(t *testing.T) {
		maxCost := 2.0 // Total cost < 2.0 per 1M tokens
		result := SmartStageResult{
			Constraints: SmartConstraintsResult{
				MaxCostPer1M: &maxCost,
			},
		}

		filtered := ApplyStageResult(models, result)
		if len(filtered) != 2 {
			t.Errorf("expected 2 models under cost constraint, got %d", len(filtered))
		}
		// claude-sonnet-4-6 (18.0) should be excluded
		for _, m := range filtered {
			if m.Name == "claude-sonnet-4-6" {
				t.Error("expensive model should be filtered by cost constraint")
			}
		}
	})

	t.Run("prefer models reorder", func(t *testing.T) {
		result := SmartStageResult{
			PreferredModels: []string{"claude-sonnet-4-6"},
		}

		filtered := ApplyStageResult(models, result)
		if len(filtered) != 3 {
			t.Errorf("expected 3 models, got %d", len(filtered))
		}
		if filtered[0].Name != "claude-sonnet-4-6" {
			t.Errorf("expected claude-sonnet-4-6 first, got %s", filtered[0].Name)
		}
	})

	t.Run("blocked returns empty", func(t *testing.T) {
		result := SmartStageResult{
			Blocked:     true,
			BlockReason: "Test block",
		}

		filtered := ApplyStageResult(models, result)
		if len(filtered) != 0 {
			t.Errorf("expected empty list for blocked request, got %d", len(filtered))
		}
	})
}

func intPtr(i int) *int {
	return &i
}
