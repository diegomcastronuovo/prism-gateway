package httpapi

import (
	"context"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// MonetizationResult holds all cost + pricing fields for an aggregated row.
type MonetizationResult struct {
	TotalRequests      int
	TotalEffectiveCost float64
	AvgCostPerRequest  float64 // TotalEffectiveCost / TotalRequests (0 when zero requests)
	TotalPrice         float64 // sum of per-model (effective_cost * (1 + markup/100))
	AvgPricePerRequest float64 // TotalPrice / TotalRequests (0 when zero requests)
	Margin             float64 // TotalPrice - TotalEffectiveCost
	MarginPct          float64 // Margin / TotalPrice (0 when TotalPrice == 0)
}

// computeMonetization calculates cost + price + margin for an aggregated usage row.
//
// Per-model:
//   effective_total_cost = token_spend + infra_allocation
//   infra_allocation     = infra_monthly_usd * (key_model_requests / tenant_model_requests)
//   model_price          = effective_total_cost * (1 + markup_percentage / 100)
//
// Aggregated:
//   total_effective_cost = sum(model effective_total_cost)
//   total_price          = sum(model_price)
//   margin               = total_price - total_effective_cost
//
// Returns zero MonetizationResult when modelBreakdown is empty or totalRequests == 0.
// Missing model config → infra = 0, markup = 0 (fail-open).
func computeMonetization(
	modelBreakdown []storage.APIKeyModelUsageRow,
	tenantModelReqs map[string]int64,
	allModels []config.ModelConfig,
) MonetizationResult {
	if len(modelBreakdown) == 0 {
		return MonetizationResult{}
	}

	modelCfg := make(map[string]config.ModelConfig, len(allModels))
	for _, m := range allModels {
		modelCfg[m.Name] = m
	}

	var totalEffectiveCost, totalPrice float64
	var totalRequests int

	for _, row := range modelBreakdown {
		totalRequests += row.Requests

		// Effective total cost for this model = token cost + infra allocation.
		effectiveTotalCost := row.Spend
		markup := 0.0

		if m, ok := modelCfg[row.Model]; ok {
			if m.InfrastructureMonthlyUSD > 0 {
				tenantTotal := tenantModelReqs[row.Model]
				if tenantTotal > 0 {
					infraPerRequest := m.InfrastructureMonthlyUSD / float64(tenantTotal)
					effectiveTotalCost += infraPerRequest * float64(row.Requests)
				}
			}
			markup = m.MarkupPercentage
		}

		totalEffectiveCost += effectiveTotalCost
		totalPrice += effectiveTotalCost * (1 + markup/100)
	}

	if totalRequests == 0 {
		return MonetizationResult{}
	}

	margin := totalPrice - totalEffectiveCost
	marginPct := 0.0
	if totalPrice > 0 {
		marginPct = margin / totalPrice
	}

	return MonetizationResult{
		TotalRequests:      totalRequests,
		TotalEffectiveCost: totalEffectiveCost,
		AvgCostPerRequest:  totalEffectiveCost / float64(totalRequests),
		TotalPrice:         totalPrice,
		AvgPricePerRequest: totalPrice / float64(totalRequests),
		Margin:             margin,
		MarginPct:          marginPct,
	}
}

// computeEffectiveCostPerRequest returns avg_cost_per_request_effective.
// Delegates to computeMonetization to avoid duplicating the per-model loop.
func computeEffectiveCostPerRequest(
	modelBreakdown []storage.APIKeyModelUsageRow,
	tenantModelReqs map[string]int64,
	allModels []config.ModelConfig,
) float64 {
	return computeMonetization(modelBreakdown, tenantModelReqs, allModels).AvgCostPerRequest
}

// applyMarkup computes price from effective cost per request and the model's markup_percentage.
//
//	price = effective_cost * (1 + markup_percentage / 100)
//
// When markupPercentage == 0, price equals effective cost.
func applyMarkup(effectiveCostPerRequest, markupPercentage float64) float64 {
	return effectiveCostPerRequest * (1 + markupPercentage/100)
}

// fetchEffectiveCostFromByModel fetches tenant model counts and computes
// avg_cost_per_request_effective for the drill-down endpoint (byModel already available).
func (h *Handlers) fetchEffectiveCostFromByModel(
	ctx context.Context,
	tenantID string,
	since time.Time,
	byModel []storage.APIKeyUsageByModelRow,
) float64 {
	return h.fetchMonetizationFromByModel(ctx, tenantID, since, byModel).AvgCostPerRequest
}

// fetchMonetizationFromByModel computes the full MonetizationResult for the drill-down
// endpoint from the already-available byModel slice.
func (h *Handlers) fetchMonetizationFromByModel(
	ctx context.Context,
	tenantID string,
	since time.Time,
	byModel []storage.APIKeyUsageByModelRow,
) MonetizationResult {
	if len(byModel) == 0 {
		return MonetizationResult{}
	}
	breakdown := make([]storage.APIKeyModelUsageRow, 0, len(byModel))
	for _, m := range byModel {
		breakdown = append(breakdown, storage.APIKeyModelUsageRow{
			Model:    m.Model,
			Requests: m.Requests,
			Spend:    m.Spend,
		})
	}
	tenantModelReqs, _ := h.store.GetTenantModelRequestCounts(ctx, tenantID, since)
	allModels := h.resolveGlobalConfig(ctx).Models
	return computeMonetization(breakdown, tenantModelReqs, allModels)
}
