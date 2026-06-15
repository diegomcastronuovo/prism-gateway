package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

func TestPlatformControlPlane_LocalAdmin_DeniedWithInsufficientPermissions(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/route-groups", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run for local_admin GET /admin/route-groups")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "insufficient permissions") {
		t.Fatalf("expected insufficient permissions in body, got %q", w.Body.String())
	}
}

func TestPlatformControlPlane_User_DeniedBenchmarks(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "viewer", []string{"user"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/benchmarks/models", nil)
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

func TestPlatformControlPlane_LocalAdmin_AllowedTenantConfig(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
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
		t.Fatal("handler must run for local_admin GET tenant config")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPlatformControlPlane_LocalAdmin_DeniedSemanticThresholdPatch(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodPatch, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodPatch, "/admin/tenants/tenant_b/semantic-threshold", nil)
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
		t.Fatal("handler must not run for local_admin PATCH semantic-threshold")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPlatformControlPlane_LocalAdmin_DeniedObservabilitySemanticEndpoint(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "martin", []string{"local_admin"})
	ctx = auth.WithAllowedTenants(ctx, []string{"tenant_b"})
	ctx = auth.WithContextTenantID(ctx, "tenant_b")

	req := httptest.NewRequest(http.MethodGet, "/admin/observability/semantic-routing", nil)
	req = req.WithContext(ctx)

	var innerHit bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if innerHit {
		t.Fatal("handler must not run for local_admin semantic observability endpoint")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "insufficient permissions") {
		t.Fatalf("expected insufficient permissions in body, got %q", w.Body.String())
	}
}
