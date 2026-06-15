package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

// SPEC_81: role user — same tenant scope as local_admin, read-only, no global catalog list.

func TestUser_AdminTenantIsolation_AllowsTenantListAndHistoryGET(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	for _, path := range []string{"/admin/tenants", "/admin/config/history"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(ctx)

		var innerHit bool
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			innerHit = true
			w.WriteHeader(http.StatusOK)
		}))

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if !innerHit {
			t.Fatalf("handler must run for user GET %s", path)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d: %s", path, w.Code, w.Body.String())
		}
	}
}

func TestUser_AdminTenantIsolation_BlocksGlobalModelsListGET(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
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

	if innerHit {
		t.Fatal("handler must not run for user GET /admin/models (global catalog)")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_AdminTenantIsolation_BlocksOtherTenantPath(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
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
		t.Fatal("handler must not run for user accessing disallowed tenant")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_AdminTenantIsolation_AllowsReadAllowedTenantPath(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_b/config", nil)
	req.SetPathValue("tenant_id", "tenant_b")
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !innerHit {
		t.Fatal("handler must run for user GET allowed tenant config")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_AdminTenantIsolation_BlocksWriteOnAllowedTenant(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodPatch, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodPatch, "/admin/tenants/tenant_b/config", nil)
	req.SetPathValue("tenant_id", "tenant_b")
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run for user PATCH tenant config")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_AdminTenantIsolation_BlocksGlobalFinOpsGET(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/finops/cache-savings", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run for user on FinOps path")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_AdminTenantIsolation_BlocksJWTSubUsageGET(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
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
		t.Fatal("handler must not run for user on jwt_sub usage path")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalAdminUserCombo_AdminTenantIsolation_UsesLocalAdminForModelsList(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin", "user"})
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
		t.Fatal("JWT with local_admin + user must match local_admin branch and allow GET /admin/models")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
