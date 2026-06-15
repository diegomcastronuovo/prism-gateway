package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

func TestAudit_AdminTenantIsolation_DeniesGlobalAndTenantPaths(t *testing.T) {
	log := testLoggerForAdmin()
	mw := AdminTenantIsolationMiddleware(log)

	ctx := auth.WithJWTAdminContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"tenant_b", "auditor", []string{"audit"})

	for _, tc := range []struct {
		name   string
		method string
		path   string
		tenant string
		allow  bool
	}{
		{name: "read non-config global endpoint", method: http.MethodGet, path: "/admin/requests/recent", tenant: "", allow: true},
		{name: "read non-config tenant endpoint", method: http.MethodGet, path: "/admin/tenants/tenant_b/usage/summary", tenant: "tenant_b", allow: true},
		{name: "deny tenant config read endpoint", method: http.MethodGet, path: "/admin/tenants/tenant_b/config", tenant: "tenant_b", allow: false},
		{name: "deny config write endpoint", method: http.MethodPatch, path: "/admin/tenants/tenant_b/config", tenant: "tenant_b", allow: false},
		{name: "deny any write endpoint", method: http.MethodPost, path: "/admin/requests/recent", tenant: "", allow: false},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.tenant != "" {
			req.SetPathValue("tenant_id", tc.tenant)
		}
		req = req.WithContext(ctx)

		var innerHit bool
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			innerHit = true
			w.WriteHeader(http.StatusOK)
		}))

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if tc.allow {
			if !innerHit {
				t.Fatalf("%s: handler must run for audit role on %s %s", tc.name, tc.method, tc.path)
			}
			if w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200 for audit role on %s %s, got %d: %s", tc.name, tc.method, tc.path, w.Code, w.Body.String())
			}
			continue
		}

		if innerHit {
			t.Fatalf("%s: handler must not run for audit role on %s %s", tc.name, tc.method, tc.path)
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: expected 403 for audit role on %s %s, got %d: %s", tc.name, tc.method, tc.path, w.Code, w.Body.String())
		}
		if got := w.Body.String(); got == "" || got == "{}" {
			t.Fatalf("%s: expected error body for audit denial on %s %s", tc.name, tc.method, tc.path)
		}
	}
}
