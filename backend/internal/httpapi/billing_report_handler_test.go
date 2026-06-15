package httpapi

import (
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type billingReportTestStore struct {
	fakeStorage
	apiRows []storage.APIKeyUsageRow
	apiBD   []storage.APIKeyModelUsageRow
	jwtRows []storage.JWTSubUsageRow
	jwtBD   []storage.JWTSubModelUsageRow
}

func (s *billingReportTestStore) GetAPIKeyUsage(_ context.Context, _ storage.APIKeyUsageFilter) (storage.APIKeyUsageSummary, []storage.APIKeyUsageRow, error) {
	return storage.APIKeyUsageSummary{}, s.apiRows, nil
}

func (s *billingReportTestStore) GetAPIKeyModelBreakdown(_ context.Context, _ storage.APIKeyUsageFilter) ([]storage.APIKeyModelUsageRow, error) {
	return s.apiBD, nil
}

func (s *billingReportTestStore) GetJWTSubUsage(_ context.Context, _ storage.JWTSubUsageFilter) ([]storage.JWTSubUsageRow, int, error) {
	return s.jwtRows, len(s.jwtRows), nil
}

func (s *billingReportTestStore) GetJWTSubModelBreakdown(_ context.Context, _ storage.JWTSubUsageFilter) ([]storage.JWTSubModelUsageRow, error) {
	return s.jwtBD, nil
}

func billingReportTestHandlers(store *billingReportTestStore) *Handlers {
	return &Handlers{
		cfg:            testConfig(),
		store:          store,
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

func TestAdminBillingReportCSV_ForbiddenFinanceJWT(t *testing.T) {
	h := billingReportTestHandlers(&billingReportTestStore{})
	req := httptest.NewRequest(http.MethodGet, "/admin/billing/report.csv?window_hours=24", nil)
	req = req.WithContext(financeJWTContext())
	rec := httptest.NewRecorder()
	h.AdminBillingReportCSV(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"message":"forbidden"`) {
		t.Fatalf("expected forbidden message, got %s", rec.Body.String())
	}
}

func TestAdminBillingReportCSV_AllowsAuditJWT(t *testing.T) {
	keyID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	store := &billingReportTestStore{
		apiRows: []storage.APIKeyUsageRow{{
			APIKeyID: keyID, APIKeyName: "k1", TenantID: "t1", Requests: 2, TopModel: "model-a",
		}},
		apiBD: []storage.APIKeyModelUsageRow{{
			APIKeyID: keyID, TenantID: "t1", Model: "model-a", Requests: 2, Spend: 0.01,
		}},
		jwtRows: []storage.JWTSubUsageRow{{
			JWTSub: "sub-x", TenantID: "t1", Requests: 1, FirstSeen: time.Now(), LastSeen: time.Now(),
		}},
		jwtBD: []storage.JWTSubModelUsageRow{{
			JWTSub: "sub-x", TenantID: "t1", Model: "model-b", Requests: 1, Spend: 0.02,
		}},
	}
	h := billingReportTestHandlers(store)
	req := httptest.NewRequest(http.MethodGet, "/admin/billing/report.csv?window_hours=24", nil)
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "t1", "auditor", []string{"audit"}))
	rec := httptest.NewRecorder()
	h.AdminBillingReportCSV(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("expected text/csv content-type, got %q", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("expected attachment disposition, got %q", rec.Header().Get("Content-Disposition"))
	}
	rows := readCSVRows(t, rec.Body.String())
	if len(rows) < 3 {
		t.Fatalf("expected header + 2 data rows, got %d lines", len(rows))
	}
	if rows[0][0] != "identity_type" {
		t.Fatalf("unexpected header: %v", rows[0])
	}
}

func TestAdminBillingReportCSV_Empty_HeaderOnly(t *testing.T) {
	h := billingReportTestHandlers(&billingReportTestStore{})
	req := httptest.NewRequest(http.MethodGet, "/admin/billing/report.csv?window_hours=168", nil)
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "t1", "admin", []string{"admin"}))
	rec := httptest.NewRecorder()
	h.AdminBillingReportCSV(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rows := readCSVRows(t, rec.Body.String())
	if len(rows) != 1 {
		t.Fatalf("expected header only, got %d rows", len(rows))
	}
}

func TestAdminBillingReportCSV_SortsByTotalPriceDesc(t *testing.T) {
	lowID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	highID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	store := &billingReportTestStore{
		apiRows: []storage.APIKeyUsageRow{
			{APIKeyID: lowID, APIKeyName: "low", TenantID: "t1", Requests: 10, TopModel: "model-a"},
			{APIKeyID: highID, APIKeyName: "high", TenantID: "t1", Requests: 10, TopModel: "model-a"},
		},
		apiBD: []storage.APIKeyModelUsageRow{
			{APIKeyID: lowID, TenantID: "t1", Model: "model-a", Requests: 10, Spend: 1.0},
			{APIKeyID: highID, TenantID: "t1", Model: "model-a", Requests: 10, Spend: 100.0},
		},
	}
	h := billingReportTestHandlers(store)
	req := httptest.NewRequest(http.MethodGet, "/admin/billing/report.csv?window_hours=24", nil)
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "t1", "admin", []string{"admin"}))
	rec := httptest.NewRecorder()
	h.AdminBillingReportCSV(rec, req)
	rows := readCSVRows(t, rec.Body.String())
	if len(rows) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(rows))
	}
	// After header: first row should be "high" (higher spend → higher total_price typically)
	if rows[1][2] != "high" {
		t.Fatalf("expected highest total_price first, got row %v", rows[1])
	}
}

func TestAdminBillingReportCSV_InvalidWindow(t *testing.T) {
	h := billingReportTestHandlers(&billingReportTestStore{})
	req := httptest.NewRequest(http.MethodGet, "/admin/billing/report.csv?window_hours=9999", nil)
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "t1", "admin", []string{"admin"}))
	rec := httptest.NewRecorder()
	h.AdminBillingReportCSV(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func readCSVRows(t *testing.T, body string) [][]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(body))
	var out [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("csv read: %v", err)
		}
		out = append(out, rec)
	}
	return out
}
