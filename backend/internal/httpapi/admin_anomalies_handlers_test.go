package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

func TestAdminAnomaliesList_Returns200Empty(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/anomalies?window_hours=24&limit=50&offset=0", nil)
	w := httptest.NewRecorder()
	h.AdminAnomaliesList(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object string          `json:"object"`
		Data   []interface{}   `json:"data"`
		Pagination struct {
			Limit    int `json:"limit"`
			Offset   int `json:"offset"`
			Returned int `json:"returned"`
			Total    int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object=%q, want list", resp.Object)
	}
	if resp.Pagination.Total != 0 || resp.Pagination.Returned != 0 {
		t.Errorf("pagination: total=%d returned=%d", resp.Pagination.Total, resp.Pagination.Returned)
	}
}

func TestAdminAnomaliesList_Returns200WithData(t *testing.T) {
	ts := time.Now().UTC()
	store := &fakeStorage{}
	// Inject list result via a wrapper that implements Storage and overrides ListAnomalies
	// fakeStorage returns nil, 0, nil by default - we need to return one row.
	// So we need a way to set listAnomaliesResult on fakeStorage. Add fields to fakeStorage.
	store.listAnomaliesRows = []storage.AnomalyListRow{
		{AnomalyID: "an_123", Timestamp: ts, TenantID: "tenant_a", Model: "", Provider: "", ExpectedCostUSD: 0.0021, ObservedCostUSD: 0.0098, DeviationPct: 366, Status: "open", AnomalyType: "cost_spike"},
	}
	store.listAnomaliesTotal = 1
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/anomalies?window_hours=24", nil)
	w := httptest.NewRecorder()
	h.AdminAnomaliesList(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			AnomalyID       string  `json:"anomaly_id"`
			TenantID        string  `json:"tenant_id"`
			ObservedCostUSD float64 `json:"observed_cost_usd"`
			Status          string  `json:"status"`
			AnomalyType     string  `json:"anomaly_type"`
		} `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].AnomalyID != "an_123" || resp.Data[0].Status != "open" || resp.Pagination.Total != 1 {
		t.Errorf("data or total: len=%d id=%s status=%s total=%d", len(resp.Data), resp.Data[0].AnomalyID, resp.Data[0].Status, resp.Pagination.Total)
	}
}

func TestAdminAnomaliesList_InvalidLimit(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	for _, bad := range []string{"0", "201", "x"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/anomalies?limit="+bad, nil)
		w := httptest.NewRecorder()
		h.AdminAnomaliesList(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("limit=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

func TestAdminAnomaliesStats_Returns200(t *testing.T) {
	store := &fakeStorage{}
	store.anomalyStats = storage.AnomalyStats{
		WindowHours: 24,
		Summary:     storage.AnomalyStatsSummary{ActiveAnomalies: 4, CostSpike24hUSD: 18.23, AffectedTenants: 3, AffectedModels: 0},
		Timeline:    []storage.AnomalyTimelineBucket{},
		TopTenants:  []storage.AnomalyTopTenant{{TenantID: "tenant_a", Anomalies: 3}},
		DeviationHistogram: []storage.AnomalyDeviationBucket{{Range: "0-25%", Count: 2}, {Range: "25-50%", Count: 1}, {Range: "50-100%", Count: 1}, {Range: "100%+", Count: 1}},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/anomalies/stats?window_hours=24", nil)
	w := httptest.NewRecorder()
	h.AdminAnomaliesStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object       string `json:"object"`
		WindowHours  int    `json:"window_hours"`
		Summary      struct {
			ActiveAnomalies   int     `json:"active_anomalies"`
			CostSpike24hUSD   float64 `json:"cost_spike_24h_usd"`
			AffectedTenants   int     `json:"affected_tenants"`
			AffectedModels    int     `json:"affected_models"`
		} `json:"summary"`
		TopTenants         []interface{} `json:"top_tenants"`
		DeviationHistogram []interface{} `json:"deviation_histogram"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "anomaly_stats" || resp.Summary.ActiveAnomalies != 4 || resp.Summary.CostSpike24hUSD != 18.23 {
		t.Errorf("object=%s active=%d cost_spike=%v", resp.Object, resp.Summary.ActiveAnomalies, resp.Summary.CostSpike24hUSD)
	}
}

func TestAdminAnomaliesStats_InvalidWindowHours(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/anomalies/stats?window_hours=0", nil)
	w := httptest.NewRecorder()
	h.AdminAnomaliesStats(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestAdminAnomaliesStats_EmptyDataset returns 200 with zeroed aggregates (no 500).
// Spec: when no anomalies exist, return zeroed aggregates instead of internal error.
func TestAdminAnomaliesStats_EmptyDataset(t *testing.T) {
	store := &fakeStorage{}
	// anomalyStats is zero value: Summary zeros, nil slices — handler must return 200
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/anomalies/stats?window_hours=24", nil)
	w := httptest.NewRecorder()
	h.AdminAnomaliesStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no anomalies (zeroed aggregates), got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object      string `json:"object"`
		WindowHours int    `json:"window_hours"`
		Summary     struct {
			ActiveAnomalies int     `json:"active_anomalies"`
			CostSpike24hUSD float64 `json:"cost_spike_24h_usd"`
			AffectedTenants int     `json:"affected_tenants"`
			AffectedModels  int     `json:"affected_models"`
		} `json:"summary"`
		Timeline           []interface{} `json:"timeline"`
		TopTenants         []interface{} `json:"top_tenants"`
		DeviationHistogram []interface{} `json:"deviation_histogram"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "anomaly_stats" || resp.Summary.ActiveAnomalies != 0 || resp.Summary.CostSpike24hUSD != 0 {
		t.Errorf("expected zeroed summary: object=%s active=%d cost_spike=%v", resp.Object, resp.Summary.ActiveAnomalies, resp.Summary.CostSpike24hUSD)
	}
	if resp.Timeline == nil || resp.TopTenants == nil {
		t.Error("timeline and top_tenants must be non-nil (empty slice)")
	}
}
