package httpapi

import (
	"context"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

func monthStartUTC(t time.Time) time.Time {
	y, m, _ := t.UTC().Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

func normalizeModelTypeForMetrics(t string) string {
	switch t {
	case "ml", "embedding", "llm":
		return t
	default:
		return "llm"
	}
}

func prometheusAuthTypeLabel(ctx context.Context) string {
	switch auth.AuthTypeFromContext(ctx) {
	case "jwt":
		return "jwt"
	case "api_key":
		return "api_key"
	case "admin_token":
		return "admin"
	default:
		return "unknown"
	}
}

func billingAuthTypeLabelForMetrics(ctx context.Context) string {
	switch auth.AuthTypeFromContext(ctx) {
	case "admin_token":
		return "admin"
	case "api_key":
		return "api_key"
	case "jwt":
		return "jwt"
	default:
		return "unknown"
	}
}

// perRequestMonetizationUSD uses computeMonetization with a single-row breakdown and
// tenant model counts since month start (UTC). The current request is included in the
// denominator for infra allocation (marginal request).
func (h *Handlers) perRequestMonetizationUSD(ctx context.Context, tenantID, modelName string, tokenCostUSD float64) (effective, price, margin float64, ok bool) {
	if h.store == nil {
		return 0, 0, 0, false
	}
	breakdown := []storage.APIKeyModelUsageRow{{
		Model:    modelName,
		Requests: 1,
		Spend:    tokenCostUSD,
	}}
	since := monthStartUTC(time.Now())
	tenantModelReqs, err := h.store.GetTenantModelRequestCounts(ctx, tenantID, since)
	if err != nil {
		return 0, 0, 0, false
	}
	adjusted := make(map[string]int64, len(tenantModelReqs)+1)
	for k, v := range tenantModelReqs {
		adjusted[k] = v
	}
	adjusted[modelName]++
	allModels := h.resolveGlobalConfig(ctx).Models
	mon := computeMonetization(breakdown, adjusted, allModels)
	if mon.TotalRequests == 0 {
		return 0, 0, 0, false
	}
	return mon.TotalEffectiveCost, mon.TotalPrice, mon.Margin, true
}

func (h *Handlers) recordGatewayPrometheus(
	ctx context.Context,
	tenantID, model, provider, modelType, status string,
	totalLatencyMs int,
	errorType string,
	recordUsage bool,
	tokenCostUSD float64,
	promptTokens, completionTokens int,
) {
	if model == "" {
		model = "unknown"
	}
	if provider == "" {
		provider = "unknown"
	}
	mt := normalizeModelTypeForMetrics(modelType)
	authLabel := prometheusAuthTypeLabel(ctx)

	gatewayotel.RequestsTotal.WithLabelValues(tenantID, model, provider, mt, authLabel, status).Inc()
	gatewayotel.RequestLatencyMs.WithLabelValues(tenantID, model, provider, mt).Observe(float64(totalLatencyMs))

	if status == "error" {
		et := errorType
		if et == "" {
			et = "unknown"
		}
		gatewayotel.RequestErrorsTotal.WithLabelValues(tenantID, model, provider, et).Inc()
		return
	}

	if promptTokens > 0 {
		gatewayotel.RequestTokensTotal.WithLabelValues(tenantID, model, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		gatewayotel.RequestTokensTotal.WithLabelValues(tenantID, model, "completion").Add(float64(completionTokens))
	}

	if !recordUsage {
		return
	}

	effective, price, margin, ok := h.perRequestMonetizationUSD(ctx, tenantID, model, tokenCostUSD)
	if !ok {
		return
	}

	gatewayotel.RequestCostEffectiveUSD.WithLabelValues(tenantID, model, provider, mt).Observe(effective)
	gatewayotel.RequestPriceUSD.WithLabelValues(tenantID, model, provider, mt).Observe(price)
	gatewayotel.RequestMarginUSD.WithLabelValues(tenantID, model, provider, mt).Observe(margin)
	gatewayotel.EffectiveSpendTotalUSD.WithLabelValues(tenantID, model).Add(effective)
	gatewayotel.TotalPriceUSD.WithLabelValues(tenantID, model).Add(price)
	gatewayotel.TotalMarginUSD.WithLabelValues(tenantID, model).Add(margin)

	if m := h.resolveModelByName(ctx, model); m != nil && m.MarkupPercentage > 0 {
		gatewayotel.MarkupAppliedRequestsTotal.WithLabelValues(model).Inc()
	}
}

// gatewayPrometheusState captures one user-visible chat/embedding outcome for defer-based metrics.
type gatewayPrometheusState struct {
	skip bool

	tenantID  string
	start     time.Time
	modelType string

	status   string
	model    string
	provider string

	errorType string

	recordUsage bool
	tokenCost   float64
	promptToks  int
	compToks    int
}

func newGatewayPrometheusState(tenantID string, start time.Time) *gatewayPrometheusState {
	return &gatewayPrometheusState{
		tenantID:  tenantID,
		start:     start,
		modelType: "llm",
		status:    "error",
		model:     "unknown",
		provider:  "unknown",
	}
}

func (s *gatewayPrometheusState) setOk(model, provider string, recordUsage bool, tokenCost float64, prompt, completion int) {
	s.status = "ok"
	s.model = model
	s.provider = provider
	s.recordUsage = recordUsage
	s.tokenCost = tokenCost
	s.promptToks = prompt
	s.compToks = completion
}

func (s *gatewayPrometheusState) setError(model, provider, errType string) {
	s.status = "error"
	if model != "" {
		s.model = model
	} else {
		s.model = "unknown"
	}
	if provider != "" {
		s.provider = provider
	} else {
		s.provider = "unknown"
	}
	s.errorType = errType
}

func (s *gatewayPrometheusState) flush(h *Handlers, ctx context.Context) {
	if s == nil || s.skip {
		return
	}
	lat := int(time.Since(s.start).Milliseconds())
	h.recordGatewayPrometheus(ctx, s.tenantID, s.model, s.provider, s.modelType, s.status, lat, s.errorType,
		s.recordUsage, s.tokenCost, s.promptToks, s.compToks)
}

// recordPrometheusMLPreUpstreamError records ML request metrics for validation/authorization
// failures before an upstream call is attempted.
func (h *Handlers) recordPrometheusMLPreUpstreamError(ctx context.Context, tenantID, model, provider, errType string, start time.Time) {
	lat := int(time.Since(start).Milliseconds())
	if model == "" {
		model = "unknown"
	}
	if provider == "" {
		provider = "unknown"
	}
	h.recordGatewayPrometheus(ctx, tenantID, model, provider, "ml", "error", lat, errType, false, 0, 0, 0)
	gatewayotel.MLRequestsTotal.WithLabelValues(tenantID, model, provider, "error").Inc()
}

func (h *Handlers) recordPrometheusMLOutcome(
	ctx context.Context,
	tenantID, modelName, provider string,
	status string,
	latencyMs int,
	upstreamErr error,
	tokenCostUSD float64,
	observableFieldCount int,
) {
	if modelName == "" {
		modelName = "unknown"
	}
	if provider == "" {
		provider = "unknown"
	}
	mt := "ml"
	authLabel := prometheusAuthTypeLabel(ctx)

	gatewayotel.RequestsTotal.WithLabelValues(tenantID, modelName, provider, mt, authLabel, status).Inc()
	gatewayotel.RequestLatencyMs.WithLabelValues(tenantID, modelName, provider, mt).Observe(float64(latencyMs))
	gatewayotel.MLRequestsTotal.WithLabelValues(tenantID, modelName, provider, status).Inc()

	if status == "error" {
		et := "unknown"
		if upstreamErr != nil {
			et = string(h.errorClassifier.Classify(upstreamErr))
		}
		gatewayotel.MLUpstreamErrorsTotal.WithLabelValues(modelName, provider, et).Inc()
		gatewayotel.RequestErrorsTotal.WithLabelValues(tenantID, modelName, provider, et).Inc()
		return
	}

	if observableFieldCount > 0 {
		gatewayotel.MLObservableFieldsLoggedTotal.WithLabelValues(modelName).Add(float64(observableFieldCount))
	}

	effective, price, margin, ok := h.perRequestMonetizationUSD(ctx, tenantID, modelName, tokenCostUSD)
	if !ok {
		return
	}
	gatewayotel.RequestCostEffectiveUSD.WithLabelValues(tenantID, modelName, provider, mt).Observe(effective)
	gatewayotel.RequestPriceUSD.WithLabelValues(tenantID, modelName, provider, mt).Observe(price)
	gatewayotel.RequestMarginUSD.WithLabelValues(tenantID, modelName, provider, mt).Observe(margin)
	gatewayotel.EffectiveSpendTotalUSD.WithLabelValues(tenantID, modelName).Add(effective)
	gatewayotel.TotalPriceUSD.WithLabelValues(tenantID, modelName).Add(price)
	gatewayotel.TotalMarginUSD.WithLabelValues(tenantID, modelName).Add(margin)

	if m := h.resolveModelByName(ctx, modelName); m != nil && m.MarkupPercentage > 0 {
		gatewayotel.MarkupAppliedRequestsTotal.WithLabelValues(modelName).Inc()
	}
}
