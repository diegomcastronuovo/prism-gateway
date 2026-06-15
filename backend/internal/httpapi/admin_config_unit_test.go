package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// adminConfigTestCfg returns a minimal config suitable for admin config handler tests.
func adminConfigTestCfg() *config.Config {
	return &config.Config{
		Models: []config.ModelConfig{
			{Name: "model-a", Provider: "openai"},
		},
		Tenants: []config.TenantConfig{
			{
				ID:            "t1",
				AllowedModels: []string{"model-a"},
				Routing:       config.RoutingConfig{Strategy: "round_robin"},
				RateLimit:     config.RateLimitConfig{RPM: 100, Burst: 10},
				Compliance:    config.ComplianceConfig{RetentionDays: 90, LogMode: "metadata_only"},
				Budgets:       config.BudgetsConfig{MonthlyUSD: 0, Timezone: "UTC"},
			},
		},
	}
}

// validPutBody returns a JSON body that passes validateTenantConfig for adminConfigTestCfg.
func validPutBody(t *testing.T) []byte {
	t.Helper()
	body := map[string]interface{}{
		"allowed_models": []string{"model-a"},
		"routing":        map[string]interface{}{"strategy": "round_robin"},
		"rate_limit":     map[string]interface{}{"rpm": 100, "burst": 10},
		"compliance":     map[string]interface{}{"retention_days": 90, "log_mode": "metadata_only"},
		"budgets":        map[string]interface{}{"monthly_usd": 0, "timezone": "UTC"},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return b
}

func TestAdminPutTenantConfig_409_VersionConflict(t *testing.T) {
	store := &fakeStorage{
		putConfigErr: storage.ErrVersionConflict{Expected: 0, Current: 1},
	}
	cfg := adminConfigTestCfg()
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/t1/config", bytes.NewReader(validPutBody(t)))
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match-Version", "0")

	w := httptest.NewRecorder()
	h.AdminPutTenantConfig(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "version_conflict_error" {
		t.Errorf("expected type version_conflict_error, got %q", resp.Error.Type)
	}
}

// --- SPEC_148: routing.route_group config validation (tests 1-3) -----------

// Test 1: routing.route_group exists in selection.route_groups → PUT succeeds.
func TestSpec148_Config_RouteGroupExists_OK(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelConfig{{Name: "model-a", Provider: "openai"}},
	}
	tc := config.TenantConfig{
		AllowedModels: []string{"model-a"},
		Routing: config.RoutingConfig{
			Strategy:   "round_robin",
			RouteGroup: "cheap",
		},
		Selection: config.SelectionConfig{
			RouteGroups: map[string][]string{
				"cheap": {"model-a"},
			},
		},
		RateLimit:  config.RateLimitConfig{RPM: 100, Burst: 10},
		Compliance: config.ComplianceConfig{RetentionDays: 90, LogMode: "metadata_only"},
		Budgets:    config.BudgetsConfig{MonthlyUSD: 0, Timezone: "UTC"},
	}
	if err := validateTenantConfig(&tc, cfg); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// Test 2: routing.route_group does not exist in selection.route_groups → error.
func TestSpec148_Config_RouteGroupMissing_Error(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelConfig{{Name: "model-a", Provider: "openai"}},
	}
	tc := config.TenantConfig{
		AllowedModels: []string{"model-a"},
		Routing: config.RoutingConfig{
			Strategy:   "round_robin",
			RouteGroup: "nonexistent",
		},
		Selection: config.SelectionConfig{
			RouteGroups: map[string][]string{
				"cheap": {"model-a"},
			},
		},
		RateLimit:  config.RateLimitConfig{RPM: 100, Burst: 10},
		Compliance: config.ComplianceConfig{RetentionDays: 90, LogMode: "metadata_only"},
		Budgets:    config.BudgetsConfig{MonthlyUSD: 0, Timezone: "UTC"},
	}
	err := validateTenantConfig(&tc, cfg)
	if err == nil {
		t.Fatal("expected validation error for missing route_group, got nil")
	}
	if !containsStr(err.Error(), "nonexistent") {
		t.Errorf("error should mention the bad group name: %v", err)
	}
}

// Test 3: routing.route_group is empty/absent → no validation error.
func TestSpec148_Config_RouteGroupEmpty_OK(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelConfig{{Name: "model-a", Provider: "openai"}},
	}
	tc := config.TenantConfig{
		AllowedModels: []string{"model-a"},
		Routing: config.RoutingConfig{
			Strategy:   "round_robin",
			RouteGroup: "", // explicitly empty
		},
		RateLimit:  config.RateLimitConfig{RPM: 100, Burst: 10},
		Compliance: config.ComplianceConfig{RetentionDays: 90, LogMode: "metadata_only"},
		Budgets:    config.BudgetsConfig{MonthlyUSD: 0, Timezone: "UTC"},
	}
	if err := validateTenantConfig(&tc, cfg); err != nil {
		t.Errorf("expected no error for empty route_group, got: %v", err)
	}
}

// containsStr is a helper to check string containment.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------

func TestAdminPutTenantConfig_ValidationFail_NoChange(t *testing.T) {
	store := &fakeStorage{}
	cfg := adminConfigTestCfg()
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	// allowed_models is empty — fails validation before any DB call
	invalidBody, _ := json.Marshal(map[string]interface{}{
		"allowed_models": []string{},
		"routing":        map[string]interface{}{"strategy": "round_robin"},
		"rate_limit":     map[string]interface{}{"rpm": 100, "burst": 10},
		"compliance":     map[string]interface{}{"retention_days": 90, "log_mode": "metadata_only"},
		"budgets":        map[string]interface{}{"monthly_usd": 0, "timezone": "UTC"},
	})

	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/t1/config", bytes.NewReader(invalidBody))
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match-Version", "0")

	w := httptest.NewRecorder()
	h.AdminPutTenantConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// Verification: no LogRequest calls (proxy) — the fakeStorage.requests stays empty.
	// More importantly: putConfigErr is nil, so if PutTenantConfig had been called it would
	// have returned (1, nil) and the handler would have returned 200 instead of 400.
	if len(store.Requests()) != 0 {
		t.Errorf("expected no request logs, got %d", len(store.Requests()))
	}
}
