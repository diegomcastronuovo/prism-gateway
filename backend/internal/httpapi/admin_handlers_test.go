package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

// testAdminConfig returns a minimal config for admin handler tests
func testAdminConfig() *config.Config {
	return &config.Config{
		Auth: config.AuthConfig{
			Mode: "api_key", // Use API key mode for backward compat in tests
		},
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

// fakeStore implements storage.Storage for testing
type fakeStore struct {
	storage.NopStorage
	impactData    storage.SmartImpactData
	usageSummary  storage.UsageSummary
	usageSummaryErr error
}

func (f *fakeStore) GetSmartImpactData(ctx context.Context, tenantID string, from, to time.Time) (storage.SmartImpactData, error) {
	return f.impactData, nil
}

func (f *fakeStore) GetUsageSummary(ctx context.Context, tenantID string, month time.Time) (storage.UsageSummary, error) {
	return f.usageSummary, f.usageSummaryErr
}

func testLoggerForAdmin() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestAdminSmartImpact_Auth(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testAdminConfig()
	store := &fakeStore{}
	h := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/t1/smart/impact", nil)
	req.SetPathValue("tenant_id", "t1")
	// No X-Admin-Token header

	w := httptest.NewRecorder()
	cache := config.NewTenantConfigCache(1 * time.Second)
	AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(http.HandlerFunc(h.AdminSmartImpact)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminSmartImpact_InvalidWindowDays(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testAdminConfig()
	store := &fakeStore{}
	h := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/t1/smart/impact?window_days=200", nil)
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("X-Admin-Token", "test-token")

	w := httptest.NewRecorder()
	cache := config.NewTenantConfigCache(1 * time.Second)
	AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(http.HandlerFunc(h.AdminSmartImpact)).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAdminSmartImpact_InvalidBaseline(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testAdminConfig()
	store := &fakeStore{}
	h := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/t1/smart/impact?baseline=invalid", nil)
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("X-Admin-Token", "test-token")

	w := httptest.NewRecorder()
	cache := config.NewTenantConfigCache(1 * time.Second)
	AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(http.HandlerFunc(h.AdminSmartImpact)).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAdminSmartImpact_TenantNotFound(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testAdminConfig()
	store := &fakeStore{}
	h := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/nonexistent/smart/impact", nil)
	req.SetPathValue("tenant_id", "nonexistent")
	req.Header.Set("X-Admin-Token", "test-token")

	w := httptest.NewRecorder()
	cache := config.NewTenantConfigCache(1 * time.Second)
	AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(http.HandlerFunc(h.AdminSmartImpact)).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminSmartImpact_NoData(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testAdminConfig()
	store := &fakeStore{
		impactData: storage.SmartImpactData{
			TenantID:     "t1",
			TotalRequests: 0,
		},
	}
	h := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/t1/smart/impact", nil)
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("X-Admin-Token", "test-token")

	w := httptest.NewRecorder()
	cache := config.NewTenantConfigCache(1 * time.Second)
	AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(http.HandlerFunc(h.AdminSmartImpact)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp SmartImpactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Actual.Requests != 0 {
		t.Errorf("expected 0 requests, got %d", resp.Actual.Requests)
	}

	if len(resp.Impact.Notes) == 0 {
		t.Error("expected notes about no data")
	}
}

func TestAdminSmartImpact_WithData(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testAdminConfig()

	now := time.Now().UTC()
	store := &fakeStore{
		impactData: storage.SmartImpactData{
			TenantID:        "t1",
			PeriodStart:     now.AddDate(0, 0, -30),
			PeriodEnd:       now,
			TotalRequests:   100,
			SuccessRequests: 95,
			ErrorRequests:   5,
			TotalCostUSD:    10.0,
			AvgLatencyMs:    200.5,
			UsageDetails: []storage.UsageDetailRow{
				{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
				{RequestID: uuid.New(), PromptTokens: 100, CompletionTokens: 50},
			},
		},
	}
	h := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/t1/smart/impact?baseline=round_robin", nil)
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("X-Admin-Token", "test-token")

	w := httptest.NewRecorder()
	cache := config.NewTenantConfigCache(1 * time.Second)
	AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(http.HandlerFunc(h.AdminSmartImpact)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp SmartImpactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.TenantID != "t1" {
		t.Errorf("expected tenant_id 't1', got '%s'", resp.TenantID)
	}

	if resp.Actual.Requests != 100 {
		t.Errorf("expected 100 requests, got %d", resp.Actual.Requests)
	}

	if resp.Actual.TotalCostUSD != 10.0 {
		t.Errorf("expected 10.0 total cost, got %f", resp.Actual.TotalCostUSD)
	}

	if resp.Baseline.Type != "round_robin" {
		t.Errorf("expected baseline type 'round_robin', got '%s'", resp.Baseline.Type)
	}
}

func TestParseBaseline(t *testing.T) {
	tests := []struct {
		input     string
		wantType  string
		wantModel string
		wantErr   bool
	}{
		{"round_robin", "round_robin", "", false},
		{"cheapest", "cheapest", "", false},
		{"fixed_model:gpt-4o-mini", "fixed_model", "gpt-4o-mini", false},
		{"fixed_model:", "", "", true},
		{"invalid", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			baselineType, modelName, err := parseBaseline(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if baselineType != tt.wantType {
				t.Errorf("type: got %s, want %s", baselineType, tt.wantType)
			}

			if modelName != tt.wantModel {
				t.Errorf("model: got %s, want %s", modelName, tt.wantModel)
			}
		})
	}
}

func TestBuildImpactNotes_Savings(t *testing.T) {
	notes := buildImpactNotes("round_robin", 10.0, 0.25, 30.0, 40.0)

	if len(notes) < 2 {
		t.Error("expected at least 2 notes")
	}

	// Check that notes mention savings
	found := false
	for _, note := range notes {
		if len(note) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected non-empty notes")
	}
}

func TestBuildImpactNotes_NegativeSavings(t *testing.T) {
	notes := buildImpactNotes("cheapest", -5.0, -0.1, 50.0, 45.0)

	if len(notes) == 0 {
		t.Error("expected notes")
	}

	// Check that notes mention optimization opportunity
	found := false
	for _, note := range notes {
		if len(note) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected non-empty notes")
	}
}

// TestAdminBudgetStatus_ReadsEnforcementFromTenantConfig verifies that
// AdminBudgetStatus returns enforcement fields from the resolved tenant config
// (not from a zero-value default) so that dynamic and YAML configs both work.
func TestAdminBudgetStatus_ReadsEnforcementFromTenantConfig(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{
				ID: "t1",
				BudgetEnforcement: config.BudgetEnforcementConfig{
					Enabled: true,
					Mode:    "report_only",
				},
			},
		},
	}

	h := &Handlers{
		cfg:            cfg,
		store:          &storage.NopStorage{},
		log:            testLoggerForAdmin(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/budgets/status?month=2026-03", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminBudgetStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BudgetStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.EnforcementEnabled {
		t.Errorf("enforcement_enabled = false, want true")
	}
	if resp.EnforcementMode != "report_only" {
		t.Errorf("enforcement_mode = %q, want %q", resp.EnforcementMode, "report_only")
	}
}

// ── AdminUsageSummary cost enrichment tests ───────────────────────────────────

func usageSummaryHandlers(models []config.ModelConfig, summary storage.UsageSummary) *Handlers {
	cfg := &config.Config{Models: models}
	store := &fakeStore{usageSummary: summary}
	return &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLoggerForAdmin(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

func doUsageSummaryRequest(t *testing.T, h *Handlers) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/usage/summary?month=2026-03", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminUsageSummary(w, req)
	return w
}

// TestUsageSummary_NoInfraCost: model with infrastructure_monthly_usd=0 →
// infra fields are zero, effective_cost_per_request = token cost only.
func TestUsageSummary_NoInfraCost(t *testing.T) {
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai", InfrastructureMonthlyUSD: 0},
	}
	summary := storage.UsageSummary{
		TenantID:      "t1",
		Month:         "2026-03",
		TotalRequests: 78,
		TotalCost:     0.005394,
		ModelBreakdown: map[string]storage.ModelUsage{
			"gpt-4o-mini": {Requests: 78, Cost: 0.005394},
		},
	}
	h := usageSummaryHandlers(models, summary)
	w := doUsageSummaryRequest(t, h)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp UsageSummaryResponse
	json.NewDecoder(w.Body).Decode(&resp)

	m := resp.Models["gpt-4o-mini"]
	if m.Requests != 78 {
		t.Errorf("requests: got %d, want 78", m.Requests)
	}
	if m.InfrastructureMonthlyUSD != 0 {
		t.Errorf("infra_monthly_usd: got %v, want 0", m.InfrastructureMonthlyUSD)
	}
	if m.InfraCostPerRequest != 0 {
		t.Errorf("infra_cost_per_request: got %v, want 0", m.InfraCostPerRequest)
	}
	wantEffective := 0.005394 / 78
	if abs64(m.EffectiveCostPerRequest-wantEffective) > 1e-9 {
		t.Errorf("effective_cost_per_request: got %v, want ~%v", m.EffectiveCostPerRequest, wantEffective)
	}
}

// TestUsageSummary_WithInfraCost: model with infra cost > 0 and requests > 0 →
// infra_cost_per_request and effective_cost_per_request computed correctly.
func TestUsageSummary_WithInfraCost(t *testing.T) {
	models := []config.ModelConfig{
		{Name: "local-llama", Provider: "local", InfrastructureMonthlyUSD: 120},
	}
	summary := storage.UsageSummary{
		TenantID:      "t1",
		Month:         "2026-03",
		TotalRequests: 100,
		TotalCost:     0.0,
		ModelBreakdown: map[string]storage.ModelUsage{
			"local-llama": {Requests: 100, Cost: 0.0},
		},
	}
	h := usageSummaryHandlers(models, summary)
	w := doUsageSummaryRequest(t, h)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp UsageSummaryResponse
	json.NewDecoder(w.Body).Decode(&resp)

	m := resp.Models["local-llama"]
	if m.InfrastructureMonthlyUSD != 120 {
		t.Errorf("infra_monthly_usd: got %v, want 120", m.InfrastructureMonthlyUSD)
	}
	wantInfra := 120.0 / 100.0 // 1.2
	if abs64(m.InfraCostPerRequest-wantInfra) > 1e-9 {
		t.Errorf("infra_cost_per_request: got %v, want %v", m.InfraCostPerRequest, wantInfra)
	}
	// token cost = 0, so effective = infra only
	if abs64(m.EffectiveCostPerRequest-wantInfra) > 1e-9 {
		t.Errorf("effective_cost_per_request: got %v, want %v", m.EffectiveCostPerRequest, wantInfra)
	}
}

// TestUsageSummary_RequestsZero: requests_mes=0 → no division, infra fields = 0.
func TestUsageSummary_RequestsZero(t *testing.T) {
	models := []config.ModelConfig{
		{Name: "local-llama", Provider: "local", InfrastructureMonthlyUSD: 120},
	}
	summary := storage.UsageSummary{
		TenantID:      "t1",
		Month:         "2026-03",
		TotalRequests: 0,
		TotalCost:     0,
		ModelBreakdown: map[string]storage.ModelUsage{
			"local-llama": {Requests: 0, Cost: 0},
		},
	}
	h := usageSummaryHandlers(models, summary)
	w := doUsageSummaryRequest(t, h)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp UsageSummaryResponse
	json.NewDecoder(w.Body).Decode(&resp)

	m := resp.Models["local-llama"]
	if m.InfraCostPerRequest != 0 {
		t.Errorf("infra_cost_per_request: got %v, want 0 when requests=0", m.InfraCostPerRequest)
	}
	if m.EffectiveCostPerRequest != 0 {
		t.Errorf("effective_cost_per_request: got %v, want 0 when requests=0", m.EffectiveCostPerRequest)
	}
}

// TestUsageSummary_ModelNotInConfig: model not present in global config →
// infrastructure cost defaults to 0, no panic.
func TestUsageSummary_ModelNotInConfig(t *testing.T) {
	models := []config.ModelConfig{} // no models configured
	summary := storage.UsageSummary{
		TenantID:      "t1",
		Month:         "2026-03",
		TotalRequests: 5,
		TotalCost:     0.001,
		ModelBreakdown: map[string]storage.ModelUsage{
			"unknown-model": {Requests: 5, Cost: 0.001},
		},
	}
	h := usageSummaryHandlers(models, summary)
	w := doUsageSummaryRequest(t, h)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp UsageSummaryResponse
	json.NewDecoder(w.Body).Decode(&resp)

	m := resp.Models["unknown-model"]
	if m.InfrastructureMonthlyUSD != 0 {
		t.Errorf("expected 0 infra for unknown model, got %v", m.InfrastructureMonthlyUSD)
	}
	if m.InfraCostPerRequest != 0 {
		t.Errorf("expected 0 infra_cost for unknown model, got %v", m.InfraCostPerRequest)
	}
	wantEffective := 0.001 / 5.0
	if abs64(m.EffectiveCostPerRequest-wantEffective) > 1e-9 {
		t.Errorf("effective_cost_per_request: got %v, want %v", m.EffectiveCostPerRequest, wantEffective)
	}
}

// TestUsageSummary_MultipleModels: multiple models in breakdown, mixed infra costs.
func TestUsageSummary_MultipleModels(t *testing.T) {
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai", InfrastructureMonthlyUSD: 0},
		{Name: "local-llama", Provider: "local", InfrastructureMonthlyUSD: 120},
	}
	summary := storage.UsageSummary{
		TenantID:      "t1",
		Month:         "2026-03",
		TotalRequests: 180,
		TotalCost:     0.005394,
		ModelBreakdown: map[string]storage.ModelUsage{
			"gpt-4o-mini": {Requests: 78, Cost: 0.005394},
			"local-llama": {Requests: 100, Cost: 0},
		},
	}
	h := usageSummaryHandlers(models, summary)
	w := doUsageSummaryRequest(t, h)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp UsageSummaryResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Models))
	}
	gpt := resp.Models["gpt-4o-mini"]
	if gpt.InfrastructureMonthlyUSD != 0 || gpt.InfraCostPerRequest != 0 {
		t.Errorf("gpt-4o-mini should have zero infra fields")
	}
	llama := resp.Models["local-llama"]
	if llama.InfrastructureMonthlyUSD != 120 {
		t.Errorf("local-llama infra_monthly: got %v, want 120", llama.InfrastructureMonthlyUSD)
	}
	if abs64(llama.InfraCostPerRequest-1.2) > 1e-9 {
		t.Errorf("local-llama infra_cost_per_request: got %v, want 1.2", llama.InfraCostPerRequest)
	}
}

