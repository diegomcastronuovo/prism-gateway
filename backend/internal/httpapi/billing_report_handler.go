package httpapi

import (
	"context"
	"encoding/csv"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminBillingReportCSV handles GET /admin/billing/report.csv?window_hours=<N>
// SPEC_108: unified CSV of API key + JWT sub usage with monetization (no FE pricing).
func (h *Handlers) AdminBillingReportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authBilling := billingAuthTypeLabelForMetrics(ctx)

	if r.Method != http.MethodGet {
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	if !billingReportAllowed(ctx) {
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		writeError(w, http.StatusForbidden, "forbidden", "authorization_error")
		return
	}

	q := r.URL.Query()
	windowHours := 720
	if v := q.Get("window_hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 720 {
			gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
			writeError(w, http.StatusBadRequest, "window_hours must be 1-720", "invalid_request_error")
			return
		}
		windowHours = n
	}

	generatedAt := time.Now().UTC()
	since := generatedAt.Add(-time.Duration(windowHours) * time.Hour)

	apiFilter := storage.APIKeyUsageFilter{
		WindowHours: windowHours,
		Limit:       100000,
		Offset:      0,
	}
	_, apiRows, err := h.store.GetAPIKeyUsage(ctx, apiFilter)
	if err != nil {
		h.log.ErrorContext(ctx, "billing report: GetAPIKeyUsage", "error", err)
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		writeError(w, http.StatusInternalServerError, "failed to retrieve API key usage", "internal_error")
		return
	}
	apiBreakdown, err := h.store.GetAPIKeyModelBreakdown(ctx, apiFilter)
	if err != nil {
		h.log.ErrorContext(ctx, "billing report: GetAPIKeyModelBreakdown", "error", err)
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		writeError(w, http.StatusInternalServerError, "failed to retrieve API key usage", "internal_error")
		return
	}
	breakdownByKey := make(map[uuid.UUID][]storage.APIKeyModelUsageRow, len(apiBreakdown))
	for _, b := range apiBreakdown {
		breakdownByKey[b.APIKeyID] = append(breakdownByKey[b.APIKeyID], b)
	}

	to := generatedAt
	from := since
	jwtFilter := storage.JWTSubUsageFilter{
		From:      &from,
		To:        &to,
		Limit:     100000,
		Offset:    0,
		SortBy:    "cost_usd",
		SortOrder: "desc",
	}
	jwtRows, _, err := h.store.GetJWTSubUsage(ctx, jwtFilter)
	if err != nil {
		h.log.ErrorContext(ctx, "billing report: GetJWTSubUsage", "error", err)
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		writeError(w, http.StatusInternalServerError, "failed to retrieve jwt_sub usage", "internal_error")
		return
	}
	jwtBreakdown, err := h.store.GetJWTSubModelBreakdown(ctx, jwtFilter)
	if err != nil {
		h.log.ErrorContext(ctx, "billing report: GetJWTSubModelBreakdown", "error", err)
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		writeError(w, http.StatusInternalServerError, "failed to retrieve jwt_sub usage", "internal_error")
		return
	}

	tenantModelReqs, _ := h.store.GetTenantModelRequestCounts(ctx, "", since)
	allModels := h.resolveGlobalConfig(ctx).Models

	type row struct {
		identityType   string
		identityID     string
		identityName   string
		tenantID       string
		requests       int
		effectiveCost  float64
		avgCostPerReq  float64
		avgPricePerReq float64
		totalPrice     float64
		margin         float64
		marginPct      float64
		topModel       string
	}

	out := make([]row, 0, len(apiRows)+len(jwtRows))

	for _, kr := range apiRows {
		if kr.Requests <= 0 {
			continue
		}
		mon := computeMonetization(breakdownByKey[kr.APIKeyID], tenantModelReqs, allModels)
		out = append(out, row{
			identityType:   "api_key",
			identityID:     kr.APIKeyID.String(),
			identityName:   kr.APIKeyName,
			tenantID:       kr.TenantID,
			requests:       kr.Requests,
			effectiveCost:  mon.TotalEffectiveCost,
			avgCostPerReq:  mon.AvgCostPerRequest,
			avgPricePerReq: mon.AvgPricePerRequest,
			totalPrice:     mon.TotalPrice,
			margin:         mon.Margin,
			marginPct:      mon.MarginPct,
			topModel:       kr.TopModel,
		})
	}

	jwtBDIndex := make(map[jwtSubKey][]storage.APIKeyModelUsageRow, len(jwtBreakdown))
	for _, b := range jwtBreakdown {
		k := jwtSubKey{b.JWTSub, b.TenantID}
		jwtBDIndex[k] = append(jwtBDIndex[k], storage.APIKeyModelUsageRow{
			Model: b.Model, Requests: b.Requests, Spend: b.Spend,
		})
	}

	for _, jr := range jwtRows {
		if jr.Requests <= 0 {
			continue
		}
		k := jwtSubKey{jr.JWTSub, jr.TenantID}
		mon := computeMonetization(jwtBDIndex[k], tenantModelReqs, allModels)
		out = append(out, row{
			identityType:   "jwt_sub",
			identityID:     jr.JWTSub,
			identityName:   jr.JWTSub,
			tenantID:       jr.TenantID,
			requests:       jr.Requests,
			effectiveCost:  mon.TotalEffectiveCost,
			avgCostPerReq:  mon.AvgCostPerRequest,
			avgPricePerReq: mon.AvgPricePerRequest,
			totalPrice:     mon.TotalPrice,
			margin:         mon.Margin,
			marginPct:      mon.MarginPct,
			topModel:       topJWTModel(jwtBreakdown, jr.JWTSub, jr.TenantID),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].totalPrice > out[j].totalPrice
	})

	ts := generatedAt.Format(time.RFC3339)
	fn := "billing_report_" + generatedAt.Format("20060102T150405Z") + ".csv"

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fn+`"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	header := []string{
		"identity_type", "identity_id", "identity_name", "tenant_id", "requests",
		"effective_cost_total", "avg_cost_per_request", "avg_price_per_request",
		"total_price", "margin", "margin_pct", "top_model", "window_hours", "generated_at",
	}
	if err := cw.Write(header); err != nil {
		h.log.ErrorContext(ctx, "billing report: write csv header", "error", err)
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		return
	}
	for _, r := range out {
		rec := []string{
			r.identityType,
			r.identityID,
			r.identityName,
			r.tenantID,
			strconv.Itoa(r.requests),
			formatCSVFloat(r.effectiveCost),
			formatCSVFloat(r.avgCostPerReq),
			formatCSVFloat(r.avgPricePerReq),
			formatCSVFloat(r.totalPrice),
			formatCSVFloat(r.margin),
			formatCSVFloat(r.marginPct),
			r.topModel,
			strconv.Itoa(windowHours),
			ts,
		}
		if err := cw.Write(rec); err != nil {
			h.log.ErrorContext(ctx, "billing report: write csv row", "error", err)
			gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
			return
		}
		gatewayotel.BillingReportRowsTotal.WithLabelValues(r.identityType).Inc()
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		h.log.ErrorContext(ctx, "billing report: csv flush", "error", err)
		gatewayotel.BillingReportExportsTotal.WithLabelValues("error", authBilling).Inc()
		return
	}
	gatewayotel.BillingReportExportsTotal.WithLabelValues("success", authBilling).Inc()
}

func billingReportAllowed(ctx context.Context) bool {
	switch auth.AuthTypeFromContext(ctx) {
	case "admin_token":
		return true
	case "api_key":
		return true
	case "jwt":
		roles := auth.RolesFromContext(ctx)
		return auth.HasAnyRole(roles, []string{"admin", "audit"})
	default:
		return false
	}
}

func topJWTModel(rows []storage.JWTSubModelUsageRow, jwtSub, tenantID string) string {
	var bestModel string
	var bestReq int
	for _, r := range rows {
		if r.JWTSub != jwtSub || r.TenantID != tenantID {
			continue
		}
		if r.Requests > bestReq {
			bestReq = r.Requests
			bestModel = r.Model
		}
	}
	return bestModel
}

func formatCSVFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
