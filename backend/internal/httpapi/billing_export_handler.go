package httpapi

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

var validGroupBy = map[string]bool{
	"none": true, "model": true, "provider": true,
	"project": true, "cost_center": true, "env": true, "application": true,
}

// AdminBillingExport handles GET /admin/tenants/{tenant_id}/billing/export.
// Query params:
//   - month  — required, YYYY-MM
//   - format — "csv" (default) or "json"
//   - group_by — "none" (default), "model", "provider", "project", "cost_center", "env", "application"
func (h *Handlers) AdminBillingExport(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	q := r.URL.Query()

	// Parse and validate month
	monthStr := q.Get("month")
	from, err := time.Parse("2006-01", monthStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'month' param, expected YYYY-MM", "invalid_request_error")
		return
	}
	to := from.AddDate(0, 1, 0)

	// Parse and validate format (default: csv)
	format := q.Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		writeError(w, http.StatusBadRequest, "format must be 'csv' or 'json'", "invalid_request_error")
		return
	}

	// Parse and validate group_by (default: none)
	groupBy := q.Get("group_by")
	if groupBy == "" {
		groupBy = "none"
	}
	if !validGroupBy[groupBy] {
		writeError(w, http.StatusBadRequest, "group_by must be one of: none, model, provider, project, cost_center, env, application", "invalid_request_error")
		return
	}

	ctx := r.Context()

	if groupBy == "none" {
		if format == "csv" {
			if _, err := h.streamBillingCSV(w, ctx, tenantID, from, to); err != nil {
				return
			}
		} else {
			h.streamBillingJSON(w, ctx, tenantID, from, to, monthStr)
		}
		return
	}

	// Grouped export
	rows, err := h.store.GetBillingGrouped(ctx, tenantID, from, to, groupBy)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get billing grouped", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to retrieve billing data", "internal_error")
		return
	}

	if format == "csv" {
		if err := h.writeGroupedCSV(w, tenantID, monthStr, groupBy, rows); err != nil {
			h.log.ErrorContext(ctx, "failed to write grouped billing CSV", "error", err, "tenant_id", tenantID)
			return
		}
	} else {
		writeJSON(w, http.StatusOK, billingGroupedResponse{
			Object:   "billing_export_grouped",
			TenantID: tenantID,
			Month:    monthStr,
			GroupBy:  groupBy,
			Data:     rows,
		})
	}
}

type billingGroupedResponse struct {
	Object   string                      `json:"object"`
	TenantID string                      `json:"tenant_id"`
	Month    string                      `json:"month"`
	GroupBy  string                      `json:"group_by"`
	Data     []storage.BillingGroupedRow `json:"data"`
}

// streamBillingCSV writes a streaming CSV of per-request billing line items.
// Headers are set before iteration begins; any mid-stream DB error is logged only
// (HTTP headers have already been sent).
func (h *Handlers) streamBillingCSV(w http.ResponseWriter, ctx context.Context, tenantID string, from, to time.Time) (int, error) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="billing_export.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{
		"timestamp", "request_id", "tenant_id", "model", "provider",
		"status", "total_tokens", "prompt_tokens", "completion_tokens",
		"cost_usd", "project", "cost_center", "env", "application",
	}); err != nil {
		return 0, err
	}

	rowCount := 0

	if err := h.store.StreamBillingLineItems(ctx, tenantID, from, to, func(item storage.BillingLineItem) error {
		if err := cw.Write([]string{
			item.Timestamp.Format(time.RFC3339),
			item.RequestID,
			item.TenantID,
			item.Model,
			item.Provider,
			item.Status,
			strconv.Itoa(item.TotalTokens),
			strconv.Itoa(item.PromptTokens),
			strconv.Itoa(item.CompletionTokens),
			fmt.Sprintf("%.6f", item.CostUSD),
			item.Project,
			item.CostCenter,
			item.Env,
			item.Application,
		}); err != nil {
			return err
		}
		rowCount++
		return nil
	}); err != nil {
		// Headers already sent; log the error but cannot change the HTTP status.
		h.log.ErrorContext(ctx, "error streaming billing CSV", "error", err, "tenant_id", tenantID)
		return rowCount, err
	}
	if err := cw.Error(); err != nil {
		return rowCount, err
	}
	return rowCount, nil
}

type billingLineItemJSON struct {
	Timestamp        string  `json:"timestamp"`
	RequestID        string  `json:"request_id"`
	TenantID         string  `json:"tenant_id"`
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	Status           string  `json:"status"`
	TotalTokens      int     `json:"total_tokens"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Project          string  `json:"project"`
	CostCenter       string  `json:"cost_center"`
	Env              string  `json:"env"`
	Application      string  `json:"application"`
}

// streamBillingJSON collects all line items into memory and writes JSON.
func (h *Handlers) streamBillingJSON(w http.ResponseWriter, ctx context.Context, tenantID string, from, to time.Time, monthStr string) {
	var items []billingLineItemJSON

	if err := h.store.StreamBillingLineItems(ctx, tenantID, from, to, func(item storage.BillingLineItem) error {
		items = append(items, billingLineItemJSON{
			Timestamp:        item.Timestamp.Format(time.RFC3339),
			RequestID:        item.RequestID,
			TenantID:         item.TenantID,
			Model:            item.Model,
			Provider:         item.Provider,
			Status:           item.Status,
			TotalTokens:      item.TotalTokens,
			PromptTokens:     item.PromptTokens,
			CompletionTokens: item.CompletionTokens,
			CostUSD:          item.CostUSD,
			Project:          item.Project,
			CostCenter:       item.CostCenter,
			Env:              item.Env,
			Application:      item.Application,
		})
		return nil
	}); err != nil {
		h.log.ErrorContext(ctx, "failed to stream billing line items", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to retrieve billing data", "internal_error")
		return
	}

	if items == nil {
		items = []billingLineItemJSON{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object":    "billing_export",
		"tenant_id": tenantID,
		"month":     monthStr,
		"data":      items,
	})
}

// writeGroupedCSV writes grouped billing data as CSV.
// Columns: month, tenant_id, group_by, group_key, requests_count, total_tokens, prompt_tokens, completion_tokens, cost_usd
func (h *Handlers) writeGroupedCSV(w http.ResponseWriter, tenantID, month, groupBy string, rows []storage.BillingGroupedRow) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="billing_export_grouped.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{
		"month", "tenant_id", "group_by", "group_key",
		"requests_count", "total_tokens", "prompt_tokens", "completion_tokens", "cost_usd",
	}); err != nil {
		return err
	}

	for _, row := range rows {
		if err := cw.Write([]string{
			month,
			tenantID,
			groupBy,
			row.GroupKey,
			strconv.Itoa(row.RequestsCount),
			strconv.Itoa(row.TotalTokens),
			strconv.Itoa(row.PromptTokens),
			strconv.Itoa(row.CompletionTokens),
			fmt.Sprintf("%.6f", row.CostUSD),
		}); err != nil {
			return err
		}
	}
	if err := cw.Error(); err != nil {
		return err
	}
	return nil
}
