package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// routeGroupFakeStorage wraps fakeStorage with controllable GetTenantConfig and PatchTenantConfig.
type routeGroupFakeStorage struct {
	fakeStorage
	configJSON    json.RawMessage
	configVersion int
	configExists  bool
	patchErr      error
	patchCalled   bool
}

func (f *routeGroupFakeStorage) GetTenantConfig(ctx context.Context, tenantID string) (json.RawMessage, int, bool, error) {
	if !f.configExists {
		return nil, 0, false, nil
	}
	return f.configJSON, f.configVersion, true, nil
}

func (f *routeGroupFakeStorage) PatchTenantConfig(ctx context.Context, tenantID string, ifMatchVersion int, patch json.RawMessage, actorSub string, actorRoles []string) (int, error) {
	f.patchCalled = true
	if f.patchErr != nil {
		return 0, f.patchErr
	}
	return ifMatchVersion + 1, nil
}

func tenantWithRouteGroup() json.RawMessage {
	tc := config.TenantConfig{
		ID:            "t1",
		AllowedModels: []string{"gpt-4"},
		Selection: config.SelectionConfig{
			RouteGroups: map[string][]string{
				"cheap": {"gpt-4"},
				"math":  {"gpt-4"},
			},
		},
	}
	b, _ := json.Marshal(tc)
	return b
}

func TestAdminDeleteRouteGroup_Happy(t *testing.T) {
	store := &routeGroupFakeStorage{
		configJSON:    tenantWithRouteGroup(),
		configVersion: 3,
		configExists:  true,
	}
	h := &Handlers{
		cfg:   testCatalogConfig(),
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/t1/route-groups/cheap", nil)
	req.SetPathValue("tenant_id", "t1")
	req.SetPathValue("name", "cheap")

	w := httptest.NewRecorder()
	h.AdminDeleteRouteGroup(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if !store.patchCalled {
		t.Error("expected PatchTenantConfig to be called")
	}
}

func TestAdminDeleteRouteGroup_RouteGroupNotFound(t *testing.T) {
	store := &routeGroupFakeStorage{
		configJSON:   tenantWithRouteGroup(),
		configExists: true,
	}
	h := &Handlers{
		cfg:   testCatalogConfig(),
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/t1/route-groups/nonexistent", nil)
	req.SetPathValue("tenant_id", "t1")
	req.SetPathValue("name", "nonexistent")

	w := httptest.NewRecorder()
	h.AdminDeleteRouteGroup(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteRouteGroup_TenantNotFound(t *testing.T) {
	store := &routeGroupFakeStorage{configExists: false}
	h := &Handlers{
		cfg:   testCatalogConfig(),
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/ghost/route-groups/cheap", nil)
	req.SetPathValue("tenant_id", "ghost")
	req.SetPathValue("name", "cheap")

	w := httptest.NewRecorder()
	h.AdminDeleteRouteGroup(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteRouteGroup_VersionConflict(t *testing.T) {
	store := &routeGroupFakeStorage{
		configJSON:   tenantWithRouteGroup(),
		configExists: true,
		patchErr:     storage.ErrVersionConflict{Expected: 0, Current: 5},
	}
	h := &Handlers{
		cfg:   testCatalogConfig(),
		store: store,
		log:   testLoggerForAdmin(),
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/t1/route-groups/cheap", nil)
	req.SetPathValue("tenant_id", "t1")
	req.SetPathValue("name", "cheap")

	w := httptest.NewRecorder()
	h.AdminDeleteRouteGroup(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}
