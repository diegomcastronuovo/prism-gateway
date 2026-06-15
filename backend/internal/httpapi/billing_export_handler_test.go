package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── billingFakeStorage ────────────────────────────────────────────────────────

type billingFakeStorage struct {
	fakeStorage
	lineItems []storage.BillingLineItem
	lineErr   error
	grouped   []storage.BillingGroupedRow
	groupErr  error
}

func (b *billingFakeStorage) StreamBillingLineItems(_ context.Context, _ string, _, _ time.Time, fn func(storage.BillingLineItem) error) error {
	if b.lineErr != nil {
		return b.lineErr
	}
	for _, item := range b.lineItems {
		if err := fn(item); err != nil {
			return err
		}
	}
	return nil
}

func (b *billingFakeStorage) GetBillingGrouped(_ context.Context, _ string, _, _ time.Time, _ string) ([]storage.BillingGroupedRow, error) {
	return b.grouped, b.groupErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func billingTestHandlers(store *billingFakeStorage) *Handlers {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "t1"}},
	}
	return &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

func billingExportGET(h *Handlers, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/billing/export"+query, nil)
	req.SetPathValue("tenant_id", "t1")
	rec := httptest.NewRecorder()
	h.AdminBillingExport(rec, req)
	return rec
}

// ── validation tests ──────────────────────────────────────────────────────────

func TestAdminBillingExport_InvalidMonth(t *testing.T) {
	h := billingTestHandlers(&billingFakeStorage{})
	rec := billingExportGET(h, "?month=not-a-month")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminBillingExport_InvalidFormat(t *testing.T) {
	h := billingTestHandlers(&billingFakeStorage{})
	rec := billingExportGET(h, "?month=2026-03&format=xml")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminBillingExport_InvalidGroupBy(t *testing.T) {
	h := billingTestHandlers(&billingFakeStorage{})
	rec := billingExportGET(h, "?month=2026-03&group_by=foo")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── line-item CSV tests ───────────────────────────────────────────────────────

func TestAdminBillingExport_EmptyCSV(t *testing.T) {
	store := &billingFakeStorage{}
	h := billingTestHandlers(store)
	rec := billingExportGET(h, "?month=2026-03&format=csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Errorf("expected text/csv Content-Type, got %q", ct)
	}
	r := csv.NewReader(rec.Body)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse error: %v", err)
	}
	// Only the header row should be present
	if len(rows) != 1 {
		t.Errorf("expected 1 row (header), got %d", len(rows))
	}
	if rows[0][0] != "timestamp" {
		t.Errorf("expected header[0]='timestamp', got %q", rows[0][0])
	}
	if len(store.complianceEvents) != 1 {
		t.Fatalf("expected 1 compliance event, got %d", len(store.complianceEvents))
	}
	if store.complianceEvents[0].EventType != "csv_exported" {
		t.Fatalf("event_type=%q, want csv_exported", store.complianceEvents[0].EventType)
	}
}

func TestAdminBillingExport_EmptyJSON(t *testing.T) {
	store := &billingFakeStorage{}
	h := billingTestHandlers(store)
	rec := billingExportGET(h, "?month=2026-03&format=json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	data, ok := resp["data"]
	if !ok {
		t.Fatalf("missing 'data' key in response")
	}
	// data must be an empty array, not null
	arr, ok := data.([]interface{})
	if !ok {
		t.Fatalf("'data' must be an array, got %T", data)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty data array, got %d items", len(arr))
	}
	if len(store.complianceEvents) != 0 {
		t.Fatalf("expected no compliance events for JSON export, got %d", len(store.complianceEvents))
	}
}

func TestAdminBillingExport_LineItemCSV_Shape(t *testing.T) {
	rid := "11111111-1111-1111-1111-111111111111"
	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store := &billingFakeStorage{
		lineItems: []storage.BillingLineItem{
			{
				Timestamp:        ts,
				RequestID:        rid,
				TenantID:         "t1",
				Model:            "gpt-4",
				Provider:         "openai",
				Status:           "ok",
				TotalTokens:      100,
				PromptTokens:     60,
				CompletionTokens: 40,
				CostUSD:          0.001234,
			},
		},
	}
	h := billingTestHandlers(store)
	rec := billingExportGET(h, "?month=2026-03&format=csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	r := csv.NewReader(rec.Body)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 data row, got %d rows", len(rows))
	}

	dataRow := rows[1]
	// timestamp
	if !strings.HasPrefix(dataRow[0], "2026-03-01") {
		t.Errorf("unexpected timestamp %q", dataRow[0])
	}
	// request_id
	if dataRow[1] != rid {
		t.Errorf("unexpected request_id %q", dataRow[1])
	}
	// model
	if dataRow[3] != "gpt-4" {
		t.Errorf("unexpected model %q", dataRow[3])
	}
	// total_tokens
	if dataRow[6] != "100" {
		t.Errorf("unexpected total_tokens %q", dataRow[6])
	}
	// cost_usd
	if !strings.HasPrefix(dataRow[9], "0.001234") {
		t.Errorf("unexpected cost_usd %q", dataRow[9])
	}
	if len(store.complianceEvents) != 1 {
		t.Fatalf("expected 1 compliance event, got %d", len(store.complianceEvents))
	}
}

func TestAdminBillingExport_MetadataFields(t *testing.T) {
	store := &billingFakeStorage{
		lineItems: []storage.BillingLineItem{
			{
				Timestamp:   time.Now(),
				RequestID:   uuid.New().String(),
				TenantID:    "t1",
				Model:       "gpt-4",
				Provider:    "openai",
				Status:      "ok",
				Project:     "analytics",
				CostCenter:  "eng",
				Env:         "prod",
				Application: "chat-ui",
			},
		},
	}
	h := billingTestHandlers(store)
	rec := billingExportGET(h, "?month=2026-03&format=csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	r := csv.NewReader(rec.Body)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected header + data row, got %d", len(rows))
	}

	dataRow := rows[1]
	// project is col 10, cost_center 11, env 12, application 13
	if dataRow[10] != "analytics" {
		t.Errorf("project: expected 'analytics', got %q", dataRow[10])
	}
	if dataRow[11] != "eng" {
		t.Errorf("cost_center: expected 'eng', got %q", dataRow[11])
	}
	if dataRow[12] != "prod" {
		t.Errorf("env: expected 'prod', got %q", dataRow[12])
	}
	if dataRow[13] != "chat-ui" {
		t.Errorf("application: expected 'chat-ui', got %q", dataRow[13])
	}
}

// ── grouped export tests ──────────────────────────────────────────────────────

func TestAdminBillingExport_Grouped_SumTokens(t *testing.T) {
	store := &billingFakeStorage{
		grouped: []storage.BillingGroupedRow{
			{GroupKey: "gpt-4", RequestsCount: 10, PromptTokens: 500, CompletionTokens: 300, TotalTokens: 800, CostUSD: 0.05},
			{GroupKey: "gpt-3.5", RequestsCount: 5, PromptTokens: 100, CompletionTokens: 80, TotalTokens: 180, CostUSD: 0.01},
		},
	}
	h := billingTestHandlers(store)
	rec := billingExportGET(h, "?month=2026-03&format=csv&group_by=model")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	r := csv.NewReader(rec.Body)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	// header + 2 data rows
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Verify header columns
	header := rows[0]
	if header[0] != "month" || header[3] != "group_key" || header[4] != "requests_count" {
		t.Errorf("unexpected header: %v", header)
	}

	// Verify first data row (gpt-4)
	dataRow := rows[1]
	if dataRow[3] != "gpt-4" {
		t.Errorf("group_key: expected 'gpt-4', got %q", dataRow[3])
	}
	if dataRow[4] != "10" {
		t.Errorf("requests_count: expected '10', got %q", dataRow[4])
	}
	if dataRow[5] != "800" {
		t.Errorf("total_tokens: expected '800', got %q", dataRow[5])
	}
	if len(store.complianceEvents) != 1 {
		t.Fatalf("expected 1 compliance event, got %d", len(store.complianceEvents))
	}
}

func TestAdminBillingExport_Grouped_EmptyGroupKey(t *testing.T) {
	store := &billingFakeStorage{
		grouped: []storage.BillingGroupedRow{
			{GroupKey: "", RequestsCount: 3, TotalTokens: 50},
		},
	}
	h := billingTestHandlers(store)
	rec := billingExportGET(h, "?month=2026-03&format=csv&group_by=project")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	r := csv.NewReader(rec.Body)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected header + 1 row, got %d", len(rows))
	}
	if rows[1][3] != "" {
		t.Errorf("expected empty group_key, got %q", rows[1][3])
	}
}

func TestAdminBillingExport_Grouped_JSON(t *testing.T) {
	store := &billingFakeStorage{
		grouped: []storage.BillingGroupedRow{
			{GroupKey: "openai", RequestsCount: 7, TotalTokens: 300, CostUSD: 0.03},
		},
	}
	h := billingTestHandlers(store)
	rec := billingExportGET(h, "?month=2026-03&format=json&group_by=provider")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}

	if resp["object"] != "billing_export_grouped" {
		t.Errorf("object: expected 'billing_export_grouped', got %v", resp["object"])
	}
	if resp["month"] != "2026-03" {
		t.Errorf("month: expected '2026-03', got %v", resp["month"])
	}
	if resp["group_by"] != "provider" {
		t.Errorf("group_by: expected 'provider', got %v", resp["group_by"])
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("'data' must be an array, got %T", resp["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 data item, got %d", len(data))
	}

	item := data[0].(map[string]interface{})
	if item["GroupKey"] != "openai" && item["group_key"] != "openai" {
		t.Errorf("expected group_key='openai', got %v", item)
	}
	if len(store.complianceEvents) != 0 {
		t.Fatalf("expected no compliance events for JSON export, got %d", len(store.complianceEvents))
	}
}
