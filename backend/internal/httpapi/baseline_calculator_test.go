package httpapi

import (
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

func testConfigForBaseline() *config.Config {
	return &config.Config{
		Models: []config.ModelConfig{
			{
				Name: "gpt-4o-mini",
				Pricing: config.Pricing{
					PromptPer1M:     0.15,
					CompletionPer1M: 0.60,
				},
			},
			{
				Name: "claude-3-5-sonnet",
				Pricing: config.Pricing{
					PromptPer1M:     3.00,
					CompletionPer1M: 15.00,
				},
			},
		},
		Tenants: []config.TenantConfig{
			{
				ID:            "t1",
				AllowedModels: []string{"gpt-4o-mini", "claude-3-5-sonnet"},
			},
		},
	}
}

func TestBaselineCalculator_RoundRobin(t *testing.T) {
	cfg := testConfigForBaseline()
	tenant := &cfg.Tenants[0]
	calc := NewBaselineCalculator(cfg, tenant)

	details := []storage.UsageDetailRow{
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50}, // gpt-4o-mini
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50}, // claude-3-5-sonnet
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50}, // gpt-4o-mini (cycle)
	}

	cost := calc.CalculateRoundRobin(details)

	// Request 1: gpt-4o-mini = (100/1M * 0.15) + (50/1M * 0.60) = 0.000045
	// Request 2: claude = (100/1M * 3.00) + (50/1M * 15.00) = 0.00105
	// Request 3: gpt-4o-mini = 0.000045
	// Total = 0.00114

	expected := 0.00114
	if cost < expected-0.000001 || cost > expected+0.000001 {
		t.Errorf("expected ~%f, got %f", expected, cost)
	}
}

func TestBaselineCalculator_Cheapest(t *testing.T) {
	cfg := testConfigForBaseline()
	tenant := &cfg.Tenants[0]
	calc := NewBaselineCalculator(cfg, tenant)

	details := []storage.UsageDetailRow{
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
	}

	cost := calc.CalculateCheapest(details)

	// All requests use gpt-4o-mini (cheapest)
	// 2 * 0.000045 = 0.00009

	expected := 0.00009
	if cost < expected-0.000001 || cost > expected+0.000001 {
		t.Errorf("expected ~%f, got %f", expected, cost)
	}
}

func TestBaselineCalculator_FixedModel(t *testing.T) {
	cfg := testConfigForBaseline()
	tenant := &cfg.Tenants[0]
	calc := NewBaselineCalculator(cfg, tenant)

	details := []storage.UsageDetailRow{
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
	}

	cost, err := calc.CalculateFixedModel("claude-3-5-sonnet", details)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All requests use claude-3-5-sonnet
	// 2 * 0.00105 = 0.0021

	expected := 0.0021
	if cost < expected-0.000001 || cost > expected+0.000001 {
		t.Errorf("expected ~%f, got %f", expected, cost)
	}
}

func TestBaselineCalculator_FixedModel_NotAllowed(t *testing.T) {
	cfg := testConfigForBaseline()
	tenant := &cfg.Tenants[0]
	calc := NewBaselineCalculator(cfg, tenant)

	details := []storage.UsageDetailRow{
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
	}

	_, err := calc.CalculateFixedModel("gpt-4o", details)
	if err == nil {
		t.Error("expected error for non-allowed model")
	}
}

func TestBaselineCalculator_FixedModel_NotFound(t *testing.T) {
	cfg := testConfigForBaseline()
	tenant := &cfg.Tenants[0]
	calc := NewBaselineCalculator(cfg, tenant)

	details := []storage.UsageDetailRow{
		{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
	}

	_, err := calc.CalculateFixedModel("non-existent-model", details)
	if err == nil {
		t.Error("expected error for non-existent model")
	}
}

func TestBaselineCalculator_EmptyDetails(t *testing.T) {
	cfg := testConfigForBaseline()
	tenant := &cfg.Tenants[0]
	calc := NewBaselineCalculator(cfg, tenant)

	details := []storage.UsageDetailRow{}

	// All strategies should return 0 for empty details
	rrCost := calc.CalculateRoundRobin(details)
	if rrCost != 0 {
		t.Errorf("expected 0 for round_robin with empty details, got %f", rrCost)
	}

	cheapestCost := calc.CalculateCheapest(details)
	if cheapestCost != 0 {
		t.Errorf("expected 0 for cheapest with empty details, got %f", cheapestCost)
	}

	fixedCost, err := calc.CalculateFixedModel("gpt-4o-mini", details)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixedCost != 0 {
		t.Errorf("expected 0 for fixed_model with empty details, got %f", fixedCost)
	}
}
