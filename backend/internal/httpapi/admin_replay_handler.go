package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
)

// TrafficReplayRequest is the body of POST /admin/traffic/replay.
type TrafficReplayRequest struct {
	TenantID      string `json:"tenant_id"`      // required
	Dataset       string `json:"dataset"`         // "last_1h" | "last_24h" | "last_7_days" | "last_30_days"
	Limit         int    `json:"limit"`           // 0 → default 1000, max 5000
	RoutingConfig string `json:"routing_config"`  // reserved, not yet applied
}

// TrafficReplayResponse is the response of POST /admin/traffic/replay.
type TrafficReplayResponse struct {
	RequestsReplayed int            `json:"requests_replayed"`
	ChangedRoutes    int            `json:"changed_routes"`
	ChangedModels    int            `json:"changed_models"`
	CostDeltaUSD     float64        `json:"cost_delta_usd"`
	ModelChanges     map[string]int `json:"model_changes"` // "old -> new": count
}

// datasetWindow converts a named dataset to a [from, to) time range.
func datasetWindow(dataset string) (from, to time.Time, err error) {
	to = time.Now().UTC()
	switch dataset {
	case "last_1h":
		from = to.Add(-1 * time.Hour)
	case "last_24h":
		from = to.Add(-24 * time.Hour)
	case "last_7_days":
		from = to.Add(-7 * 24 * time.Hour)
	case "last_30_days":
		from = to.Add(-30 * 24 * time.Hour)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported dataset %q: must be one of last_1h, last_24h, last_7_days, last_30_days", dataset)
	}
	return from, to, nil
}

// AdminTrafficReplay simulates routing decisions for historical traffic.
// POST /admin/traffic/replay
func (h *Handlers) AdminTrafficReplay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	var req TrafficReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	if req.TenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required", "invalid_request_error")
		return
	}
	if req.Dataset == "" {
		writeError(w, http.StatusBadRequest, "dataset is required", "invalid_request_error")
		return
	}

	// Tenant isolation: api_key callers may only access their own tenant.
	if auth.AuthTypeFromContext(ctx) == "api_key" {
		callerTenantID := auth.TenantIDFromContext(ctx)
		if callerTenantID != req.TenantID {
			writeError(w, http.StatusForbidden, "access denied", "authorization_error")
			return
		}
	}

	// Apply limit defaults.
	if req.Limit <= 0 {
		req.Limit = 1000
	}
	if req.Limit > 5000 {
		req.Limit = 5000
	}

	from, to, err := datasetWindow(req.Dataset)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	h.log.InfoContext(ctx, "traffic replay started",
		"tenant_id", req.TenantID,
		"dataset", req.Dataset,
		"routing_config", req.RoutingConfig,
		"limit", req.Limit,
	)

	rows, err := h.store.GetReplayRequests(ctx, req.TenantID, from, to, req.Limit)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch replay requests", "error", err, "tenant_id", req.TenantID)
		writeError(w, http.StatusInternalServerError, "failed to fetch replay requests", "internal_error")
		return
	}

	// Resolve current tenant config for routing strategy and route groups.
	tenant, err := h.cfg.ResolveTenantConfig(ctx, req.TenantID, h.tenantCache, h.store)
	if err != nil || tenant == nil {
		h.log.ErrorContext(ctx, "failed to resolve tenant config", "error", err, "tenant_id", req.TenantID)
		writeError(w, http.StatusInternalServerError, "failed to resolve tenant config", "internal_error")
		return
	}

	resp := TrafficReplayResponse{
		ModelChanges: make(map[string]int),
	}

	for _, row := range rows {
		var snap router.RoutingSnapshot
		if err := json.Unmarshal(row.RoutingSnapshot, &snap); err != nil {
			// Skip unparseable snapshots.
			continue
		}

		// Build the current candidate pool from models still present in global config.
		var currentCandidates []config.ModelConfig
		for _, name := range snap.CandidateModels {
			mc := h.resolveModelByName(ctx, name)
			if mc != nil {
				currentCandidates = append(currentCandidates, *mc)
			}
		}
		if len(currentCandidates) == 0 {
			// Cannot simulate — no models remaining.
			continue
		}

		routeReq := router.Request{
			TenantID:    req.TenantID,
			Strategy:    tenant.Routing.Strategy,
			Candidates:  currentCandidates,
			RouteGroup:  snap.RouteGroup,
			RouteGroups: tenant.Selection.RouteGroups,
			SmartConfig: &tenant.Routing.Smart,
		}

		result, err := h.router.Select(routeReq)
		if err != nil {
			// Skip rows where routing fails.
			continue
		}

		resp.RequestsReplayed++

		selected := result.Selected
		original := row.Model

		if selected != original {
			resp.ChangedModels++
			resp.ChangedRoutes++
			key := original + " -> " + selected
			resp.ModelChanges[key]++
		} else if len(result.Candidates) > 0 && result.Candidates[0] != original {
			resp.ChangedRoutes++
		}

		// Cost delta calculation.
		newModelCfg := h.resolveModelByName(ctx, selected)
		if newModelCfg != nil {
			newCost := computeCost(newModelCfg.Pricing, providers.Usage{
				PromptTokens:     row.PromptTokens,
				CompletionTokens: row.CompletionTokens,
			}, nil)
			resp.CostDeltaUSD += newCost - row.CostUSD
		}
	}

	// Emit metrics.
	gatewayotel.TrafficReplayTotalCounter.WithLabelValues(req.TenantID).Inc()
	gatewayotel.TrafficReplayChangedRoutesCounter.WithLabelValues(req.TenantID).Add(float64(resp.ChangedRoutes))
	gatewayotel.TrafficReplayChangedModelsCounter.WithLabelValues(req.TenantID).Add(float64(resp.ChangedModels))
	gatewayotel.TrafficReplayCostDeltaHist.WithLabelValues(req.TenantID).Observe(resp.CostDeltaUSD)

	durationMs := time.Since(start).Milliseconds()
	h.log.InfoContext(ctx, "traffic replay completed",
		"tenant_id", req.TenantID,
		"requests_replayed", resp.RequestsReplayed,
		"changed_models", resp.ChangedModels,
		"cost_delta_usd", resp.CostDeltaUSD,
		"duration_ms", durationMs,
	)

	writeJSON(w, http.StatusOK, resp)
}
