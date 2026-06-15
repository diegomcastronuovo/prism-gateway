package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
)

// GetRoutingSnapshot returns the flat routing decision for a successful request.
// GET /admin/requests/{request_id}/routing
func (h *Handlers) GetRoutingSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	requestID := r.PathValue("request_id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "request_id is required", "invalid_request_error")
		return
	}

	tenantID, snapshotJSON, found, err := h.store.GetRoutingSnapshot(ctx, requestID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch routing snapshot", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, "failed to fetch routing snapshot", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "routing snapshot not found", "not_found")
		return
	}

	// Tenant isolation: api_key callers may only access their own tenant's snapshots.
	if auth.AuthTypeFromContext(ctx) == "api_key" {
		callerTenantID := auth.TenantIDFromContext(ctx)
		if callerTenantID != tenantID {
			writeError(w, http.StatusForbidden, "access denied", "authorization_error")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"request_id":       requestID,
		"tenant_id":        tenantID,
		"routing_snapshot": json.RawMessage(snapshotJSON),
	})
}

// ReplayRequest returns the resolved routing intent for a previous successful request.
// POST /admin/replay/{request_id}?mode=deterministic
// Response includes full diagnostics: decision_reason, decision_snapshot, route_group, routing_strategy, fallback_attempts.
func (h *Handlers) ReplayRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	requestID := r.PathValue("request_id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "request_id is required", "invalid_request_error")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode != "deterministic" {
		writeError(w, http.StatusBadRequest, "missing or invalid query parameter: mode must be 'deterministic'", "invalid_request_error")
		return
	}

	diag, found, err := h.store.GetReplayDiagnostics(ctx, requestID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch replay diagnostics", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, "failed to fetch replay diagnostics", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "routing snapshot not found", "not_found")
		return
	}

	// Tenant isolation: api_key callers may only replay their own tenant's requests.
	if auth.AuthTypeFromContext(ctx) == "api_key" {
		callerTenantID := auth.TenantIDFromContext(ctx)
		if callerTenantID != diag.TenantID {
			writeError(w, http.StatusForbidden, "access denied", "authorization_error")
			return
		}
	}

	var snap router.RoutingSnapshot
	if err := json.Unmarshal(diag.RoutingSnapshot, &snap); err != nil {
		h.log.ErrorContext(ctx, "failed to unmarshal routing snapshot", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, "failed to parse routing snapshot", "internal_error")
		return
	}

	// Validate that the stored model still exists.
	if h.resolveModelByName(ctx, snap.SelectedModel) == nil {
		writeError(w, http.StatusUnprocessableEntity,
			"stored model no longer exists: "+snap.SelectedModel,
			"invalid_request_error")
		return
	}

	// Derive top-level fields: prefer stored (strategy from row), else from routing_snapshot
	routeGroup := (*string)(nil)
	if snap.RouteGroup != "" {
		routeGroup = &snap.RouteGroup
	}
	routingStrategy := (*string)(nil)
	if snap.RoutingStrategy != "" {
		routingStrategy = &snap.RoutingStrategy
	}
	if diag.Strategy != "" && routingStrategy == nil {
		routingStrategy = &diag.Strategy
	}
	fallbackAttempts := (*int)(nil)
	fallbackAttemptsVal := snap.FallbackAttempts
	fallbackAttempts = &fallbackAttemptsVal

	// decision_snapshot as object or null (never omit key)
	var decisionSnapshotObj interface{}
	if len(diag.DecisionSnapshot) > 0 {
		if err := json.Unmarshal(diag.DecisionSnapshot, &decisionSnapshotObj); err != nil {
			decisionSnapshotObj = nil
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"request_id":        requestID,
		"tenant_id":         diag.TenantID,
		"mode":              mode,
		"selected_model":    snap.SelectedModel,
		"provider":          snap.Provider,
		"routing_snapshot":  json.RawMessage(diag.RoutingSnapshot),
		"decision_reason":   diag.DecisionReason,
		"decision_snapshot": decisionSnapshotObj,
		"route_group":       routeGroup,
		"routing_strategy":  routingStrategy,
		"fallback_attempts": fallbackAttempts,
	})
}
