package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// patchDeleteFakeStorage extends fakeStorage with controllable patch/delete behavior.
type patchDeleteFakeStorage struct {
	fakeStorage
	updateFound bool
	updateErr   error
	deleteFound bool
	deleteErr   error

	// capture last patch to assert fields
	lastPatch storage.SemanticAnchorPatch
}

func (s *patchDeleteFakeStorage) UpdateSemanticAnchor(_ context.Context, _, _ string, patch storage.SemanticAnchorPatch) (bool, error) {
	s.lastPatch = patch
	return s.updateFound, s.updateErr
}

func (s *patchDeleteFakeStorage) DeleteSemanticAnchor(_ context.Context, _, _ string) (bool, error) {
	return s.deleteFound, s.deleteErr
}

func patchDeleteHandlers(store *patchDeleteFakeStorage) *Handlers {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{
			ID: "t1",
			Routing: config.RoutingConfig{
				Semantic: config.SemanticRoutingConfig{EmbeddingModel: "embed-mock"},
			},
		}},
		Models: []config.ModelConfig{
			{Name: "embed-mock", Provider: "mock", Type: "embedding",
				Mock: config.MockConfig{Enabled: true}},
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

func patchAnchor(h *Handlers, name, body string) *httptest.ResponseRecorder {
	url := "/v1/semantic/anchors/" + name + "?tenant_id=t1"
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if name != "" {
		req.SetPathValue("name", name)
	}
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.PatchSemanticAnchor(rec, req)
	return rec
}

func deleteAnchor(h *Handlers, name string) *httptest.ResponseRecorder {
	url := "/v1/semantic/anchors/" + name + "?tenant_id=t1"
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	if name != "" {
		req.SetPathValue("name", name)
	}
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.DeleteSemanticAnchor(rec, req)
	return rec
}

// ── PATCH tests ───────────────────────────────────────────────────────────────

func TestPatchSemanticAnchor_UpdateRouteGroup(t *testing.T) {
	store := &patchDeleteFakeStorage{updateFound: true}
	h := patchDeleteHandlers(store)
	rec := patchAnchor(h, "my-anchor", `{"route_group":"premium"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %q", resp["status"])
	}
	if resp["name"] != "my-anchor" {
		t.Errorf("expected name=my-anchor, got %q", resp["name"])
	}
	if store.lastPatch.RouteGroup == nil || *store.lastPatch.RouteGroup != "premium" {
		t.Errorf("expected patch.RouteGroup=premium, got %v", store.lastPatch.RouteGroup)
	}
}

func TestPatchSemanticAnchor_NotFound(t *testing.T) {
	store := &patchDeleteFakeStorage{updateFound: false}
	h := patchDeleteHandlers(store)
	rec := patchAnchor(h, "no-such-anchor", `{"route_group":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSemanticAnchor_EmptyBody(t *testing.T) {
	store := &patchDeleteFakeStorage{updateFound: true}
	h := patchDeleteHandlers(store)
	rec := patchAnchor(h, "my-anchor", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty patch body, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSemanticAnchor_EmptyPreferredModels(t *testing.T) {
	store := &patchDeleteFakeStorage{updateFound: true}
	h := patchDeleteHandlers(store)
	rec := patchAnchor(h, "my-anchor", `{"preferred_models":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty preferred_models, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSemanticAnchor_WithAnchorText_ReembedAndStore(t *testing.T) {
	store := &patchDeleteFakeStorage{updateFound: true}
	h := patchDeleteHandlers(store)
	// anchor_text triggers re-embedding via mock provider
	rec := patchAnchor(h, "my-anchor", `{"anchor_text":"new text"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// The patch should have set an embedding vector
	if store.lastPatch.Embedding == nil {
		t.Error("expected patch.Embedding to be set when anchor_text is non-empty")
	}
}

func TestPatchSemanticAnchor_NoTenant_Returns400(t *testing.T) {
	store := &patchDeleteFakeStorage{}
	h := patchDeleteHandlers(store)
	req := httptest.NewRequest(http.MethodPatch, "/v1/semantic/anchors/x",
		strings.NewReader(`{"route_group":"y"}`))
	req.SetPathValue("name", "x")
	rec := httptest.NewRecorder()
	h.PatchSemanticAnchor(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ── DELETE tests ──────────────────────────────────────────────────────────────

func TestDeleteSemanticAnchor_Success(t *testing.T) {
	store := &patchDeleteFakeStorage{deleteFound: true}
	h := patchDeleteHandlers(store)
	rec := deleteAnchor(h, "my-anchor")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %q", resp["status"])
	}
	if resp["name"] != "my-anchor" {
		t.Errorf("expected name=my-anchor, got %q", resp["name"])
	}
}

func TestDeleteSemanticAnchor_NotFound(t *testing.T) {
	store := &patchDeleteFakeStorage{deleteFound: false}
	h := patchDeleteHandlers(store)
	rec := deleteAnchor(h, "no-such")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteSemanticAnchor_EmptyName(t *testing.T) {
	store := &patchDeleteFakeStorage{}
	h := patchDeleteHandlers(store)
	// Do NOT set path value — simulates missing {name}
	req := httptest.NewRequest(http.MethodDelete, "/v1/semantic/anchors/", nil)
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.DeleteSemanticAnchor(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteSemanticAnchor_NoTenant_Returns400(t *testing.T) {
	store := &patchDeleteFakeStorage{}
	h := patchDeleteHandlers(store)
	req := httptest.NewRequest(http.MethodDelete, "/v1/semantic/anchors/x", nil)
	req.SetPathValue("name", "x")
	rec := httptest.NewRecorder()
	h.DeleteSemanticAnchor(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
