package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// apiKeyHash computes the SHA256 hex hash that AdminMiddleware uses for DB lookup.
func apiKeyHash(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// crossTenantSetup builds the config and store for cross-tenant admin tests.
// tenant_a is in YAML (so auth can find it), tenant_b is only in DB / dynamic.
func crossTenantSetup() (*config.Config, *fakeStorage) {
	cfg := &config.Config{
		Models: []config.ModelConfig{
			{Name: "model-a", Provider: "openai"},
		},
		Tenants: []config.TenantConfig{
			{
				ID:            "tenant_a",
				AllowedModels: []string{"model-a"},
				Routing:       config.RoutingConfig{Strategy: "round_robin"},
			},
		},
	}

	const testKey = "rtk_test_admin_key"
	store := &fakeStorage{
		lookupAPIKeyFound: true,
		lookupAPIKeyResult: storage.APIKeyRecord{
			TenantID: "tenant_a",
			Name:     "admin-key",
			KeyHash:  apiKeyHash(testKey),
			Scopes:   []string{"admin_write"},
		},
	}
	return cfg, store
}

// TestAdminConfig_CrossTenant_GET verifies that an admin API key belonging to
// tenant_a can GET the config of tenant_b without a 403 tenant-mismatch error.
func TestAdminConfig_CrossTenant_GET(t *testing.T) {
	cfg, store := crossTenantSetup()
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	cache := config.NewTenantConfigCache(1 * time.Second)
	chain := AdminMiddleware(cfg, cache, store, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(
		AdminScopeMiddleware(testLoggerForAdmin())(
			http.HandlerFunc(h.AdminGetTenantConfig),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_b/config", nil)
	req.SetPathValue("tenant_id", "tenant_b")
	req.Header.Set("X-API-Key", "rtk_test_admin_key")

	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	// Must NOT return 403 (tenant mismatch).  tenant_b has no config → 404 is expected.
	if w.Code == http.StatusForbidden {
		t.Fatalf("got 403 tenant-mismatch for cross-tenant GET; fix not applied or adminTenantMW still in chain")
	}
	// 404 means auth passed and the handler correctly reported tenant not found.
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (tenant not found), got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminConfig_CrossTenant_PATCH verifies that an admin API key belonging to
// tenant_a can PATCH the config of tenant_b without a 403 tenant-mismatch error.
func TestAdminConfig_CrossTenant_PATCH(t *testing.T) {
	cfg, store := crossTenantSetup()
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	cache := config.NewTenantConfigCache(1 * time.Second)
	chain := AdminMiddleware(cfg, cache, store, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(
		AdminScopeMiddleware(testLoggerForAdmin())(
			http.HandlerFunc(h.AdminPatchTenantConfig),
		),
	)

	body, _ := json.Marshal(map[string]interface{}{"rate_limit": map[string]interface{}{"rpm": 200}})
	req := httptest.NewRequest(http.MethodPatch, "/admin/tenants/tenant_b/config", bytes.NewReader(body))
	req.SetPathValue("tenant_id", "tenant_b")
	req.Header.Set("X-API-Key", "rtk_test_admin_key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match-Version", "0")

	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	// Must NOT return 403 (tenant mismatch).
	if w.Code == http.StatusForbidden {
		t.Fatalf("got 403 tenant-mismatch for cross-tenant PATCH; fix not applied or adminTenantMW still in chain")
	}
	// The handler may succeed or fail for other reasons (e.g. tenant not found
	// in YAML → 404, or DB not found → patch applies anyway), but it must not
	// return 403 due to tenant isolation.
	if w.Code == http.StatusForbidden {
		t.Errorf("unexpected 403: %s", w.Body.String())
	}
}

// TestAdminConfig_NonAdminKey_Rejected verifies that a key with only inference
// scope cannot access admin config endpoints.
func TestAdminConfig_NonAdminKey_Rejected(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "super-secret")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: "tenant_a", AllowedModels: []string{"model-a"}},
		},
	}

	const testKey = "rtk_inference_only"
	store := &fakeStorage{
		lookupAPIKeyFound: true,
		lookupAPIKeyResult: storage.APIKeyRecord{
			TenantID: "tenant_a",
			Name:     "inference-key",
			KeyHash:  apiKeyHash(testKey),
			Scopes:   []string{"inference"}, // no admin scope
		},
	}

	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	cache := config.NewTenantConfigCache(1 * time.Second)
	chain := AdminMiddleware(cfg, cache, store, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(
		AdminScopeMiddleware(testLoggerForAdmin())(
			http.HandlerFunc(h.AdminGetTenantConfig),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/config", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	req.Header.Set("X-API-Key", testKey)

	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin key, got %d: %s", w.Code, w.Body.String())
	}
}
