package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

func TestIsWriteMethod(t *testing.T) {
	writeMethods := []string{"POST", "PUT", "PATCH", "DELETE"}
	for _, m := range writeMethods {
		if !isWriteMethod(m) {
			t.Errorf("isWriteMethod(%q) = false, want true", m)
		}
	}
	readMethods := []string{"GET", "HEAD", "OPTIONS"}
	for _, m := range readMethods {
		if isWriteMethod(m) {
			t.Errorf("isWriteMethod(%q) = true, want false", m)
		}
	}
}

func TestIsFinanceAllowedEndpoint(t *testing.T) {
	allowed := []string{
		"/admin/finops/anomalies/explain",
		"/admin/finops/cache-savings",
		"/admin/observability/semantic-cache",
		"/admin/requests",
		"/admin/requests/recent",
		"/admin/requests/stats",
		"/admin/requests/req-1/routing",
		"/admin/routing/stats",
		"/admin/anomalies",
		"/admin/anomalies/stats",
		"/admin/tenants/tenant_a/billing/export",
		"/admin/api-keys/usage",
		"/admin/api-keys/requests",
		"/admin/api-keys/key-abc/usage",
		"/admin/tenants/t1/usage/summary",
		"/admin/tenants/t1/budget/forecast",
		"/admin/tenants/t1/budgets/status",
		"/admin/tenants/t1/usage",
	}
	for _, path := range allowed {
		if !isFinanceAllowedEndpoint(path) {
			t.Errorf("isFinanceAllowedEndpoint(%q) = false, want true", path)
		}
	}
	denied := []string{
		"/admin/config/global",
		"/admin/config/history",
		"/admin/tenants",
		"/admin/tenants/t1/config",
		"/admin/tenants/t1/api-keys",
		"/admin/tenants/t1/pii/foo",
		"/admin/providers",
		"/admin/models",
		"/admin/benchmarks/models",
		"/admin/version",
		"/admin/replay/x",
		"/admin/features",
		"/metrics",
	}
	for _, path := range denied {
		if isFinanceAllowedEndpoint(path) {
			t.Errorf("isFinanceAllowedEndpoint(%q) = true, want false", path)
		}
	}
}

func TestHasLogsReadAccess(t *testing.T) {
	cases := []struct {
		name  string
		roles []string
		want  bool
	}{
		{name: "admin allowed", roles: []string{"admin"}, want: true},
		{name: "audit allowed", roles: []string{"audit"}, want: true},
		{name: "local_admin denied", roles: []string{"local_admin"}, want: false},
		{name: "financial denied", roles: []string{"financial"}, want: false},
		{name: "user denied", roles: []string{"user"}, want: false},
	}
	for _, tc := range cases {
		if got := hasLogsReadAccess(tc.roles); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestHasAdminWriteAccess(t *testing.T) {
	if !hasAdminWriteAccess([]string{"admin"}) {
		t.Fatal("admin must have write access")
	}
	for _, roles := range [][]string{
		{"audit"},
		{"financial"},
		{"local_admin"},
		{"user"},
	} {
		if hasAdminWriteAccess(roles) {
			t.Fatalf("roles %v must not have admin write access", roles)
		}
	}
}

func TestRequireLogsReadAccessMiddleware(t *testing.T) {
	log := testLoggerForAdmin()
	mw := RequireLogsReadAccessMiddleware(log)

	run := func(roles []string) int {
		ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
			"tenant_x", "sub-1", roles)
		req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent", nil).WithContext(ctx)
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}

	if code := run([]string{"admin"}); code != http.StatusOK {
		t.Fatalf("admin expected 200, got %d", code)
	}
	if code := run([]string{"audit"}); code != http.StatusOK {
		t.Fatalf("audit expected 200, got %d", code)
	}
	if code := run([]string{"user"}); code != http.StatusForbidden {
		t.Fatalf("user expected 403, got %d", code)
	}
}
