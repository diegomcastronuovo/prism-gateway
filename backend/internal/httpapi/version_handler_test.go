package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/diegomcastronuovo/prism-gateway/internal/version"
)

// versionChain builds an admin-auth'd handler chain for GET /admin/version.
func versionChain(apiKey string) (http.Handler, *httptest.ResponseRecorder) {
	cfg := &config.Config{
		Models: []config.ModelConfig{{Name: "model-a", Provider: "openai"}},
		Tenants: []config.TenantConfig{{
			ID:            "t1",
			AllowedModels: []string{"model-a"},
		}},
	}

	store := &fakeStorage{}
	if apiKey != "" {
		store.lookupAPIKeyFound = true
		store.lookupAPIKeyResult = storage.APIKeyRecord{
			TenantID: "t1",
			Name:     "admin-key",
			KeyHash:  apiKeyHash(apiKey),
			Scopes:   []string{"admin_write"},
		}
	}

	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLoggerForAdmin(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	cache := config.NewTenantConfigCache(1 * time.Second)
	chain := AdminMiddleware(cfg, cache, store, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(
		AdminScopeMiddleware(testLoggerForAdmin())(
			http.HandlerFunc(h.AdminGetVersion),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/version", nil)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)
	return chain, w
}

// Test 1: endpoint returns HTTP 200.
func TestAdminGetVersion_HTTP200(t *testing.T) {
	_, w := versionChain("rtk_test_admin_key")
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

// Test 2: backend_version field is present and matches the package variable.
func TestAdminGetVersion_BackendVersionPresent(t *testing.T) {
	_, w := versionChain("rtk_test_admin_key")
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	v, ok := resp["backend_version"]
	if !ok {
		t.Fatal("backend_version field missing from response")
	}
	if v != version.BackendVersion {
		t.Errorf("backend_version: want %q, got %q", version.BackendVersion, v)
	}
}

// Test 3: endpoint requires admin API key — no key → 401/403.
func TestAdminGetVersion_RequiresAdminKey(t *testing.T) {
	_, w := versionChain("") // no key
	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 when no API key provided, got 200")
	}
}

// Test 4: response contains all fields defined in the spec.
func TestAdminGetVersion_JSONStructure(t *testing.T) {
	_, w := versionChain("rtk_test_admin_key")
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, f := range []string{"service", "backend_version", "git_commit", "build_time", "release_notes"} {
		if _, ok := resp[f]; !ok {
			t.Errorf("required field %q missing from response", f)
		}
	}
	if resp["service"] != "ai-gateway" {
		t.Errorf("service: want \"ai-gateway\", got %q", resp["service"])
	}
}
