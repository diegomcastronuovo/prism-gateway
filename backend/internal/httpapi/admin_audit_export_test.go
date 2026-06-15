package httpapi

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

func TestAdminAuditExport_CSV_EmitsComplianceEvent(t *testing.T) {
	store := &fakeStorage{
		auditRecords: []storage.AuditRecord{
			{
				RequestID:        uuidFromString(t, "11111111-1111-1111-1111-111111111111"),
				Timestamp:        time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
				TenantID:         "tenant_a",
				Model:            "gpt-4o-mini",
				Provider:         "openai",
				Strategy:         "round_robin",
				Status:           "ok",
				LatencyMs:        120,
				PromptTokens:     10,
				CompletionTokens: 12,
				TotalTokens:      22,
				CostUSD:          0.001,
			},
		},
	}
	cfg := &config.Config{Tenants: []config.TenantConfig{{ID: "tenant_a"}}}
	h := &Handlers{cfg: cfg, store: store, log: testLogger()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/audit/export?from=2026-03-01&to=2026-03-12&format=csv", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	w := httptest.NewRecorder()
	h.AdminAuditExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	r := csv.NewReader(w.Body)
	if _, err := r.ReadAll(); err != nil {
		t.Fatalf("csv parse error: %v", err)
	}

	if len(store.complianceEvents) != 1 {
		t.Fatalf("expected 1 compliance event, got %d", len(store.complianceEvents))
	}
	if store.complianceEvents[0].EventType != "csv_exported" {
		t.Fatalf("event_type=%q, want csv_exported", store.complianceEvents[0].EventType)
	}
}

func TestAdminAuditExport_JSON_NoComplianceEvent(t *testing.T) {
	store := &fakeStorage{}
	cfg := &config.Config{Tenants: []config.TenantConfig{{ID: "tenant_a"}}}
	h := &Handlers{cfg: cfg, store: store, log: testLogger()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/audit/export?from=2026-03-01&to=2026-03-12&format=json", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	w := httptest.NewRecorder()
	h.AdminAuditExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(store.complianceEvents) != 0 {
		t.Fatalf("expected no compliance events for JSON export, got %d", len(store.complianceEvents))
	}
}

func uuidFromString(t *testing.T, v string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(v)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}
