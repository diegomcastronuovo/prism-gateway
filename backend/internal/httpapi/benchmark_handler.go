package httpapi

import (
	"net/http"
	"strconv"
)

// AdminDeleteModelBenchmarks handles DELETE /admin/benchmarks/models
// and purges all rows from model_benchmarks (admin reset of stale data).
func (h *Handlers) AdminDeleteModelBenchmarks(w http.ResponseWriter, r *http.Request) {
	deleted, err := h.store.TruncateModelBenchmarks(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to truncate model benchmarks", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to clear benchmark data", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted": deleted,
	})
}

// AdminListModelBenchmarks handles GET /admin/benchmarks/models?window_hours=24
// and returns per-model aggregate benchmark stats.
func (h *Handlers) AdminListModelBenchmarks(w http.ResponseWriter, r *http.Request) {
	windowHours := 24
	if v := r.URL.Query().Get("window_hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowHours = n
		}
	}

	aggregates, err := h.store.GetModelBenchmarkAggregates(r.Context(), windowHours)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to query model benchmark aggregates", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve benchmark data", "internal_error")
		return
	}

	type item struct {
		Model        string  `json:"model"`
		Provider     string  `json:"provider"`
		AvgLatencyMs float64 `json:"avg_latency_ms"`
		P95LatencyMs float64 `json:"p95_latency_ms"`
		SuccessRate  float64 `json:"success_rate"`
		AvgCostUSD   float64 `json:"avg_cost_usd"`
		Samples      int     `json:"samples"`
	}

	data := make([]item, 0, len(aggregates))
	for _, a := range aggregates {
		data = append(data, item{
			Model:        a.Model,
			Provider:     a.Provider,
			AvgLatencyMs: a.AvgLatencyMs,
			P95LatencyMs: a.P95LatencyMs,
			SuccessRate:  a.SuccessRate,
			AvgCostUSD:   a.AvgCostUSD,
			Samples:      a.Samples,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}
