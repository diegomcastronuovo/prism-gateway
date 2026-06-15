package httpapi

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── computeEffectiveCostPerRequest unit tests ─────────────────────────────────

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// TestEffectiveCost_TokenOnlyZeroInfra verifies that when infrastructure_monthly_usd = 0,
// the effective cost equals the token-only average.
func TestEffectiveCost_TokenOnlyZeroInfra(t *testing.T) {
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "gpt-4o-mini", Requests: 10, Spend: 0.001},
	}
	tenantCounts := map[string]int64{"gpt-4o-mini": 100}
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", InfrastructureMonthlyUSD: 0},
	}

	got := computeEffectiveCostPerRequest(breakdown, tenantCounts, models)
	want := 0.001 / 10 // pure token average
	if !approxEqual(got, want, 1e-10) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// TestEffectiveCost_InfraAdded verifies that infrastructure_monthly_usd is proportionally
// allocated to the API key and added to the token cost.
func TestEffectiveCost_InfraAdded(t *testing.T) {
	// model has 100 USD/month infra; tenant made 1000 requests total;
	// this key made 200 requests (20% share) → 20 USD infra allocated.
	// token spend = 0 (pure ML model), requests = 200.
	// effective spend = 20 USD → avg = 20/200 = 0.10 USD/request.
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "fraud-v1", Requests: 200, Spend: 0},
	}
	tenantCounts := map[string]int64{"fraud-v1": 1000}
	models := []config.ModelConfig{
		{Name: "fraud-v1", InfrastructureMonthlyUSD: 100},
	}

	got := computeEffectiveCostPerRequest(breakdown, tenantCounts, models)
	// infra_per_request = 100 / 1000 = 0.10; effective_spend = 200 * 0.10 = 20; avg = 20/200 = 0.10
	want := 0.10
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// TestEffectiveCost_InfraOnly_NoTokenCost verifies ML/local model with zero token cost
// returns infra-only average.
func TestEffectiveCost_InfraOnly_NoTokenCost(t *testing.T) {
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "local-llm", Requests: 50, Spend: 0},
	}
	tenantCounts := map[string]int64{"local-llm": 500}
	models := []config.ModelConfig{
		{Name: "local-llm", InfrastructureMonthlyUSD: 200},
	}

	got := computeEffectiveCostPerRequest(breakdown, tenantCounts, models)
	// infra_per_req = 200/500 = 0.40; effective = 50 * 0.40 = 20; avg = 20/50 = 0.40
	want := 0.40
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// TestEffectiveCost_ZeroRequests verifies that zero total requests returns 0 (no divide-by-zero).
func TestEffectiveCost_ZeroRequests(t *testing.T) {
	got := computeEffectiveCostPerRequest(nil, nil, nil)
	if got != 0 {
		t.Errorf("expected 0 for zero requests, got %v", got)
	}
}

// TestEffectiveCost_MultipleModels verifies weighted effective average across multiple models.
// Key uses two models: one token-only, one with infra.
func TestEffectiveCost_MultipleModels(t *testing.T) {
	// Model A: gpt-4o-mini, 100 requests, 0.01 token spend, no infra → token avg = 0.0001/req
	// Model B: local-llm, 50 requests, 0 token spend, 100 USD/month infra, tenant=500 reqs
	//          infra_per_req = 100/500 = 0.20 → effective_spend_B = 50*0.20 = 10
	// total_effective = 0.01 + 10 = 10.01; total_requests = 150
	// avg = 10.01/150 ≈ 0.066733
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "gpt-4o-mini", Requests: 100, Spend: 0.01},
		{Model: "local-llm", Requests: 50, Spend: 0},
	}
	tenantCounts := map[string]int64{"gpt-4o-mini": 1000, "local-llm": 500}
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", InfrastructureMonthlyUSD: 0},
		{Name: "local-llm", InfrastructureMonthlyUSD: 100},
	}

	got := computeEffectiveCostPerRequest(breakdown, tenantCounts, models)
	want := 10.01 / 150.0
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// TestEffectiveCost_MissingModelConfig verifies that unknown models contribute only token spend.
func TestEffectiveCost_MissingModelConfig(t *testing.T) {
	breakdown := []storage.APIKeyModelUsageRow{
		{Model: "unknown-model", Requests: 10, Spend: 0.005},
	}
	tenantCounts := map[string]int64{}
	models := []config.ModelConfig{} // no matching model

	got := computeEffectiveCostPerRequest(breakdown, tenantCounts, models)
	want := 0.005 / 10.0
	if !approxEqual(got, want, 1e-10) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// ── Handler integration tests ─────────────────────────────────────────────────

// TestAdminAPIKeysUsage_ExposesAvgCostPerRequestEffective verifies the leaderboard
// endpoint returns avg_cost_per_request_effective in each data row.
func TestAdminAPIKeysUsage_ExposesAvgCostPerRequestEffective(t *testing.T) {
	keyID := uuid.MustParse("d39b2040-e3d4-40ee-afd6-58964f1d7c4c")
	store := &fakeStorage{
		apiKeyModelBreakdown: []storage.APIKeyModelUsageRow{
			{APIKeyID: keyID, TenantID: "tenant_a", Model: "gpt-4o-mini", Requests: 10, Spend: 0.01},
		},
		tenantModelCounts: map[string]int64{"gpt-4o-mini": 100},
	}
	// Override GetAPIKeyUsage to return a row for that key.
	// fakeStorage returns empty list by default — wire up manually.
	// Since fakeStorage.GetAPIKeyUsage returns nil rows, we use a wrapped store.
	wrappedStore := &apiKeyUsageFakeStore{
		fakeStorage: store,
		rows: []storage.APIKeyUsageRow{{
			APIKeyID:     keyID,
			APIKeyName:   "my-key",
			TenantID:     "tenant_a",
			Requests:     10,
			Spend:        0.01,
			SuccessRate:  1.0,
			AvgLatencyMs: 200,
			TopModel:     "gpt-4o-mini",
			TopProvider:  "openai",
			LastSeen:     time.Now().UTC(),
		}},
	}

	cfg := testAdminConfig()
	// model with zero infra → effective = token-only average = 0.01/10 = 0.001
	cfg.Models = append(cfg.Models, config.ModelConfig{Name: "gpt-4o-mini", InfrastructureMonthlyUSD: 0})

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
			AvgCostPerRequestEffective float64 `json:"avg_cost_per_request_effective"`
			Spend                      float64 `json:"spend"`
			Requests                   int     `json:"requests"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 data row, got %d", len(resp.Data))
	}
	row := resp.Data[0]
	// token-only: 0.01 / 10 = 0.001
	want := 0.01 / 10.0
	if !approxEqual(row.AvgCostPerRequestEffective, want, 1e-9) {
		t.Errorf("avg_cost_per_request_effective: want %v, got %v", want, row.AvgCostPerRequestEffective)
	}
}

// TestAdminAPIKeyUsageDetail_ExposesAvgCostPerRequestEffective verifies the drill-down
// endpoint includes avg_cost_per_request_effective in the summary.
func TestAdminAPIKeyUsageDetail_ExposesAvgCostPerRequestEffective(t *testing.T) {
	keyID := uuid.MustParse("d39b2040-e3d4-40ee-afd6-58964f1d7c4c")
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			return storage.APIKeyMeta{ID: keyID, Name: "my-key", TenantID: "tenant_a"}, true, nil
		},
		getAPIKeyUsageDetail: func() (storage.APIKeyUsageDetailSummary, []storage.APIKeyUsageByModelRow, []storage.APIKeyUsageByProviderRow, []storage.APIKeyUsageRecentRow, int, storage.LatencyStats, []storage.ErrorCountRow, error) {
			return storage.APIKeyUsageDetailSummary{
					Requests: 20, Spend: 0, TopModel: "local-llm", TopProvider: "local",
				},
				[]storage.APIKeyUsageByModelRow{{Model: "local-llm", Requests: 20, Spend: 0}},
				nil, nil, 0, storage.LatencyStats{}, nil, nil
		},
		// local-llm: 200 USD/month infra, tenant=200 total → infra_per_req=1 USD
		// key used 20 reqs → effective_spend=20; avg=1 USD/req
		tenantModelCounts: map[string]int64{"local-llm": 200},
	}

	cfg := testAdminConfig()
	cfg.Models = append(cfg.Models, config.ModelConfig{
		Name: "local-llm", InfrastructureMonthlyUSD: 200,
	})

	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0), globalCfgCache: config.NewGlobalConfigCache(0)}

	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+keyID.String()+"/usage", nil)
	req.SetPathValue("api_key_id", keyID.String())
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Summary struct {
			AvgCostPerRequestEffective float64 `json:"avg_cost_per_request_effective"`
		} `json:"summary"`
		RequestsByModel []struct {
			EffectiveSpend             float64 `json:"effective_spend"`
			AvgCostPerRequestEffective float64 `json:"avg_cost_per_request_effective"`
		} `json:"requests_by_model"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// infra_per_req = 200/200 = 1.0; effective_spend = 20*1 = 20; avg = 20/20 = 1.0
	want := 1.0
	if !approxEqual(resp.Summary.AvgCostPerRequestEffective, want, 1e-9) {
		t.Errorf("avg_cost_per_request_effective: want %v, got %v", want, resp.Summary.AvgCostPerRequestEffective)
	}
	if len(resp.RequestsByModel) != 1 {
		t.Fatalf("expected 1 model row, got %d", len(resp.RequestsByModel))
	}
	mr := resp.RequestsByModel[0]
	if !approxEqual(mr.EffectiveSpend, 20.0, 1e-9) {
		t.Errorf("effective_spend: want 20, got %v", mr.EffectiveSpend)
	}
	if !approxEqual(mr.AvgCostPerRequestEffective, 1.0, 1e-9) {
		t.Errorf("requests_by_model avg_cost_per_request_effective: want 1, got %v", mr.AvgCostPerRequestEffective)
	}
}

// SPEC_109: LLM row — no infra → effective_spend equals token spend.
func TestAdminAPIKeyUsageDetail_RequestsByModel_TokenOnlyEffectiveEqualsSpend(t *testing.T) {
	keyID := uuid.MustParse("d39b2040-e3d4-40ee-afd6-58964f1d7c4c")
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			return storage.APIKeyMeta{ID: keyID, Name: "k", TenantID: "tenant_a"}, true, nil
		},
		getAPIKeyUsageDetail: func() (storage.APIKeyUsageDetailSummary, []storage.APIKeyUsageByModelRow, []storage.APIKeyUsageByProviderRow, []storage.APIKeyUsageRecentRow, int, storage.LatencyStats, []storage.ErrorCountRow, error) {
			return storage.APIKeyUsageDetailSummary{Requests: 10, Spend: 0.001},
				[]storage.APIKeyUsageByModelRow{{Model: "gpt-4o-mini", Requests: 10, Spend: 0.0001}},
				nil, nil, 0, storage.LatencyStats{}, nil, nil
		},
		tenantModelCounts: map[string]int64{"gpt-4o-mini": 100},
	}
	cfg := testAdminConfig()
	cfg.Models = append(cfg.Models, config.ModelConfig{Name: "gpt-4o-mini", InfrastructureMonthlyUSD: 0})

	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0), globalCfgCache: config.NewGlobalConfigCache(0)}

	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+keyID.String()+"/usage", nil)
	req.SetPathValue("api_key_id", keyID.String())
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		RequestsByModel []struct {
			Spend              float64 `json:"spend"`
			EffectiveSpend     float64 `json:"effective_spend"`
			AvgCostPerRequestE float64 `json:"avg_cost_per_request_effective"`
		} `json:"requests_by_model"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.RequestsByModel) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.RequestsByModel))
	}
	r := resp.RequestsByModel[0]
	if !approxEqual(r.EffectiveSpend, r.Spend, 1e-12) {
		t.Errorf("effective_spend should equal spend for LLM with no infra: spend=%v effective=%v", r.Spend, r.EffectiveSpend)
	}
	if !approxEqual(r.AvgCostPerRequestE, r.Spend/10.0, 1e-12) {
		t.Errorf("avg effective: want %v, got %v", r.Spend/10.0, r.AvgCostPerRequestE)
	}
}

// SPEC_109: zero requests row → safe zeros.
func TestAdminAPIKeyUsageDetail_RequestsByModel_ZeroRequests(t *testing.T) {
	keyID := uuid.MustParse("d39b2040-e3d4-40ee-afd6-58964f1d7c4c")
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			return storage.APIKeyMeta{ID: keyID, Name: "k", TenantID: "tenant_a"}, true, nil
		},
		getAPIKeyUsageDetail: func() (storage.APIKeyUsageDetailSummary, []storage.APIKeyUsageByModelRow, []storage.APIKeyUsageByProviderRow, []storage.APIKeyUsageRecentRow, int, storage.LatencyStats, []storage.ErrorCountRow, error) {
			return storage.APIKeyUsageDetailSummary{Requests: 0, Spend: 0},
				[]storage.APIKeyUsageByModelRow{{Model: "orphan-model", Requests: 0, Spend: 0}},
				nil, nil, 0, storage.LatencyStats{}, nil, nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0), globalCfgCache: config.NewGlobalConfigCache(0)}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+keyID.String()+"/usage", nil)
	req.SetPathValue("api_key_id", keyID.String())
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	var resp struct {
		RequestsByModel []struct {
			EffectiveSpend     float64 `json:"effective_spend"`
			AvgCostPerRequestE float64 `json:"avg_cost_per_request_effective"`
		} `json:"requests_by_model"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.RequestsByModel) != 1 {
		t.Fatalf("expected 1 row")
	}
	if resp.RequestsByModel[0].EffectiveSpend != 0 || resp.RequestsByModel[0].AvgCostPerRequestE != 0 {
		t.Errorf("want zeros, got effective=%v avg=%v",
			resp.RequestsByModel[0].EffectiveSpend, resp.RequestsByModel[0].AvgCostPerRequestE)
	}
}

// SPEC_109: mixed LLM (token) + ML (infra) rows both enriched.
func TestAdminAPIKeyUsageDetail_RequestsByModel_MixedLLMandML(t *testing.T) {
	keyID := uuid.MustParse("d39b2040-e3d4-40ee-afd6-58964f1d7c4c")
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			return storage.APIKeyMeta{ID: keyID, Name: "k", TenantID: "tenant_a"}, true, nil
		},
		getAPIKeyUsageDetail: func() (storage.APIKeyUsageDetailSummary, []storage.APIKeyUsageByModelRow, []storage.APIKeyUsageByProviderRow, []storage.APIKeyUsageRecentRow, int, storage.LatencyStats, []storage.ErrorCountRow, error) {
			return storage.APIKeyUsageDetailSummary{Requests: 30, Spend: 0.01},
				[]storage.APIKeyUsageByModelRow{
					{Model: "gpt-4o-mini", Requests: 10, Spend: 0.01},
					{Model: "Fake", Requests: 20, Spend: 0},
				},
				nil, nil, 0, storage.LatencyStats{}, nil, nil
		},
		tenantModelCounts: map[string]int64{"gpt-4o-mini": 100, "Fake": 200},
	}
	cfg := testAdminConfig()
	cfg.Models = append(cfg.Models,
		config.ModelConfig{Name: "gpt-4o-mini", InfrastructureMonthlyUSD: 0},
		config.ModelConfig{Name: "Fake", InfrastructureMonthlyUSD: 200},
	)
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0), globalCfgCache: config.NewGlobalConfigCache(0)}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+keyID.String()+"/usage", nil)
	req.SetPathValue("api_key_id", keyID.String())
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	var resp struct {
		RequestsByModel []struct {
			Model          string  `json:"model"`
			Spend          float64 `json:"spend"`
			EffectiveSpend float64 `json:"effective_spend"`
		} `json:"requests_by_model"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.RequestsByModel) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(resp.RequestsByModel))
	}
	for _, row := range resp.RequestsByModel {
		if row.Model == "gpt-4o-mini" {
			if !approxEqual(row.EffectiveSpend, row.Spend, 1e-12) {
				t.Errorf("LLM effective_spend should equal spend, got %v vs %v", row.EffectiveSpend, row.Spend)
			}
		}
		if row.Model == "Fake" {
			// infra_per_req = 200/200 = 1; effective = 20*1 = 20
			if !approxEqual(row.EffectiveSpend, 20.0, 1e-9) {
				t.Errorf("ML effective_spend want 20, got %v", row.EffectiveSpend)
			}
		}
	}
}

// SPEC_109: markup > 0 → margin > 0 on per-model row.
func TestAdminAPIKeyUsageDetail_RequestsByModel_MarkupPositive(t *testing.T) {
	keyID := uuid.MustParse("d39b2040-e3d4-40ee-afd6-58964f1d7c4c")
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			return storage.APIKeyMeta{ID: keyID, Name: "k", TenantID: "tenant_a"}, true, nil
		},
		getAPIKeyUsageDetail: func() (storage.APIKeyUsageDetailSummary, []storage.APIKeyUsageByModelRow, []storage.APIKeyUsageByProviderRow, []storage.APIKeyUsageRecentRow, int, storage.LatencyStats, []storage.ErrorCountRow, error) {
			return storage.APIKeyUsageDetailSummary{Requests: 10, Spend: 1.0},
				[]storage.APIKeyUsageByModelRow{{Model: "priced-model", Requests: 10, Spend: 1.0}},
				nil, nil, 0, storage.LatencyStats{}, nil, nil
		},
		tenantModelCounts: map[string]int64{"priced-model": 10},
	}
	cfg := testAdminConfig()
	cfg.Models = append(cfg.Models, config.ModelConfig{Name: "priced-model", InfrastructureMonthlyUSD: 0, MarkupPercentage: 20})

	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0), globalCfgCache: config.NewGlobalConfigCache(0)}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+keyID.String()+"/usage", nil)
	req.SetPathValue("api_key_id", keyID.String())
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	var resp struct {
		RequestsByModel []struct {
			AvgCostPerRequestE float64 `json:"avg_cost_per_request_effective"`
			AvgPricePerRequest float64 `json:"avg_price_per_request"`
			Margin             float64 `json:"margin"`
		} `json:"requests_by_model"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	r := resp.RequestsByModel[0]
	if r.AvgPricePerRequest <= r.AvgCostPerRequestE {
		t.Errorf("avg_price should exceed avg cost when markup>0: price=%v cost=%v", r.AvgPricePerRequest, r.AvgCostPerRequestE)
	}
	if r.Margin <= 0 {
		t.Errorf("margin should be > 0, got %v", r.Margin)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// apiKeyUsageFakeStore wraps fakeStorage and overrides GetAPIKeyUsage to return
// controllable rows for effective cost integration tests.
type apiKeyUsageFakeStore struct {
	*fakeStorage
	rows    []storage.APIKeyUsageRow
	summary storage.APIKeyUsageSummary
}

func (s *apiKeyUsageFakeStore) GetAPIKeyUsage(_ context.Context, _ storage.APIKeyUsageFilter) (storage.APIKeyUsageSummary, []storage.APIKeyUsageRow, error) {
	return s.summary, s.rows, nil
}

// ── applyMarkup unit tests ────────────────────────────────────────────────────

// TestApplyMarkup_ZeroMarkup verifies that price equals effective cost when markup = 0.
func TestApplyMarkup_ZeroMarkup(t *testing.T) {
	got := applyMarkup(1.00, 0)
	if !approxEqual(got, 1.00, 1e-10) {
		t.Errorf("want 1.00, got %v", got)
	}
}

// TestApplyMarkup_PositiveMarkup verifies the formula: price = cost * (1 + markup/100).
func TestApplyMarkup_PositiveMarkup(t *testing.T) {
	got := applyMarkup(1.00, 20)
	if !approxEqual(got, 1.20, 1e-10) {
		t.Errorf("want 1.20, got %v", got)
	}
}

// TestApplyMarkup_HundredPercent verifies doubling at 100% markup.
func TestApplyMarkup_HundredPercent(t *testing.T) {
	got := applyMarkup(0.50, 100)
	if !approxEqual(got, 1.00, 1e-10) {
		t.Errorf("want 1.00, got %v", got)
	}
}

// TestApplyMarkup_ZeroEffectiveCost verifies that price stays 0 when effective cost is 0.
func TestApplyMarkup_ZeroEffectiveCost(t *testing.T) {
	got := applyMarkup(0, 50)
	if got != 0 {
		t.Errorf("want 0, got %v", got)
	}
}
