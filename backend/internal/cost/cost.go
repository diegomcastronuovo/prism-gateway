package cost

import "github.com/diegomcastronuovo/prism-gateway/internal/config"

// EffectiveCostResult holds the breakdown of a per-request cost calculation.
type EffectiveCostResult struct {
	TokenCostPerRequest  float64
	InfraCostPerRequest  float64
	EffectiveCostPerRequest float64
}

// CalculateEffectiveCost computes the effective cost per request for a model,
// combining variable token cost with amortized infrastructure cost.
//
// tokenCostPerRequest is the already-computed token cost for the request.
// requestsThisMonth is the number of requests for the model in the current month
// (sourced from GET /admin/tenants/{tenant_id}/usage/summary?month=YYYY-MM →
// models[model].requests). When 0, the infra allocation is treated as 0 (safe
// divide-by-zero guard).
func CalculateEffectiveCost(m config.ModelConfig, tokenCostPerRequest float64, requestsThisMonth int) EffectiveCostResult {
	var infraCost float64
	if m.InfrastructureMonthlyUSD > 0 && requestsThisMonth > 0 {
		infraCost = m.InfrastructureMonthlyUSD / float64(requestsThisMonth)
	}
	return EffectiveCostResult{
		TokenCostPerRequest:     tokenCostPerRequest,
		InfraCostPerRequest:     infraCost,
		EffectiveCostPerRequest: tokenCostPerRequest + infraCost,
	}
}
