package router

import (
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func TestErrorTypes(t *testing.T) {
	t.Run("ErrBlockedBySmartStage", func(t *testing.T) {
		err := &ErrBlockedBySmartStage{
			Reason: "PII detected",
			Stage:  "compliance",
		}
		expected := "request blocked by smart stage 'compliance': PII detected"
		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrNoCandidatesAfterSmartStages with bans", func(t *testing.T) {
		err := &ErrNoCandidatesAfterSmartStages{
			BannedModels: []string{"gpt-4o", "claude-3"},
		}
		if err.Error() != "no candidates remain after smart stage bans: [gpt-4o claude-3]" {
			t.Errorf("unexpected error message: %s", err.Error())
		}
	})

	t.Run("ErrNoCandidatesAfterSmartStages with constraints", func(t *testing.T) {
		err := &ErrNoCandidatesAfterSmartStages{
			ConstraintsUsed: true,
		}
		if err.Error() != "no candidates remain after smart stage constraints (cost/latency)" {
			t.Errorf("unexpected error message: %s", err.Error())
		}
	})

	t.Run("ErrNoAllowedModels", func(t *testing.T) {
		err := &ErrNoAllowedModels{TenantID: "tenant_123"}
		expected := "no models allowed for tenant 'tenant_123'"
		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrModelNotAllowed", func(t *testing.T) {
		err := &ErrModelNotAllowed{Model: "gpt-4o", TenantID: "tenant_123"}
		expected := "model 'gpt-4o' is not allowed for tenant 'tenant_123'"
		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrModelNotInRouteGroup", func(t *testing.T) {
		err := &ErrModelNotInRouteGroup{Model: "gpt-4o", RouteGroup: "cheap"}
		expected := "model 'gpt-4o' is not in route group 'cheap'"
		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrRouteGroupNotFound", func(t *testing.T) {
		err := &ErrRouteGroupNotFound{RouteGroup: "nonexistent"}
		expected := "route group 'nonexistent' not found"
		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})
}

func TestPrecedenceErrors(t *testing.T) {
	globalModels := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai"},
		{Name: "claude-sonnet-4-6", Provider: "anthropic"},
	}

	t.Run("no allowed models returns ErrNoAllowedModels", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "tenant_a",
			AllowedModels: []string{}, // Empty allowlist
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "", "")

		if _, ok := err.(*ErrNoAllowedModels); !ok {
			t.Errorf("expected ErrNoAllowedModels, got %T: %v", err, err)
		}
	})

	t.Run("route group not found returns ErrRouteGroupNotFound", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "tenant_a",
			AllowedModels: []string{"gpt-4o-mini"},
			Selection: config.SelectionConfig{
				RouteGroups: map[string][]string{
					"cheap": {"gpt-4o-mini"},
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "", "nonexistent")

		if _, ok := err.(*ErrRouteGroupNotFound); !ok {
			t.Errorf("expected ErrRouteGroupNotFound, got %T: %v", err, err)
		}
	})

	t.Run("model not in route group returns ErrModelNotInRouteGroup", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "tenant_a",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6"},
			Selection: config.SelectionConfig{
				RouteGroups: map[string][]string{
					"cheap": {"gpt-4o-mini"},
				},
				Precedence: config.PrecedenceConfig{
					ConflictPolicy: "error",
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "claude-sonnet-4-6", "cheap")

		if _, ok := err.(*ErrModelNotInRouteGroup); !ok {
			t.Errorf("expected ErrModelNotInRouteGroup, got %T: %v", err, err)
		}
	})

	t.Run("disallowed model returns ErrModelNotAllowed", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "tenant_a",
			AllowedModels: []string{"gpt-4o-mini"},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "claude-sonnet-4-6", "")

		if _, ok := err.(*ErrModelNotAllowed); !ok {
			t.Errorf("expected ErrModelNotAllowed, got %T: %v", err, err)
		}
	})
}

func TestSmartStageErrors(t *testing.T) {
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}},
		{Name: "claude-sonnet-4-6", Provider: "anthropic", Pricing: config.Pricing{PromptPer1M: 3.0, CompletionPer1M: 15.0}},
	}

	t.Run("blocked by smart stage returns ErrBlockedBySmartStage", func(t *testing.T) {
		rt := New()
		smartCfg := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "compliance",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								Contains: []string{"pii"},
							},
							Action: config.SmartAction{
								Block:  true,
								Reason: "PII detected in prompt",
							},
						},
					},
				},
			},
		}

		req := Request{
			TenantID:    "t1",
			Strategy:    "smart",
			Candidates:  models,
			SmartConfig: &smartCfg,
			Messages:    []string{"My PII is sensitive"},
		}

		_, err := rt.Select(req)

		errBlocked, ok := err.(*ErrBlockedBySmartStage)
		if !ok {
			t.Fatalf("expected ErrBlockedBySmartStage, got %T: %v", err, err)
		}
		if errBlocked.Reason != "PII detected in prompt" {
			t.Errorf("expected reason 'PII detected in prompt', got '%s'", errBlocked.Reason)
		}
		if errBlocked.Stage != "compliance" {
			t.Errorf("expected stage 'compliance', got '%s'", errBlocked.Stage)
		}
	})

	t.Run("all models banned returns ErrNoCandidatesAfterSmartStages", func(t *testing.T) {
		rt := New()
		smartCfg := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "ban_all",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								Contains: []string{"test"},
							},
							Action: config.SmartAction{
								BanModels: []string{"gpt-4o-mini", "claude-sonnet-4-6"},
							},
						},
					},
				},
			},
		}

		req := Request{
			TenantID:    "t1",
			Strategy:    "smart",
			Candidates:  models,
			SmartConfig: &smartCfg,
			Messages:    []string{"This is a test"},
		}

		_, err := rt.Select(req)

		errNoCandidates, ok := err.(*ErrNoCandidatesAfterSmartStages)
		if !ok {
			t.Fatalf("expected ErrNoCandidatesAfterSmartStages, got %T: %v", err, err)
		}
		if len(errNoCandidates.BannedModels) != 2 {
			t.Errorf("expected 2 banned models, got %d", len(errNoCandidates.BannedModels))
		}
	})

	t.Run("cost constraint filters all returns ErrNoCandidatesAfterSmartStages", func(t *testing.T) {
		rt := New()
		maxCost := 0.01 // Impossibly low
		smartCfg := config.SmartConfig{
			Stages: []config.SmartStage{
				{
					Name: "strict_cost",
					Rules: []config.SmartStageRule{
						{
							When: config.SmartRuleCondition{
								Contains: []string{"budget"},
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

		req := Request{
			TenantID:    "t1",
			Strategy:    "smart",
			Candidates:  models,
			SmartConfig: &smartCfg,
			Messages:    []string{"I'm on a tight budget"},
		}

		_, err := rt.Select(req)

		errNoCandidates, ok := err.(*ErrNoCandidatesAfterSmartStages)
		if !ok {
			t.Fatalf("expected ErrNoCandidatesAfterSmartStages, got %T: %v", err, err)
		}
		if !errNoCandidates.ConstraintsUsed {
			t.Error("expected ConstraintsUsed to be true")
		}
	})
}
