package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

func adminSemanticTestHandlers(store *similarityFakeStorage) *Handlers {
	cfg := &config.Config{
		// Tenant explicitly sets routing.semantic.embedding_model (Phase 2 strict requirement).
		Tenants: []config.TenantConfig{{
			ID: "tenant_a",
			Routing: config.RoutingConfig{
				Semantic: config.SemanticRoutingConfig{EmbeddingModel: "embed-mock"},
			},
		}},
		Models: []config.ModelConfig{{
			Name:     "embed-mock",
			Provider: "mock",
			Type:     "embedding",
			Mock:     config.MockConfig{Enabled: true},
		}},
	}
	return &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

func TestAdminSemanticTest_AccessByRole(t *testing.T) {
	roles := []struct {
		role string
		want int
	}{
		{role: "admin", want: http.StatusBadRequest},
		{role: "local_admin", want: http.StatusBadRequest},
		{role: "audit", want: http.StatusForbidden},
		{role: "user", want: http.StatusForbidden},
		{role: "financial", want: http.StatusForbidden},
	}
	for _, role := range roles {
		t.Run(role.role, func(t *testing.T) {
			h := adminSemanticTestHandlers(&similarityFakeStorage{})
			req := httptest.NewRequest(http.MethodPost, "/admin/semantic/test?tenant_id=tenant_a", strings.NewReader(`{"text":"hello"}`))
			req.Header.Set("Content-Type", "application/json")
			ctx := auth.WithJWTAdminContext(req.Context(), "tenant_a", "sub", []string{role.role})
			if role.role == "local_admin" {
				ctx = auth.WithAllowedTenants(ctx, []string{"tenant_a"})
			}
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			h.AdminSemanticTest(rec, req)
			if rec.Code != role.want {
				t.Fatalf("expected %d for role %s, got %d", role.want, role.role, rec.Code)
			}
		})
	}
}

func TestAdminSemanticTest_NoAnchors(t *testing.T) {
	h := adminSemanticTestHandlers(&similarityFakeStorage{})
	req := httptest.NewRequest(http.MethodPost, "/admin/semantic/test?tenant_id=tenant_a", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "tenant_a", "sub", []string{"admin"}))
	rec := httptest.NewRecorder()
	h.AdminSemanticTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminSemanticTest_NoMatch(t *testing.T) {
	store := &similarityFakeStorage{
		rows: []storage.SemanticAnchorRow{{
			Name:       "finance",
			RouteGroup: "finance",
			Distance:   0.9,
			VectorDims: 4,
			Modality:   "text",
		}},
	}
	h := adminSemanticTestHandlers(store)
	req := httptest.NewRequest(http.MethodPost, "/admin/semantic/test?tenant_id=tenant_a", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "tenant_a", "sub", []string{"admin"}))
	rec := httptest.NewRecorder()
	h.AdminSemanticTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["top_match"] != nil {
		t.Fatalf("expected top_match nil, got %v", resp["top_match"])
	}
}

func TestAdminSemanticTest_Match(t *testing.T) {
	store := &similarityFakeStorage{
		rows: []storage.SemanticAnchorRow{{
			Name:       "finance",
			RouteGroup: "finance",
			Distance:   0.2,
			VectorDims: 4,
			Modality:   "text",
		}},
	}
	h := adminSemanticTestHandlers(store)
	req := httptest.NewRequest(http.MethodPost, "/admin/semantic/test?tenant_id=tenant_a", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "tenant_a", "sub", []string{"admin"}))
	rec := httptest.NewRecorder()
	h.AdminSemanticTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["top_match"] == nil {
		t.Fatalf("expected top_match, got nil")
	}
}
