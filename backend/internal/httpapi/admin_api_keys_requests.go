package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminAPIKeysRequests returns raw per-request activity attributed to API keys.
// GET /admin/api-keys/requests?from=...&to=...&tenant_id=...&api_key_name=...&model=...&provider=...&status=...&limit=50&offset=0
func (h *Handlers) AdminAPIKeysRequests(w http.ResponseWriter, r *http.Request) {
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

	filter := storage.APIKeyRawUsageFilter{
		From:       from,
		To:         to,
		TenantID:   q.Get("tenant_id"),
		APIKeyName: q.Get("api_key_name"),
		Model:      q.Get("model"),
		Provider:   q.Get("provider"),
		Status:     q.Get("status"),
		Limit:      limit,
		Offset:     offset,
	}

	rows, total, err := h.store.ListAPIKeyRawUsage(r.Context(), filter)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list api key raw usage", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve requests", "internal_error")
		return
	}

	data := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]interface{}{
			"timestamp":         row.Timestamp.UTC().Format(time.RFC3339),
			"tenant_id":         row.TenantID,
			"api_key_id":        row.APIKeyID.String(),
			"api_key_name":      row.APIKeyName,
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
		"object": "api_key_raw_usage",
		"data":   data,
		"pagination": map[string]interface{}{
			"limit":    limit,
			"offset":   offset,
			"returned": len(data),
			"total":    total,
		},
	})
}
