package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// jwtSubKey is used to index JWT sub model breakdown by (jwt_sub, tenant_id).
type jwtSubKey struct{ JWTSub, TenantID string }

// AdminJWTSubsUsage returns aggregated usage grouped by jwt_sub.
// GET /admin/jwt-subs/usage?from=...&to=...&tenant_id=...&model=...&provider=...&limit=50&offset=0&sort_by=cost_usd&sort_order=desc
func (h *Handlers) AdminJWTSubsUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339", "invalid_request_error")
			return
		}
		from = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339", "invalid_request_error")
			return
		}
		to = &t
	}
	if from != nil && to != nil && from.After(*to) {
		writeError(w, http.StatusBadRequest, "from must be <= to", "invalid_request_error")
		return
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

	sortBy := q.Get("sort_by")
	if sortBy == "" {
		sortBy = "cost_usd"
	}
	if sortBy != "cost_usd" && sortBy != "requests" && sortBy != "total_tokens" {
		writeError(w, http.StatusBadRequest, "sort_by must be cost_usd, requests, or total_tokens", "invalid_request_error")
		return
	}
	sortOrder := q.Get("sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		writeError(w, http.StatusBadRequest, "sort_order must be asc or desc", "invalid_request_error")
		return
	}

	filter := storage.JWTSubUsageFilter{
		From:      from,
		To:        to,
		TenantID:  q.Get("tenant_id"),
		Model:     q.Get("model"),
		Provider:  q.Get("provider"),
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	ctx := r.Context()
	rows, total, err := h.store.GetJWTSubUsage(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get jwt_sub usage", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve jwt_sub usage", "internal_error")
		return
	}

	// Fetch per-(jwt_sub, tenant_id, model) breakdown for monetization computation.
	modelBreakdown, _ := h.store.GetJWTSubModelBreakdown(ctx, filter)
	breakdownByJWT := make(map[jwtSubKey][]storage.APIKeyModelUsageRow, len(modelBreakdown))
	for _, b := range modelBreakdown {
		k := jwtSubKey{b.JWTSub, b.TenantID}
		breakdownByJWT[k] = append(breakdownByJWT[k], storage.APIKeyModelUsageRow{
			Model: b.Model, Requests: b.Requests, Spend: b.Spend,
		})
	}

	// Fetch tenant model request counts (denominator for infra allocation).
	since := time.Now().UTC().Add(-720 * time.Hour)
	if filter.From != nil {
		since = *filter.From
	}
	tenantModelReqs, _ := h.store.GetTenantModelRequestCounts(ctx, filter.TenantID, since)
	allModels := h.resolveGlobalConfig(ctx).Models

	data := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		k := jwtSubKey{row.JWTSub, row.TenantID}
		mon := computeMonetization(breakdownByJWT[k], tenantModelReqs, allModels)
		data = append(data, map[string]interface{}{
			"tenant_id":                      row.TenantID,
			"jwt_sub":                        row.JWTSub,
			"requests":                       row.Requests,
			"prompt_tokens":                  row.PromptTokens,
			"completion_tokens":              row.CompletionTokens,
			"total_tokens":                   row.TotalTokens,
			"total_cost_usd":                 row.TotalCostUSD,
			"avg_cost_per_request_effective": mon.AvgCostPerRequest,
			"avg_price_per_request":          mon.AvgPricePerRequest,
			"total_price":                    mon.TotalPrice,
			"margin":                         mon.Margin,
			"margin_pct":                     mon.MarginPct,
			"first_seen":                     row.FirstSeen.UTC().Format(time.RFC3339),
			"last_seen":                      row.LastSeen.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "jwt_sub_usage",
		"data":   data,
		"pagination": map[string]interface{}{
			"limit":    limit,
			"offset":   offset,
			"returned": len(data),
			"total":    total,
		},
	})
}

// AdminJWTSubUsageDetail returns summary and breakdown for a single jwt_sub.
// GET /admin/jwt-subs/{jwt_sub}/usage?from=...&to=...&tenant_id=...&group_by=model
func (h *Handlers) AdminJWTSubUsageDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	jwtSub := r.PathValue("jwt_sub")
	if jwtSub == "" {
		writeError(w, http.StatusBadRequest, "jwt_sub is required", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339", "invalid_request_error")
			return
		}
		from = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339", "invalid_request_error")
			return
		}
		to = &t
	}
	if from != nil && to != nil && from.After(*to) {
		writeError(w, http.StatusBadRequest, "from must be <= to", "invalid_request_error")
		return
	}

	groupBy := q.Get("group_by")
	if groupBy == "" {
		groupBy = "model"
	}
	if groupBy != "model" && groupBy != "provider" && groupBy != "day" {
		writeError(w, http.StatusBadRequest, "group_by must be model, provider, or day", "invalid_request_error")
		return
	}

	filter := storage.JWTSubUsageDetailFilter{
		From:     from,
		To:       to,
		TenantID: q.Get("tenant_id"),
		GroupBy:  groupBy,
	}

	summary, breakdown, err := h.store.GetJWTSubUsageDetail(r.Context(), jwtSub, filter)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get jwt_sub usage detail", "jwt_sub", jwtSub, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve jwt_sub usage detail", "internal_error")
		return
	}

	breakdownRows := make([]map[string]interface{}, 0, len(breakdown))
	for _, row := range breakdown {
		breakdownRows = append(breakdownRows, map[string]interface{}{
			"group":          row.Group,
			"requests":       row.Requests,
			"total_tokens":   row.TotalTokens,
			"total_cost_usd": row.TotalCostUSD,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object":  "jwt_sub_usage_detail",
		"jwt_sub": jwtSub,
		"summary": map[string]interface{}{
			"requests":          summary.Requests,
			"prompt_tokens":     summary.PromptTokens,
			"completion_tokens": summary.CompletionTokens,
			"total_tokens":      summary.TotalTokens,
			"total_cost_usd":    summary.TotalCostUSD,
		},
		"breakdown": breakdownRows,
	})
}

// AdminJWTSubsRequests returns raw request activity attributed to jwt_sub.
// GET /admin/jwt-subs/requests?from=...&to=...&tenant_id=...&jwt_sub=...&model=...&provider=...&status=...&limit=50&offset=0
func (h *Handlers) AdminJWTSubsRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339", "invalid_request_error")
			return
		}
		from = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339", "invalid_request_error")
			return
		}
		to = &t
	}
	if from != nil && to != nil && from.After(*to) {
		writeError(w, http.StatusBadRequest, "from must be <= to", "invalid_request_error")
		return
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

	filter := storage.JWTSubRawUsageFilter{
		From:     from,
		To:       to,
		TenantID: q.Get("tenant_id"),
		JWTSub:   q.Get("jwt_sub"),
		Model:    q.Get("model"),
		Provider: q.Get("provider"),
		Status:   q.Get("status"),
		Limit:    limit,
		Offset:   offset,
	}

	rows, total, err := h.store.ListJWTSubRawUsage(r.Context(), filter)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list jwt_sub raw usage", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve requests", "internal_error")
		return
	}

	data := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]interface{}{
			"timestamp":         row.Timestamp.UTC().Format(time.RFC3339),
			"tenant_id":         row.TenantID,
			"jwt_sub":           row.JWTSub,
			"request_id":        row.RequestID,
			"model":             row.Model,
			"provider":          row.Provider,
			"status":            row.Status,
			"latency_ms":        row.LatencyMs,
			"cost_usd":          row.CostUSD,
			"prompt_tokens":     row.PromptTokens,
			"completion_tokens": row.CompletionTokens,
			"total_tokens":      row.TotalTokens,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "jwt_sub_raw_usage",
		"data":   data,
		"pagination": map[string]interface{}{
			"limit":    limit,
			"offset":   offset,
			"returned": len(data),
			"total":    total,
		},
	})
}
