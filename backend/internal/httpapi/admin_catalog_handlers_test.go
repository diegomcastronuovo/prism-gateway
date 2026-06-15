package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// testCatalogConfig builds a rich config for catalog handler tests.
func testCatalogConfig() *config.Config {
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai":    {Type: "openai", BaseURL: "https://api.openai.com/v1", APIKeyEnv: "TEST_OPENAI_API_KEY"},
			"anthropic": {Type: "anthropic", BaseURL: "https://api.anthropic.com", APIKeyEnv: "TEST_ANTHROPIC_API_KEY"},
			"google":    {Type: "google", BaseURL: "https://generativelanguage.googleapis.com", APIKeyEnv: "TEST_GOOGLE_API_KEY"},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
			{Name: "claude-3-5-sonnet", Provider: "anthropic"},
			{Name: "gemini-2.5-flash", Provider: "google"},
		},
		Tenants: []config.TenantConfig{
			{
				ID:            "t1",
				AllowedModels: []string{"gpt-4o-mini", "gemini-2.5-flash"},
				Routing:       config.RoutingConfig{Strategy: "smart"},
				Selection: config.SelectionConfig{
					RouteGroups: map[string][]string{
						"cheap": {"gpt-4o-mini", "gemini-2.5-flash"},
						"math":  {"gemini-2.5-flash"},
					},
				},
				SemanticCache:     config.SemanticCacheConfig{Enabled: true},
				BudgetEnforcement: config.BudgetEnforcementConfig{Enabled: true},
			},
			{
				ID:            "t2",
				AllowedModels: []string{"claude-3-5-sonnet"},
				Selection: config.SelectionConfig{
					RouteGroups: map[string][]string{
						"premium": {"claude-3-5-sonnet"},
					},
				},
			},
		},
	}
}

// listTenantsResult decodes the data array and returns the tenant_id values.
func listTenantsResult(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var resp struct {
		Object string              `json:"object"`
		Data   []catalogTenantItem `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object=%q, want list", resp.Object)
	}
	ids := make([]string, len(resp.Data))
	for i, item := range resp.Data {
		ids[i] = item.TenantID
	}
	return ids
}

func TestAdminListTenants_YAMLOnly(t *testing.T) {
	// SPEC_147: tenants come from DB only. Bootstrap seeds YAML tenants to DB at startup,
	// so by runtime they appear in store.ListTenants().
	cfg := testCatalogConfig()
	store := &fakeStorage{listTenantsResult: []string{"t1", "t2"}}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	h.AdminListTenants(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ids := listTenantsResult(t, w)
	if len(ids) != 2 {
		t.Fatalf("expected 2 tenants (from DB), got %d: %v", len(ids), ids)
	}
	if ids[0] != "t1" || ids[1] != "t2" {
		t.Errorf("tenants=%v, want [t1 t2]", ids)
	}
}

func TestAdminListTenants_DBOnly(t *testing.T) {
	cfg := &config.Config{} // no YAML tenants
	store := &fakeStorage{listTenantsResult: []string{"db-tenant-1", "db-tenant-2"}}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	h.AdminListTenants(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ids := listTenantsResult(t, w)
	if len(ids) != 2 {
		t.Fatalf("expected 2 tenants, got %d: %v", len(ids), ids)
	}
	if ids[0] != "db-tenant-1" || ids[1] != "db-tenant-2" {
		t.Errorf("tenants=%v, want [db-tenant-1 db-tenant-2]", ids)
	}
}

func TestAdminListTenants_MergedAndSorted(t *testing.T) {
	cfg := testCatalogConfig()
	// SPEC_147: DB is single source of truth — only DB tenants are returned.
	store := &fakeStorage{listTenantsResult: []string{"t2", "t3"}}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	h.AdminListTenants(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ids := listTenantsResult(t, w)
	// Only DB tenants: t2, t3 (YAML-only t1 is not returned at runtime).
	if len(ids) != 2 {
		t.Fatalf("expected 2 DB tenants, got %d: %v", len(ids), ids)
	}
	want := []string{"t2", "t3"}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("tenants[%d]=%q, want %q", i, ids[i], w)
		}
	}
}

func TestAdminListTenants_DBErrorFailOpen(t *testing.T) {
	cfg := testCatalogConfig()
	store := &fakeStorage{listTenantsErr: fmt.Errorf("connection refused")}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	w := httptest.NewRecorder()
	h.AdminListTenants(w, req)

	// SPEC_147: DB error → empty list (no YAML fallback). Still returns 200.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open), got %d", w.Code)
	}
	ids := listTenantsResult(t, w)
	if len(ids) != 0 {
		t.Fatalf("expected empty list on DB error (SPEC_147: no YAML fallback), got %d: %v", len(ids), ids)
	}
}

func TestAdminListTenants_LocalAdminFilteredToAllowed(t *testing.T) {
	cfg := testCatalogConfig() // t1, t2 YAML
	store := &fakeStorage{listTenantsResult: []string{"t2", "t3"}}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	ctx := auth.WithJWTAdminContext(req.Context(), "t2", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"t2"})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.AdminListTenants(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ids := listTenantsResult(t, w)
	if len(ids) != 1 || ids[0] != "t2" {
		t.Fatalf("expected only allowed tenant t2, got %v", ids)
	}
}

func TestAdminListTenants_AdminRoleNotFilteredByAllowed(t *testing.T) {
	cfg := testCatalogConfig()
	// SPEC_147: admin sees exactly what DB has — no YAML supplement.
	store := &fakeStorage{listTenantsResult: []string{"t1", "t2", "t3"}}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	ctx := auth.WithJWTAdminContext(req.Context(), "t1", "admin-user", []string{"admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"t1"}) // admin role bypasses allowlist filter
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.AdminListTenants(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ids := listTenantsResult(t, w)
	want := []string{"t1", "t2", "t3"}
	if len(ids) != len(want) {
		t.Fatalf("expected %d tenants (full DB list), got %d: %v", len(want), len(ids), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ids[%d]=%q, want %q", i, ids[i], want[i])
		}
	}
}

func tenantConfigsMapFromConfig(cfg *config.Config) map[string]json.RawMessage {
	m := make(map[string]json.RawMessage)
	for _, t := range cfg.Tenants {
		b, _ := json.Marshal(t)
		m[t.ID] = b
	}
	return m
}

func TestAdminListModels(t *testing.T) {
	cfg := testCatalogConfig()
	// SPEC_147: route groups come from DB tenant configs.
	store := &fakeStorage{
		listTenantsResult: []string{"t1", "t2"},
		tenantConfigsMap:  tenantConfigsMapFromConfig(cfg),
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	w := httptest.NewRecorder()
	h.AdminListModels(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Object string             `json:"object"`
		Data   []catalogModelItem `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object=%q, want list", resp.Object)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 models, got %d", len(resp.Data))
	}

	byID := make(map[string]catalogModelItem)
	for _, m := range resp.Data {
		byID[m.ID] = m
	}

	mini, ok := byID["gpt-4o-mini"]
	if !ok {
		t.Fatal("gpt-4o-mini missing from response")
	}
	if mini.Provider != "openai" {
		t.Errorf("gpt-4o-mini provider=%q, want openai", mini.Provider)
	}
	if len(mini.RouteGroups) != 1 || mini.RouteGroups[0] != "cheap" {
		t.Errorf("gpt-4o-mini route_groups=%v, want [cheap]", mini.RouteGroups)
	}

	gemini, ok := byID["gemini-2.5-flash"]
	if !ok {
		t.Fatal("gemini-2.5-flash missing from response")
	}
	if len(gemini.RouteGroups) != 2 {
		t.Errorf("gemini-2.5-flash route_groups=%v, want [cheap math]", gemini.RouteGroups)
	}
	// Route groups must be sorted.
	if gemini.RouteGroups[0] != "cheap" || gemini.RouteGroups[1] != "math" {
		t.Errorf("gemini-2.5-flash route_groups=%v, want [cheap math]", gemini.RouteGroups)
	}

	claude, ok := byID["claude-3-5-sonnet"]
	if !ok {
		t.Fatal("claude-3-5-sonnet missing from response")
	}
	if claude.Provider != "anthropic" {
		t.Errorf("claude-3-5-sonnet provider=%q, want anthropic", claude.Provider)
	}
	if len(claude.RouteGroups) != 1 || claude.RouteGroups[0] != "premium" {
		t.Errorf("claude-3-5-sonnet route_groups=%v, want [premium]", claude.RouteGroups)
	}
	// Enriched response: each item must have mock block (backward compat: id, provider, route_groups kept).
	for _, m := range resp.Data {
		if m.Mock == nil {
			t.Errorf("model %q missing mock block", m.ID)
		}
	}
}

func TestAdminListModels_NoRouteGroups(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
		Tenants: []config.TenantConfig{
			{ID: "t1", AllowedModels: []string{"gpt-4o-mini"}},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	w := httptest.NewRecorder()
	h.AdminListModels(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []catalogModelItem `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(resp.Data))
	}
	if len(resp.Data[0].RouteGroups) != 0 {
		t.Errorf("expected empty route_groups, got %v", resp.Data[0].RouteGroups)
	}
}

func TestAdminListProviders(t *testing.T) {
	cfg := testCatalogConfig() // has openai, anthropic, google in Providers map
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	w := httptest.NewRecorder()
	h.AdminListProviders(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Object string                `json:"object"`
		Data   []providerRuntimeItem `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object=%q, want list", resp.Object)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(resp.Data))
	}
	// Must be sorted alphabetically.
	if resp.Data[0].ID != "anthropic" {
		t.Errorf("providers[0]=%q, want anthropic", resp.Data[0].ID)
	}
	if resp.Data[1].ID != "google" {
		t.Errorf("providers[1]=%q, want google", resp.Data[1].ID)
	}
	if resp.Data[2].ID != "openai" {
		t.Errorf("providers[2]=%q, want openai", resp.Data[2].ID)
	}
	// All have enriched fields from cfg.Providers.
	for _, p := range resp.Data {
		if p.BaseURL == "" {
			t.Errorf("provider %q: base_url should be set", p.ID)
		}
		if p.Type == "" {
			t.Errorf("provider %q: type should be set", p.ID)
		}
		if !p.Enabled {
			t.Errorf("provider %q: enabled should be true", p.ID)
		}
		// No env vars set in this test → missing_credentials.
		if p.Status != "missing_credentials" {
			t.Errorf("provider %q: status=%q, want missing_credentials", p.ID, p.Status)
		}
		if p.APIKeySource != "missing" {
			t.Errorf("provider %q: api_key_source=%q, want missing", p.ID, p.APIKeySource)
		}
	}
}

// TestAdminListProviders_ModelFallback verifies that a provider referenced in
// cfg.Models but absent from cfg.Providers is still listed (backward compat).
func TestAdminListProviders_ModelFallback(t *testing.T) {
	cfg := &config.Config{
		// No cfg.Providers
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
			{Name: "gpt-4o", Provider: "openai"},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	w := httptest.NewRecorder()
	h.AdminListProviders(w, req)

	var resp struct {
		Data []providerRuntimeItem `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	// Two models with the same provider → one deduplicated entry.
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 provider (deduplicated), got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "openai" {
		t.Errorf("provider=%q, want openai", resp.Data[0].ID)
	}
	// No ProviderConfig → minimal metadata defaults.
	if !resp.Data[0].Enabled {
		t.Errorf("fallback provider should be enabled by default")
	}
}

// TestAdminListProviders_EnvCredential verifies env-var credential detection.
func TestAdminListProviders_EnvCredential(t *testing.T) {
	const envVar = "TEST_PROV_RUNTIME_OPENAI_KEY"
	t.Setenv(envVar, "sk-test-key")

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai", BaseURL: "https://api.openai.com/v1", APIKeyEnv: envVar},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	w := httptest.NewRecorder()
	h.AdminListProviders(w, req)

	var resp struct {
		Data []providerRuntimeItem `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data))
	}
	p := resp.Data[0]
	if !p.HasAPIKey {
		t.Error("has_api_key should be true when env var is set")
	}
	if p.APIKeySource != "env" {
		t.Errorf("api_key_source=%q, want env", p.APIKeySource)
	}
	if p.Status != "ready" {
		t.Errorf("status=%q, want ready", p.Status)
	}
}

// TestAdminListProviders_MissingCredential verifies the missing-key path.
func TestAdminListProviders_MissingCredential(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"xai": {Type: "xai", BaseURL: "https://api.x.ai/v1", APIKeyEnv: "XAI_KEY_THAT_DOES_NOT_EXIST_IN_TEST"},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	w := httptest.NewRecorder()
	h.AdminListProviders(w, req)

	var resp struct {
		Data []providerRuntimeItem `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data))
	}
	p := resp.Data[0]
	if p.HasAPIKey {
		t.Error("has_api_key should be false when env var is unset")
	}
	if p.APIKeySource != "missing" {
		t.Errorf("api_key_source=%q, want missing", p.APIKeySource)
	}
	if p.Status != "missing_credentials" {
		t.Errorf("status=%q, want missing_credentials", p.Status)
	}
}

// TestAdminListProviders_StoredCredential verifies that a key stored directly in
// ProviderConfig.APIKey is detected with api_key_source="stored".
func TestAdminListProviders_StoredCredential(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				Type:      "anthropic",
				BaseURL:   "https://api.anthropic.com",
				APIKeyEnv: "ANTHROPIC_KEY_NOT_IN_ENV",
				APIKey:    "sk-ant-stored-key",
			},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	w := httptest.NewRecorder()
	h.AdminListProviders(w, req)

	var resp struct {
		Data []providerRuntimeItem `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data))
	}
	p := resp.Data[0]
	if !p.HasAPIKey {
		t.Error("has_api_key should be true when APIKey is stored in config")
	}
	if p.APIKeySource != "stored" {
		t.Errorf("api_key_source=%q, want stored", p.APIKeySource)
	}
	if p.Status != "ready" {
		t.Errorf("status=%q, want ready", p.Status)
	}
}

// TestAdminListProviders_DisabledProvider verifies the disabled status path.
func TestAdminListProviders_DisabledProvider(t *testing.T) {
	disabled := false
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"local": {Type: "local", BaseURL: "http://localhost:9000", Enabled: &disabled},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	w := httptest.NewRecorder()
	h.AdminListProviders(w, req)

	var resp struct {
		Data []providerRuntimeItem `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data))
	}
	p := resp.Data[0]
	if p.Enabled {
		t.Error("enabled should be false for explicitly disabled provider")
	}
	if p.Status != "disabled" {
		t.Errorf("status=%q, want disabled", p.Status)
	}
}

// TestAdminListProviders_NoSecretExposed verifies that raw API keys never appear in the response body.
func TestAdminListProviders_NoSecretExposed(t *testing.T) {
	const secret = "sk-super-secret-key-that-must-not-appear"
	t.Setenv("TEST_PROV_SECRET_ENV", secret)
	t.Cleanup(func() { os.Unsetenv("TEST_PROV_SECRET_ENV") })

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {
				Type:      "openai",
				BaseURL:   "https://api.openai.com/v1",
				APIKeyEnv: "TEST_PROV_SECRET_ENV",
				APIKey:    "stored-" + secret,
			},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/providers", nil)
	w := httptest.NewRecorder()
	h.AdminListProviders(w, req)

	body := w.Body.String()
	if strings.Contains(body, secret) {
		t.Errorf("response body contains plaintext secret — must never expose credentials")
	}
}

func TestAdminListRouteGroups(t *testing.T) {
	cfg := testCatalogConfig()
	store := &fakeStorage{
		listTenantsResult: []string{"t1", "t2"},
		tenantConfigsMap:  tenantConfigsMapFromConfig(cfg),
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/route-groups", nil)
	w := httptest.NewRecorder()
	h.AdminListRouteGroups(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Object string          `json:"object"`
		Data   []catalogIDItem `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object=%q, want list", resp.Object)
	}
	// cheap (t1), math (t1), premium (t2) — 3 unique groups, sorted.
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 route groups, got %d: %v", len(resp.Data), resp.Data)
	}
	if resp.Data[0].ID != "cheap" {
		t.Errorf("groups[0]=%q, want cheap", resp.Data[0].ID)
	}
	if resp.Data[1].ID != "math" {
		t.Errorf("groups[1]=%q, want math", resp.Data[1].ID)
	}
	if resp.Data[2].ID != "premium" {
		t.Errorf("groups[2]=%q, want premium", resp.Data[2].ID)
	}
}

func TestAdminListFeatures_AllEnabled(t *testing.T) {
	cfg := testCatalogConfig()
	cfg.DynamicConfig.Enabled = true
	store := &fakeStorage{
		listTenantsResult: []string{"t1", "t2"},
		tenantConfigsMap:  tenantConfigsMapFromConfig(cfg),
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/features", nil)
	w := httptest.NewRecorder()
	h.AdminListFeatures(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, flag := range []string{"semantic_routing", "semantic_cache", "budget_enforcement", "dynamic_routes"} {
		if !resp[flag] {
			t.Errorf("feature %q should be true", flag)
		}
	}
}

func TestAdminListFeatures_NoneEnabled(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: "t1", Routing: config.RoutingConfig{Strategy: "round_robin"}},
		},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/features", nil)
	w := httptest.NewRecorder()
	h.AdminListFeatures(w, req)

	var resp map[string]bool
	json.NewDecoder(w.Body).Decode(&resp)

	for _, flag := range []string{"semantic_routing", "semantic_cache", "budget_enforcement", "dynamic_routes"} {
		if resp[flag] {
			t.Errorf("feature %q should be false", flag)
		}
	}
}

func TestAdminCatalog_RequiresAuth(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testCatalogConfig()
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin(), store: &storage.NopStorage{}}

	endpoints := []struct {
		method string
		path   string
		fn     http.HandlerFunc
	}{
		{http.MethodGet, "/admin/tenants", h.AdminListTenants},
		{http.MethodGet, "/admin/models", h.AdminListModels},
		{http.MethodGet, "/admin/providers", h.AdminListProviders},
		{http.MethodGet, "/admin/route-groups", h.AdminListRouteGroups},
		{http.MethodGet, "/admin/features", h.AdminListFeatures},
	}

	cache := config.NewTenantConfigCache(1 * time.Second)
	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			// No X-Admin-Token header → expect 401.
			w := httptest.NewRecorder()
			AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(ep.fn).ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", w.Code)
			}
		})
	}
}

// ── markup_percentage catalog tests ──────────────────────────────────────────

// TestAdminListModels_MarkupExposed verifies that markup_percentage is returned by GET /admin/models.
func TestAdminListModels_MarkupExposed(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai", MarkupPercentage: 20},
			{Name: "no-markup-model", Provider: "openai"},
		},
		Tenants: []config.TenantConfig{{ID: "t1"}},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	w := httptest.NewRecorder()
	h.AdminListModels(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []catalogModelItem `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	byID := make(map[string]catalogModelItem)
	for _, m := range resp.Data {
		byID[m.ID] = m
	}

	if byID["gpt-4o-mini"].MarkupPercentage != 20 {
		t.Errorf("gpt-4o-mini markup_percentage=%v, want 20", byID["gpt-4o-mini"].MarkupPercentage)
	}
	if byID["no-markup-model"].MarkupPercentage != 0 {
		t.Errorf("no-markup-model markup_percentage=%v, want 0", byID["no-markup-model"].MarkupPercentage)
	}
}

// TestAdminListModels_ZeroMarkupDefault verifies that a model with no markup_percentage
// defaults to 0 (backward-compatible).
func TestAdminListModels_ZeroMarkupDefault(t *testing.T) {
	cfg := &config.Config{
		Models:  []config.ModelConfig{{Name: "gpt-4o-mini", Provider: "openai"}},
		Tenants: []config.TenantConfig{{ID: "t1"}},
	}
	h := &Handlers{cfg: cfg, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	w := httptest.NewRecorder()
	h.AdminListModels(w, req)

	var resp struct {
		Data []catalogModelItem `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(resp.Data))
	}
	if resp.Data[0].MarkupPercentage != 0 {
		t.Errorf("want markup_percentage=0, got %v", resp.Data[0].MarkupPercentage)
	}
}
