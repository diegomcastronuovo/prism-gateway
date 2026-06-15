package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// routeTestConfig returns a Config with an embedding model and a basic tenant.
func routeTestConfig() *config.Config {
	cfg := testConfig()
	cfg.Models = append(cfg.Models, config.ModelConfig{
		Name:     "text-embedding-ada-002",
		Provider: "openai",
		Type:     "embedding",
	})
	cfg.Tenants[0].Routing.Semantic.EmbeddingModel = "text-embedding-ada-002"
	cfg.Tenants[0].AllowedModels = append(cfg.Tenants[0].AllowedModels, "text-embedding-ada-002")
	return cfg
}

// routeTestHandlers builds a minimal Handlers without spinning up a full server.
func routeTestHandlers(store *fakeStorage) *Handlers {
	cfg := routeTestConfig()
	reg := providers.NewRegistry()
	reg.RegisterEmbedding("openai", fakeEmbeddingProvider{vec: fixedVector()})
	return &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		registry:       reg,
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

// doCreateRoute fires POST /admin/semantic/routes with the given body against h.
func doCreateRoute(h *Handlers, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/semantic/routes?tenant_id=t1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.CreateSemanticRoute(rec, req)
	return rec
}

// ─── Admin CRUD handler tests ────────────────────────────────────────────────

func TestCreateSemanticRoute_Success(t *testing.T) {
	store := &fakeStorage{}
	h := routeTestHandlers(store)

	body := `{"name":"weather","action":"get_weather","utterances":["what is the weather?","is it raining?"]}`
	rec := doCreateRoute(h, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp semanticRouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "weather" {
		t.Errorf("name: want weather, got %q", resp.Name)
	}
	if resp.Action != "get_weather" {
		t.Errorf("action: want get_weather, got %q", resp.Action)
	}
	if resp.Threshold != 0.80 {
		t.Errorf("threshold: want 0.80 (default), got %f", resp.Threshold)
	}
}

func TestCreateSemanticRoute_MissingAction(t *testing.T) {
	store := &fakeStorage{}
	h := routeTestHandlers(store)

	body := `{"name":"weather","utterances":["what is the weather?"]}`
	rec := doCreateRoute(h, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSemanticRoute_NoUtterances(t *testing.T) {
	store := &fakeStorage{}
	h := routeTestHandlers(store)

	body := `{"name":"weather","action":"get_weather","utterances":[]}`
	rec := doCreateRoute(h, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSemanticRoute_Conflict(t *testing.T) {
	store := &fakeStorage{semanticRouteCreateErr: storage.ErrRouteAlreadyExists}
	h := routeTestHandlers(store)

	body := `{"name":"weather","action":"get_weather","utterances":["what is the weather?"]}`
	rec := doCreateRoute(h, body)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── Dynamic route matching in ChatCompletions ────────────────────────────────

// dynamicRouteRegistry builds a registry with both chat and embedding providers.
func dynamicRouteRegistry() *providers.Registry {
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	reg.Register("backup", successProvider(""))
	reg.RegisterEmbedding("openai", fakeEmbeddingProvider{vec: fixedVector()})
	return reg
}

func TestDynamicRoute_MatchAboveThreshold(t *testing.T) {
	// Tool routing stage fires first, so a matching route returns model="tool-route".
	cfg := routeTestConfig()
	match := &storage.SemanticRouteMatch{
		Name:       "weather",
		Action:     "get_weather",
		Similarity: 0.90,
		Threshold:  0.80,
	}
	store := &fakeStorage{semanticRouteMatch: match}
	reg := dynamicRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"what is the weather?"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Tool routing fires first and sets X-Tool-Route.
	if got := w.Header().Get("X-Tool-Route"); got != "weather" {
		t.Errorf("X-Tool-Route: want weather, got %q", got)
	}
	if got := w.Header().Get("X-Tool-Action"); got != "get_weather" {
		t.Errorf("X-Tool-Action: want get_weather, got %q", got)
	}
	if got := w.Header().Get("X-Tool-Route-Similarity"); got == "" {
		t.Error("X-Tool-Route-Similarity header missing")
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Model != "tool-route" {
		t.Errorf("model: want tool-route, got %q", resp.Model)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
}

func TestDynamicRoute_BelowThreshold(t *testing.T) {
	cfg := routeTestConfig()
	// Similarity below threshold → router should proceed normally
	match := &storage.SemanticRouteMatch{
		Name:       "weather",
		Action:     "get_weather",
		Similarity: 0.50,
		Threshold:  0.80,
	}
	store := &fakeStorage{semanticRouteMatch: match}
	reg := dynamicRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (normal routing), got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Dynamic-Route"); got != "" {
		t.Errorf("X-Dynamic-Route should be absent on below-threshold, got %q", got)
	}
}

func TestDynamicRoute_NoRoutes(t *testing.T) {
	cfg := routeTestConfig()
	store := &fakeStorage{} // semanticRouteMatch = nil → no routes
	reg := dynamicRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (normal routing), got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Dynamic-Route"); got != "" {
		t.Errorf("X-Dynamic-Route should be absent when no routes, got %q", got)
	}
}

// ─── PATCH handler tests ──────────────────────────────────────────────────────

func doPatchRoute(h *Handlers, name, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/admin/semantic/routes/"+name+"?tenant_id=t1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", name)
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.PatchSemanticRoute(rec, req)
	return rec
}

func existingRoute() *storage.SemanticRouteDetail {
	return &storage.SemanticRouteDetail{
		Name:        "weather",
		Description: "weather queries",
		Action:      "get_weather",
		Threshold:   0.80,
		Utterances:  []string{"what is the weather?", "is it raining?"},
	}
}

func TestPatchSemanticRoute_ThresholdOnly(t *testing.T) {
	store := &fakeStorage{routeGetResult: existingRoute()}
	h := routeTestHandlers(store)

	rec := doPatchRoute(h, "weather", `{"threshold":0.90}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patchSemanticRouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Threshold != 0.90 {
		t.Errorf("threshold: want 0.90, got %f", resp.Threshold)
	}
	if resp.Action != "get_weather" {
		t.Errorf("action preserved: want get_weather, got %q", resp.Action)
	}
}

func TestPatchSemanticRoute_ActionOnly(t *testing.T) {
	store := &fakeStorage{routeGetResult: existingRoute()}
	h := routeTestHandlers(store)

	rec := doPatchRoute(h, "weather", `{"action":"get_weather_v2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patchSemanticRouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Action != "get_weather_v2" {
		t.Errorf("action: want get_weather_v2, got %q", resp.Action)
	}
	if resp.Threshold != 0.80 {
		t.Errorf("threshold preserved: want 0.80, got %f", resp.Threshold)
	}
}

func TestPatchSemanticRoute_UtterancesOnly(t *testing.T) {
	store := &fakeStorage{routeGetResult: existingRoute()}
	h := routeTestHandlers(store)

	body := `{"utterances":["está lloviendo hoy","va a llover hoy"]}`
	rec := doPatchRoute(h, "weather", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patchSemanticRouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Utterances) != 2 {
		t.Errorf("utterances: want 2, got %d", len(resp.Utterances))
	}
	if resp.Action != "get_weather" {
		t.Errorf("action preserved: want get_weather, got %q", resp.Action)
	}
}

func TestPatchSemanticRoute_FullUpdate(t *testing.T) {
	store := &fakeStorage{routeGetResult: existingRoute()}
	h := routeTestHandlers(store)

	body := `{"description":"rain queries","action":"get_weather_v2","threshold":0.75,"utterances":["va a llover hoy","qué clima hay"]}`
	rec := doPatchRoute(h, "weather", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp patchSemanticRouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Description != "rain queries" {
		t.Errorf("description: want 'rain queries', got %q", resp.Description)
	}
	if resp.Action != "get_weather_v2" {
		t.Errorf("action: want get_weather_v2, got %q", resp.Action)
	}
	if resp.Threshold != 0.75 {
		t.Errorf("threshold: want 0.75, got %f", resp.Threshold)
	}
	if len(resp.Utterances) != 2 {
		t.Errorf("utterances: want 2, got %d", len(resp.Utterances))
	}
	if resp.TenantID != "t1" {
		t.Errorf("tenant_id: want t1, got %q", resp.TenantID)
	}
}

func TestPatchSemanticRoute_NotFound(t *testing.T) {
	store := &fakeStorage{} // routeGetResult=nil → not found
	h := routeTestHandlers(store)

	rec := doPatchRoute(h, "nonexistent", `{"threshold":0.90}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSemanticRoute_InvalidThreshold(t *testing.T) {
	store := &fakeStorage{routeGetResult: existingRoute()}
	h := routeTestHandlers(store)

	rec := doPatchRoute(h, "weather", `{"threshold":1.5}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSemanticRoute_InvalidUtterances(t *testing.T) {
	store := &fakeStorage{routeGetResult: existingRoute()}
	h := routeTestHandlers(store)

	rec := doPatchRoute(h, "weather", `{"utterances":["   ","  "]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSemanticRoute_NoExplicitTenantID(t *testing.T) {
	store := &fakeStorage{routeGetResult: existingRoute()}
	h := routeTestHandlers(store)

	// No tenant_id query param — API key auth context provides the tenant.
	req := httptest.NewRequest(http.MethodPatch, "/admin/semantic/routes/weather", strings.NewReader(`{"threshold":0.85}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "weather")
	ctx := auth.WithContextTenantID(req.Context(), "t1")
	ctx = auth.WithAuthType(ctx, "api_key")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.PatchSemanticRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 via auth context tenant, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDynamicRoute_DBError_FailOpen(t *testing.T) {
	cfg := routeTestConfig()
	store := &fakeStorage{semanticRouteErr: errors.New("db down")}
	reg := dynamicRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	// Fail open: routing continues normally
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open), got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Dynamic-Route"); got != "" {
		t.Errorf("X-Dynamic-Route should be absent on DB error, got %q", got)
	}
}
