package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// envFakeStorage controls what GetTenantConfig returns for SPEC_146 tests.
type envFakeStorage struct {
	fakeStorage
	configJSON   json.RawMessage
	version      int
	exists       bool
}

func (f *envFakeStorage) GetTenantConfig(_ context.Context, _ string) (json.RawMessage, int, bool, error) {
	return f.configJSON, f.version, f.exists, nil
}

func tenantConfigWithEnv(env string) json.RawMessage {
	tc := config.TenantConfig{ID: "t1", Environment: env, AllowedModels: []string{"m1"}}
	b, _ := json.Marshal(tc)
	return b
}

func getEnvironmentFromResponse(t *testing.T, body string) string {
	t.Helper()
	var resp struct {
		Config struct {
			Environment string `json:"environment"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, body)
	}
	return resp.Config.Environment
}

// Test 1: Tenant seeded from YAML (no environment set) → DB returns empty env → normalized to "DEV".
// SPEC_147: all tenants come from DB (seeded from YAML at bootstrap).
func TestSpec146_YAMLTenantNoEnv_ReturnsDEV(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: "t1", AllowedModels: []string{"m1"}},
		},
	}
	// Simulate: YAML tenant was seeded to DB at bootstrap with empty environment.
	store := &envFakeStorage{
		configJSON: tenantConfigWithEnv(""), // seeded to DB with no env
		version:    1,
		exists:     true,
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/config", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminGetTenantConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if env := getEnvironmentFromResponse(t, w.Body.String()); env != "DEV" {
		t.Errorf("expected environment=DEV, got %q", env)
	}
}

// Test 2: DB tenant with "dev" → returns "DEV" (normalized)
func TestSpec146_DBTenantLowercase_Normalized(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "t1"}},
	}
	store := &envFakeStorage{
		configJSON: tenantConfigWithEnv("dev"),
		version:    1,
		exists:     true,
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/config", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminGetTenantConfig(w, req)

	if env := getEnvironmentFromResponse(t, w.Body.String()); env != "DEV" {
		t.Errorf("expected DEV, got %q", env)
	}
}

// Test 3: DB tenant with "PROD" → unchanged
func TestSpec146_DBTenantPROD_Unchanged(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "t1"}},
	}
	store := &envFakeStorage{
		configJSON: tenantConfigWithEnv("PROD"),
		version:    2,
		exists:     true,
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/config", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminGetTenantConfig(w, req)

	if env := getEnvironmentFromResponse(t, w.Body.String()); env != "PROD" {
		t.Errorf("expected PROD, got %q", env)
	}
}

// Test 4: response ALWAYS contains config.environment (DB tenant without env → DEV)
func TestSpec146_ResponseAlwaysContainsEnvironment(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "t1"}},
	}
	store := &envFakeStorage{
		configJSON: tenantConfigWithEnv(""), // empty env in DB
		version:    1,
		exists:     true,
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/config", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminGetTenantConfig(w, req)

	env := getEnvironmentFromResponse(t, w.Body.String())
	if env == "" {
		t.Error("config.environment must never be empty")
	}
	if env != "DEV" {
		t.Errorf("expected DEV fallback, got %q", env)
	}
}
