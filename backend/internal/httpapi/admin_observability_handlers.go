package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminTenantUsage returns total request count, token count, and cost for a tenant.
// GET /admin/tenants/{tenant_id}/usage?month=YYYY-MM
func (h *Handlers) AdminTenantUsage(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	monthStr := r.URL.Query().Get("month")
	if monthStr == "" {
		monthStr = time.Now().Format("2006-01")
	}
	month, err := time.Parse("2006-01", monthStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid month format, expected YYYY-MM", "invalid_request_error")
		return
	}
	from := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)

	ov, err := h.store.GetTenantUsageOverview(r.Context(), tenantID, from, to)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get tenant usage overview", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to retrieve usage data", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"month":     monthStr,
		"requests":  ov.TotalRequests,
		"tokens":    ov.TotalTokens,
		"cost_usd":  ov.TotalCostUSD,
	})
}

// AdminRoutingStats returns model and route group request counts.
// GET /admin/routing/stats?window_days=30
func (h *Handlers) AdminRoutingStats(w http.ResponseWriter, r *http.Request) {
	windowDays := 30
	if v := r.URL.Query().Get("window_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 90 {
			windowDays = n
		} else if err != nil || n < 1 || n > 90 {
			writeError(w, http.StatusBadRequest, "window_days must be between 1 and 90", "invalid_request_error")
			return
		}
	}

	modelCounts, err := h.store.GetModelRequestCounts(r.Context(), windowDays)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get model request counts", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve routing stats", "internal_error")
		return
	}

	// Derive route group counts from config: sum model counts across each route group.
	routes := make(map[string]int64)
	for _, t := range h.cfg.Tenants {
		for groupName, models := range t.Selection.RouteGroups {
			for _, m := range models {
				if count, ok := modelCounts[m]; ok {
					routes[groupName] += count
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models": modelCounts,
		"routes": routes,
	})
}

// AdminListRequests returns recent requests for debugging.
// GET /admin/requests?tenant_id=...&window_hours=24&limit=50&offset=0
// window_hours: 1, 24, 168 (7d), 720 (30d); default 24 when omitted.
func (h *Handlers) AdminListRequests(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")

	// Enforce tenant scope: non-super-admin callers may only query their own tenant.
	if auth.AuthTypeFromContext(r.Context()) != "admin_token" && tenantID != "" {
		callerTenant := auth.TenantIDFromContext(r.Context())
		allowed := tenantID == callerTenant
		if !allowed {
			for _, t := range auth.AllowedTenantsFromContext(r.Context()) {
				if t == tenantID {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "access denied", "authorization_error")
			return
		}
	}

	windowHours := 24
	if v := r.URL.Query().Get("window_hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 720 {
			windowHours = n
		}
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	rows, hasMore, err := h.store.ListRecentRequests(r.Context(), tenantID, windowHours, limit, offset)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list recent requests", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve requests", "internal_error")
		return
	}

	type requestItem struct {
		RequestID      string  `json:"request_id"`
		TenantID       string  `json:"tenant_id"`
		Model          string  `json:"model"`
		Provider       string  `json:"provider"`
		Status         string  `json:"status"`           // "success" | "error"
		LatencyMs      int     `json:"latency_ms"`
		Strategy       string  `json:"strategy"`
		FallbackUsed   bool    `json:"fallback_used"`
		CacheHit       bool    `json:"cache_hit"`
		ErrorType      *string `json:"error_type"`
		DecisionReason *string `json:"decision_reason"`
		Created        int64   `json:"created"` // Unix timestamp
	}
	items := make([]requestItem, 0, len(rows))
	for _, r := range rows {
		status := r.Status
		if status == "ok" {
			status = "success"
		}
		items = append(items, requestItem{
			RequestID:      r.RequestID,
			TenantID:       r.TenantID,
			Model:          r.Model,
			Provider:       r.Provider,
			Status:         status,
			LatencyMs:      r.LatencyMs,
			Strategy:       r.Strategy,
			FallbackUsed:   r.FallbackUsed,
			CacheHit:       r.CacheHit,
			ErrorType:      r.ErrorType,
			DecisionReason: r.DecisionReason,
			Created:        r.CreatedAt.Unix(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"data":     items,
		"has_more": hasMore,
	})
}

// AdminRequestsRecent returns paginated recent requests from request_log for observability.
// GET /admin/requests/recent?tenant_id=...&model=...&provider=...&status=...&fallback_used=...&from=...&to=...&limit=50&offset=0
func (h *Handlers) AdminRequestsRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	filter := storage.RequestLogRecentFilter{}

	if v := q.Get("tenant_id"); v != "" {
		filter.TenantID = &v
	}
	if v := q.Get("jwt_sub"); v != "" {
		filter.JWTSub = &v
	}
	if v := q.Get("model"); v != "" {
		filter.Model = &v
	}
	if v := q.Get("provider"); v != "" {
		filter.Provider = &v
	}
	if v := q.Get("status"); v != "" {
		filter.Status = &v
	}
	if v := q.Get("fallback_used"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "fallback_used must be true or false", "invalid_request_error")
			return
		}
		filter.FallbackUsed = &b
	}
	// window_hours is a convenience alternative to from/to (ignored if from is explicitly set).
	if v := q.Get("window_hours"); v != "" && q.Get("from") == "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 720 {
			t := time.Now().UTC().Add(-time.Duration(n) * time.Hour)
			filter.From = &t
		}
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.To = &t
	}
	// SPEC_170: workflow/conversation drill-down filters
	if v := q.Get("workflow_id"); v != "" {
		filter.WorkflowID = &v
	}
	if v := q.Get("conversation_id"); v != "" {
		filter.ConversationID = &v
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

	rows, total, err := h.store.ListRequestLogRecent(r.Context(), filter, limit, offset)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list request log recent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve requests", "internal_error")
		return
	}

	type cacheStatus struct {
		Status string `json:"status"`
	}
	type dataItem struct {
		RequestID        string          `json:"request_id"`
		Timestamp        string          `json:"timestamp"`
		TenantID         string          `json:"tenant_id"`
		Model            string          `json:"model"`
		Provider         string          `json:"provider"`
		Strategy         string          `json:"strategy"`
		LatencyMs        int             `json:"latency_ms"`
		Status           string          `json:"status"`
		FallbackUsed     bool            `json:"fallback_used"`
		Attempt          int             `json:"attempt"`
		Cache            cacheStatus     `json:"cache"`
		APIKeyName       *string         `json:"api_key_name"`
		JWTSub           *string         `json:"jwt_sub"`
		Metadata         json.RawMessage `json:"metadata,omitempty"`
		DecisionReason   *string         `json:"decision_reason,omitempty"`
		ErrorType        *string         `json:"error_type,omitempty"`
		RoutingSnapshot  json.RawMessage `json:"routing_snapshot,omitempty"`
		DecisionSnapshot json.RawMessage `json:"decision_snapshot,omitempty"`
		// SPEC_170: workflow/conversation context
		WorkflowID     *string `json:"workflow_id,omitempty"`
		ConversationID *string `json:"conversation_id,omitempty"`
	}
	data := make([]dataItem, 0, len(rows))
	for _, row := range rows {
		var apiKeyName *string
		if row.APIKeyName != "" {
			s := row.APIKeyName
			apiKeyName = &s
		}
		var jwtSub *string
		if row.JWTSub != "" {
			s := row.JWTSub
			jwtSub = &s
		}
		var decisionReason, errorType *string
		if row.DecisionReason != "" {
			s := row.DecisionReason
			decisionReason = &s
		}
		if row.ErrorType != "" {
			s := row.ErrorType
			errorType = &s
		}
		var workflowID, conversationID *string
		if row.WorkflowID != "" {
			s := row.WorkflowID
			workflowID = &s
		}
		if row.ConversationID != "" {
			s := row.ConversationID
			conversationID = &s
		}
		data = append(data, dataItem{
			RequestID:        row.RequestID,
			Timestamp:        row.Timestamp.UTC().Format(time.RFC3339),
			TenantID:         row.TenantID,
			Model:            row.Model,
			Provider:         row.Provider,
			Strategy:         row.Strategy,
			LatencyMs:        row.LatencyMs,
			Status:           row.Status,
			FallbackUsed:     row.FallbackUsed,
			Attempt:          row.Attempt,
			Cache:            cacheStatus{Status: "unknown"},
			APIKeyName:       apiKeyName,
			JWTSub:           jwtSub,
			Metadata:         row.Metadata,
			DecisionReason:   decisionReason,
			ErrorType:        errorType,
			RoutingSnapshot:  row.RoutingSnapshot,
			DecisionSnapshot: row.DecisionSnapshot,
			WorkflowID:       workflowID,
			ConversationID:   conversationID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
		"pagination": map[string]any{
			"limit":   limit,
			"offset":  offset,
			"returned": len(data),
			"total":   total,
		},
	})
}

// AdminRequestsStats returns aggregated request telemetry for the observability dashboard.
// GET /admin/requests/stats?tenant_id=...&window_hours=24&bucket=hour
func (h *Handlers) AdminRequestsStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	tenantID := q.Get("tenant_id")

	windowHours := 24
	if v := q.Get("window_hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 720 {
			writeError(w, http.StatusBadRequest, "window_hours must be 1-720", "invalid_request_error")
			return
		}
		windowHours = n
	}
	bucket := "hour"
	if v := q.Get("bucket"); v != "" {
		if v == "minute" || v == "hour" {
			bucket = v
		}
		// else keep default hour (clamp invalid to hour)
	}

	stats, err := h.store.GetRequestStats(r.Context(), tenantID, windowHours, bucket)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get request stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve request stats", "internal_error")
		return
	}

	// Map status_breakdown: DB has "ok"/"error", spec example uses "success"/"error"
	statusBreakdown := make(map[string]int)
	for k, v := range stats.StatusBreakdown {
		if k == "ok" {
			statusBreakdown["success"] = statusBreakdown["success"] + v
		} else {
			statusBreakdown[k] = statusBreakdown[k] + v
		}
	}

	traffic := make([]map[string]any, 0, len(stats.TrafficOverTime))
	for _, b := range stats.TrafficOverTime {
		traffic = append(traffic, map[string]any{
			"bucket":    b.Bucket.UTC().Format(time.RFC3339),
			"requests": b.Requests,
			"successes": b.Successes,
			"errors":    b.Errors,
		})
	}
	providerHealth := make([]map[string]any, 0, len(stats.ProviderHealth))
	for _, p := range stats.ProviderHealth {
		providerHealth = append(providerHealth, map[string]any{
			"provider":       p.Provider,
			"success_rate":   p.SuccessRate,
			"avg_latency_ms": p.AvgLatencyMs,
			"total_requests": p.TotalRequests,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object":        "request_stats",
		"window_hours":  stats.WindowHours,
		"summary": map[string]any{
			"total_requests":    stats.Summary.TotalRequests,
			"success_rate":      stats.Summary.SuccessRate,
			"avg_latency_ms":    stats.Summary.AvgLatencyMs,
			"fallback_rate":     stats.Summary.FallbackRate,
			"fallback_requests": stats.Summary.FallbackRequests,
			"cache_hit_rate":    stats.Summary.CacheHitRate,
		},
		"traffic_over_time": traffic,
		"provider_health":   providerHealth,
		"status_breakdown":  statusBreakdown,
	})
}

// AdminAuditRequests returns audit-oriented request rows from request_log.
// GET /admin/audit/requests?from=...&to=...&tenant_id=...&jwt_sub=...&status=...
func (h *Handlers) AdminAuditRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	filter := storage.RequestLogRecentFilter{}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.To = &t
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		writeError(w, http.StatusBadRequest, "from must be <= to", "invalid_request_error")
		return
	}
	if v := q.Get("tenant_id"); v != "" {
		filter.TenantID = &v
	}
	if v := q.Get("jwt_sub"); v != "" {
		filter.JWTSub = &v
	}
	if v := q.Get("status"); v != "" {
		filter.Status = &v
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

	rows, _, err := h.store.ListRequestLogRecent(r.Context(), filter, limit, offset)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list audit requests", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve audit requests", "internal_error")
		return
	}

	type auditItem struct {
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
	data := make([]auditItem, 0, len(rows))
	for _, row := range rows {
		actor := row.JWTSub
		if actor == "" {
			actor = row.APIKeyName
		}
		if actor == "" {
			actor = row.APIKeyID
		}
		data = append(data, auditItem{
			Timestamp:      row.Timestamp.UTC().Format(time.RFC3339),
			TenantID:       row.TenantID,
			JWTSub:         row.JWTSub,
			Actor:          actor,
			Model:          row.Model,
			Provider:       row.Provider,
			Strategy:       row.Strategy,
			Status:         row.Status,
			LatencyMs:      row.LatencyMs,
			RequestID:      row.RequestID,
			Decision:       row.DecisionReason,
			DecisionReason: row.DecisionReason,
			ErrorType:      row.ErrorType,
			Error:          row.Error,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
	})
}

// AdminComplianceEvents returns compliance events from compliance_event_log.
// GET /admin/compliance/events?from=...&to=...&tenant_id=...&event_type=...&request_id=...
func (h *Handlers) AdminComplianceEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	q := r.URL.Query()
	filter := storage.ComplianceEventFilter{}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.To = &t
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		writeError(w, http.StatusBadRequest, "from must be <= to", "invalid_request_error")
		return
	}
	if v := q.Get("tenant_id"); v != "" {
		filter.TenantID = &v
	}
	if v := q.Get("request_id"); v != "" {
		filter.RequestID = &v
	}
	if v := q.Get("event_type"); v != "" {
		filter.EventType = &v
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

	rows, _, err := h.store.ListComplianceEvents(r.Context(), filter, limit, offset)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list compliance events", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve compliance events", "internal_error")
		return
	}

	type eventItem struct {
		ID          string         `json:"id"`
		TenantID    string         `json:"tenant_id"`
		RequestID   string         `json:"request_id"`
		EventType   string         `json:"event_type"`
		ActionTaken string         `json:"action_taken"`
		Metadata    map[string]any `json:"metadata,omitempty"`
		CreatedAt   string         `json:"created_at"`
	}
	data := make([]eventItem, 0, len(rows))
	for _, row := range rows {
		var meta map[string]any
		if len(row.Metadata) > 0 {
			_ = json.Unmarshal(row.Metadata, &meta)
		}
		data = append(data, eventItem{
			ID:          row.ID.String(),
			TenantID:    row.TenantID,
			RequestID:   row.RequestID,
			EventType:   row.EventType,
			ActionTaken: row.ActionTaken,
			Metadata:    meta,
			CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
	})
}

// AdminConversations returns conversation logs for Logs/Compliance surfaces.
// GET /admin/conversations?from=...&to=...&tenant_id=...&jwt_sub=...
func (h *Handlers) AdminConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}
	q := r.URL.Query()
	filter := storage.ConversationLogFilter{}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339 timestamp", "invalid_request_error")
			return
		}
		filter.To = &t
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		writeError(w, http.StatusBadRequest, "from must be <= to", "invalid_request_error")
		return
	}
	if v := q.Get("tenant_id"); v != "" {
		filter.TenantID = &v
	}
	if v := q.Get("jwt_sub"); v != "" {
		filter.JWTSub = &v
	}
	if v := q.Get("workflow_id"); v != "" {
		filter.WorkflowID = &v
	}
	if v := q.Get("conversation_id"); v != "" {
		filter.ConversationID = &v
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
	rows, total, err := h.store.ListConversations(r.Context(), filter, limit, offset)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list conversations", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve conversations", "internal_error")
		return
	}

	type conversationLogItem struct {
		ID               string  `json:"id"`
		RequestID        string  `json:"request_id"`
		TenantID         string  `json:"tenant_id"`
		JWTSub           *string `json:"jwt_sub,omitempty"`
		WorkflowID       *string `json:"workflow_id,omitempty"`
		ConversationID   *string `json:"conversation_id,omitempty"`
		CustomerID       *string `json:"customer_id,omitempty"`
		PromptPreview    string  `json:"prompt_preview"`
		ResponsePreview  string  `json:"response_preview"`
		PromptRedacted   *string `json:"prompt_redacted,omitempty"`
		ResponseRedacted *string `json:"response_redacted,omitempty"`
		PromptFull       *string `json:"prompt_full,omitempty"`
		ResponseFull     *string `json:"response_full,omitempty"`
		PIIDetected      bool    `json:"pii_detected"`
		LoggingMode      string  `json:"logging_mode"`
		CreatedAt        string  `json:"created_at"`
	}
	items := make([]conversationLogItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, conversationLogItem{
			ID:               r.ID.String(),
			RequestID:        r.RequestID,
			TenantID:         r.TenantID,
			JWTSub:           r.JWTSub,
			WorkflowID:       r.WorkflowID,
			ConversationID:   r.ConversationID,
			CustomerID:       r.CustomerID,
			PromptPreview:    r.PromptPreview,
			ResponsePreview:  r.ResponsePreview,
			PromptRedacted:   r.PromptRedacted,
			ResponseRedacted: r.ResponseRedacted,
			PromptFull:       r.PromptFull,
			ResponseFull:     r.ResponseFull,
			PIIDetected:      r.PIIDetected,
			LoggingMode:      r.LoggingMode,
			CreatedAt:        r.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": total})
}

// AdminSemanticCacheStats returns aggregated semantic cache analytics for the observability dashboard.
// GET /admin/observability/semantic-cache?tenant_id=...&limit=10
func (h *Handlers) AdminSemanticCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 {
			writeError(w, http.StatusBadRequest, "limit must be 1-50", "invalid_request_error")
			return
		}
		limit = n
	}

	stats, err := h.store.GetSemanticCacheStats(r.Context(), tenantID, limit)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get semantic cache stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve semantic cache stats", "internal_error")
		return
	}

	topPrompts := make([]map[string]any, 0, len(stats.TopPrompts))
	for _, p := range stats.TopPrompts {
		lastHitAt := ""
		if p.LastHitAt != nil {
			lastHitAt = p.LastHitAt.UTC().Format(time.RFC3339)
		}
		topPrompts = append(topPrompts, map[string]any{
			"request_text": p.RequestText,
			"hit_count":    p.HitCount,
			"last_hit_at":  lastHitAt,
			"expires_at":   p.ExpiresAt.UTC().Format(time.RFC3339),
			"model":        p.Model,
			"route_group":  p.RouteGroup,
		})
	}
	topModels := make([]map[string]any, 0, len(stats.TopModels))
	for _, m := range stats.TopModels {
		topModels = append(topModels, map[string]any{
			"model":       m.Model,
			"entries":     m.Entries,
			"total_hits":  m.TotalHits,
		})
	}
	topRouteGroups := make([]map[string]any, 0, len(stats.TopRouteGroups))
	for _, rg := range stats.TopRouteGroups {
		topRouteGroups = append(topRouteGroups, map[string]any{
			"route_group": rg.RouteGroup,
			"entries":     rg.Entries,
			"total_hits":  rg.TotalHits,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "semantic_cache_stats",
		"summary": map[string]any{
			"total_entries":     stats.Summary.TotalEntries,
			"total_hits":        stats.Summary.TotalHits,
			"avg_hits_per_entry": stats.Summary.AvgHitsPerEntry,
			"active_entries":    stats.Summary.ActiveEntries,
			"expired_entries":   stats.Summary.ExpiredEntries,
		},
		"top_prompts":     topPrompts,
		"top_models":      topModels,
		"top_route_groups": topRouteGroups,
		"expiration": map[string]any{
			"active":  stats.Expiration.Active,
			"expired": stats.Expiration.Expired,
		},
	})
}

// AdminSemanticRoutingStats returns semantic routing analytics (top routes, top anchors, coverage).
// GET /admin/observability/semantic-routing?tenant_id=...&window_days=30
func (h *Handlers) AdminSemanticRoutingStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	windowDays := 30
	if v := r.URL.Query().Get("window_days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 90 {
			writeError(w, http.StatusBadRequest, "window_days must be 1-90", "invalid_request_error")
			return
		}
		windowDays = n
	}

	stats, err := h.store.GetSemanticRoutingStats(r.Context(), tenantID, windowDays)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get semantic routing stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve semantic routing stats", "internal_error")
		return
	}

	topRoutes := make([]map[string]any, 0, len(stats.TopRoutes))
	for _, r := range stats.TopRoutes {
		topRoutes = append(topRoutes, map[string]any{"route_group": r.RouteGroup, "matches": r.Matches})
	}
	topAnchors := make([]map[string]any, 0, len(stats.TopAnchors))
	for _, a := range stats.TopAnchors {
		topAnchors = append(topAnchors, map[string]any{"anchor": a.Anchor, "matches": a.Matches})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object":       "semantic_routing_stats",
		"top_routes":   topRoutes,
		"top_anchors":  topAnchors,
		"coverage": map[string]any{
			"total_requests":   stats.Coverage.TotalRequests,
			"matched_requests": stats.Coverage.MatchedRequests,
			"coverage_rate":    stats.Coverage.CoverageRate,
		},
	})
}

// AdminSemanticCorrelation returns correlation of semantic cache hits and requests by route_group.
// GET /admin/observability/semantic-correlation?tenant_id=...&window_days=30
func (h *Handlers) AdminSemanticCorrelation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	windowDays := 30
	if v := r.URL.Query().Get("window_days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 90 {
			writeError(w, http.StatusBadRequest, "window_days must be 1-90", "invalid_request_error")
			return
		}
		windowDays = n
	}

	corr, err := h.store.GetSemanticCorrelation(r.Context(), tenantID, windowDays)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get semantic correlation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve semantic correlation", "internal_error")
		return
	}

	byRouteGroup := make([]map[string]any, 0, len(corr.ByRouteGroup))
	for _, row := range corr.ByRouteGroup {
		byRouteGroup = append(byRouteGroup, map[string]any{
			"route_group":     row.RouteGroup,
			"cache_hits":      row.CacheHits,
			"total_requests":  row.TotalRequests,
			"hit_rate":        row.HitRate,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object":         "semantic_correlation",
		"by_route_group": byRouteGroup,
	})
}
