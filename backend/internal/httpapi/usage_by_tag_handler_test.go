package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── tagFakeStorage ────────────────────────────────────────────────────────────

type tagFakeStorage struct {
	fakeStorage
	rows   []storage.UsageByTagRow
	tagErr error
}

func (s *tagFakeStorage) GetUsageByTag(_ context.Context, _ string, _, _ time.Time, _ string) ([]storage.UsageByTagRow, error) {
	return s.rows, s.tagErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func tagTestHandlers(store *tagFakeStorage) *Handlers {
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

func usageByTagGET(h *Handlers, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/usage/by-tag"+query, nil)
	req.SetPathValue("tenant_id", "t1")
	rec := httptest.NewRecorder()
	h.AdminUsageByTag(rec, req)
	return rec
}

// ── validation tests ──────────────────────────────────────────────────────────

func TestAdminUsageByTag_MissingMonth(t *testing.T) {
	h := tagTestHandlers(&tagFakeStorage{})
	rec := usageByTagGET(h, "?tag=project")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing month, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsageByTag_InvalidMonth(t *testing.T) {
	h := tagTestHandlers(&tagFakeStorage{})
	rec := usageByTagGET(h, "?month=not-a-month&tag=project")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid month, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsageByTag_MissingTag(t *testing.T) {
	h := tagTestHandlers(&tagFakeStorage{})
	rec := usageByTagGET(h, "?month=2026-03")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing tag, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsageByTag_InvalidTag(t *testing.T) {
	h := tagTestHandlers(&tagFakeStorage{})
	// "!bad" starts with non-alpha
	rec := usageByTagGET(h, "?month=2026-03&tag=!bad")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid tag, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsageByTag_Empty(t *testing.T) {
	h := tagTestHandlers(&tagFakeStorage{rows: []storage.UsageByTagRow{}})
	rec := usageByTagGET(h, "?month=2026-03&tag=project")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got: %v", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(data))
	}
}

func TestAdminUsageByTag_Results(t *testing.T) {
	store := &tagFakeStorage{
		rows: []storage.UsageByTagRow{
			{Value: "marketing", Requests: 1540, TotalTokens: 2100000, CostUSD: 312.44},
			{Value: "product", Requests: 800, TotalTokens: 950000, CostUSD: 140.50},
		},
	}
	h := tagTestHandlers(store)
	rec := usageByTagGET(h, "?month=2026-03&tag=project")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["tenant_id"] != "t1" {
		t.Errorf("tenant_id: got %v, want t1", resp["tenant_id"])
	}
	if resp["month"] != "2026-03" {
		t.Errorf("month: got %v, want 2026-03", resp["month"])
	}
	if resp["tag"] != "project" {
		t.Errorf("tag: got %v, want project", resp["tag"])
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array: %v", resp["data"])
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(data))
	}

	first := data[0].(map[string]interface{})
	if first["value"] != "marketing" {
		t.Errorf("first row value: got %v, want marketing", first["value"])
	}
	if first["requests"].(float64) != 1540 {
		t.Errorf("first row requests: got %v, want 1540", first["requests"])
	}
	if first["cost_usd"].(float64) != 312.44 {
		t.Errorf("first row cost_usd: got %v, want 312.44", first["cost_usd"])
	}
}
