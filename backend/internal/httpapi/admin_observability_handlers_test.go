package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

// ── AdminTenantUsage ──────────────────────────────────────────────────────────

func TestAdminTenantUsage_Happy(t *testing.T) {
	cfg := testAdminConfig()
	store := &fakeStorage{
		usageOverview: storage.TenantUsageOverview{
			TotalRequests: 1200,
			TotalTokens:   450000,
			TotalCostUSD:  12.45,
		},
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/usage?month=2026-03", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminTenantUsage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["tenant_id"] != "t1" {
		t.Errorf("tenant_id=%v, want t1", resp["tenant_id"])
	}
	if resp["requests"] != float64(1200) {
		t.Errorf("requests=%v, want 1200", resp["requests"])
	}
	if resp["tokens"] != float64(450000) {
		t.Errorf("tokens=%v, want 450000", resp["tokens"])
	}
	if resp["cost_usd"] != 12.45 {
		t.Errorf("cost_usd=%v, want 12.45", resp["cost_usd"])
	}
	if resp["month"] != "2026-03" {
		t.Errorf("month=%v, want 2026-03", resp["month"])
	}
}

func TestAdminTenantUsage_DefaultsToCurrentMonth(t *testing.T) {
	cfg := testAdminConfig()
	h := &Handlers{cfg: cfg, store: &fakeStorage{}, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/usage", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminTenantUsage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	// Month should be set (current month format).
	if resp["month"] == nil || resp["month"] == "" {
		t.Error("expected month field to be populated")
	}
}

func TestAdminTenantUsage_InvalidMonth(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/t1/usage?month=not-a-month", nil)
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()
	h.AdminTenantUsage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── AdminRoutingStats ─────────────────────────────────────────────────────────

func TestAdminRoutingStats_Happy(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
			{Name: "gemini-2.5-flash", Provider: "google"},
		},
		Tenants: []config.TenantConfig{
			{
				ID: "t1",
				Selection: config.SelectionConfig{
					RouteGroups: map[string][]string{
						"cheap":   {"gpt-4o-mini", "gemini-2.5-flash"},
						"premium": {"gemini-2.5-flash"},
					},
				},
			},
		},
	}
	store := &fakeStorage{
		modelCounts: map[string]int64{
			"gpt-4o-mini":      450,
			"gemini-2.5-flash": 200,
		},
	}
	h := &Handlers{cfg: cfg, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/routing/stats", nil)
	w := httptest.NewRecorder()
	h.AdminRoutingStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Models map[string]int64 `json:"models"`
		Routes map[string]int64 `json:"routes"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Models["gpt-4o-mini"] != 450 {
		t.Errorf("models[gpt-4o-mini]=%d, want 450", resp.Models["gpt-4o-mini"])
	}
	if resp.Models["gemini-2.5-flash"] != 200 {
		t.Errorf("models[gemini-2.5-flash]=%d, want 200", resp.Models["gemini-2.5-flash"])
	}
	// cheap = gpt-4o-mini(450) + gemini-2.5-flash(200) = 650
	if resp.Routes["cheap"] != 650 {
		t.Errorf("routes[cheap]=%d, want 650", resp.Routes["cheap"])
	}
	// premium = gemini-2.5-flash(200)
	if resp.Routes["premium"] != 200 {
		t.Errorf("routes[premium]=%d, want 200", resp.Routes["premium"])
	}
}

func TestAdminRoutingStats_WindowDaysDefault(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/routing/stats", nil)
	w := httptest.NewRecorder()
	h.AdminRoutingStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAdminRoutingStats_InvalidWindowDays(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	for _, bad := range []string{"0", "91", "abc", "-1"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/routing/stats?window_days="+bad, nil)
		w := httptest.NewRecorder()
		h.AdminRoutingStats(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("window_days=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

// ── AdminListRequests ─────────────────────────────────────────────────────────

func TestAdminListRequests_Happy(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeStorage{
		recentRequests: []storage.RequestListRow{
			{RequestID: "req-1", TenantID: "t1", Model: "gpt-4o-mini", CreatedAt: now},
			{RequestID: "req-2", TenantID: "t2", Model: "gemini-2.5-flash", CreatedAt: now.Add(-time.Minute)},
		},
		recentRequestsMore: false,
	}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests", nil)
	w := httptest.NewRecorder()
	h.AdminListRequests(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Object  string `json:"object"`
		HasMore bool   `json:"has_more"`
		Data    []struct {
			RequestID string `json:"request_id"`
			TenantID  string `json:"tenant_id"`
			Model     string `json:"model"`
			Created   int64  `json:"created"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("object=%q, want list", resp.Object)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(resp.Data))
	}
	if resp.Data[0].RequestID != "req-1" {
		t.Errorf("data[0].request_id=%q, want req-1", resp.Data[0].RequestID)
	}
	if resp.Data[0].Created == 0 {
		t.Error("created should be a non-zero unix timestamp")
	}
	if resp.HasMore {
		t.Error("has_more should be false")
	}
}

func TestAdminListRequests_HasMore(t *testing.T) {
	store := &fakeStorage{
		recentRequests:     []storage.RequestListRow{{RequestID: "r1", TenantID: "t1", Model: "m", CreatedAt: time.Now()}},
		recentRequestsMore: true,
	}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests?limit=1", nil)
	w := httptest.NewRecorder()
	h.AdminListRequests(w, req)

	var resp struct {
		HasMore bool `json:"has_more"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.HasMore {
		t.Error("has_more should be true")
	}
}

func TestAdminListRequests_Empty(t *testing.T) {
	h := &Handlers{cfg: &config.Config{}, store: &fakeStorage{}, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests", nil)
	w := httptest.NewRecorder()
	h.AdminListRequests(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data []any `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data, got %d items", len(resp.Data))
	}
}

func TestAdminListRequests_WindowHoursApplied(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeStorage{
		recentRequests: []storage.RequestListRow{
			{RequestID: "req-1", TenantID: "t1", Model: "gpt-4o", CreatedAt: now},
		},
		recentRequestsMore: false,
	}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	for _, tc := range []struct {
		name        string
		query       string
		wantCode    int
		wantDataLen int
	}{
		{"window_hours=1", "/admin/requests?window_hours=1", http.StatusOK, 1},
		{"window_hours=24", "/admin/requests?window_hours=24", http.StatusOK, 1},
		{"window_hours=168", "/admin/requests?window_hours=168", http.StatusOK, 1},
		{"no window_hours uses default", "/admin/requests", http.StatusOK, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.query, nil)
			w := httptest.NewRecorder()
			h.AdminListRequests(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("code: got %d, want %d", w.Code, tc.wantCode)
			}
			var resp struct {
				Data []struct {
					RequestID string `json:"request_id"`
				} `json:"data"`
			}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.Data) != tc.wantDataLen {
				t.Errorf("data length: got %d, want %d", len(resp.Data), tc.wantDataLen)
			}
		})
	}
}

func TestAdminListRequests_InvalidWindowHoursDefaulted(t *testing.T) {
	store := &fakeStorage{recentRequests: []storage.RequestListRow{}}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	for _, q := range []string{"window_hours=0", "window_hours=-1", "window_hours=1000"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/requests?"+q, nil)
		w := httptest.NewRecorder()
		h.AdminListRequests(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: got code %d, want 200", q, w.Code)
		}
	}
}

func TestAdminListRequests_ReturnsExplorerFields(t *testing.T) {
	now := time.Now().UTC()
	reason := "lowest_latency"
	store := &fakeStorage{
		recentRequests: []storage.RequestListRow{
			{
				RequestID:      "chatcmpl-abc",
				TenantID:       "tenant_a",
				Model:          "gpt-4o-mini",
				CreatedAt:      now,
				Provider:       "openai",
				Status:         "ok",
				LatencyMs:      812,
				Strategy:       "smart_routing",
				FallbackUsed:   false,
				CacheHit:       false,
				ErrorType:      nil,
				DecisionReason: &reason,
			},
		},
		recentRequestsMore: false,
	}
	h := &Handlers{cfg: &config.Config{}, store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests", nil)
	w := httptest.NewRecorder()
	h.AdminListRequests(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			RequestID      string  `json:"request_id"`
			TenantID       string  `json:"tenant_id"`
			Model          string  `json:"model"`
			Provider       string  `json:"provider"`
			Status         string  `json:"status"`
			LatencyMs      int     `json:"latency_ms"`
			Strategy       string  `json:"strategy"`
			FallbackUsed   bool    `json:"fallback_used"`
			CacheHit       bool    `json:"cache_hit"`
			ErrorType      *string `json:"error_type"`
			DecisionReason *string `json:"decision_reason"`
			Created        int64   `json:"created"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Data))
	}
	d := resp.Data[0]
	if d.RequestID != "chatcmpl-abc" || d.TenantID != "tenant_a" || d.Model != "gpt-4o-mini" {
		t.Errorf("request_id/tenant_id/model: %q %q %q", d.RequestID, d.TenantID, d.Model)
	}
	if d.Provider != "openai" {
		t.Errorf("provider=%q, want openai", d.Provider)
	}
	if d.Status != "success" {
		t.Errorf("status=%q, want success (mapped from ok)", d.Status)
	}
	if d.LatencyMs != 812 {
		t.Errorf("latency_ms=%d, want 812", d.LatencyMs)
	}
	if d.Strategy != "smart_routing" {
		t.Errorf("strategy=%q, want smart_routing", d.Strategy)
	}
	if d.FallbackUsed {
		t.Error("fallback_used should be false")
	}
	if d.CacheHit {
		t.Error("cache_hit should be false")
	}
	if d.ErrorType != nil {
		t.Errorf("error_type should be null, got %q", *d.ErrorType)
	}
	if d.DecisionReason == nil || *d.DecisionReason != "lowest_latency" {
		t.Errorf("decision_reason=%v, want ptr to lowest_latency", d.DecisionReason)
	}
	if d.Created == 0 {
		t.Error("created should be non-zero unix timestamp")
	}
}

// ── Auth required ─────────────────────────────────────────────────────────────

func TestAdminObservability_RequiresAuth(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	cfg := testAdminConfig()
	h := &Handlers{cfg: cfg, store: &fakeStorage{}, log: testLoggerForAdmin()}

	endpoints := []struct {
		method string
		path   string
		fn     http.HandlerFunc
	}{
		{http.MethodGet, "/admin/tenants/t1/usage", h.AdminTenantUsage},
		{http.MethodGet, "/admin/routing/stats", h.AdminRoutingStats},
		{http.MethodGet, "/admin/requests", h.AdminListRequests},
		{http.MethodGet, "/admin/requests/recent", h.AdminRequestsRecent},
		{http.MethodGet, "/admin/requests/stats", h.AdminRequestsStats},
		{http.MethodGet, "/admin/anomalies", h.AdminAnomaliesList},
		{http.MethodGet, "/admin/anomalies/stats", h.AdminAnomaliesStats},
		{http.MethodGet, "/admin/api-keys/usage", h.AdminAPIKeysUsage},
		{http.MethodGet, "/admin/api-keys/d39b2040-e3d4-40ee-afd6-58964f1d7c4c/usage", h.AdminAPIKeyUsageDetail},
		{http.MethodGet, "/admin/api-keys/requests", h.AdminAPIKeysRequests},
	}

	cache := config.NewTenantConfigCache(1 * time.Second)
	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			AdminMiddleware(cfg, cache, &storage.NopStorage{}, nil, auth.NewJWTValidatorCache(testLoggerForAdmin()), testLoggerForAdmin())(ep.fn).ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", w.Code)
			}
		})
	}
}

// ── AdminAPIKeysUsage ────────────────────────────────────────────────────────

func TestAdminAPIKeysUsage_Returns200WithSummaryAndData(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/usage?window_hours=24&limit=50&offset=0", nil)
	w := httptest.NewRecorder()
	h.AdminAPIKeysUsage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object  string `json:"object"`
		Summary struct {
			TotalActiveAPIKeys int     `json:"total_active_api_keys"`
			TotalRequests      int     `json:"total_requests"`
			TotalSpend         float64 `json:"total_spend"`
			AvgSuccessRate     float64 `json:"avg_success_rate"`
			HighestSpendKey    string  `json:"highest_spend_key"`
			MostActiveKey      string  `json:"most_active_key"`
		} `json:"summary"`
		Data []interface{} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "api_key_usage" {
		t.Errorf("object want api_key_usage, got %q", resp.Object)
	}
	if resp.Data == nil {
		t.Error("data must be present (array)")
	}
}

func TestAdminAPIKeysUsage_QueryParamsValidation(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	for _, bad := range []string{"window_hours=0", "window_hours=9999", "limit=0", "limit=999", "offset=-1"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/usage?"+bad, nil)
		w := httptest.NewRecorder()
		h.AdminAPIKeysUsage(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", bad, w.Code)
		}
	}
}

// ── AdminAPIKeyUsageDetail (drilldown) ───────────────────────────────────────

const testAPIKeyID = "d39b2040-e3d4-40ee-afd6-58964f1d7c4c"

func TestAdminAPIKeyUsageDetail_Returns200WithPopulatedResponse(t *testing.T) {
	keyID := mustParseUUID(t, testAPIKeyID)
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			if id == keyID {
				return storage.APIKeyMeta{ID: keyID, Name: "bootstrap-key", TenantID: "tenant_a"}, true, nil
			}
			return storage.APIKeyMeta{}, false, nil
		},
		getAPIKeyUsageDetail: func() (storage.APIKeyUsageDetailSummary, []storage.APIKeyUsageByModelRow, []storage.APIKeyUsageByProviderRow, []storage.APIKeyUsageRecentRow, int, storage.LatencyStats, []storage.ErrorCountRow, error) {
			return storage.APIKeyUsageDetailSummary{
					Requests: 12, Spend: 0.00008, SuccessRate: 1, AvgLatencyMs: 100, TopModel: "gpt-4o-mini", TopProvider: "openai",
					LastSeen: time.Date(2026, 3, 13, 19, 44, 26, 0, time.UTC),
				},
				[]storage.APIKeyUsageByModelRow{{Model: "gpt-4o-mini", Requests: 12, Spend: 0.00008}},
				[]storage.APIKeyUsageByProviderRow{{Provider: "openai", Requests: 12}},
				[]storage.APIKeyUsageRecentRow{{Timestamp: time.Date(2026, 3, 13, 19, 44, 26, 0, time.UTC), RequestID: "chatcmpl-abc", Model: "gpt-4o-mini", Provider: "openai", Status: "ok", LatencyMs: 7676, CostUSD: 0.000003}},
				1,
				storage.LatencyStats{P50: 7200, P95: 18100, Max: 27900},
				[]storage.ErrorCountRow{{ErrorType: "timeout", Count: 12}, {ErrorType: "rate_limit", Count: 4}},
				nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+testAPIKeyID+"/usage?window_hours=720&limit=50&offset=0", nil)
	req.SetPathValue("api_key_id", testAPIKeyID)
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		APIKeyID           string                   `json:"api_key_id"`
		APIKeyName         string                   `json:"api_key_name"`
		TenantID           string                   `json:"tenant_id"`
		Summary            map[string]interface{}   `json:"summary"`
		LatencyStats       map[string]interface{}   `json:"latency_stats"`
		ErrorsByType       []map[string]interface{} `json:"errors_by_type"`
		RequestsByModel    []interface{}            `json:"requests_by_model"`
		RequestsByProvider []interface{}            `json:"requests_by_provider"`
		RecentRequests     []interface{}            `json:"recent_requests"`
		Pagination         map[string]interface{}   `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.APIKeyID != testAPIKeyID || resp.APIKeyName != "bootstrap-key" || resp.TenantID != "tenant_a" {
		t.Errorf("api_key_id=%q api_key_name=%q tenant_id=%q", resp.APIKeyID, resp.APIKeyName, resp.TenantID)
	}
	if resp.Summary["requests"].(float64) != 12 || len(resp.RecentRequests) != 1 {
		t.Errorf("summary.requests or recent_requests len unexpected")
	}
	if resp.LatencyStats["p50"].(float64) != 7200 || resp.LatencyStats["p95"].(float64) != 18100 || resp.LatencyStats["max"].(float64) != 27900 {
		t.Errorf("latency_stats unexpected: %v", resp.LatencyStats)
	}
	if len(resp.ErrorsByType) != 2 || resp.ErrorsByType[0]["error_type"] != "timeout" || resp.ErrorsByType[0]["count"].(float64) != 12 {
		t.Errorf("errors_by_type unexpected: %v", resp.ErrorsByType)
	}
}

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}

func TestAdminAPIKeyUsageDetail_Returns200WithEmptyResponse(t *testing.T) {
	keyID := mustParseUUID(t, testAPIKeyID)
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			if id == keyID {
				return storage.APIKeyMeta{ID: keyID, Name: "bootstrap-key", TenantID: "tenant_a"}, true, nil
			}
			return storage.APIKeyMeta{}, false, nil
		},
		getAPIKeyUsageDetail: func() (storage.APIKeyUsageDetailSummary, []storage.APIKeyUsageByModelRow, []storage.APIKeyUsageByProviderRow, []storage.APIKeyUsageRecentRow, int, storage.LatencyStats, []storage.ErrorCountRow, error) {
			return storage.APIKeyUsageDetailSummary{}, nil, nil, nil, 0, storage.LatencyStats{}, nil, nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+testAPIKeyID+"/usage", nil)
	req.SetPathValue("api_key_id", testAPIKeyID)
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty drilldown, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Summary struct {
			Requests float64 `json:"requests"`
		} `json:"summary"`
		LatencyStats map[string]interface{} `json:"latency_stats"`
		ErrorsByType []interface{}          `json:"errors_by_type"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Summary.Requests != 0 {
		t.Errorf("expected 0 requests, got %v", resp.Summary.Requests)
	}
	if resp.LatencyStats["p50"].(float64) != 0 || resp.LatencyStats["max"].(float64) != 0 {
		t.Errorf("expected zero latency_stats, got %v", resp.LatencyStats)
	}
	if len(resp.ErrorsByType) != 0 {
		t.Errorf("expected empty errors_by_type, got %v", resp.ErrorsByType)
	}
}

func TestAdminAPIKeyUsageDetail_Returns404WhenKeyNotFound(t *testing.T) {
	store := &fakeStorage{} // getAPIKeyMetaByID returns false by default
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+testAPIKeyID+"/usage", nil)
	req.SetPathValue("api_key_id", testAPIKeyID)
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when key not found, got %d", w.Code)
	}
}

func TestAdminAPIKeyUsageDetail_Returns400ForInvalidUUID(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/not-a-uuid/usage", nil)
	req.SetPathValue("api_key_id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.AdminAPIKeyUsageDetail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", w.Code)
	}
}

func TestAdminAPIKeyUsageDetail_Returns400ForInvalidQueryParams(t *testing.T) {
	keyID := mustParseUUID(t, testAPIKeyID)
	store := &fakeStorage{
		getAPIKeyMetaByID: func(id uuid.UUID) (storage.APIKeyMeta, bool, error) {
			if id == keyID {
				return storage.APIKeyMeta{ID: keyID, Name: "x", TenantID: "t"}, true, nil
			}
			return storage.APIKeyMeta{}, false, nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	for _, q := range []string{"window_hours=0", "limit=999", "offset=-1"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/"+testAPIKeyID+"/usage?"+q, nil)
		req.SetPathValue("api_key_id", testAPIKeyID)
		w := httptest.NewRecorder()
		h.AdminAPIKeyUsageDetail(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", q, w.Code)
		}
	}
}

// ── AdminAPIKeysRequests (raw usage) ─────────────────────────────────────────

func TestAdminAPIKeysRequests_Returns200WithPopulatedResponse(t *testing.T) {
	ts := time.Date(2026, 3, 13, 19, 44, 26, 0, time.UTC)
	keyID := mustParseUUID(t, testAPIKeyID)
	store := &fakeStorage{
		listAPIKeyRawUsage: func(filter storage.APIKeyRawUsageFilter) ([]storage.APIKeyRawUsageRow, int, error) {
			return []storage.APIKeyRawUsageRow{
				{Timestamp: ts, TenantID: "tenant_a", APIKeyID: keyID, APIKeyName: "bootstrap-key", RequestID: "chatcmpl-abc", Model: "gpt-4o-mini", Provider: "openai", Status: "ok", LatencyMs: 7676, CostUSD: 0.000007, PromptTokens: 12, CompletionTokens: 2, TotalTokens: 14},
			}, 1, nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/requests?limit=50&offset=0", nil)
	w := httptest.NewRecorder()
	h.AdminAPIKeysRequests(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object     string                   `json:"object"`
		Data       []map[string]interface{} `json:"data"`
		Pagination map[string]interface{}   `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "api_key_raw_usage" {
		t.Errorf("object want api_key_raw_usage, got %q", resp.Object)
	}
	if len(resp.Data) != 1 || resp.Data[0]["request_id"] != "chatcmpl-abc" || resp.Data[0]["total_tokens"].(float64) != 14 {
		t.Errorf("data unexpected: %v", resp.Data)
	}
	if resp.Pagination["total"].(float64) != 1 {
		t.Errorf("pagination.total want 1, got %v", resp.Pagination["total"])
	}
}

func TestAdminAPIKeysRequests_Returns200WithEmptyResponse(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/requests", nil)
	w := httptest.NewRecorder()
	h.AdminAPIKeysRequests(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object string        `json:"object"`
		Data   []interface{} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "api_key_raw_usage" || len(resp.Data) != 0 {
		t.Errorf("object=%q data len=%d", resp.Object, len(resp.Data))
	}
}

func TestAdminAPIKeysRequests_Returns400ForInvalidFromTo(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	tests := []struct {
		query string
		desc  string
	}{
		{"from=not-rfc3339", "invalid from"},
		{"to=invalid", "invalid to"},
		{"from=2026-03-13T00:00:00Z&to=2026-03-01T00:00:00Z", "from > to"},
		{"limit=0", "invalid limit"},
		{"limit=999", "limit > 200"},
		{"offset=-1", "invalid offset"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin/api-keys/requests?"+tt.query, nil)
			w := httptest.NewRecorder()
			h.AdminAPIKeysRequests(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// ── AdminJWTSubsUsage ─────────────────────────────────────────────────────────

func TestAdminJWTSubsUsage_Returns200WithPopulatedResponse(t *testing.T) {
	first := time.Date(2026, 3, 25, 18, 10, 0, 0, time.UTC)
	last := time.Date(2026, 3, 25, 21, 26, 44, 0, time.UTC)
	store := &fakeStorage{
		getJWTSubUsage: func(filter storage.JWTSubUsageFilter) ([]storage.JWTSubUsageRow, int, error) {
			return []storage.JWTSubUsageRow{
				{JWTSub: "sub-1", Requests: 2, PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, TotalCostUSD: 0.001, FirstSeen: first, LastSeen: last},
			}, 1, nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/usage?limit=50&offset=0", nil)
	w := httptest.NewRecorder()
	h.AdminJWTSubsUsage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object     string                   `json:"object"`
		Data       []map[string]interface{} `json:"data"`
		Pagination map[string]interface{}   `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "jwt_sub_usage" {
		t.Errorf("object want jwt_sub_usage, got %q", resp.Object)
	}
	if len(resp.Data) != 1 || resp.Data[0]["jwt_sub"] != "sub-1" {
		t.Errorf("data unexpected: %v", resp.Data)
	}
	if resp.Pagination["total"].(float64) != 1 {
		t.Errorf("pagination.total want 1, got %v", resp.Pagination["total"])
	}
}

func TestAdminJWTSubsUsage_Returns400ForInvalidQuery(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	tests := []struct {
		query string
		desc  string
	}{
		{"from=not-rfc3339", "invalid from"},
		{"to=invalid", "invalid to"},
		{"from=2026-03-13T00:00:00Z&to=2026-03-01T00:00:00Z", "from > to"},
		{"limit=0", "invalid limit"},
		{"limit=999", "limit > 200"},
		{"offset=-1", "invalid offset"},
		{"sort_by=invalid", "invalid sort_by"},
		{"sort_order=up", "invalid sort_order"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/usage?"+tt.query, nil)
			w := httptest.NewRecorder()
			h.AdminJWTSubsUsage(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// ── AdminJWTSubUsageDetail ───────────────────────────────────────────────────

func TestAdminJWTSubUsageDetail_Returns200WithPopulatedResponse(t *testing.T) {
	store := &fakeStorage{
		getJWTSubUsageDetail: func(jwtSub string, filter storage.JWTSubUsageDetailFilter) (storage.JWTSubUsageDetailSummary, []storage.JWTSubUsageBreakdownRow, error) {
			return storage.JWTSubUsageDetailSummary{Requests: 3, PromptTokens: 12, CompletionTokens: 4, TotalTokens: 16, TotalCostUSD: 0.002},
				[]storage.JWTSubUsageBreakdownRow{{Group: "gpt-4o-mini", Requests: 3, TotalTokens: 16, TotalCostUSD: 0.002}}, nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/sub-1/usage", nil)
	req.SetPathValue("jwt_sub", "sub-1")
	w := httptest.NewRecorder()
	h.AdminJWTSubUsageDetail(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object    string                   `json:"object"`
		JWTSub    string                   `json:"jwt_sub"`
		Summary   map[string]interface{}   `json:"summary"`
		Breakdown []map[string]interface{} `json:"breakdown"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "jwt_sub_usage_detail" || resp.JWTSub != "sub-1" {
		t.Errorf("object/jwt_sub unexpected: %v", resp)
	}
	if resp.Summary["total_tokens"].(float64) != 16 {
		t.Errorf("summary total_tokens want 16, got %v", resp.Summary["total_tokens"])
	}
	if len(resp.Breakdown) != 1 || resp.Breakdown[0]["group"] != "gpt-4o-mini" {
		t.Errorf("breakdown unexpected: %v", resp.Breakdown)
	}
}

func TestAdminJWTSubUsageDetail_Returns400ForInvalidQuery(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	tests := []struct {
		path  string
		query string
		desc  string
	}{
		{"/admin/jwt-subs//usage", "", "missing jwt_sub"},
		{"/admin/jwt-subs/sub-1/usage", "from=not-rfc3339", "invalid from"},
		{"/admin/jwt-subs/sub-1/usage", "to=invalid", "invalid to"},
		{"/admin/jwt-subs/sub-1/usage", "from=2026-03-13T00:00:00Z&to=2026-03-01T00:00:00Z", "from > to"},
		{"/admin/jwt-subs/sub-1/usage", "group_by=invalid", "invalid group_by"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path+"?"+tt.query, nil)
			if tt.desc != "missing jwt_sub" {
				req.SetPathValue("jwt_sub", "sub-1")
			}
			w := httptest.NewRecorder()
			h.AdminJWTSubUsageDetail(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// ── AdminJWTSubsRequests ─────────────────────────────────────────────────────

func TestAdminJWTSubsRequests_Returns200WithPopulatedResponse(t *testing.T) {
	ts := time.Date(2026, 3, 25, 21, 26, 44, 0, time.UTC)
	store := &fakeStorage{
		listJWTSubRawUsage: func(filter storage.JWTSubRawUsageFilter) ([]storage.JWTSubRawUsageRow, int, error) {
			return []storage.JWTSubRawUsageRow{
				{Timestamp: ts, TenantID: "tenant_a", JWTSub: "sub-1", RequestID: "chatcmpl-abc", Model: "gpt-4o-mini", Provider: "openai", Status: "ok", LatencyMs: 397, CostUSD: 0.000014, PromptTokens: 1, CompletionTokens: 22, TotalTokens: 23},
			}, 1, nil
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/requests?limit=50&offset=0", nil)
	w := httptest.NewRecorder()
	h.AdminJWTSubsRequests(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object     string                   `json:"object"`
		Data       []map[string]interface{} `json:"data"`
		Pagination map[string]interface{}   `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "jwt_sub_raw_usage" {
		t.Errorf("object want jwt_sub_raw_usage, got %q", resp.Object)
	}
	if len(resp.Data) != 1 || resp.Data[0]["request_id"] != "chatcmpl-abc" {
		t.Errorf("data unexpected: %v", resp.Data)
	}
	if resp.Pagination["total"].(float64) != 1 {
		t.Errorf("pagination.total want 1, got %v", resp.Pagination["total"])
	}
}

func TestAdminJWTSubsRequests_Returns200WithEmptyResponse(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/requests", nil)
	w := httptest.NewRecorder()
	h.AdminJWTSubsRequests(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object string        `json:"object"`
		Data   []interface{} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "jwt_sub_raw_usage" || len(resp.Data) != 0 {
		t.Errorf("object=%q data len=%d", resp.Object, len(resp.Data))
	}
}

func TestAdminJWTSubsRequests_Returns400ForInvalidQuery(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	tests := []struct {
		query string
		desc  string
	}{
		{"from=not-rfc3339", "invalid from"},
		{"to=invalid", "invalid to"},
		{"from=2026-03-13T00:00:00Z&to=2026-03-01T00:00:00Z", "from > to"},
		{"limit=0", "invalid limit"},
		{"limit=999", "limit > 200"},
		{"offset=-1", "invalid offset"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin/jwt-subs/requests?"+tt.query, nil)
			w := httptest.NewRecorder()
			h.AdminJWTSubsRequests(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// ── AdminRequestsRecent ─────────────────────────────────────────────────────

func TestAdminRequestsRecent_Returns200WithValidAdminKey(t *testing.T) {
	ts := time.Now().UTC()
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{RequestID: "chatcmpl-123", Timestamp: ts, TenantID: "tenant_a", Model: "gpt-4o-mini", Provider: "openai", Strategy: "smart", LatencyMs: 245, Status: "ok", FallbackUsed: false, Attempt: 1},
		},
		requestLogRecentTotal: 1,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent?limit=50&offset=0", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsRecent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			RequestID    string `json:"request_id"`
			TenantID     string `json:"tenant_id"`
			Model        string `json:"model"`
			Provider     string `json:"provider"`
			Strategy     string `json:"strategy"`
			LatencyMs    int    `json:"latency_ms"`
			Status       string `json:"status"`
			FallbackUsed bool   `json:"fallback_used"`
			Attempt      int    `json:"attempt"`
			Cache        struct {
				Status string `json:"status"`
			} `json:"cache"`
		} `json:"data"`
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
	if len(resp.Data) != 1 {
		t.Fatalf("len(data)=%d, want 1", len(resp.Data))
	}
	if resp.Data[0].RequestID != "chatcmpl-123" || resp.Data[0].TenantID != "tenant_a" || resp.Data[0].Status != "ok" {
		t.Errorf("data[0]: request_id=%q tenant_id=%q status=%q", resp.Data[0].RequestID, resp.Data[0].TenantID, resp.Data[0].Status)
	}
	if resp.Data[0].Cache.Status != "unknown" {
		t.Errorf("cache.status=%q, want unknown", resp.Data[0].Cache.Status)
	}
	if resp.Pagination.Total != 1 || resp.Pagination.Returned != 1 {
		t.Errorf("pagination: total=%d returned=%d", resp.Pagination.Total, resp.Pagination.Returned)
	}
}

func TestAdminRequestsRecent_SupportsTenantFilter(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent:      []storage.RequestLogRecentRow{},
		requestLogRecentTotal: 0,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent?tenant_id=tenant_a&limit=10", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsRecent(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAdminRequestsRecent_SupportsPagination(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent:      []storage.RequestLogRecentRow{},
		requestLogRecentTotal: 39,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent?limit=25&offset=10", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsRecent(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Pagination struct {
			Limit  int `json:"limit"`
			Offset int `json:"offset"`
			Total  int `json:"total"`
		} `json:"pagination"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Pagination.Limit != 25 || resp.Pagination.Offset != 10 || resp.Pagination.Total != 39 {
		t.Errorf("pagination: limit=%d offset=%d total=%d", resp.Pagination.Limit, resp.Pagination.Offset, resp.Pagination.Total)
	}
}

func TestAdminRequestsRecent_InvalidLimit(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	for _, bad := range []string{"0", "201", "x"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent?limit="+bad, nil)
		w := httptest.NewRecorder()
		h.AdminRequestsRecent(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("limit=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

func TestAdminAuditRequests_ReturnsExpectedShape(t *testing.T) {
	ts := time.Now().UTC()
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{
				RequestID: "chatcmpl-abc",
				Timestamp: ts,
				TenantID:  "tenant_a",
				JWTSub:    "user-123",
				Model:     "gpt-4o-mini",
				Status:    "ok",
				LatencyMs: 123,
			},
		},
		requestLogRecentTotal: 1,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/audit/requests", nil)
	w := httptest.NewRecorder()
	h.AdminAuditRequests(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			TenantID  string `json:"tenant_id"`
			JWTSub    string `json:"jwt_sub"`
			Model     string `json:"model"`
			Status    string `json:"status"`
			LatencyMs int    `json:"latency_ms"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Data))
	}
	if resp.Data[0].TenantID != "tenant_a" || resp.Data[0].JWTSub != "user-123" {
		t.Fatalf("unexpected row: %+v", resp.Data[0])
	}
}

func TestAdminAuditRequests_AcceptsFiltersAndValidatesRange(t *testing.T) {
	store := &fakeStorage{requestLogRecent: []storage.RequestLogRecentRow{}}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	from := "2026-03-01T00:00:00Z"
	to := "2026-03-26T00:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/admin/audit/requests?tenant_id=tenant_a&jwt_sub=sub-1&status=error&from="+from+"&to="+to, nil)
	w := httptest.NewRecorder()
	h.AdminAuditRequests(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.lastRequestLogFilter.TenantID == nil || *store.lastRequestLogFilter.TenantID != "tenant_a" {
		t.Fatal("tenant_id filter not propagated")
	}
	if store.lastRequestLogFilter.JWTSub == nil || *store.lastRequestLogFilter.JWTSub != "sub-1" {
		t.Fatal("jwt_sub filter not propagated")
	}
	if store.lastRequestLogFilter.Status == nil || *store.lastRequestLogFilter.Status != "error" {
		t.Fatal("status filter not propagated")
	}

	reqBad := httptest.NewRequest(http.MethodGet, "/admin/audit/requests?from=2026-03-26T00:00:00Z&to=2026-03-01T00:00:00Z", nil)
	wBad := httptest.NewRecorder()
	h.AdminAuditRequests(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when from > to, got %d", wBad.Code)
	}
}

// ---- SPEC_129: extended audit fields tests --------------------------------

// auditRespItem is a helper for decoding audit response items in SPEC_129 tests.
type auditRespItem struct {
	Timestamp      string `json:"timestamp"`
	TenantID       string `json:"tenant_id"`
	JWTSub         string `json:"jwt_sub"`
	Actor          string `json:"actor"`
	Model          string `json:"model"`
	Provider       string `json:"provider"`
	Strategy       string `json:"strategy"`
	Status         string `json:"status"`
	LatencyMs      int    `json:"latency_ms"`
	RequestID      string `json:"request_id"`
	Decision       string `json:"decision"`
	DecisionReason string `json:"decision_reason"`
	ErrorType      string `json:"error_type"`
	Error          string `json:"error"`
}

func auditRequest(h *Handlers) ([]auditRespItem, int) {
	req := httptest.NewRequest(http.MethodGet, "/admin/audit/requests", nil)
	w := httptest.NewRecorder()
	h.AdminAuditRequests(w, req)
	var resp struct {
		Data []auditRespItem `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	return resp.Data, w.Code
}

func TestAdminAuditRequests_ExistingFieldsUnchanged(t *testing.T) {
	ts := time.Now().UTC()
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{Timestamp: ts, TenantID: "t1", JWTSub: "u1", Model: "m1", Status: "ok", LatencyMs: 42},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, code := auditRequest(h)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.TenantID != "t1" {
		t.Errorf("tenant_id: want t1, got %s", it.TenantID)
	}
	if it.JWTSub != "u1" {
		t.Errorf("jwt_sub: want u1, got %s", it.JWTSub)
	}
	if it.Model != "m1" {
		t.Errorf("model: want m1, got %s", it.Model)
	}
	if it.Status != "ok" {
		t.Errorf("status: want ok, got %s", it.Status)
	}
	if it.LatencyMs != 42 {
		t.Errorf("latency_ms: want 42, got %d", it.LatencyMs)
	}
}

func TestAdminAuditRequests_ProviderReturned(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", Provider: "openai"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].Provider != "openai" {
		t.Errorf("provider: want openai, got %s", items[0].Provider)
	}
}

func TestAdminAuditRequests_StrategyReturned(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", Strategy: "smart"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].Strategy != "smart" {
		t.Errorf("strategy: want smart, got %s", items[0].Strategy)
	}
}

func TestAdminAuditRequests_RequestIDReturned(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", RequestID: "req-abc-123"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].RequestID != "req-abc-123" {
		t.Errorf("request_id: want req-abc-123, got %s", items[0].RequestID)
	}
}

func TestAdminAuditRequests_DecisionFromDecisionReason(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", DecisionReason: "explicit_ml_header|model:Fake4"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].Decision != "explicit_ml_header|model:Fake4" {
		t.Errorf("decision: want explicit_ml_header|model:Fake4, got %s", items[0].Decision)
	}
	if items[0].DecisionReason != "explicit_ml_header|model:Fake4" {
		t.Errorf("decision_reason: want explicit_ml_header|model:Fake4, got %s", items[0].DecisionReason)
	}
}

func TestAdminAuditRequests_ActorFromJWTSub(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", JWTSub: "user-xyz", APIKeyName: "key-name", APIKeyID: "key-id"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].Actor != "user-xyz" {
		t.Errorf("actor: want user-xyz (jwt_sub), got %s", items[0].Actor)
	}
}

func TestAdminAuditRequests_ActorFallbackToAPIKeyName(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", JWTSub: "", APIKeyName: "bootstrap-9cebb794", APIKeyID: "key-id"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].Actor != "bootstrap-9cebb794" {
		t.Errorf("actor: want bootstrap-9cebb794 (api_key_name), got %s", items[0].Actor)
	}
}

func TestAdminAuditRequests_ActorFallbackToAPIKeyID(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", JWTSub: "", APIKeyName: "", APIKeyID: "550e8400-e29b-41d4-a716-446655440000"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].Actor != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("actor: want api_key_id, got %s", items[0].Actor)
	}
}

func TestAdminAuditRequests_ErrorFieldsReturned(t *testing.T) {
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{TenantID: "t1", Status: "error", ErrorType: "timeout", Error: "upstream call timed out"},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	items, _ := auditRequest(h)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].ErrorType != "timeout" {
		t.Errorf("error_type: want timeout, got %s", items[0].ErrorType)
	}
	if items[0].Error != "upstream call timed out" {
		t.Errorf("error: want 'upstream call timed out', got %s", items[0].Error)
	}
}

func TestAdminComplianceEvents_ReturnsExpectedShape(t *testing.T) {
	ts := time.Now().UTC()
	meta := json.RawMessage(`{"rule":"pii","severity":"high"}`)
	store := &fakeStorage{
		complianceEvents: []storage.ComplianceEventLog{
			{
				ID:          uuid.New(),
				TenantID:    "tenant_a",
				RequestID:   "req-1",
				EventType:   "pii_blocked",
				ActionTaken: "blocked",
				Metadata:    meta,
				CreatedAt:   ts,
			},
		},
		complianceEventsTotal: 1,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/compliance/events", nil)
	w := httptest.NewRecorder()
	h.AdminComplianceEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			TenantID    string `json:"tenant_id"`
			RequestID   string `json:"request_id"`
			EventType   string `json:"event_type"`
			ActionTaken string `json:"action_taken"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].EventType != "pii_blocked" {
		t.Fatalf("unexpected response data: %+v", resp.Data)
	}
}

func TestAdminComplianceEvents_ValidatesAndPropagatesFilters(t *testing.T) {
	store := &fakeStorage{complianceEvents: []storage.ComplianceEventLog{}}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/compliance/events?tenant_id=tenant_a&request_id=req-1&event_type=pii_blocked&from=2026-03-01T00:00:00Z&to=2026-03-26T00:00:00Z", nil)
	w := httptest.NewRecorder()
	h.AdminComplianceEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.lastComplianceFilter.TenantID == nil || *store.lastComplianceFilter.TenantID != "tenant_a" {
		t.Fatal("tenant_id filter not propagated")
	}
	if store.lastComplianceFilter.EventType == nil || *store.lastComplianceFilter.EventType != "pii_blocked" {
		t.Fatal("event_type filter not propagated")
	}
	reqBad := httptest.NewRequest(http.MethodGet, "/admin/compliance/events?from=2026-03-26T00:00:00Z&to=2026-03-01T00:00:00Z", nil)
	wBad := httptest.NewRecorder()
	h.AdminComplianceEvents(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", wBad.Code)
	}
}

func TestAdminConversations_ReturnsData(t *testing.T) {
	store := &fakeStorage{
		conversations: []storage.ConversationLog{
			{
				ID:              uuid.New(),
				RequestID:       "req-1",
				TenantID:        "tenant_a",
				PromptPreview:   "hi",
				ResponsePreview: "hello",
				PIIDetected:     false,
				LoggingMode:     "metadata_only",
				CreatedAt:       time.Now().UTC(),
			},
		},
		conversationsTotal: 1,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/conversations", nil)
	w := httptest.NewRecorder()
	h.AdminConversations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminConversations_PropagatesFilters(t *testing.T) {
	store := &fakeStorage{conversations: []storage.ConversationLog{}}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/conversations?tenant_id=tenant_a&jwt_sub=sub-1", nil)
	w := httptest.NewRecorder()
	h.AdminConversations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if store.lastConversationFilter.TenantID == nil || *store.lastConversationFilter.TenantID != "tenant_a" {
		t.Fatal("tenant filter not propagated")
	}
	if store.lastConversationFilter.JWTSub == nil || *store.lastConversationFilter.JWTSub != "sub-1" {
		t.Fatal("jwt_sub filter not propagated")
	}
}

// ── AdminRequestsStats ─────────────────────────────────────────────────────

func TestAdminRequestsStats_Returns200WithValidAdminKey(t *testing.T) {
	bucketTs := time.Date(2026, 3, 12, 18, 0, 0, 0, time.UTC)
	store := &fakeStorage{
		requestStats: storage.RequestStats{
			WindowHours: 24,
			Summary: storage.RequestStatsSummary{
				TotalRequests: 39, SuccessRate: 0.94, AvgLatencyMs: 245.2,
				FallbackRate: 0.05, FallbackRequests: 2, CacheHitRate: nil,
			},
			TrafficOverTime: []storage.TrafficBucket{
				{Bucket: bucketTs, Requests: 10, Successes: 9, Errors: 1},
			},
			ProviderHealth: []storage.ProviderHealthRow{
				{Provider: "openai", SuccessRate: 0.99, AvgLatencyMs: 210, TotalRequests: 25},
			},
			StatusBreakdown: map[string]int{"ok": 37, "error": 2},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests/stats", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Object      string `json:"object"`
		WindowHours int    `json:"window_hours"`
		Summary     struct {
			TotalRequests    int      `json:"total_requests"`
			SuccessRate      float64  `json:"success_rate"`
			AvgLatencyMs     float64  `json:"avg_latency_ms"`
			FallbackRate     float64  `json:"fallback_rate"`
			FallbackRequests int      `json:"fallback_requests"`
			CacheHitRate     *float64 `json:"cache_hit_rate"`
		} `json:"summary"`
		TrafficOverTime []map[string]any `json:"traffic_over_time"`
		ProviderHealth  []map[string]any `json:"provider_health"`
		StatusBreakdown map[string]int   `json:"status_breakdown"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "request_stats" {
		t.Errorf("object=%q, want request_stats", resp.Object)
	}
	if resp.WindowHours != 24 {
		t.Errorf("window_hours=%d, want 24", resp.WindowHours)
	}
	if resp.Summary.TotalRequests != 39 || resp.Summary.SuccessRate != 0.94 || resp.Summary.FallbackRequests != 2 {
		t.Errorf("summary: total=%d success_rate=%v fallback_requests=%d", resp.Summary.TotalRequests, resp.Summary.SuccessRate, resp.Summary.FallbackRequests)
	}
	if len(resp.TrafficOverTime) != 1 || resp.TrafficOverTime[0]["requests"] != float64(10) {
		t.Errorf("traffic_over_time: len=%d or requests wrong", len(resp.TrafficOverTime))
	}
	if len(resp.ProviderHealth) != 1 || resp.ProviderHealth[0]["provider"] != "openai" {
		t.Errorf("provider_health: len=%d or provider wrong", len(resp.ProviderHealth))
	}
	if resp.StatusBreakdown["success"] != 37 || resp.StatusBreakdown["error"] != 2 {
		t.Errorf("status_breakdown: success=%v error=%v", resp.StatusBreakdown["success"], resp.StatusBreakdown["error"])
	}
}

func TestAdminRequestsStats_DefaultsTo24hHour(t *testing.T) {
	store := &fakeStorage{
		requestStats: storage.RequestStats{
			WindowHours:     24,
			Summary:         storage.RequestStatsSummary{},
			TrafficOverTime: nil,
			ProviderHealth:  nil,
			StatusBreakdown: map[string]int{},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/requests/stats", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		WindowHours int `json:"window_hours"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.WindowHours != 24 {
		t.Errorf("window_hours=%d, want 24", resp.WindowHours)
	}
}

func TestAdminRequestsStats_SupportsTenantFilter(t *testing.T) {
	store := &fakeStorage{
		requestStats: storage.RequestStats{
			WindowHours: 24, StatusBreakdown: map[string]int{},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/requests/stats?tenant_id=tenant_a", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAdminRequestsStats_SupportsBucketMinute(t *testing.T) {
	store := &fakeStorage{
		requestStats: storage.RequestStats{
			WindowHours: 1, StatusBreakdown: map[string]int{},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}
	req := httptest.NewRequest(http.MethodGet, "/admin/requests/stats?window_hours=1&bucket=minute", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAdminRequestsStats_InvalidWindowHours(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
	for _, bad := range []string{"0", "721", "x"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/requests/stats?window_hours="+bad, nil)
		w := httptest.NewRecorder()
		h.AdminRequestsStats(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("window_hours=%q: expected 400, got %d", bad, w.Code)
		}
	}
}

// ── AdminRouterPerformance ───────────────────────────────────────────────────

func TestAdminRouterPerformance_Happy(t *testing.T) {
	store := &fakeStorage{
		routerPerformance: storage.RouterPerformanceMetrics{
			Summary: storage.RouterPerformanceSummary{
				Requests:             5,
				AvgRouterPreMs:       12.5,
				AvgLLMLatencyMs:      210.4,
				AvgRouterPostMs:      4.2,
				AvgTotalLatencyMs:    227.1,
				P50TotalLatencyMs:    220,
				P95TotalLatencyMs:    310,
				SuccessRate:          0.9,
				ErrorRate:            0.1,
				AvgPreTenantConfigMs: 2.1,
			},
			Timeseries: []storage.RouterPerformanceTimeseriesRow{
				{
					BucketStart:       time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC),
					Requests:          2,
					AvgRouterPreMs:    11,
					AvgLLMLatencyMs:   200,
					AvgRouterPostMs:   3,
					AvgTotalLatencyMs: 214,
					P95RouterPreMs:    18,
					P95LLMLatencyMs:   260,
					P95RouterPostMs:   6,
				},
			},
			Breakdowns: storage.RouterPerformanceBreakdowns{
				PreBreakdownAvgMs: storage.RouterPreBreakdownAvgMs{
					TenantConfig:  2.1,
					ToolRoutes:    0.0,
					DynamicRoutes: 0.0,
				},
				ToolRoutesBreakdownAvgMs: storage.RouterToolRoutesBreakdownAvgMs{
					EmbeddingModel: 0.4,
				},
			},
		},
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/router/performance?bucket=hour", nil)
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "", "admin", []string{"admin"}))
	w := httptest.NewRecorder()
	h.AdminRouterPerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["object"] != "router_performance" {
		t.Fatalf("object=%v, want router_performance", resp["object"])
	}

	filters, ok := resp["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters not found in response")
	}
	if filters["bucket"] != "hour" {
		t.Fatalf("filters.bucket=%v, want hour", filters["bucket"])
	}

	summary, ok := resp["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary not found in response")
	}
	if summary["requests"] != float64(5) {
		t.Fatalf("summary.requests=%v, want 5", summary["requests"])
	}

	timeseries, ok := resp["timeseries"].([]any)
	if !ok || len(timeseries) != 1 {
		t.Fatalf("timeseries length=%d, want 1", len(timeseries))
	}
	first, ok := timeseries[0].(map[string]any)
	if !ok {
		t.Fatalf("timeseries[0] not an object")
	}
	if first["bucket_start"] != "2026-03-26T12:00:00Z" {
		t.Fatalf("bucket_start=%v, want 2026-03-26T12:00:00Z", first["bucket_start"])
	}

	breakdowns, ok := resp["breakdowns"].(map[string]any)
	if !ok {
		t.Fatalf("breakdowns not found in response")
	}
	preBreakdown, ok := breakdowns["pre_breakdown_avg_ms"].(map[string]any)
	if !ok {
		t.Fatalf("pre_breakdown_avg_ms not found")
	}
	if preBreakdown["tenant_config"] != 2.1 {
		t.Fatalf("tenant_config=%v, want 2.1", preBreakdown["tenant_config"])
	}
}

func TestAdminRouterPerformance_BlocksNonAdminRoles(t *testing.T) {
	roles := []string{"financial", "local_admin", "user"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}
			req := httptest.NewRequest(http.MethodGet, "/admin/router/performance", nil)
			req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "", "sub", []string{role}))
			w := httptest.NewRecorder()
			h.AdminRouterPerformance(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for role %s, got %d", role, w.Code)
			}
		})
	}
}

func TestAdminRouterPerformance_InvalidParams(t *testing.T) {
	h := &Handlers{cfg: testAdminConfig(), store: &fakeStorage{}, log: testLoggerForAdmin()}

	badBucket := httptest.NewRequest(http.MethodGet, "/admin/router/performance?bucket=week", nil)
	badBucket = badBucket.WithContext(auth.WithJWTAdminContext(badBucket.Context(), "", "admin", []string{"admin"}))
	w := httptest.NewRecorder()
	h.AdminRouterPerformance(w, badBucket)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid bucket, got %d", w.Code)
	}

	badFrom := httptest.NewRequest(http.MethodGet, "/admin/router/performance?from=not-a-date", nil)
	badFrom = badFrom.WithContext(auth.WithJWTAdminContext(badFrom.Context(), "", "admin", []string{"admin"}))
	w = httptest.NewRecorder()
	h.AdminRouterPerformance(w, badFrom)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid from, got %d", w.Code)
	}

	badRange := httptest.NewRequest(http.MethodGet, "/admin/router/performance?from=2026-03-27T00:00:00Z&to=2026-03-26T00:00:00Z", nil)
	badRange = badRange.WithContext(auth.WithJWTAdminContext(badRange.Context(), "", "admin", []string{"admin"}))
	w = httptest.NewRecorder()
	h.AdminRouterPerformance(w, badRange)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid range, got %d", w.Code)
	}
}

func TestAdminRouterPerformance_PassesFiltersToStorage(t *testing.T) {
	store := &fakeStorage{}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/router/performance?from=2026-03-26T00:00:00Z&to=2026-03-26T12:00:00Z&tenant_id=t1&model=gpt-4o-mini&provider=openai&status=ok&bucket=day", nil)
	req = req.WithContext(auth.WithJWTAdminContext(req.Context(), "", "admin", []string{"admin"}))
	w := httptest.NewRecorder()
	h.AdminRouterPerformance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if store.lastRouterPerformanceFilter.Bucket != "day" {
		t.Fatalf("bucket=%v, want day", store.lastRouterPerformanceFilter.Bucket)
	}
	if store.lastRouterPerformanceFilter.TenantID == nil || *store.lastRouterPerformanceFilter.TenantID != "t1" {
		t.Fatalf("tenant_id not passed to storage")
	}
	if store.lastRouterPerformanceFilter.Model == nil || *store.lastRouterPerformanceFilter.Model != "gpt-4o-mini" {
		t.Fatalf("model not passed to storage")
	}
	if store.lastRouterPerformanceFilter.Provider == nil || *store.lastRouterPerformanceFilter.Provider != "openai" {
		t.Fatalf("provider not passed to storage")
	}
	if store.lastRouterPerformanceFilter.Status == nil || *store.lastRouterPerformanceFilter.Status != "ok" {
		t.Fatalf("status not passed to storage")
	}
	if store.lastRouterPerformanceFilter.From == nil || store.lastRouterPerformanceFilter.From.Format(time.RFC3339) != "2026-03-26T00:00:00Z" {
		t.Fatalf("from not passed to storage")
	}
	if store.lastRouterPerformanceFilter.To == nil || store.lastRouterPerformanceFilter.To.Format(time.RFC3339) != "2026-03-26T12:00:00Z" {
		t.Fatalf("to not passed to storage")
	}
}

// ── AdminRequestsRecent field exposure ──────────────────────────────────────

// TestAdminRequestsRecent_ExposesMetadataAndCallerFields verifies that metadata,
// api_key_name, and jwt_sub are included in the response when present.
func TestAdminRequestsRecent_ExposesMetadataAndCallerFields(t *testing.T) {
	ts := time.Now().UTC()
	meta := json.RawMessage(`{"observable":{"output.score":0.92,"input.features":{"amount":1000}}}`)
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{
				RequestID:  "req-ml-1",
				Timestamp:  ts,
				TenantID:   "tenant_a",
				Model:      "Fake",
				Provider:   "local",
				Strategy:   "ml",
				LatencyMs:  71,
				Status:     "ok",
				Attempt:    1,
				JWTSub:     "user-sub-abc",
				APIKeyName: "",
				Metadata:   meta,
			},
		},
		requestLogRecentTotal: 1,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent?strategy=ml", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsRecent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			Strategy   string          `json:"strategy"`
			APIKeyName *string         `json:"api_key_name"`
			JWTSub     *string         `json:"jwt_sub"`
			Metadata   json.RawMessage `json:"metadata"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Data))
	}
	row := resp.Data[0]
	if row.Strategy != "ml" {
		t.Errorf("strategy: want ml, got %q", row.Strategy)
	}
	if row.JWTSub == nil || *row.JWTSub != "user-sub-abc" {
		t.Errorf("jwt_sub: want user-sub-abc, got %v", row.JWTSub)
	}
	if row.APIKeyName != nil {
		t.Errorf("api_key_name: want nil for empty string, got %v", row.APIKeyName)
	}
	if len(row.Metadata) == 0 {
		t.Fatal("metadata must not be empty")
	}
	var obs map[string]interface{}
	if err := json.Unmarshal(row.Metadata, &obs); err != nil {
		t.Fatalf("metadata is not valid JSON: %v", err)
	}
	if obs["observable"] == nil {
		t.Error("metadata.observable must be present")
	}
}

// TestAdminRequestsRecent_APIKeyNameExposed verifies api_key_name is returned when set.
func TestAdminRequestsRecent_APIKeyNameExposed(t *testing.T) {
	ts := time.Now().UTC()
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{
				RequestID:  "req-api-1",
				Timestamp:  ts,
				TenantID:   "tenant_a",
				Model:      "gpt-4o-mini",
				Strategy:   "smart",
				Status:     "ok",
				Attempt:    1,
				APIKeyName: "my-key-name",
				JWTSub:     "",
			},
		},
		requestLogRecentTotal: 1,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsRecent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data []struct {
			APIKeyName *string `json:"api_key_name"`
			JWTSub     *string `json:"jwt_sub"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row")
	}
	if resp.Data[0].APIKeyName == nil || *resp.Data[0].APIKeyName != "my-key-name" {
		t.Errorf("api_key_name: want my-key-name, got %v", resp.Data[0].APIKeyName)
	}
	if resp.Data[0].JWTSub != nil {
		t.Errorf("jwt_sub: want nil for empty string, got %v", resp.Data[0].JWTSub)
	}
}

// TestAdminRequestsRecent_NullMetadataOmitted verifies that rows with no metadata
// do not include a metadata key in the response.
func TestAdminRequestsRecent_NullMetadataOmitted(t *testing.T) {
	ts := time.Now().UTC()
	store := &fakeStorage{
		requestLogRecent: []storage.RequestLogRecentRow{
			{RequestID: "req-1", Timestamp: ts, TenantID: "t1", Model: "gpt-4o-mini", Status: "ok", Attempt: 1},
		},
		requestLogRecentTotal: 1,
	}
	h := &Handlers{cfg: testAdminConfig(), store: store, log: testLoggerForAdmin()}

	req := httptest.NewRequest(http.MethodGet, "/admin/requests/recent", nil)
	w := httptest.NewRecorder()
	h.AdminRequestsRecent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Decode raw to check absence of metadata key
	var resp struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row")
	}
	if _, ok := resp.Data[0]["metadata"]; ok {
		t.Error("metadata key must be absent when nil")
	}
}
