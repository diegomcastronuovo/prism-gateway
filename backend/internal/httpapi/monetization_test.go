package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── computeMonetization unit tests ───────────────────────────────────────────

// TestMonetization_ZeroMarkup verifies price == cost and margin == 0 when markup = 0.
func TestMonetization_ZeroMarkup(t *testing.T) {
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "gpt-4o-mini", Requests: 10, Spend: 0.01},
	}
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", MarkupPercentage: 0},
	}
	res := computeMonetization(breakdown, nil, models)

	if !approxEqual(res.AvgCostPerRequest, 0.001, 1e-10) {
		t.Errorf("avg_cost want 0.001, got %v", res.AvgCostPerRequest)
	}
	if !approxEqual(res.TotalPrice, 0.01, 1e-10) {
		t.Errorf("total_price want 0.01, got %v", res.TotalPrice)
	}
	if !approxEqual(res.Margin, 0, 1e-10) {
		t.Errorf("margin want 0, got %v", res.Margin)
	}
	if res.MarginPct != 0 {
		t.Errorf("margin_pct want 0, got %v", res.MarginPct)
	}
}

// TestMonetization_PositiveMarkup verifies price and margin with 20% markup.
func TestMonetization_PositiveMarkup(t *testing.T) {
	// 10 requests, token spend = 0.01, markup = 20%
	// effective_cost = 0.01, price = 0.01 * 1.20 = 0.012
	// margin = 0.002, avg_price_per_request = 0.0012
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "gpt-4o-mini", Requests: 10, Spend: 0.01},
	}
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", MarkupPercentage: 20},
	}
	res := computeMonetization(breakdown, nil, models)

	if !approxEqual(res.TotalPrice, 0.012, 1e-10) {
		t.Errorf("total_price want 0.012, got %v", res.TotalPrice)
	}
	if !approxEqual(res.AvgPricePerRequest, 0.0012, 1e-10) {
		t.Errorf("avg_price_per_request want 0.0012, got %v", res.AvgPricePerRequest)
	}
	if !approxEqual(res.Margin, 0.002, 1e-10) {
		t.Errorf("margin want 0.002, got %v", res.Margin)
	}
	// margin_pct = 0.002 / 0.012 ≈ 0.1666...
	wantMarginPct := 0.002 / 0.012
	if !approxEqual(res.MarginPct, wantMarginPct, 1e-9) {
		t.Errorf("margin_pct want %v, got %v", wantMarginPct, res.MarginPct)
	}
}

// TestMonetization_MLModelInfraAndMarkup verifies infra-only effective cost with markup.
func TestMonetization_MLModelInfraAndMarkup(t *testing.T) {
	// ML model: 0 token spend, 100 USD/month infra, tenant=1000 reqs, key=200 reqs
	// infra_per_req = 100/1000 = 0.10; effective_total = 200*0.10 = 20 USD
	// markup=50% → total_price = 20 * 1.50 = 30 USD
	// avg_price_per_request = 30/200 = 0.15
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "fraud-v1", Requests: 200, Spend: 0},
	}
	tenantCounts := map[string]int64{"fraud-v1": 1000}
	models := []config.ModelConfig{
		{Name: "fraud-v1", InfrastructureMonthlyUSD: 100, MarkupPercentage: 50},
	}
	res := computeMonetization(breakdown, tenantCounts, models)

	if !approxEqual(res.TotalEffectiveCost, 20, 1e-9) {
		t.Errorf("total_effective_cost want 20, got %v", res.TotalEffectiveCost)
	}
	if !approxEqual(res.TotalPrice, 30, 1e-9) {
		t.Errorf("total_price want 30, got %v", res.TotalPrice)
	}
	if !approxEqual(res.AvgPricePerRequest, 0.15, 1e-9) {
		t.Errorf("avg_price_per_request want 0.15, got %v", res.AvgPricePerRequest)
	}
	if !approxEqual(res.Margin, 10, 1e-9) {
		t.Errorf("margin want 10, got %v", res.Margin)
	}
}

// TestMonetization_MultiModelDifferentMarkups verifies per-model price summed across models.
func TestMonetization_MultiModelDifferentMarkups(t *testing.T) {
	// Model A: gpt-4o-mini, 100 reqs, 0.01 spend, markup=20%
	//   effective=0.01, price=0.012
	// Model B: local-llm, 50 reqs, 0 spend, 100 USD/month, tenant=500, markup=0%
	//   infra_per_req=100/500=0.20, effective=50*0.20=10, price=10
	// total_effective = 10.01, total_price = 10.012, total_requests = 150
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "gpt-4o-mini", Requests: 100, Spend: 0.01},
		{Model: "local-llm", Requests: 50, Spend: 0},
	}
	tenantCounts := map[string]int64{"local-llm": 500}
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", MarkupPercentage: 20},
		{Name: "local-llm", InfrastructureMonthlyUSD: 100, MarkupPercentage: 0},
	}
	res := computeMonetization(breakdown, tenantCounts, models)

	if !approxEqual(res.TotalEffectiveCost, 10.01, 1e-9) {
		t.Errorf("total_effective_cost want 10.01, got %v", res.TotalEffectiveCost)
	}
	if !approxEqual(res.TotalPrice, 10.012, 1e-9) {
		t.Errorf("total_price want 10.012, got %v", res.TotalPrice)
	}
	if !approxEqual(res.AvgPricePerRequest, 10.012/150.0, 1e-9) {
		t.Errorf("avg_price_per_request want %v, got %v", 10.012/150.0, res.AvgPricePerRequest)
	}
}

// TestMonetization_ZeroRequests verifies no divide-by-zero when empty breakdown.
func TestMonetization_ZeroRequests(t *testing.T) {
	res := computeMonetization(nil, nil, nil)
	if res.TotalRequests != 0 || res.TotalPrice != 0 || res.Margin != 0 || res.MarginPct != 0 {
		t.Errorf("expected all zeros for empty breakdown, got %+v", res)
	}
}

// TestMonetization_MissingModelConfig verifies unknown models default to 0 markup.
func TestMonetization_MissingModelConfig(t *testing.T) {
	// Model not in config → markup = 0, infra = 0 → price == effective cost
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "unknown-model", Requests: 5, Spend: 0.005},
	}
	res := computeMonetization(breakdown, nil, nil)

	if !approxEqual(res.TotalPrice, 0.005, 1e-10) {
		t.Errorf("total_price want 0.005 (no markup), got %v", res.TotalPrice)
	}
	if res.Margin != 0 {
		t.Errorf("margin want 0 for missing config, got %v", res.Margin)
	}
}

// ── Handler integration tests ─────────────────────────────────────────────────

// TestAdminAPIKeysUsage_MonetizationFields verifies that the leaderboard endpoint
// returns avg_price_per_request, total_price, margin, margin_pct.
func TestAdminAPIKeysUsage_MonetizationFields(t *testing.T) {
	keyID := uuid.MustParse("d39b2040-e3d4-40ee-afd6-58964f1d7c4c")
	store := &fakeStorage{
		apiKeyModelBreakdown: []storage.APIKeyModelUsageRow{
			{APIKeyID: keyID, TenantID: "tenant_a", Model: "gpt-4o-mini", Requests: 10, Spend: 0.01},
		},
		tenantModelCounts: map[string]int64{"gpt-4o-mini": 100},
	}
	wrappedStore := &apiKeyUsageFakeStore{
		fakeStorage: store,
		rows: []storage.APIKeyUsageRow{{
			APIKeyID:  keyID,
			APIKeyName: "my-key",
			TenantID:  "tenant_a",
			Requests:  10,
			Spend:     0.01,
			LastSeen:  time.Now().UTC(),
		}},
	}

	cfg := testAdminConfig()
	// 20% markup → price = 0.01 * 1.20 = 0.012, margin = 0.002
	cfg.Models = append(cfg.Models, config.ModelConfig{Name: "gpt-4o-mini", MarkupPercentage: 20})

	h := &Handlers{cfg: cfg, store: wrappedStore, log: testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0), globalCfgCache: config.NewGlobalConfigCache(0)}

	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/usage?window_hours=720", nil)
	w := httptest.NewRecorder()
	h.AdminAPIKeysUsage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			AvgPricePerRequest float64 `json:"avg_price_per_request"`
			TotalPrice         float64 `json:"total_price"`
			Margin             float64 `json:"margin"`
			MarginPct          float64 `json:"margin_pct"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Data))
	}
	row := resp.Data[0]
	if !approxEqual(row.TotalPrice, 0.012, 1e-9) {
		t.Errorf("total_price: want 0.012, got %v", row.TotalPrice)
	}
	if !approxEqual(row.Margin, 0.002, 1e-9) {
		t.Errorf("margin: want 0.002, got %v", row.Margin)
	}
	if !approxEqual(row.AvgPricePerRequest, 0.0012, 1e-9) {
		t.Errorf("avg_price_per_request: want 0.0012, got %v", row.AvgPricePerRequest)
	}
}

// TestAdminJWTSubsUsage_MonetizationFields verifies the JWT sub endpoint returns
// avg_cost_per_request_effective, avg_price_per_request, total_price, margin, margin_pct.
func TestAdminJWTSubsUsage_MonetizationFields(t *testing.T) {
	store := &jwtSubMonetizationFakeStore{
		rows: []storage.JWTSubUsageRow{{
			JWTSub:       "user-abc",
			TenantID:     "tenant_a",
			Requests:     10,
			TotalCostUSD: 0.01,
			FirstSeen:    time.Now().UTC(),
			LastSeen:     time.Now().UTC(),
		}},
		modelBreakdown: []storage.JWTSubModelUsageRow{
			{JWTSub: "user-abc", TenantID: "tenant_a", Model: "gpt-4o-mini", Requests: 10, Spend: 0.01},
		},
		tenantModelCounts: map[string]int64{"gpt-4o-mini": 100},
	}

	cfg := testAdminConfig()
	cfg.Models = append(cfg.Models, config.ModelConfig{Name: "gpt-4o-mini", MarkupPercentage: 20})

	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0), globalCfgCache: config.NewGlobalConfigCache(0)}

	req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/usage", nil)
	w := httptest.NewRecorder()
	h.AdminJWTSubsUsage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			AvgCostPerRequestEffective float64 `json:"avg_cost_per_request_effective"`
			AvgPricePerRequest         float64 `json:"avg_price_per_request"`
			TotalPrice                 float64 `json:"total_price"`
			Margin                     float64 `json:"margin"`
			MarginPct                  float64 `json:"margin_pct"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Data))
	}
	row := resp.Data[0]
	if !approxEqual(row.AvgCostPerRequestEffective, 0.001, 1e-9) {
		t.Errorf("avg_cost_per_request_effective: want 0.001, got %v", row.AvgCostPerRequestEffective)
	}
	if !approxEqual(row.TotalPrice, 0.012, 1e-9) {
		t.Errorf("total_price: want 0.012, got %v", row.TotalPrice)
	}
	if !approxEqual(row.Margin, 0.002, 1e-9) {
		t.Errorf("margin: want 0.002, got %v", row.Margin)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// jwtSubMonetizationFakeStore is a minimal fakeStorage for JWT sub monetization tests.
type jwtSubMonetizationFakeStore struct {
	fakeStorage
	rows              []storage.JWTSubUsageRow
	modelBreakdown    []storage.JWTSubModelUsageRow
	tenantModelCounts map[string]int64
}

func (s *jwtSubMonetizationFakeStore) GetJWTSubUsage(_ context.Context, _ storage.JWTSubUsageFilter) ([]storage.JWTSubUsageRow, int, error) {
	return s.rows, len(s.rows), nil
}
func (s *jwtSubMonetizationFakeStore) GetJWTSubModelBreakdown(_ context.Context, _ storage.JWTSubUsageFilter) ([]storage.JWTSubModelUsageRow, error) {
	return s.modelBreakdown, nil
}
func (s *jwtSubMonetizationFakeStore) GetTenantModelRequestCounts(_ context.Context, _ string, _ time.Time) (map[string]int64, error) {
	return s.tenantModelCounts, nil
}
