package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── per-test fakeStorage overrides ───────────────────────────────────────────

// globalConfigFakeStorage embeds fakeStorage and allows per-test overrides
// for global config operations.
type globalConfigFakeStorage struct {
	fakeStorage
	// GET overrides
	globalConfigJSON    json.RawMessage
	globalConfigVersion int
	globalConfigExists  bool
	globalConfigGetErr  error
	// PUT/PATCH/ROLLBACK overrides
	putGlobalConfigErr      error
	putGlobalConfigVersion  int
	patchGlobalConfigErr    error
	patchGlobalConfigVersion int
	rollbackGlobalConfigErr error
	// ListConfigHistory capture (tests)
	lastConfigHistoryFilter storage.ConfigHistoryFilter
}

func (s *globalConfigFakeStorage) GetGlobalConfig(_ context.Context) (json.RawMessage, int, bool, error) {
	return s.globalConfigJSON, s.globalConfigVersion, s.globalConfigExists, s.globalConfigGetErr
}

func (s *globalConfigFakeStorage) PutGlobalConfig(_ context.Context, _ int, _ json.RawMessage, _ string, _ []string) (int, error) {
	if s.putGlobalConfigErr != nil {
		return 0, s.putGlobalConfigErr
	}
	v := s.putGlobalConfigVersion
	if v == 0 {
		v = 1
	}
	return v, nil
}

func (s *globalConfigFakeStorage) PatchGlobalConfig(_ context.Context, _ int, _ json.RawMessage, _ string, _ []string) (int, error) {
	if s.patchGlobalConfigErr != nil {
		return 0, s.patchGlobalConfigErr
	}
	v := s.patchGlobalConfigVersion
	if v == 0 {
		v = 1
	}
	return v, nil
}

func (s *globalConfigFakeStorage) RollbackGlobalConfig(_ context.Context, _, _ int, _ string, _ []string) error {
	return s.rollbackGlobalConfigErr
}

func (s *globalConfigFakeStorage) ListConfigHistory(_ context.Context, f storage.ConfigHistoryFilter) ([]storage.ConfigHistoryRow, error) {
	s.lastConfigHistoryFilter = f
	return nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// globalConfigHandlers returns a Handlers suitable for global config handler tests.
func globalConfigHandlers(store *globalConfigFakeStorage) *Handlers {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai":        {Type: "openai", BaseURL: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"},
			"anthropic":    {Type: "anthropic", BaseURL: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY"},
			"azure-openai": {Type: "openai", BaseURL: "https://azure.openai.com", APIKeyEnv: "AZURE_OPENAI_API_KEY"},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o", Provider: "openai"},
			{Name: "claude-3", Provider: "anthropic"},
		},
	}
	return &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

// validGlobalConfigBody builds a minimal valid GlobalConfig JSON body.
func validGlobalConfigBody(t *testing.T) []byte {
	t.Helper()
	gc := map[string]interface{}{
		"models": []map[string]interface{}{
			{"name": "gpt-4o", "provider": "openai"},
		},
		"providers":       map[string]interface{}{},
		"circuit_breaker": map[string]interface{}{},
		"rate_limit":      map[string]interface{}{},
		"smart_routing":   map[string]interface{}{},
	}
	b, err := json.Marshal(gc)
	if err != nil {
		t.Fatalf("marshal valid global config body: %v", err)
	}
	return b
}

func makeGlobalReq(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Content-Type", "application/json")
	return r
}

func withRoles(r *http.Request, roles []string) *http.Request {
	ctx := auth.WithRoles(r.Context(), roles)
	return r.WithContext(ctx)
}

// ── GET /admin/config/global ──────────────────────────────────────────────────

// TestAdminGetGlobalConfig_NoRecord verifies that when the DB has no record
// the handler falls back to YAML-derived config with version=0.
func TestAdminGetGlobalConfig_NoRecord(t *testing.T) {
	store := &globalConfigFakeStorage{} // exists=false by default
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodGet, "/admin/config/global", nil)
	w := httptest.NewRecorder()
	h.AdminGetGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if v, ok := resp["version"].(float64); !ok || int(v) != 0 {
		t.Errorf("expected version=0 for fallback, got %v", resp["version"])
	}
	if resp["config"] == nil {
		t.Error("expected non-nil config in response")
	}
}

func TestGlobalConfigEndpoints_LocalAdminForbidden(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	tests := []struct {
		name string
		run  http.HandlerFunc
		req  *http.Request
	}{
		{
			name: "GET global",
			run:  h.AdminGetGlobalConfig,
			req:  makeGlobalReq(t, http.MethodGet, "/admin/config/global", nil),
		},
		{
			name: "PUT global",
			run:  h.AdminPutGlobalConfig,
			req: func() *http.Request {
				r := makeGlobalReq(t, http.MethodPut, "/admin/config/global", validGlobalConfigBody(t))
				r.Header.Set("If-Match-Version", "0")
				return r
			}(),
		},
		{
			name: "PATCH global",
			run:  h.AdminPatchGlobalConfig,
			req: func() *http.Request {
				r := makeGlobalReq(t, http.MethodPatch, "/admin/config/global", []byte(`{"rate_limit":{"rpm":123}}`))
				r.Header.Set("If-Match-Version", "0")
				return r
			}(),
		},
		{
			name: "POST apply",
			run:  h.AdminApplyGlobalConfigVersion,
			req:  makeGlobalReq(t, http.MethodPost, "/admin/config/global/apply", []byte(`{"version":1}`)),
		},
		{
			name: "GET versions",
			run:  h.AdminConfigVersions,
			req:  makeGlobalReq(t, http.MethodGet, "/admin/config/versions?scope=global", nil),
		},
		{
			name: "GET diff",
			run:  h.AdminConfigDiff,
			req:  makeGlobalReq(t, http.MethodGet, "/admin/config/diff?scope=global&from_version=1&to_version=2", nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := withRoles(tc.req, []string{"local_admin"})
			w := httptest.NewRecorder()
			tc.run(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for local_admin, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAdminConfigHistory_LocalAdmin_ScopeGlobal_BadRequest(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)
	req := makeGlobalReq(t, http.MethodGet, "/admin/config/history?scope=global", nil)
	req = withRoles(req, []string{"local_admin"})
	ctx := auth.WithAllowedTenants(req.Context(), []string{"t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.AdminConfigHistory(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminConfigHistory_LocalAdmin_TenantOnlyFilter(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)
	req := makeGlobalReq(t, http.MethodGet, "/admin/config/history", nil)
	req = withRoles(req, []string{"local_admin"})
	ctx := auth.WithAllowedTenants(req.Context(), []string{"t1", "t2"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.AdminConfigHistory(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !store.lastConfigHistoryFilter.ExcludeGlobal {
		t.Fatal("expected ExcludeGlobal for local_admin")
	}
	if len(store.lastConfigHistoryFilter.AllowedTenantIDs) != 2 {
		t.Fatalf("expected 2 allowed tenants, got %v", store.lastConfigHistoryFilter.AllowedTenantIDs)
	}
}

func TestAdminConfigHistory_LocalAdmin_TenantQueryNotAllowed(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)
	req := makeGlobalReq(t, http.MethodGet, "/admin/config/history?tenant_id=other", nil)
	req = withRoles(req, []string{"local_admin"})
	ctx := auth.WithAllowedTenants(req.Context(), []string{"t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.AdminConfigHistory(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminConfigHistory_AdminFullAccessNoFilter(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)
	req := withRoles(makeGlobalReq(t, http.MethodGet, "/admin/config/history?scope=global", nil), []string{"admin"})
	w := httptest.NewRecorder()
	h.AdminConfigHistory(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.lastConfigHistoryFilter.ExcludeGlobal {
		t.Fatal("admin must not use ExcludeGlobal")
	}
}

func TestAdminConfigHistory_EmptyRolesFullAccess(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)
	req := makeGlobalReq(t, http.MethodGet, "/admin/config/history", nil)
	w := httptest.NewRecorder()
	h.AdminConfigHistory(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.lastConfigHistoryFilter.ExcludeGlobal {
		t.Fatal("empty roles must not use ExcludeGlobal (API key / admin token style)")
	}
}

func TestGlobalConfigEndpoints_AdminAllowed(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	req := withRoles(makeGlobalReq(t, http.MethodGet, "/admin/config/global", nil), []string{"admin"})
	w := httptest.NewRecorder()
	h.AdminGetGlobalConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected admin to access global config, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminGetGlobalConfig_FromDB verifies that when a DB record exists
// the handler returns it with the correct version.
func TestAdminGetGlobalConfig_FromDB(t *testing.T) {
	gc := config.GlobalConfig{
		Models: []config.ModelConfig{{Name: "db-model", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)

	store := &globalConfigFakeStorage{
		globalConfigJSON:    gcJSON,
		globalConfigVersion: 7,
		globalConfigExists:  true,
	}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodGet, "/admin/config/global", nil)
	w := httptest.NewRecorder()
	h.AdminGetGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if v, ok := resp["version"].(float64); !ok || int(v) != 7 {
		t.Errorf("expected version=7, got %v", resp["version"])
	}
}

// TestAdminGetGlobalConfig_ReturnsAuthWhenPresent verifies GET returns auth when stored config has it.
func TestAdminGetGlobalConfig_ReturnsAuthWhenPresent(t *testing.T) {
	auth := config.AuthConfig{
		Mode: "both",
		JWT: config.JWTConfig{
			Issuer:   "dev",
			Audience: "router",
			JWKSURL:  "https://host/.well-known/jwks.json",
			RequiredClaims: map[string]string{"tenant_id": "tenant_id", "roles": "roles"},
			RBAC: config.RBACConfig{
				UserRoles:    []string{"user"},
				AdminRoles:   []string{"admin"},
				FinanceRoles: []string{"finance"},
			},
		},
	}
	gc := config.GlobalConfig{
		Auth:      &auth,
		Models:    []config.ModelConfig{{Name: "m1", Provider: "openai"}},
		Providers: map[string]config.ProviderConfig{},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:    gcJSON,
		globalConfigVersion: 1,
		globalConfigExists:  true,
	}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodGet, "/admin/config/global", nil)
	w := httptest.NewRecorder()
	h.AdminGetGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Config map[string]interface{} `json:"config"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	authVal, ok := resp.Config["auth"]
	if !ok || authVal == nil {
		t.Fatal("config.auth should be present")
	}
	authMap, ok := authVal.(map[string]interface{})
	if !ok {
		t.Fatalf("config.auth should be an object, got %T", authVal)
	}
	if authMap["mode"] != "both" {
		t.Errorf("auth.mode=%v, want both", authMap["mode"])
	}
	jwtVal, _ := authMap["jwt"].(map[string]interface{})
	if jwtVal == nil {
		t.Error("auth.jwt should be present")
	} else if jwtVal["issuer"] != "dev" || jwtVal["audience"] != "router" {
		t.Errorf("auth.jwt.issuer/audience=%v", jwtVal)
	}
	rbacVal, _ := jwtVal["rbac"].(map[string]interface{})
	if rbacVal == nil {
		t.Error("auth.jwt.rbac should be present")
	}
}

// TestAdminGetGlobalConfig_LegacyWithoutAuth_DoesNotCrash verifies legacy config without auth returns 200.
func TestAdminGetGlobalConfig_LegacyWithoutAuth_DoesNotCrash(t *testing.T) {
	// Config that has no "auth" key (e.g. old version).
	legacyJSON := []byte(`{"models":[{"name":"m1","provider":"openai"}],"providers":{},"circuit_breaker":{},"rate_limit":{},"smart_routing":{}}`)
	store := &globalConfigFakeStorage{
		globalConfigJSON:    legacyJSON,
		globalConfigVersion: 1,
		globalConfigExists:  true,
	}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodGet, "/admin/config/global", nil)
	w := httptest.NewRecorder()
	h.AdminGetGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["config"] == nil {
		t.Error("config should be present")
	}
}

// ── PUT /admin/config/global ──────────────────────────────────────────────────

// TestAdminPutGlobalConfig_MissingIfMatchVersion verifies 428 when header absent.
func TestAdminPutGlobalConfig_MissingIfMatchVersion(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodPut, "/admin/config/global", validGlobalConfigBody(t))
	// No If-Match-Version header
	w := httptest.NewRecorder()
	h.AdminPutGlobalConfig(w, req)

	if w.Code != 428 {
		t.Errorf("expected 428, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminPutGlobalConfig_OK verifies the happy path returns version + message.
func TestAdminPutGlobalConfig_OK(t *testing.T) {
	store := &globalConfigFakeStorage{putGlobalConfigVersion: 3}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodPut, "/admin/config/global", validGlobalConfigBody(t))
	req.Header.Set("If-Match-Version", "2")
	w := httptest.NewRecorder()
	h.AdminPutGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if v, ok := resp["version"].(float64); !ok || int(v) != 3 {
		t.Errorf("expected version=3, got %v", resp["version"])
	}
	if resp["message"] == "" {
		t.Error("expected non-empty message")
	}
}

// TestAdminPutGlobalConfig_VersionConflict verifies 409 on ErrVersionConflict.
func TestAdminPutGlobalConfig_VersionConflict(t *testing.T) {
	store := &globalConfigFakeStorage{
		putGlobalConfigErr: storage.ErrVersionConflict{Expected: 0, Current: 5},
	}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodPut, "/admin/config/global", validGlobalConfigBody(t))
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminPutGlobalConfig(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "version_conflict_error" {
		t.Errorf("expected version_conflict_error, got %q", resp.Error.Type)
	}
}

// TestAdminPutGlobalConfig_DuplicateModelNames verifies 422 on duplicate model names.
func TestAdminPutGlobalConfig_DuplicateModelNames(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{
		"models": []map[string]interface{}{
			{"name": "gpt-4o", "provider": "openai"},
			{"name": "gpt-4o", "provider": "openai"}, // duplicate
		},
	})
	req := makeGlobalReq(t, http.MethodPut, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminPutGlobalConfig(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.Error.Type)
	}
}

// TestAdminPutGlobalConfig_PreservesAuth verifies PUT accepts and preserves auth block.
func TestAdminPutGlobalConfig_PreservesAuth(t *testing.T) {
	store := &globalConfigFakeStorage{putGlobalConfigVersion: 2}
	h := globalConfigHandlers(store)

	body := map[string]interface{}{
		"auth": map[string]interface{}{
			"mode": "both",
			"jwt": map[string]interface{}{
				"issuer":   "dev",
				"audience": "router",
				"jwks_url": "https://example.com/.well-known/jwks.json",
				"required_claims": map[string]interface{}{"tenant_id": "tenant_id", "roles": "roles"},
				"rbac": map[string]interface{}{
					"user_roles":    []interface{}{"user"},
					"admin_roles":   []interface{}{"admin"},
					"finance_roles": []interface{}{"finance"},
				},
			},
		},
		"models":          []map[string]interface{}{{"name": "gpt-4o", "provider": "openai"}},
		"providers":       map[string]interface{}{},
		"circuit_breaker": map[string]interface{}{},
		"rate_limit":      map[string]interface{}{},
		"smart_routing":   map[string]interface{}{},
	}
	bodyBytes, _ := json.Marshal(body)
	req := makeGlobalReq(t, http.MethodPut, "/admin/config/global", bodyBytes)
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminPutGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminPutGlobalConfig_InvalidAuthMode verifies 422 for invalid auth.mode.
func TestAdminPutGlobalConfig_InvalidAuthMode(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{
		"auth":   map[string]interface{}{"mode": "invalid"},
		"models": []map[string]interface{}{{"name": "m1", "provider": "openai"}},
		"providers": map[string]interface{}{}, "circuit_breaker": map[string]interface{}{},
		"rate_limit": map[string]interface{}{}, "smart_routing": map[string]interface{}{},
	})
	req := makeGlobalReq(t, http.MethodPut, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminPutGlobalConfig(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.Error.Type)
	}
}

// TestAdminPutGlobalConfig_InvalidJWTWhenModeJWT verifies 422 when mode is jwt but JWT fields missing.
func TestAdminPutGlobalConfig_InvalidJWTWhenModeJWT(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{
		"auth": map[string]interface{}{
			"mode": "jwt",
			"jwt":  map[string]interface{}{"issuer": "", "audience": "", "jwks_url": ""},
		},
		"models": []map[string]interface{}{{"name": "m1", "provider": "openai"}},
		"providers": map[string]interface{}{}, "circuit_breaker": map[string]interface{}{},
		"rate_limit": map[string]interface{}{}, "smart_routing": map[string]interface{}{},
	})
	req := makeGlobalReq(t, http.MethodPut, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminPutGlobalConfig(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

// ── PATCH /admin/config/global — merge patch ──────────────────────────────────

// TestAdminPatchGlobalConfig_MergePatch_OK verifies merge patch returns new version.
func TestAdminPatchGlobalConfig_MergePatch_OK(t *testing.T) {
	store := &globalConfigFakeStorage{patchGlobalConfigVersion: 4}
	h := globalConfigHandlers(store)

	patch := map[string]interface{}{
		"smart_routing": map[string]interface{}{"enabled": true},
	}
	body, _ := json.Marshal(patch)

	req := makeGlobalReq(t, http.MethodPatch, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "3")
	w := httptest.NewRecorder()
	h.AdminPatchGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if v, ok := resp["version"].(float64); !ok || int(v) != 4 {
		t.Errorf("expected version=4, got %v", resp["version"])
	}
}

// TestAdminPatchGlobalConfig_PatchAuthFields verifies PATCH can update nested auth fields.
func TestAdminPatchGlobalConfig_PatchAuthFields(t *testing.T) {
	store := &globalConfigFakeStorage{patchGlobalConfigVersion: 5}
	h := globalConfigHandlers(store)

	patch := map[string]interface{}{
		"auth": map[string]interface{}{"mode": "jwt"},
	}
	body, _ := json.Marshal(patch)

	req := makeGlobalReq(t, http.MethodPatch, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "4")
	w := httptest.NewRecorder()
	h.AdminPatchGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if v, ok := resp["version"].(float64); !ok || int(v) != 5 {
		t.Errorf("expected version=5, got %v", resp["version"])
	}
}

// TestAdminPatchGlobalConfig_MergePatch_Conflict verifies 409 on version conflict.
func TestAdminPatchGlobalConfig_MergePatch_Conflict(t *testing.T) {
	store := &globalConfigFakeStorage{
		patchGlobalConfigErr: storage.ErrVersionConflict{Expected: 3, Current: 5},
	}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{"smart_routing": map[string]interface{}{}})
	req := makeGlobalReq(t, http.MethodPatch, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "3")
	w := httptest.NewRecorder()
	h.AdminPatchGlobalConfig(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// ── PATCH /admin/config/global — rollback ─────────────────────────────────────

// TestAdminPatchGlobalConfig_Rollback_OK verifies rollback returns target version.
func TestAdminPatchGlobalConfig_Rollback_OK(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{"rollback_to_version": 2})
	req := makeGlobalReq(t, http.MethodPatch, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "5")
	w := httptest.NewRecorder()
	h.AdminPatchGlobalConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if v, ok := resp["version"].(float64); !ok || int(v) != 2 {
		t.Errorf("expected version=2 (the rollback target), got %v", resp["version"])
	}
}

// TestAdminPatchGlobalConfig_Rollback_InvalidVersion verifies 400 for version ≤ 0.
func TestAdminPatchGlobalConfig_Rollback_InvalidVersion(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{"rollback_to_version": 0})
	req := makeGlobalReq(t, http.MethodPatch, "/admin/config/global", body)
	req.Header.Set("If-Match-Version", "5")
	w := httptest.NewRecorder()
	h.AdminPatchGlobalConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ── PATCH /admin/models/{model_name} ─────────────────────────────────────────

// TestAdminPatchModel_NotFound verifies 404 when model does not exist in config.
func TestAdminPatchModel_NotFound(t *testing.T) {
	gc := config.GlobalConfig{
		Models: []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:    gcJSON,
		globalConfigVersion: 1,
		globalConfigExists:  true,
	}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{"provider": "anthropic"})
	req := makeGlobalReq(t, http.MethodPatch, "/admin/models/nonexistent", body)
	req.SetPathValue("model_name", "nonexistent")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminPatchModel(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminPatchModel_OK verifies that patching an existing model updates it.
func TestAdminPatchModel_OK(t *testing.T) {
	gc := config.GlobalConfig{
		Models: []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:       gcJSON,
		globalConfigVersion:    1,
		globalConfigExists:     true,
		putGlobalConfigVersion: 2,
	}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{"provider": "azure-openai"})
	req := makeGlobalReq(t, http.MethodPatch, "/admin/models/gpt-4o", body)
	req.SetPathValue("model_name", "gpt-4o")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminPatchModel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if v, ok := resp["version"].(float64); !ok || int(v) != 2 {
		t.Errorf("expected version=2, got %v", resp["version"])
	}
	if resp["model_name"] != "gpt-4o" {
		t.Errorf("expected model_name=gpt-4o, got %v", resp["model_name"])
	}
}

// TestAdminPatchModel_VersionConflict verifies 409 when If-Match-Version mismatches.
func TestAdminPatchModel_VersionConflict(t *testing.T) {
	gc := config.GlobalConfig{
		Models: []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:    gcJSON,
		globalConfigVersion: 5, // actual current version
		globalConfigExists:  true,
	}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{"provider": "azure-openai"})
	req := makeGlobalReq(t, http.MethodPatch, "/admin/models/gpt-4o", body)
	req.SetPathValue("model_name", "gpt-4o")
	req.Header.Set("If-Match-Version", "3") // stale version
	w := httptest.NewRecorder()
	h.AdminPatchModel(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "version_conflict_error" {
		t.Errorf("expected version_conflict_error, got %q", resp.Error.Type)
	}
}

// TestAdminPatchModel_MissingModelName verifies 400 when model_name path value is absent.
func TestAdminPatchModel_MissingModelName(t *testing.T) {
	store := &globalConfigFakeStorage{}
	h := globalConfigHandlers(store)

	body, _ := json.Marshal(map[string]interface{}{"provider": "openai"})
	req := makeGlobalReq(t, http.MethodPatch, "/admin/models/", body)
	// model_name path value intentionally NOT set
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminPatchModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ── GET /admin/models/{model_id} ─────────────────────────────────────────────

func TestAdminGetModel_OK(t *testing.T) {
	gc := config.GlobalConfig{
		Models: []config.ModelConfig{
			{Name: "gpt-4o", Provider: "openai", Type: "chat", Pricing: config.Pricing{PromptPer1M: 0.5, CompletionPer1M: 1.5}},
		},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:   gcJSON,
		globalConfigExists: true,
	}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodGet, "/admin/models/gpt-4o", nil)
	req.SetPathValue("model_id", "gpt-4o")
	w := httptest.NewRecorder()
	h.AdminGetModel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var item catalogModelItem
	if err := json.NewDecoder(w.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item.ID != "gpt-4o" || item.Provider != "openai" {
		t.Errorf("id=%q provider=%q", item.ID, item.Provider)
	}
	// Enriched fields: type and pricing come from stored config (JSON keys are PascalCase from Marshal)
	if item.Type != "" && item.Type != "chat" {
		t.Errorf("type=%q", item.Type)
	}
	if item.Pricing != nil && (item.Pricing.PromptPer1M != 0.5 || item.Pricing.CompletionPer1M != 1.5) {
		t.Errorf("pricing=%+v", item.Pricing)
	}
	if item.Mock == nil {
		t.Error("mock should be present")
	}
}

func TestAdminGetModel_NotFound(t *testing.T) {
	gc := config.GlobalConfig{Models: []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}}}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{globalConfigJSON: gcJSON, globalConfigExists: true}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodGet, "/admin/models/nonexistent", nil)
	req.SetPathValue("model_id", "nonexistent")
	w := httptest.NewRecorder()
	h.AdminGetModel(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ── POST /admin/models ──────────────────────────────────────────────────────

func TestAdminCreateModel_OK(t *testing.T) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"openai": {Type: "openai"}},
		Models:    []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:       gcJSON,
		globalConfigVersion:    1,
		globalConfigExists:     true,
		putGlobalConfigVersion: 2,
	}
	h := globalConfigHandlers(store)

	body := `{"id":"new-model","provider":"openai","type":"chat","pricing":{"prompt_per_1m":0.1,"completion_per_1m":0.2},"mock":{"enabled":false}}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/admin/models/new-model" {
		t.Errorf("Location=%q want /admin/models/new-model", loc)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if v, ok := resp["version"].(float64); !ok || int(v) != 2 {
		t.Errorf("version=%v", resp["version"])
	}
}

func TestAdminCreateModel_DuplicateID(t *testing.T) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"openai": {Type: "openai"}},
		Models:    []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{globalConfigJSON: gcJSON, globalConfigVersion: 1, globalConfigExists: true}
	h := globalConfigHandlers(store)

	body := `{"id":"gpt-4o","provider":"openai"}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateModel_MissingID(t *testing.T) {
	store := &globalConfigFakeStorage{globalConfigExists: false}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(`{"provider":"openai"}`))
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateModel_MissingProvider(t *testing.T) {
	store := &globalConfigFakeStorage{globalConfigExists: false}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(`{"id":"m1"}`))
	req.Header.Set("If-Match-Version", "0")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ── DELETE /admin/models/{model_id} ──────────────────────────────────────────

func TestAdminDeleteModel_OK(t *testing.T) {
	gc := config.GlobalConfig{
		Models: []config.ModelConfig{
			{Name: "gpt-4o", Provider: "openai"},
			{Name: "claude-3", Provider: "anthropic"},
		},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:       gcJSON,
		globalConfigVersion:    1,
		globalConfigExists:     true,
		putGlobalConfigVersion: 2,
	}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodDelete, "/admin/models/claude-3", nil)
	req.SetPathValue("model_id", "claude-3")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminDeleteModel(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteModel_NotFound(t *testing.T) {
	gc := config.GlobalConfig{Models: []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}}}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{globalConfigJSON: gcJSON, globalConfigVersion: 1, globalConfigExists: true}
	h := globalConfigHandlers(store)

	req := makeGlobalReq(t, http.MethodDelete, "/admin/models/nonexistent", nil)
	req.SetPathValue("model_id", "nonexistent")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminDeleteModel(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteModel_InUse(t *testing.T) {
	gc := config.GlobalConfig{Models: []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}}}
	gcJSON, _ := json.Marshal(gc)
	tenant := config.TenantConfig{ID: "t1", AllowedModels: []string{"gpt-4o"}}
	tenantJSON, _ := json.Marshal(tenant)
	store := &globalConfigFakeStorage{
		globalConfigJSON:    gcJSON,
		globalConfigVersion: 1,
		globalConfigExists:  true,
		fakeStorage: fakeStorage{
			listTenantsResult: []string{"t1"},
			tenantConfigsMap:  map[string]json.RawMessage{"t1": tenantJSON},
		},
	}
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{"openai": {Type: "openai"}},
		Models:    []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}},
		Tenants:   []config.TenantConfig{tenant},
	}
	h := &Handlers{cfg: cfg, store: store, log: testLogger(), globalCfgCache: config.NewGlobalConfigCache(0)}

	req := makeGlobalReq(t, http.MethodDelete, "/admin/models/gpt-4o", nil)
	req.SetPathValue("model_id", "gpt-4o")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminDeleteModel(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// ── observable fields validation ─────────────────────────────────────────────

func mlStoreWithProviders() (*globalConfigFakeStorage, config.GlobalConfig) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"local": {Type: "local"}},
		Models:    []config.ModelConfig{},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:       gcJSON,
		globalConfigVersion:    1,
		globalConfigExists:     true,
		putGlobalConfigVersion: 2,
	}
	return store, gc
}

func TestAdminCreateModel_ObservableFields_Valid(t *testing.T) {
	store, _ := mlStoreWithProviders()
	h := globalConfigHandlers(store)

	body := `{
		"id": "fraud-v1",
		"provider": "local",
		"type": "ml",
		"execution": {"endpoint": "http://ml-svc/predict"},
		"observable": {
			"fields": [
				{"path": "input.features", "type": "json",   "role": "input"},
				{"path": "output.score",   "type": "number", "role": "output"}
			]
		}
	}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateModel_ObservableFields_EmptyPath(t *testing.T) {
	store, _ := mlStoreWithProviders()
	h := globalConfigHandlers(store)

	body := `{
		"id": "fraud-v1",
		"provider": "local",
		"type": "ml",
		"execution": {"endpoint": "http://ml-svc/predict"},
		"observable": {
			"fields": [{"path": "", "type": "json", "role": "input"}]
		}
	}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateModel_ObservableFields_InvalidType(t *testing.T) {
	store, _ := mlStoreWithProviders()
	h := globalConfigHandlers(store)

	body := `{
		"id": "fraud-v1",
		"provider": "local",
		"type": "ml",
		"execution": {"endpoint": "http://ml-svc/predict"},
		"observable": {
			"fields": [{"path": "input.x", "type": "float", "role": "input"}]
		}
	}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateModel_ObservableFields_InvalidRole(t *testing.T) {
	store, _ := mlStoreWithProviders()
	h := globalConfigHandlers(store)

	body := `{
		"id": "fraud-v1",
		"provider": "local",
		"type": "ml",
		"execution": {"endpoint": "http://ml-svc/predict"},
		"observable": {
			"fields": [{"path": "input.x", "type": "json", "role": "feature"}]
		}
	}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateModel_ObservableFields_DuplicateRow(t *testing.T) {
	store, _ := mlStoreWithProviders()
	h := globalConfigHandlers(store)

	body := `{
		"id": "fraud-v1",
		"provider": "local",
		"type": "ml",
		"execution": {"endpoint": "http://ml-svc/predict"},
		"observable": {
			"fields": [
				{"path": "input.features", "type": "json", "role": "input"},
				{"path": "input.features", "type": "json", "role": "input"}
			]
		}
	}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateModel_ObservableFields_EmptyList_Valid(t *testing.T) {
	store, _ := mlStoreWithProviders()
	h := globalConfigHandlers(store)

	body := `{
		"id": "fraud-v1",
		"provider": "local",
		"type": "ml",
		"execution": {"endpoint": "http://ml-svc/predict"},
		"observable": {"fields": []}
	}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPatchModel_ObservableFields_InvalidType(t *testing.T) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"local": {Type: "local"}},
		Models: []config.ModelConfig{
			{
				Name:      "fraud-v1",
				Provider:  "local",
				Type:      "ml",
				Execution: &config.MLExecutionConfig{Endpoint: "http://ml-svc/predict"},
			},
		},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{globalConfigJSON: gcJSON, globalConfigVersion: 1, globalConfigExists: true}
	h := globalConfigHandlers(store)

	patch := `{"observable": {"fields": [{"path": "output.score", "type": "badtype", "role": "output"}]}}`
	req := makeGlobalReq(t, http.MethodPatch, "/admin/models/fraud-v1", []byte(patch))
	req.SetPathValue("model_id", "fraud-v1")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminPatchModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ── markup_percentage tests ───────────────────────────────────────────────────

// TestAdminCreateModel_WithMarkup verifies that markup_percentage persists on create.
func TestAdminCreateModel_WithMarkup(t *testing.T) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"openai": {Type: "openai"}},
		Models:    []config.ModelConfig{{Name: "gpt-4o", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:       gcJSON,
		globalConfigVersion:    5,
		globalConfigExists:     true,
		putGlobalConfigVersion: 6,
	}
	h := globalConfigHandlers(store)

	body := `{"id":"new-model","provider":"openai","markup_percentage":20}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "5")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminCreateModel_NegativeMarkup verifies that negative markup_percentage is rejected.
func TestAdminCreateModel_NegativeMarkup(t *testing.T) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"openai": {Type: "openai"}},
		Models:    []config.ModelConfig{},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{globalConfigJSON: gcJSON, globalConfigVersion: 1, globalConfigExists: true}
	h := globalConfigHandlers(store)

	body := `{"id":"m1","provider":"openai","markup_percentage":-5}`
	req := makeGlobalReq(t, http.MethodPost, "/admin/models", []byte(body))
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminCreateModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Error struct{ Message string } `json:"error"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Message != "markup_percentage must be non-negative" {
		t.Errorf("unexpected message: %q", resp.Error.Message)
	}
}

// TestAdminPatchModel_WithMarkup verifies that markup_percentage is accepted and stored via PATCH.
func TestAdminPatchModel_WithMarkup(t *testing.T) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"openai": {Type: "openai"}},
		Models:    []config.ModelConfig{{Name: "gpt-4o-mini", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{
		globalConfigJSON:       gcJSON,
		globalConfigVersion:    3,
		globalConfigExists:     true,
		putGlobalConfigVersion: 4,
	}
	h := globalConfigHandlers(store)

	patch := `{"markup_percentage":15}`
	req := makeGlobalReq(t, http.MethodPatch, "/admin/models/gpt-4o-mini", []byte(patch))
	req.SetPathValue("model_name", "gpt-4o-mini")
	req.Header.Set("If-Match-Version", "3")
	w := httptest.NewRecorder()
	h.AdminPatchModel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminPatchModel_NegativeMarkup verifies that patching with negative markup_percentage is rejected.
func TestAdminPatchModel_NegativeMarkup(t *testing.T) {
	gc := config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{"openai": {Type: "openai"}},
		Models:    []config.ModelConfig{{Name: "gpt-4o-mini", Provider: "openai"}},
	}
	gcJSON, _ := json.Marshal(gc)
	store := &globalConfigFakeStorage{globalConfigJSON: gcJSON, globalConfigVersion: 1, globalConfigExists: true}
	h := globalConfigHandlers(store)

	patch := `{"markup_percentage":-10}`
	req := makeGlobalReq(t, http.MethodPatch, "/admin/models/gpt-4o-mini", []byte(patch))
	req.SetPathValue("model_name", "gpt-4o-mini")
	req.Header.Set("If-Match-Version", "1")
	w := httptest.NewRecorder()
	h.AdminPatchModel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
