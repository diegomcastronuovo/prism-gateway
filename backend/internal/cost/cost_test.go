package cost

import (
	"math"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func modelWith(infraMonthlyUSD float64) config.ModelConfig {
	return config.ModelConfig{
		Name:                     "test-model",
		Provider:                 "openai",
		InfrastructureMonthlyUSD: infraMonthlyUSD,
	}
}

// TC1: Model with infra cost = 0 — infra allocation is zero regardless of requests.
func TestCalculateEffectiveCost_InfraZero(t *testing.T) {
	result := CalculateEffectiveCost(modelWith(0), 0.005, 100)
	if result.InfraCostPerRequest != 0 {
		t.Errorf("expected infra=0, got %v", result.InfraCostPerRequest)
	}
	if result.EffectiveCostPerRequest != 0.005 {
		t.Errorf("expected effective=0.005, got %v", result.EffectiveCostPerRequest)
	}
}

// TC2: Model with infra cost > 0 and requests > 0.
func TestCalculateEffectiveCost_InfraPositive(t *testing.T) {
	result := CalculateEffectiveCost(modelWith(120), 0.005, 100)
	wantInfra := 120.0 / 100.0 // 1.2
	if math.Abs(result.InfraCostPerRequest-wantInfra) > 1e-9 {
		t.Errorf("expected infra=%v, got %v", wantInfra, result.InfraCostPerRequest)
	}
	wantEffective := 0.005 + wantInfra
	if math.Abs(result.EffectiveCostPerRequest-wantEffective) > 1e-9 {
		t.Errorf("expected effective=%v, got %v", wantEffective, result.EffectiveCostPerRequest)
	}
}

// TC3: requests_mes > 0, infra > 0 — amortization applied correctly.
func TestCalculateEffectiveCost_RequestsPositive(t *testing.T) {
	result := CalculateEffectiveCost(modelWith(60), 0.001, 200)
	wantInfra := 60.0 / 200.0 // 0.3
	if math.Abs(result.InfraCostPerRequest-wantInfra) > 1e-9 {
		t.Errorf("expected infra=%v, got %v", wantInfra, result.InfraCostPerRequest)
	}
}

// TC4: requests_mes = 0 — must not divide; infra allocation is 0.
func TestCalculateEffectiveCost_RequestsZero(t *testing.T) {
	result := CalculateEffectiveCost(modelWith(120), 0.005, 0)
	if result.InfraCostPerRequest != 0 {
		t.Errorf("expected infra=0 when requests=0, got %v", result.InfraCostPerRequest)
	}
	if result.EffectiveCostPerRequest != 0.005 {
		t.Errorf("expected effective=token_cost when requests=0, got %v", result.EffectiveCostPerRequest)
	}
}

// TC5: Existing model without infrastructure_monthly_usd field (zero value).
func TestCalculateEffectiveCost_ExistingModelNoField(t *testing.T) {
	m := config.ModelConfig{Name: "legacy-model", Provider: "openai"}
	result := CalculateEffectiveCost(m, 0.002, 50)
	if result.InfraCostPerRequest != 0 {
		t.Errorf("expected infra=0 for legacy model, got %v", result.InfraCostPerRequest)
	}
	if result.EffectiveCostPerRequest != 0.002 {
		t.Errorf("expected effective=token cost for legacy model, got %v", result.EffectiveCostPerRequest)
	}
}
