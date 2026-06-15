package httpapi

import (
	"fmt"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// BaselineCalculator simulates what costs would be under different strategies
type BaselineCalculator struct {
	cfg           *config.Config
	allowedModels []config.ModelConfig
}

// NewBaselineCalculator creates a calculator for a specific tenant
func NewBaselineCalculator(cfg *config.Config, tenant *config.TenantConfig) *BaselineCalculator {
	return &BaselineCalculator{
		cfg:           cfg,
		allowedModels: cfg.AllowedModelsForTenant(tenant),
	}
}

// CalculateRoundRobin distributes requests evenly across allowed models
func (bc *BaselineCalculator) CalculateRoundRobin(details []storage.UsageDetailRow) float64 {
	if len(bc.allowedModels) == 0 {
		return 0
	}

	totalCost := 0.0
	for i, row := range details {
		// Round-robin: cycle through models in order
		modelIdx := i % len(bc.allowedModels)
		model := bc.allowedModels[modelIdx]

		// Calculate cost with this model's pricing
		cost := bc.computeCost(model.Pricing, row.PromptTokens, row.CompletionTokens)
		totalCost += cost
	}

	return totalCost
}

// CalculateCheapest uses cheapest model for every request
func (bc *BaselineCalculator) CalculateCheapest(details []storage.UsageDetailRow) float64 {
	// Find cheapest model once
	cheapestModel := bc.getCheapestModel()
	if cheapestModel == nil {
		return 0
	}

	totalCost := 0.0
	for _, row := range details {
		cost := bc.computeCost(cheapestModel.Pricing, row.PromptTokens, row.CompletionTokens)
		totalCost += cost
	}

	return totalCost
}

// CalculateFixedModel uses specified model for every request
func (bc *BaselineCalculator) CalculateFixedModel(modelName string, details []storage.UsageDetailRow) (float64, error) {
	model := bc.cfg.ModelByName(modelName)
	if model == nil {
		return 0, fmt.Errorf("model '%s' not found", modelName)
	}

	// Verify model is allowed for this tenant
	allowed := false
	for _, m := range bc.allowedModels {
		if m.Name == modelName {
			allowed = true
			break
		}
	}
	if !allowed {
		return 0, fmt.Errorf("model '%s' not allowed for tenant", modelName)
	}

	totalCost := 0.0
	for _, row := range details {
		cost := bc.computeCost(model.Pricing, row.PromptTokens, row.CompletionTokens)
		totalCost += cost
	}

	return totalCost, nil
}

// computeCost calculates cost for tokens at given pricing
func (bc *BaselineCalculator) computeCost(pricing config.Pricing, promptTokens, completionTokens int) float64 {
	promptCost := (float64(promptTokens) / 1_000_000.0) * pricing.PromptPer1M
	completionCost := (float64(completionTokens) / 1_000_000.0) * pricing.CompletionPer1M
	return promptCost + completionCost
}

// getCheapestModel finds cheapest model by total pricing
func (bc *BaselineCalculator) getCheapestModel() *config.ModelConfig {
	if len(bc.allowedModels) == 0 {
		return nil
	}

	cheapest := &bc.allowedModels[0]
	cheapestPrice := cheapest.Pricing.PromptPer1M + cheapest.Pricing.CompletionPer1M

	for i := range bc.allowedModels {
		model := &bc.allowedModels[i]
		price := model.Pricing.PromptPer1M + model.Pricing.CompletionPer1M
		if price < cheapestPrice {
			cheapest = model
			cheapestPrice = price
		}
	}

	return cheapest
}
