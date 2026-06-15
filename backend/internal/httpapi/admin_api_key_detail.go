package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminAPIKeyUsageDetail returns full usage drilldown for one API key.
// GET /admin/api-keys/{api_key_id}/usage?window_hours=720&limit=50&offset=0
func (h *Handlers) AdminAPIKeyUsageDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	apiKeyIDStr := r.PathValue("api_key_id")
	if apiKeyIDStr == "" {
		writeError(w, http.StatusBadRequest, "api_key_id is required", "invalid_request_error")
		return
	}
	apiKeyID, err := uuid.Parse(apiKeyIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "api_key_id must be a valid UUID", "invalid_request_error")
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

	// Resolve key meta for response (and 404 if key does not exist)
	meta, found, err := h.store.GetAPIKeyMetaByID(r.Context(), apiKeyID)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get api key meta", "api_key_id", apiKeyID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve API key", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "API key not found", "not_found")
		return
	}

	ctx := r.Context()

	summary, byModel, byProvider, recent, totalRecent, latencyStats, errorsByType, err := h.store.GetAPIKeyUsageDetail(ctx, apiKeyID, windowHours, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get api key usage detail", "api_key_id", apiKeyID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve usage detail", "internal_error")
		return
	}

	// Compute cost + price + margin (token cost + infra allocation + markup).
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	tenantModelReqs, _ := h.store.GetTenantModelRequestCounts(ctx, meta.TenantID, since)
	allModels := h.resolveGlobalConfig(ctx).Models

	// Convert byModel to APIKeyModelUsageRow slice for the shared monetization helper.
	breakdown := make([]storage.APIKeyModelUsageRow, 0, len(byModel))
	for _, m := range byModel {
		breakdown = append(breakdown, storage.APIKeyModelUsageRow{Model: m.Model, Requests: m.Requests, Spend: m.Spend})
	}
	mon := computeMonetization(breakdown, tenantModelReqs, allModels)

	lastSeenStr := ""
	if !summary.LastSeen.IsZero() {
		lastSeenStr = summary.LastSeen.UTC().Format(time.RFC3339)
	}

	summaryMap := map[string]interface{}{
		"requests":                       summary.Requests,
		"spend":                          summary.Spend,
		"avg_cost_per_request_effective": mon.AvgCostPerRequest,
		"avg_price_per_request":          mon.AvgPricePerRequest,
		"total_price":                    mon.TotalPrice,
		"margin":                         mon.Margin,
		"margin_pct":                     mon.MarginPct,
		"success_rate":                   summary.SuccessRate,
		"avg_latency_ms":                 summary.AvgLatencyMs,
		"top_model":                      summary.TopModel,
		"top_provider":                   summary.TopProvider,
		"last_seen":                      lastSeenStr,
	}

	requestsByModel := make([]map[string]interface{}, 0, len(byModel))
	for _, row := range byModel {
		if row.Requests == 0 {
			requestsByModel = append(requestsByModel, map[string]interface{}{
				"model":                          row.Model,
				"requests":                       row.Requests,
				"spend":                          row.Spend,
				"effective_spend":                0.0,
				"avg_cost_per_request_effective": 0.0,
				"avg_price_per_request":          0.0,
				"total_price":                    0.0,
				"margin":                         0.0,
				"margin_pct":                     0.0,
			})
			continue
		}
		singleModel := []storage.APIKeyModelUsageRow{{Model: row.Model, Requests: row.Requests, Spend: row.Spend}}
		mMon := computeMonetization(singleModel, tenantModelReqs, allModels)
		requestsByModel = append(requestsByModel, map[string]interface{}{
			"model":                          row.Model,
			"requests":                       row.Requests,
			"spend":                          row.Spend,
			"effective_spend":                mMon.TotalEffectiveCost,
			"avg_cost_per_request_effective":   mMon.AvgCostPerRequest,
			"avg_price_per_request":          mMon.AvgPricePerRequest,
			"total_price":                    mMon.TotalPrice,
			"margin":                         mMon.Margin,
			"margin_pct":                     mMon.MarginPct,
		})
	}

	requestsByProvider := make([]map[string]interface{}, 0, len(byProvider))
	for _, row := range byProvider {
		requestsByProvider = append(requestsByProvider, map[string]interface{}{
			"provider": row.Provider,
			"requests": row.Requests,
		})
	}

	recentList := make([]map[string]interface{}, 0, len(recent))
	for _, row := range recent {
		recentList = append(recentList, map[string]interface{}{
			"timestamp":   row.Timestamp.UTC().Format(time.RFC3339),
			"request_id":  row.RequestID,
			"model":       row.Model,
			"provider":    row.Provider,
			"status":      row.Status,
			"latency_ms":  row.LatencyMs,
			"cost_usd":    row.CostUSD,
		})
	}

	errorsByTypeList := make([]map[string]interface{}, 0, len(errorsByType))
	for _, row := range errorsByType {
		errorsByTypeList = append(errorsByTypeList, map[string]interface{}{
			"error_type": row.ErrorType,
			"count":      row.Count,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"api_key_id":            apiKeyID.String(),
		"api_key_name":          meta.Name,
		"tenant_id":             meta.TenantID,
		"summary":               summaryMap,
		"latency_stats":         map[string]interface{}{"p50": latencyStats.P50, "p95": latencyStats.P95, "max": latencyStats.Max},
		"errors_by_type":        errorsByTypeList,
		"requests_by_model":     requestsByModel,
		"requests_by_provider":  requestsByProvider,
		"recent_requests":       recentList,
		"pagination": map[string]interface{}{
			"limit":    limit,
			"offset":   offset,
			"returned": len(recentList),
			"total":    totalRecent,
		},
	})
}
