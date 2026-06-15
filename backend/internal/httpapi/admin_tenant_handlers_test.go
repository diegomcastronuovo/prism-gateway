package httpapi

import (
	"bytes"
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

// ── AdminCreateTenant ─────────────────────────────────────────────────────────

func TestAdminCreateTenant_Happy(t *testing.T) {
	cfg := &config.Config{Tenants: []config.TenantConfig{{ID: "existing"}}}
	store := &fakeStorage{deleteTenantFound: true}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin(), tenantCache: config.NewTenantConfigCache(0)}

	body := `{"tenant_id":"new-tenant"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["tenant_id"] != "new-tenant" {
		t.Errorf("tenant_id=%v, want new-tenant", resp["tenant_id"])
	}
	// Version should be 1.
	if resp["version"] != float64(1) {
		t.Errorf("version=%v, want 1", resp["version"])
	}
}

func TestAdminCreateTenant_MissingTenantID(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAdminCreateTenant_InvalidTenantID(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	for _, bad := range []string{"1starts-with-digit", "has space", "has!bang", ""} {
		body, _ := json.Marshal(map[string]string{"tenant_id": bad})
		req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		h.AdminCreateTenant(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("tenant_id=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

func TestAdminCreateTenant_ConflictYAML(t *testing.T) {
	// Tenant exists in static YAML config → 409.
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "existing-tenant"}},
	}
	h := &Handlers{cfg: cfg, store: &fakeStorage{}, log: testLoggerForAdmin()}

	body := `{"tenant_id":"existing-tenant"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestAdminCreateTenant_ConflictDB(t *testing.T) {
	// Storage returns ErrTenantAlreadyExists → 409.
	store := &fakeStorage{createTenantErr: storage.ErrTenantAlreadyExists{TenantID: "dup"}}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	body := `{"tenant_id":"dup"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestAdminCreateTenant_LocalAdminForbidden(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	body := `{"tenant_id":"tenant-new"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	req = req.WithContext(auth.WithRoles(req.Context(), []string{"local_admin"}))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for local_admin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateTenant_AdminAllowed(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	body := `{"tenant_id":"tenant-admin-ok"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	req = req.WithContext(auth.WithRoles(req.Context(), []string{"admin"}))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for admin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateTenant_InvalidJSON(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString("not-json"))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── AdminDeleteTenant ─────────────────────────────────────────────────────────

func TestAdminDeleteTenant_Happy(t *testing.T) {
	store := &fakeStorage{deleteTenantFound: true}
	h := &Handlers{
		cfg:         &config.Config{},
		store:       store,
		log:         testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/some-tenant", nil)
	req.SetPathValue("tenant_id", "some-tenant")
	w := httptest.NewRecorder()
	h.AdminDeleteTenant(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteTenant_NotFound(t *testing.T) {
	store := &fakeStorage{deleteTenantFound: false}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/ghost", nil)
	req.SetPathValue("tenant_id", "ghost")
	w := httptest.NewRecorder()
	h.AdminDeleteTenant(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminDeleteTenant_StoreError(t *testing.T) {
	store := &fakeStorage{deleteTenantErr: storage.ErrVersionConflict{}} // any error
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/t1", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminDeleteTenant(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Auth required ─────────────────────────────────────────────────────────────

func TestAdminTenantManagement_RequiresAuth(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := &config.Config{}
	store := &fakeStorage{deleteTenantFound: true}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	endpoints := []struct {
		method string
		path   string
		body   string
		fn     http.HandlerFunc
	}{
		{http.MethodPost, "/admin/tenants", `{"tenant_id":"t"}`, h.AdminCreateTenant},
		{http.MethodDelete, "/admin/tenants/t1", "", h.AdminDeleteTenant},
	}

	cache := config.NewTenantConfigCache(1 * time.Second)
	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var buf *bytes.Buffer
			if ep.body != "" {
				buf = bytes.NewBufferString(ep.body)
			} else {
				buf = &bytes.Buffer{}
			}
			req := httptest.NewRequest(ep.method, ep.path, buf)
			// No X-Admin-Token → expect 401.
			w := httptest.NewRecorder()
			AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(ep.fn).ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", w.Code)
			}
		})
	}
}

// ── isValidTenantID ───────────────────────────────────────────────────────────

func TestIsValidTenantID(t *testing.T) {
	valid := []string{"tenant-a", "tenant_b", "Tenant1", "a", "abc123", "my-tenant-99"}
	for _, id := range valid {
		if !validTenantIDRe.MatchString(id) {
			t.Errorf("%q should be valid", id)
		}
	}

	invalid := []string{"", "1startsWithDigit", "has space", "has!bang", "has.dot"}
	for _, id := range invalid {
		if validTenantIDRe.MatchString(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

// ── Environment (SPEC_133) ─────────────────────────────────────────────────────

func TestAdminCreateTenant_NoEnvironment_DefaultsDEV(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	body := `{"tenant_id":"tenant-env-default"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cfg map[string]any
	json.Unmarshal(store.lastCreateInitialConfig, &cfg)
	if cfg["environment"] != "DEV" {
		t.Errorf("environment=%v, want DEV", cfg["environment"])
	}
}

func TestAdminCreateTenant_ExplicitDEV(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	body := `{"tenant_id":"tenant-dev","environment":"DEV"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cfg map[string]any
	json.Unmarshal(store.lastCreateInitialConfig, &cfg)
	if cfg["environment"] != "DEV" {
		t.Errorf("environment=%v, want DEV", cfg["environment"])
	}
}

func TestAdminCreateTenant_STAGING(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	body := `{"tenant_id":"tenant-staging","environment":"STAGING"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cfg map[string]any
	json.Unmarshal(store.lastCreateInitialConfig, &cfg)
	if cfg["environment"] != "STAGING" {
		t.Errorf("environment=%v, want STAGING", cfg["environment"])
	}
}

func TestAdminCreateTenant_PROD(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	body := `{"tenant_id":"tenant-prod","environment":"PROD"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.AdminCreateTenant(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cfg map[string]any
	json.Unmarshal(store.lastCreateInitialConfig, &cfg)
	if cfg["environment"] != "PROD" {
		t.Errorf("environment=%v, want PROD", cfg["environment"])
	}
}

func TestAdminCreateTenant_InvalidEnvironment(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	for _, bad := range []string{"dev", "Prod", "production", "qa", "test"} {
		body, _ := json.Marshal(map[string]string{"tenant_id": "tenant-bad", "environment": bad})
		req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		h.AdminCreateTenant(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("environment=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

func TestAdminPatchTenantConfig_EnvironmentImmutable(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{
		cfg:         adminConfigTestCfg(),
		store:       store,
		log:         testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	patch := `{"environment":"PROD"}`
	req := httptest.NewRequest(http.MethodPatch, "/admin/tenants/t1/config", bytes.NewBufferString(patch))
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminPatchTenantConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error.Message != "environment is immutable" {
		t.Errorf("message=%q, want 'environment is immutable'", errResp.Error.Message)
	}
}

func TestAdminPutTenantConfig_EnvironmentImmutable(t *testing.T) {
	// Current stored config has environment=PROD; attempt to PUT with environment=DEV → 400.
	currentCfg := json.RawMessage(`{"environment":"PROD","allowed_models":["model-a"],"routing":{"strategy":"round_robin"},"rate_limit":{"rpm":100,"burst":10},"compliance":{"retention_days":90,"log_mode":"metadata_only"},"budgets":{"monthly_usd":0,"timezone":"UTC"}}`)
	store := &fakeStorage{tenantConfigJSON: currentCfg}
	cfg := adminConfigTestCfg()
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	putBody := map[string]interface{}{
		"environment":    "DEV",
		"allowed_models": []string{"model-a"},
		"routing":        map[string]interface{}{"strategy": "round_robin"},
		"rate_limit":     map[string]interface{}{"rpm": 100, "burst": 10},
		"compliance":     map[string]interface{}{"retention_days": 90, "log_mode": "metadata_only"},
		"budgets":        map[string]interface{}{"monthly_usd": 0, "timezone": "UTC"},
	}
	b, _ := json.Marshal(putBody)
	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/t1/config", bytes.NewReader(b))
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminPutTenantConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error.Message != "environment is immutable" {
		t.Errorf("message=%q, want 'environment is immutable'", errResp.Error.Message)
	}
}

func TestAdminPutTenantConfig_NonEnvironmentFieldsStillWork(t *testing.T) {
	// PUT with no environment field should succeed (no immutability clash).
	store := &fakeStorage{}
	cfg := adminConfigTestCfg()
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLoggerForAdmin(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/t1/config", bytes.NewReader(validPutBody(t)))
	req.SetPathValue("tenant_id", "t1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminPutTenantConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
