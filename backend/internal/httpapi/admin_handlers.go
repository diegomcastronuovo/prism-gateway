package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/cost"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type UsageSummaryResponse struct {
	TenantID      string                   `json:"tenant_id"`
	Month         string                   `json:"month"`
	TotalRequests int                      `json:"total_requests"`
	TotalCost     float64                  `json:"total_cost_usd"`
	Models        map[string]ModelUsageAPI `json:"models"`
}

type ModelUsageAPI struct {
	Requests                 int     `json:"requests"`
	Cost                     float64 `json:"cost_usd"`
	InfrastructureMonthlyUSD float64 `json:"infrastructure_monthly_usd"`
	InfraCostPerRequest      float64 `json:"infra_cost_per_request"`
	EffectiveCostPerRequest  float64 `json:"effective_cost_per_request"`
}

type BudgetForecastResponse struct {
	TenantID       string  `json:"tenant_id"`
	Month          string  `json:"month"`
	CurrentSpend   float64 `json:"current_spend_usd"`
	ProjectedSpend float64 `json:"projected_spend_usd"`
	DaysElapsed    int     `json:"days_elapsed"`
	DaysInMonth    int     `json:"days_in_month"`
	IsOverBudget   bool    `json:"is_over_budget"`
	BudgetLimit    float64 `json:"budget_limit_usd"`
}

type ModelStatsResponse struct {
	TenantID   string         `json:"tenant_id"`
	WindowDays int            `json:"window_days"`
	Stats      []ModelStatAPI `json:"stats"`
}

type ModelStatAPI struct {
	Date         string  `json:"date"`
	Model        string  `json:"model"`
	Requests     int     `json:"requests"`
	Successes    int     `json:"successes"`
	Errors       int     `json:"errors"`
	ErrorRate    float64 `json:"error_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

func (h *Handlers) AdminUsageSummary(w http.ResponseWriter, r *http.Request) {
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

	summary, err := h.store.GetUsageSummary(r.Context(), tenantID, month)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get usage summary", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve usage summary", "internal_error")
		return
	}

	resp := UsageSummaryResponse{
		TenantID:      summary.TenantID,
		Month:         summary.Month,
		TotalRequests: summary.TotalRequests,
		TotalCost:     summary.TotalCost,
		Models:        make(map[string]ModelUsageAPI),
	}

	// Resolve global config once to look up per-model infrastructure costs.
	gc := h.resolveGlobalConfig(r.Context())

	for modelName, usage := range summary.ModelBreakdown {
		// Look up model config for infrastructure cost; default to zero if not found.
		var modelCfg config.ModelConfig
		if m := gc.ModelByName(modelName); m != nil {
			modelCfg = *m
		}

		// Average token cost per request for this month.
		var tokenCostPerRequest float64
		if usage.Requests > 0 {
			tokenCostPerRequest = usage.Cost / float64(usage.Requests)
		}

		result := cost.CalculateEffectiveCost(modelCfg, tokenCostPerRequest, usage.Requests)

		resp.Models[modelName] = ModelUsageAPI{
			Requests:                 usage.Requests,
			Cost:                     usage.Cost,
			InfrastructureMonthlyUSD: modelCfg.InfrastructureMonthlyUSD,
			InfraCostPerRequest:      result.InfraCostPerRequest,
			EffectiveCostPerRequest:  result.EffectiveCostPerRequest,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// BudgetStatusResponse is the response body for GET /admin/tenants/{tenant_id}/budgets/status.
type BudgetStatusResponse struct {
	TenantID           string  `json:"tenant_id"`
	Month              string  `json:"month"`
	SpendUSD           float64 `json:"spend_usd"`
	ReservedUSD        float64 `json:"reserved_usd"`
	EffectiveSpendUSD  float64 `json:"effective_spend_usd"`
	BudgetUSD          float64 `json:"budget_usd"`
	Pct                float64 `json:"pct"`
	PctEffective       float64 `json:"pct_effective"`
	EnforcementMode    string  `json:"enforcement_mode"`
	EnforcementEnabled bool    `json:"enforcement_enabled"`
	WarnPct            float64 `json:"warn_pct"`
	HardPct            float64 `json:"hard_pct"`
}

// AdminBudgetStatus returns the current spend, budget, and enforcement details for a tenant.
// GET /admin/tenants/{tenant_id}/budgets/status?month=YYYY-MM
func (h *Handlers) AdminBudgetStatus(w http.ResponseWriter, r *http.Request) {
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

	ctx := r.Context()

	// Resolve tenant config using the same authoritative path as AdminGetTenantConfig:
	// call h.store.GetTenantConfig directly so that budget_enforcement fields are always
	// read from the dynamic config stored in the DB. ResolveTenantConfig silently drops DB
	// errors and falls back to the static YAML baseline (which may not have budget_enforcement),
	// causing enforcement_enabled=false even when the DB has it set to true.
	var tenant *config.TenantConfig
	configJSON, _, exists, dbErr := h.store.GetTenantConfig(ctx, tenantID)
	if dbErr != nil {
		h.log.ErrorContext(ctx, "failed to fetch tenant config from DB for budget status",
			"error", dbErr, "tenant_id", tenantID)
		// Fall through to YAML fallback below.
	} else if exists {
		var tc config.TenantConfig
		if umErr := json.Unmarshal(configJSON, &tc); umErr != nil {
			h.log.ErrorContext(ctx, "failed to unmarshal tenant config for budget status",
				"error", umErr, "tenant_id", tenantID)
		} else {
			tc.ID = tenantID
			tenant = &tc
		}
	}
	if tenant == nil {
		// YAML fallback (tenant not yet in DB, or DB/unmarshal error above).
		tenant = h.cfg.TenantByID(tenantID)
	}
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}

	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	spend, err := h.store.GetMonthlySpend(ctx, tenantID, monthStart, monthEnd)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get monthly spend for budget status", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve spend data", "internal_error")
		return
	}

	reserved, err := h.store.GetMonthlyReservedSpend(ctx, tenantID, monthStart, monthEnd)
	if err != nil {
		h.log.WarnContext(ctx, "failed to get monthly reserved spend for budget status", "error", err)
		// Non-fatal: proceed with reserved=0
		reserved = 0
	}

	enf := tenant.BudgetEnforcement
	h.log.InfoContext(ctx, "budget enforcement resolved",
		"tenant", tenantID,
		"enabled", enf.Enabled,
		"mode", enf.Mode,
		"db_record_exists", exists,
	)
	warnPct := enf.Thresholds.WarnPct
	if warnPct <= 0 {
		warnPct = 0.80
	}
	hardPct := enf.Thresholds.HardPct
	if hardPct <= 0 {
		hardPct = 1.00
	}

	budget := tenant.Budgets.MonthlyUSD
	effectiveSpend := spend + reserved
	pct := 0.0
	pctEffective := 0.0
	if budget > 0 {
		pct = spend / budget
		pctEffective = effectiveSpend / budget
	}

	writeJSON(w, http.StatusOK, BudgetStatusResponse{
		TenantID:           tenantID,
		Month:              monthStr,
		SpendUSD:           spend,
		ReservedUSD:        reserved,
		EffectiveSpendUSD:  effectiveSpend,
		BudgetUSD:          budget,
		Pct:                pct,
		PctEffective:       pctEffective,
		EnforcementMode:    enf.Mode,
		EnforcementEnabled: enf.Enabled,
		WarnPct:            warnPct,
		HardPct:            hardPct,
	})
}

func (h *Handlers) AdminBudgetForecast(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	monthStr := r.URL.Query().Get("month")
	if monthStr == "" {
		monthStr = time.Now().Format("2006-01")
	}

	month, err := time.Parse("2006-01", monthStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid month format", "invalid_request_error")
		return
	}

	tenant := h.cfg.TenantByID(tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}

	forecast, err := h.store.GetBudgetForecast(r.Context(), tenantID, month, tenant.Budgets.MonthlyUSD)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get forecast", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve forecast", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, BudgetForecastResponse{
		TenantID:       forecast.TenantID,
		Month:          forecast.Month,
		CurrentSpend:   forecast.CurrentSpend,
		ProjectedSpend: forecast.ProjectedSpend,
		DaysElapsed:    forecast.DaysElapsed,
		DaysInMonth:    forecast.DaysInMonth,
		IsOverBudget:   forecast.IsOverBudget,
		BudgetLimit:    forecast.BudgetLimit,
	})
}

func (h *Handlers) AdminModelStats(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	windowDaysStr := r.URL.Query().Get("window_days")

	windowDays := 7
	if windowDaysStr != "" {
		parsed, err := strconv.Atoi(windowDaysStr)
		if err != nil || parsed < 1 || parsed > 90 {
			writeError(w, http.StatusBadRequest, "window_days must be 1-90", "invalid_request_error")
			return
		}
		windowDays = parsed
	}

	stats, err := h.store.GetModelStats(r.Context(), tenantID, windowDays)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve stats", "internal_error")
		return
	}

	resp := ModelStatsResponse{
		TenantID:   tenantID,
		WindowDays: windowDays,
		Stats:      make([]ModelStatAPI, 0, len(stats)),
	}

	for _, stat := range stats {
		errorRate := 0.0
		if stat.RequestCount > 0 {
			errorRate = float64(stat.ErrorCount) / float64(stat.RequestCount)
		}

		resp.Stats = append(resp.Stats, ModelStatAPI{
			Date:         stat.Date.Format("2006-01-02"),
			Model:        stat.Model,
			Requests:     stat.RequestCount,
			Successes:    stat.SuccessCount,
			Errors:       stat.ErrorCount,
			ErrorRate:    errorRate,
			AvgLatencyMs: stat.AvgLatencyMs,
			TotalCostUSD: stat.TotalCostUSD,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) AdminSmartImpact(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	// Parse query params
	windowDaysStr := r.URL.Query().Get("window_days")
	windowDays := 30
	if windowDaysStr != "" {
		parsed, err := strconv.Atoi(windowDaysStr)
		if err != nil || parsed < 1 || parsed > 180 {
			writeError(w, http.StatusBadRequest, "window_days must be 1-180", "invalid_request_error")
			return
		}
		windowDays = parsed
	}

	baseline := r.URL.Query().Get("baseline")
	if baseline == "" {
		baseline = "round_robin"
	}

	// Validate baseline format
	baselineType, fixedModel, err := parseBaseline(baseline)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	// Validate tenant exists
	tenant := h.cfg.TenantByID(tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}

	// Validate fixed_model is allowed
	if baselineType == "fixed_model" {
		allowed := false
		for _, m := range tenant.AllowedModels {
			if m == fixedModel {
				allowed = true
				break
			}
		}
		if !allowed {
			writeError(w, http.StatusBadRequest,
				"model '"+fixedModel+"' not allowed for tenant",
				"invalid_request_error")
			return
		}
	}

	// Calculate time window
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -windowDays)

	// Fetch data from storage
	data, err := h.store.GetSmartImpactData(r.Context(), tenantID, from, to)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get impact data", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve impact data", "internal_error")
		return
	}

	// Handle no data case
	if data.TotalRequests == 0 {
		writeJSON(w, http.StatusOK, SmartImpactResponse{
			TenantID:   tenantID,
			WindowDays: windowDays,
			Period: PeriodInfo{
				From: from,
				To:   to,
			},
			Actual: ActualMetrics{
				Requests: 0,
			},
			Baseline: BaselineMetrics{
				Type: baseline,
			},
			Impact: ImpactCalculation{
				Notes: []string{"No requests in selected time window"},
			},
		})
		return
	}

	// Calculate baseline cost
	calc := NewBaselineCalculator(h.cfg, tenant)
	var baselineCost float64

	switch baselineType {
	case "round_robin":
		baselineCost = calc.CalculateRoundRobin(data.UsageDetails)
	case "cheapest":
		baselineCost = calc.CalculateCheapest(data.UsageDetails)
	case "fixed_model":
		baselineCost, err = calc.CalculateFixedModel(fixedModel, data.UsageDetails)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid baseline type", "invalid_request_error")
		return
	}

	// Build response
	actualAvgCost := data.TotalCostUSD / float64(data.TotalRequests)
	baselineAvgCost := baselineCost / float64(data.TotalRequests)
	savings := baselineCost - data.TotalCostUSD
	savingsPct := 0.0
	if baselineCost > 0 {
		savingsPct = savings / baselineCost
	}

	errorRate := 0.0
	if data.TotalRequests > 0 {
		errorRate = float64(data.ErrorRequests) / float64(data.TotalRequests)
	}

	// Generate notes
	notes := buildImpactNotes(baseline, savings, savingsPct, actualAvgCost, baselineAvgCost)

	resp := SmartImpactResponse{
		TenantID:   tenantID,
		WindowDays: windowDays,
		Period: PeriodInfo{
			From: from,
			To:   to,
		},
		Actual: ActualMetrics{
			Requests:     data.TotalRequests,
			TotalCostUSD: data.TotalCostUSD,
			AvgCostUSD:   actualAvgCost,
			AvgLatencyMs: data.AvgLatencyMs,
			ErrorRate:    errorRate,
		},
		Baseline: BaselineMetrics{
			Type:         baseline,
			TotalCostUSD: baselineCost,
			AvgCostUSD:   baselineAvgCost,
		},
		Impact: ImpactCalculation{
			SavingsUSD: savings,
			SavingsPct: savingsPct,
			Notes:      notes,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// parseBaseline extracts baseline type and optional model name
func parseBaseline(baseline string) (baselineType, modelName string, err error) {
	if baseline == "round_robin" || baseline == "cheapest" {
		return baseline, "", nil
	}

	// Parse fixed_model:xxx format
	const prefix = "fixed_model:"
	if len(baseline) > len(prefix) && baseline[:len(prefix)] == prefix {
		modelName = baseline[len(prefix):]
		if modelName == "" {
			return "", "", fmt.Errorf("baseline format must be 'fixed_model:<model_name>'")
		}
		return "fixed_model", modelName, nil
	}

	return "", "", fmt.Errorf("baseline must be 'round_robin', 'cheapest', or 'fixed_model:<name>'")
}

// buildImpactNotes generates human-readable insights
func buildImpactNotes(baseline string, savings, savingsPct, actualAvg, baselineAvg float64) []string {
	notes := make([]string, 0, 3)

	if savings > 0 {
		notes = append(notes, fmt.Sprintf(
			"Smart routing saved $%.2f (%.1f%%) over %s strategy",
			savings, savingsPct*100, baseline))
		notes = append(notes, fmt.Sprintf(
			"Average cost per request reduced by $%.5f",
			baselineAvg-actualAvg))
	} else if savings < 0 {
		notes = append(notes, fmt.Sprintf(
			"Smart routing cost $%.2f (%.1f%%) more than %s (may indicate optimization opportunity)",
			-savings, -savingsPct*100, baseline))
	} else {
		notes = append(notes, fmt.Sprintf(
			"Smart routing performed identically to %s strategy", baseline))
	}

	return notes
}

func (h *Handlers) AdminAuditExport(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	// Parse query params
	fromStr := r.URL.Query().Get("from")  // YYYY-MM-DD
	toStr := r.URL.Query().Get("to")      // YYYY-MM-DD
	format := r.URL.Query().Get("format") // json or csv

	// Validate dates
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from' date, expected YYYY-MM-DD", "invalid_request_error")
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to' date, expected YYYY-MM-DD", "invalid_request_error")
		return
	}

	// Add one day to 'to' for inclusive end date
	to = to.Add(24 * time.Hour)

	// Validate window
	if to.Sub(from) > 90*24*time.Hour {
		writeError(w, http.StatusBadRequest, "date range cannot exceed 90 days", "invalid_request_error")
		return
	}

	// Validate format
	if format != "json" && format != "csv" {
		writeError(w, http.StatusBadRequest, "format must be 'json' or 'csv'", "invalid_request_error")
		return
	}

	// Validate tenant exists
	tenant := h.cfg.TenantByID(tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}

	// Fetch audit records
	records, err := h.store.GetAuditRecords(r.Context(), tenantID, from, to)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get audit records", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve audit records", "internal_error")
		return
	}

	if format == "csv" {
		if err := h.writeAuditCSV(w, records); err != nil {
			h.log.ErrorContext(r.Context(), "failed to write audit CSV", "error", err, "tenant_id", tenantID)
			return
		}
	} else {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tenant_id": tenantID,
			"from":      from.Format("2006-01-02"),
			"to":        to.Add(-24 * time.Hour).Format("2006-01-02"),
			"records":   records,
		})
	}
}

func (h *Handlers) writeAuditCSV(w http.ResponseWriter, records []storage.AuditRecord) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=audit_export.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{
		"request_id", "timestamp", "tenant_id", "model", "provider",
		"strategy", "status", "latency_ms", "prompt_tokens",
		"completion_tokens", "total_tokens", "cost_usd",
		"fallback_used", "pii_webhook_request_decision",
		"pii_webhook_response_decision",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write rows
	for _, r := range records {
		row := []string{
			r.RequestID.String(),
			r.Timestamp.Format(time.RFC3339),
			r.TenantID,
			r.Model,
			r.Provider,
			r.Strategy,
			r.Status,
			fmt.Sprintf("%d", r.LatencyMs),
			fmt.Sprintf("%d", r.PromptTokens),
			fmt.Sprintf("%d", r.CompletionTokens),
			fmt.Sprintf("%d", r.TotalTokens),
			fmt.Sprintf("%.6f", r.CostUSD),
			fmt.Sprintf("%t", r.FallbackUsed),
			stringOrEmpty(r.PIIWebhookRequestDecision),
			stringOrEmpty(r.PIIWebhookResponseDecision),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	if err := writer.Error(); err != nil {
		return err
	}
	return nil
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// AdminBudgetAlerts retrieves budget alerts for a tenant in a specific month
func (h *Handlers) AdminBudgetAlerts(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	// Parse month param
	monthStr := r.URL.Query().Get("month")
	if monthStr == "" {
		monthStr = time.Now().Format("2006-01")
	}

	_, err := time.Parse("2006-01", monthStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid month format, expected YYYY-MM", "invalid_request_error")
		return
	}

	// Validate tenant exists
	tenant := h.cfg.TenantByID(tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}

	// Fetch alerts
	alerts, err := h.store.GetBudgetAlerts(r.Context(), tenantID, monthStr)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get budget alerts", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve budget alerts", "internal_error")
		return
	}

	// Build response
	type AlertResponse struct {
		ID              string    `json:"id"`
		Threshold       float64   `json:"threshold"`
		TriggeredAt     time.Time `json:"triggered_at"`
		CurrentSpendUSD float64   `json:"current_spend_usd"`
		BudgetLimitUSD  float64   `json:"budget_limit_usd"`
		Percentage      float64   `json:"percentage"`
	}

	alertResponses := make([]AlertResponse, len(alerts))
	for i, a := range alerts {
		alertResponses[i] = AlertResponse{
			ID:              a.ID.String(),
			Threshold:       a.Threshold,
			TriggeredAt:     a.TriggeredAt,
			CurrentSpendUSD: a.CurrentSpendUSD,
			BudgetLimitUSD:  a.BudgetLimitUSD,
			Percentage:      a.CurrentSpendUSD / a.BudgetLimitUSD,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenant_id": tenantID,
		"month":     monthStr,
		"alerts":    alertResponses,
	})
}

// AdminCostAnomalies retrieves cost anomalies for a tenant within a window
func (h *Handlers) AdminCostAnomalies(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	// Parse window_days param
	windowDaysStr := r.URL.Query().Get("window_days")
	windowDays := 30 // Default
	if windowDaysStr != "" {
		parsed, err := strconv.Atoi(windowDaysStr)
		if err != nil || parsed < 1 || parsed > 180 {
			writeError(w, http.StatusBadRequest, "window_days must be 1-180", "invalid_request_error")
			return
		}
		windowDays = parsed
	}

	// Validate tenant exists
	tenant := h.cfg.TenantByID(tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}

	// Fetch anomalies
	anomalies, err := h.store.GetCostAnomalies(r.Context(), tenantID, windowDays)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get cost anomalies", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve cost anomalies", "internal_error")
		return
	}

	// Build response
	type AnomalyResponse struct {
		ID             string    `json:"id"`
		DetectedAt     time.Time `json:"detected_at"`
		Date           string    `json:"date"`
		DailySpendUSD  float64   `json:"daily_spend_usd"`
		BaselineAvgUSD float64   `json:"baseline_avg_usd"`
		Multiplier     float64   `json:"multiplier"`
	}

	anomalyResponses := make([]AnomalyResponse, len(anomalies))
	for i, a := range anomalies {
		anomalyResponses[i] = AnomalyResponse{
			ID:             a.ID.String(),
			DetectedAt:     a.DetectedAt,
			Date:           a.Date,
			DailySpendUSD:  a.DailySpendUSD,
			BaselineAvgUSD: a.BaselineAvgUSD,
			Multiplier:     a.Multiplier,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenant_id":   tenantID,
		"window_days": windowDays,
		"anomalies":   anomalyResponses,
	})
}

// validTagKey matches ^[a-zA-Z][a-zA-Z0-9_]{0,63}$
var validTagKey = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)

// AdminUsageByTag handles GET /admin/tenants/{tenant_id}/usage/by-tag
// Query params: month=YYYY-MM (required), tag=<key> (required)
func (h *Handlers) AdminUsageByTag(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	monthStr := r.URL.Query().Get("month")
	if monthStr == "" {
		writeError(w, http.StatusBadRequest, "month is required (format: YYYY-MM)", "invalid_request_error")
		return
	}
	month, err := time.Parse("2006-01", monthStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid month format, expected YYYY-MM", "invalid_request_error")
		return
	}

	tag := r.URL.Query().Get("tag")
	if tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required", "invalid_request_error")
		return
	}
	if !validTagKey.MatchString(tag) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid tag %q: must match ^[a-zA-Z][a-zA-Z0-9_]{0,63}$", tag), "invalid_request_error")
		return
	}

	from := month
	to := month.AddDate(0, 1, 0)

	rows, err := h.store.GetUsageByTag(r.Context(), tenantID, from, to, tag)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to get usage by tag", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve usage by tag", "internal_error")
		return
	}

	type dataRow struct {
		Value       string  `json:"value"`
		Requests    int     `json:"requests"`
		TotalTokens int     `json:"total_tokens"`
		CostUSD     float64 `json:"cost_usd"`
	}

	data := make([]dataRow, len(rows))
	for i, r := range rows {
		data[i] = dataRow{
			Value:       r.Value,
			Requests:    r.Requests,
			TotalTokens: r.TotalTokens,
			CostUSD:     r.CostUSD,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenant_id": tenantID,
		"month":     monthStr,
		"tag":       tag,
		"data":      data,
	})
}
