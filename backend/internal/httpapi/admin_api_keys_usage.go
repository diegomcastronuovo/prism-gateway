package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminAPIKeysUsage returns aggregated API key usage for the FinOps dashboard.
// GET /admin/api-keys/usage?window_hours=720&tenant_id=...&provider=...&model=...&status=...&api_key_name=...&limit=50&offset=0
func (h *Handlers) AdminAPIKeysUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	windowHours := 720
	if v := q.Get("window_hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 720 {
			writeError(w, http.StatusBadRequest, "window_hours must be 1-720", "invalid_request_error")
			return
		}
		windowHours = n
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be 1-200", "invalid_request_error")
			return
		}
		limit = n
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "offset must be >= 0", "invalid_request_error")
			return
		}
		offset = n
	}

	filter := storage.APIKeyUsageFilter{
		WindowHours: windowHours,
		TenantID:    q.Get("tenant_id"),
		Provider:    q.Get("provider"),
		Model:       q.Get("model"),
		Status:      q.Get("status"),
		APIKeyName:  q.Get("api_key_name"),
		Limit:       limit,
		Offset:      offset,
	}

	ctx := r.Context()

	summary, rows, err := h.store.GetAPIKeyUsage(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get api key usage", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve API key usage", "internal_error")
		return
	}

	// Fetch per-(api_key_id, model) breakdown for effective cost computation.
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	modelBreakdown, _ := h.store.GetAPIKeyModelBreakdown(ctx, filter)

	// Index breakdown by api_key_id for O(1) lookup.
	breakdownByKey := make(map[uuid.UUID][]storage.APIKeyModelUsageRow, len(modelBreakdown))
	for _, b := range modelBreakdown {
		breakdownByKey[b.APIKeyID] = append(breakdownByKey[b.APIKeyID], b)
	}

	// Fetch tenant-level model request totals (denominator for infra allocation).
	// If no tenant filter, fetch across all tenants (tenantID="").
	tenantModelReqs, _ := h.store.GetTenantModelRequestCounts(ctx, filter.TenantID, since)
	allModels := h.resolveGlobalConfig(ctx).Models

	data := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		mon := computeMonetization(breakdownByKey[row.APIKeyID], tenantModelReqs, allModels)
		data = append(data, map[string]interface{}{
			"api_key_id":                     row.APIKeyID.String(),
			"api_key_name":                   row.APIKeyName,
			"tenant_id":                      row.TenantID,
			"requests":                       row.Requests,
			"spend":                          row.Spend,
			"avg_cost_per_request_effective": mon.AvgCostPerRequest,
			"avg_price_per_request":          mon.AvgPricePerRequest,
			"total_price":                    mon.TotalPrice,
			"margin":                         mon.Margin,
			"margin_pct":                     mon.MarginPct,
			"success_rate":                   row.SuccessRate,
			"avg_latency_ms":                 row.AvgLatencyMs,
			"top_model":                      row.TopModel,
			"top_provider":                   row.TopProvider,
			"last_seen":                      row.LastSeen.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "api_key_usage",
		"summary": map[string]interface{}{
			"total_active_api_keys": summary.TotalActiveAPIKeys,
			"total_requests":        summary.TotalRequests,
			"total_spend":           summary.TotalSpend,
			"avg_success_rate":      summary.AvgSuccessRate,
			"highest_spend_key":     summary.HighestSpendKey,
			"most_active_key":       summary.MostActiveKey,
		},
		"data": data,
	})
}
