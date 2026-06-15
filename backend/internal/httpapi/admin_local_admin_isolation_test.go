package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// Regression: AdminMiddleware must use auth package typed context keys so
// AdminTenantIsolationMiddleware and TenantIsolationMiddleware see roles/auth_type.

func TestLocalAdmin_AdminTenantIsolation_BlocksOtherTenant(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/config", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run for local_admin accessing disallowed tenant")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdmin_AdminTenantIsolation_AllowsGlobalTenantList(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !innerHit {
		t.Fatal("handler must run for local_admin GET /admin/tenants")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdmin_AdminTenantIsolation_AllowsModelsListGET(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !innerHit {
		t.Fatal("handler must run for local_admin GET /admin/models (catalog for tenant config)")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdmin_AdminTenantIsolation_AllowsConfigHistoryGET(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/config/history", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !innerHit {
		t.Fatal("handler must run for local_admin GET /admin/config/history")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdmin_AdminTenantIsolation_BlocksJWTSubUsageGET(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/usage", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run for local_admin on jwt_sub usage path")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdmin_AdminTenantIsolation_BlocksGlobalTenantCreate(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodPost, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run for local_admin POST /admin/tenants")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdmin_TenantIsolationMiddleware_BlocksOtherTenant(t *testing.T) {
	cfg := &config.Config{}
	log := testLoggerForAdmin()
	mw := auth.TenantIsolationMiddleware(cfg, log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/usage", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdmin_TenantIsolationMiddleware_AllowsSecondAllowedTenant(t *testing.T) {
	cfg := &config.Config{}
	log := testLoggerForAdmin()
	mw := auth.TenantIsolationMiddleware(cfg, log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b", "tenant_c"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_c/usage", nil)
	req.SetPathValue("tenant_id", "tenant_c")
	req = req.WithContext(ctx)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for path tenant in allowed list, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminRole_TenantIsolationMiddleware_AllowsAnyTenant(t *testing.T) {
	cfg := &config.Config{}
	log := testLoggerForAdmin()
	mw := auth.TenantIsolationMiddleware(cfg, log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_x", "admin-user", []string{"admin"})

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_z/usage", nil)
	req.SetPathValue("tenant_id", "tenant_z")
	req = req.WithContext(ctx)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
