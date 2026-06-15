package httpapi

import (
	"net/http"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type routerPerformanceFiltersResponse struct {
	From     *string `json:"from"`
	To       *string `json:"to"`
	TenantID *string `json:"tenant_id"`
	Model    *string `json:"model"`
	Provider *string `json:"provider"`
	Status   *string `json:"status"`
	Bucket   string  `json:"bucket"`
}

type routerPerformanceSummaryResponse struct {
	Requests int `json:"requests"`

	AvgRouterPreMs float64 `json:"avg_router_pre_ms"`
	MinRouterPreMs float64 `json:"min_router_pre_ms"`
	MaxRouterPreMs float64 `json:"max_router_pre_ms"`
	P50RouterPreMs float64 `json:"p50_router_pre_ms"`
	P95RouterPreMs float64 `json:"p95_router_pre_ms"`

	AvgLLMLatencyMs float64 `json:"avg_llm_latency_ms"`
	MinLLMLatencyMs float64 `json:"min_llm_latency_ms"`
	MaxLLMLatencyMs float64 `json:"max_llm_latency_ms"`
	P50LLMLatencyMs float64 `json:"p50_llm_latency_ms"`
	P95LLMLatencyMs float64 `json:"p95_llm_latency_ms"`

	AvgRouterPostMs float64 `json:"avg_router_post_ms"`
	MinRouterPostMs float64 `json:"min_router_post_ms"`
	MaxRouterPostMs float64 `json:"max_router_post_ms"`
	P50RouterPostMs float64 `json:"p50_router_post_ms"`
	P95RouterPostMs float64 `json:"p95_router_post_ms"`

	AvgTotalLatencyMs float64 `json:"avg_total_latency_ms"`
	P50TotalLatencyMs float64 `json:"p50_total_latency_ms"`
	P95TotalLatencyMs float64 `json:"p95_total_latency_ms"`

	SuccessRate float64 `json:"success_rate"`
	ErrorRate   float64 `json:"error_rate"`

	AvgPreTenantConfigMs    float64 `json:"avg_pre_tenant_config_ms"`
	AvgCfgToolRoutesMs      float64 `json:"avg_cfg_tool_routes_ms"`
	AvgCfgDynamicRoutesMs   float64 `json:"avg_cfg_dynamic_routes_ms"`
	AvgCfgDecisionOpsMs     float64 `json:"avg_cfg_decision_ops_ms"`
	AvgCfgBudgetPressureMs  float64 `json:"avg_cfg_budget_pressure_ms"`
	AvgCfgSemanticMs        float64 `json:"avg_cfg_semantic_ms"`
	AvgCfgModelResolutionMs float64 `json:"avg_cfg_model_resolution_ms"`
}

type routerPerformanceTimeseriesResponse struct {
	BucketStart       string  `json:"bucket_start"`
	Requests          int     `json:"requests"`
	AvgRouterPreMs    float64 `json:"avg_router_pre_ms"`
	AvgLLMLatencyMs   float64 `json:"avg_llm_latency_ms"`
	AvgRouterPostMs   float64 `json:"avg_router_post_ms"`
	AvgTotalLatencyMs float64 `json:"avg_total_latency_ms"`
	P95RouterPreMs    float64 `json:"p95_router_pre_ms"`
	P95LLMLatencyMs   float64 `json:"p95_llm_latency_ms"`
	P95RouterPostMs   float64 `json:"p95_router_post_ms"`
}

type routerPerformanceBreakdownsResponse struct {
	PreBreakdownAvgMs struct {
		TenantConfig    float64 `json:"tenant_config"`
		ToolRoutes      float64 `json:"tool_routes"`
		DynamicRoutes   float64 `json:"dynamic_routes"`
		DecisionOps     float64 `json:"decision_ops"`
		BudgetPressure  float64 `json:"budget_pressure"`
		Semantic        float64 `json:"semantic"`
		ModelResolution float64 `json:"model_resolution"`
	} `json:"pre_breakdown_avg_ms"`
	ToolRoutesBreakdownAvgMs struct {
		EmbeddingModel    float64 `json:"embedding_model"`
		EmbeddingGenerate float64 `json:"embedding_generate"`
		SemanticDB        float64 `json:"semantic_db"`
		MatchEval         float64 `json:"match_eval"`
	} `json:"tool_routes_breakdown_avg_ms"`
}

type routerPerformanceResponse struct {
	Object     string                                `json:"object"`
	Filters    routerPerformanceFiltersResponse      `json:"filters"`
	Summary    routerPerformanceSummaryResponse      `json:"summary"`
	Timeseries []routerPerformanceTimeseriesResponse `json:"timeseries"`
	Breakdowns routerPerformanceBreakdownsResponse   `json:"breakdowns"`
}

// AdminRouterPerformance returns router-only performance metrics.
// GET /admin/router/performance
func (h *Handlers) AdminRouterPerformance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	roles := auth.RolesFromContext(r.Context())
	if len(roles) > 0 && !auth.HasAnyRole(roles, adminBypassRoles) {
		writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
		return
	}

	q := r.URL.Query()
	filter := storage.RouterPerformanceFilter{}

	bucket := q.Get("bucket")
	if bucket == "" {
		bucket = "hour"
	}
	if bucket != "minute" && bucket != "hour" && bucket != "day" {
		writeError(w, http.StatusBadRequest, "bucket must be one of minute, hour, day", "invalid_request_error")
		return
	}
	filter.Bucket = bucket

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
	if v := q.Get("model"); v != "" {
		filter.Model = &v
	}
	if v := q.Get("provider"); v != "" {
		filter.Provider = &v
	}
	if v := q.Get("status"); v != "" {
		filter.Status = &v
	}

	metrics, err := h.store.GetRouterPerformance(r.Context(), filter)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get router performance", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve router performance", "internal_error")
		return
	}

	var fromStr *string
	if filter.From != nil {
		v := filter.From.UTC().Format(time.RFC3339)
		fromStr = &v
	}
	var toStr *string
	if filter.To != nil {
		v := filter.To.UTC().Format(time.RFC3339)
		toStr = &v
	}

	filters := routerPerformanceFiltersResponse{
		From:     fromStr,
		To:       toStr,
		TenantID: filter.TenantID,
		Model:    filter.Model,
		Provider: filter.Provider,
		Status:   filter.Status,
		Bucket:   filter.Bucket,
	}

	summary := routerPerformanceSummaryResponse{
		Requests: metrics.Summary.Requests,

		AvgRouterPreMs: metrics.Summary.AvgRouterPreMs,
		MinRouterPreMs: metrics.Summary.MinRouterPreMs,
		MaxRouterPreMs: metrics.Summary.MaxRouterPreMs,
		P50RouterPreMs: metrics.Summary.P50RouterPreMs,
		P95RouterPreMs: metrics.Summary.P95RouterPreMs,

		AvgLLMLatencyMs: metrics.Summary.AvgLLMLatencyMs,
		MinLLMLatencyMs: metrics.Summary.MinLLMLatencyMs,
		MaxLLMLatencyMs: metrics.Summary.MaxLLMLatencyMs,
		P50LLMLatencyMs: metrics.Summary.P50LLMLatencyMs,
		P95LLMLatencyMs: metrics.Summary.P95LLMLatencyMs,

		AvgRouterPostMs: metrics.Summary.AvgRouterPostMs,
		MinRouterPostMs: metrics.Summary.MinRouterPostMs,
		MaxRouterPostMs: metrics.Summary.MaxRouterPostMs,
		P50RouterPostMs: metrics.Summary.P50RouterPostMs,
		P95RouterPostMs: metrics.Summary.P95RouterPostMs,

		AvgTotalLatencyMs: metrics.Summary.AvgTotalLatencyMs,
		P50TotalLatencyMs: metrics.Summary.P50TotalLatencyMs,
		P95TotalLatencyMs: metrics.Summary.P95TotalLatencyMs,

		SuccessRate: metrics.Summary.SuccessRate,
		ErrorRate:   metrics.Summary.ErrorRate,

		AvgPreTenantConfigMs:    metrics.Summary.AvgPreTenantConfigMs,
		AvgCfgToolRoutesMs:      metrics.Summary.AvgCfgToolRoutesMs,
		AvgCfgDynamicRoutesMs:   metrics.Summary.AvgCfgDynamicRoutesMs,
		AvgCfgDecisionOpsMs:     metrics.Summary.AvgCfgDecisionOpsMs,
		AvgCfgBudgetPressureMs:  metrics.Summary.AvgCfgBudgetPressureMs,
		AvgCfgSemanticMs:        metrics.Summary.AvgCfgSemanticMs,
		AvgCfgModelResolutionMs: metrics.Summary.AvgCfgModelResolutionMs,
	}

	timeseries := make([]routerPerformanceTimeseriesResponse, 0, len(metrics.Timeseries))
	for _, row := range metrics.Timeseries {
		timeseries = append(timeseries, routerPerformanceTimeseriesResponse{
			BucketStart:       row.BucketStart.UTC().Format(time.RFC3339),
			Requests:          row.Requests,
			AvgRouterPreMs:    row.AvgRouterPreMs,
			AvgLLMLatencyMs:   row.AvgLLMLatencyMs,
			AvgRouterPostMs:   row.AvgRouterPostMs,
			AvgTotalLatencyMs: row.AvgTotalLatencyMs,
			P95RouterPreMs:    row.P95RouterPreMs,
			P95LLMLatencyMs:   row.P95LLMLatencyMs,
			P95RouterPostMs:   row.P95RouterPostMs,
		})
	}

	breakdowns := routerPerformanceBreakdownsResponse{}
	breakdowns.PreBreakdownAvgMs.TenantConfig = metrics.Breakdowns.PreBreakdownAvgMs.TenantConfig
	breakdowns.PreBreakdownAvgMs.ToolRoutes = metrics.Breakdowns.PreBreakdownAvgMs.ToolRoutes
	breakdowns.PreBreakdownAvgMs.DynamicRoutes = metrics.Breakdowns.PreBreakdownAvgMs.DynamicRoutes
	breakdowns.PreBreakdownAvgMs.DecisionOps = metrics.Breakdowns.PreBreakdownAvgMs.DecisionOps
	breakdowns.PreBreakdownAvgMs.BudgetPressure = metrics.Breakdowns.PreBreakdownAvgMs.BudgetPressure
	breakdowns.PreBreakdownAvgMs.Semantic = metrics.Breakdowns.PreBreakdownAvgMs.Semantic
	breakdowns.PreBreakdownAvgMs.ModelResolution = metrics.Breakdowns.PreBreakdownAvgMs.ModelResolution
	breakdowns.ToolRoutesBreakdownAvgMs.EmbeddingModel = metrics.Breakdowns.ToolRoutesBreakdownAvgMs.EmbeddingModel
	breakdowns.ToolRoutesBreakdownAvgMs.EmbeddingGenerate = metrics.Breakdowns.ToolRoutesBreakdownAvgMs.EmbeddingGenerate
	breakdowns.ToolRoutesBreakdownAvgMs.SemanticDB = metrics.Breakdowns.ToolRoutesBreakdownAvgMs.SemanticDB
	breakdowns.ToolRoutesBreakdownAvgMs.MatchEval = metrics.Breakdowns.ToolRoutesBreakdownAvgMs.MatchEval

	writeJSON(w, http.StatusOK, routerPerformanceResponse{
		Object:     "router_performance",
		Filters:    filters,
		Summary:    summary,
		Timeseries: timeseries,
		Breakdowns: breakdowns,
	})
}
