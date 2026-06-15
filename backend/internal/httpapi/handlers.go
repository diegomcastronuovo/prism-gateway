package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/circuitbreaker"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/events"
	"github.com/diegomcastronuovo/prism-gateway/internal/hooks"
	"github.com/diegomcastronuovo/prism-gateway/internal/loggingcore"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type Handlers struct {
	cfg             *config.Config
	log             *slog.Logger
	router          *router.Router
	registry        *providers.Registry
	hooks           *hooks.Registry
	store           storage.Storage
	statsDispatcher *StatsDispatcher
	tenantCache     *config.TenantConfigCache // Cache for dynamic tenant configs
	globalCfgCache  *config.GlobalConfigCache // Cache for dynamic global config
	lazyProvMu      sync.Mutex
	lazyChatProv    map[string]providers.Provider
	lazyEmbProv     map[string]providers.EmbeddingProvider
	breaker         circuitbreaker.Breaker // distributed circuit breaker
	errorClassifier *router.ErrorClassifier
	piiTestLimiter        *piiTestRateLimiter // optional: rate limit PII test-connection per tenant
	classifierTestLimiter *piiTestRateLimiter // optional: rate limit classifier test-connection per tenant
	loggingService  *loggingcore.Service
	budgetEmitter      events.BudgetWarnEmitter         // fire-and-forget budget WARN events to Redis Streams
	orchestrator    *Orchestrator             // pipeline core; wired in server.go after Handlers construction
	asyncWg         sync.WaitGroup            // tracks fire-and-forget goroutines; drained on shutdown
	semCacheAsync   *semaphore.Weighted       // bounds fire-and-forget cache goroutines
}

type toolRouteBreakdown struct {
	embeddingModelMS    *int
	embeddingGenerateMS *int
	semanticDBMS        *int
	matchEvalMS         *int
}

func NewHandlers(cfg *config.Config, log *slog.Logger, rt *router.Router, reg *providers.Registry, hookReg *hooks.Registry, store storage.Storage, breaker circuitbreaker.Breaker) *Handlers {
	// Create stats dispatcher with bounded queue (2000 items) and 3 workers
	dispatcher := NewStatsDispatcher(store, log, 2000, 3)
	asyncCap := cfg.Server.AsyncSemaphoreCapacity
	if asyncCap <= 0 {
		asyncCap = 500
	}
	return &Handlers{
		cfg:             cfg,
		log:             log,
		router:          rt,
		registry:        reg,
		hooks:           hookReg,
		store:           store,
		statsDispatcher: dispatcher,
		breaker:         breaker,
		budgetEmitter:    events.NoopBudgetWarnEmitter{},
		errorClassifier: router.NewErrorClassifier(),
		piiTestLimiter:        newPIITestRateLimiter(),
		classifierTestLimiter: newPIITestRateLimiter(),
		loggingService:  loggingcore.New(log, store),
		semCacheAsync:   semaphore.NewWeighted(int64(asyncCap)),
	}
}

// Close gracefully shuts down the handlers, flushing pending stats.
func (h *Handlers) Close() {
	// Drain fire-and-forget goroutines before stopping the stats dispatcher
	// so all pending DB writes complete cleanly.
	h.asyncWg.Wait()
	if h.statsDispatcher != nil {
		h.statsDispatcher.Stop(10 * time.Second)
	}
}

// resolveGlobalConfig returns the active GlobalConfig: cache → DB → YAML fallback.
func (h *Handlers) resolveGlobalConfig(ctx context.Context) *config.GlobalConfig {
	gc, err := h.cfg.ResolveGlobalConfig(ctx, h.globalCfgCache, h.store, h.store, h.store, h.log)
	if err != nil {
		h.log.WarnContext(ctx, "failed to resolve global config, using YAML", "error", err)
		return config.GlobalConfigFromYAML(h.cfg)
	}
	return gc
}

// resolveModelByName looks up a model by name using the resolved global config.
func (h *Handlers) resolveModelByName(ctx context.Context, name string) *config.ModelConfig {
	return h.resolveGlobalConfig(ctx).ModelByName(name)
}

// resolveAllowedModels returns model configs allowed for the tenant from the resolved global config.
func (h *Handlers) resolveAllowedModels(ctx context.Context, tenant *config.TenantConfig) []config.ModelConfig {
	return h.resolveGlobalConfig(ctx).AllowedModelsForTenant(tenant)
}

var (
	errTenantIDRequiredForSemantic   = errors.New("tenant_id required for semantic admin operation")
	errTenantNotFoundForSemantic     = errors.New("tenant not found for semantic admin operation")
	errTenantAccessDeniedForSemantic = errors.New("tenant access denied for semantic admin operation")
)

// resolveTenantForAdminSemantic resolves *TenantConfig for semantic handlers.
//
// tenant_id resolution order:
//  1. ?tenant_id= query parameter (explicit, works for all auth types)
//  2. API-key auth context fallback: if no query param and auth type is "api_key",
//     use the tenant already bound to the key — consistent with all other endpoints.
//  3. JWT without explicit tenant_id: still required (JWT has no implicit tenant binding).
func (h *Handlers) resolveTenantForAdminSemantic(ctx context.Context, r *http.Request) (*config.TenantConfig, error) {
	tenantID := r.URL.Query().Get("tenant_id")

	// Fallback: API-key auth already implies a tenant — use it when no query param is given.
	// This aligns with how every other endpoint resolves the tenant, so callers with a valid
	// API key don't have to repeat the tenant_id they can't change anyway.
	if tenantID == "" && auth.AuthTypeFromContext(ctx) == "api_key" {
		tenantID = auth.TenantIDFromContext(ctx)
	}

	if tenantID == "" {
		return nil, errTenantIDRequiredForSemantic
	}

	roles := auth.RolesFromContext(ctx)
	if len(roles) > 0 {
		// JWT role-based authorization: only admin/local_admin are allowed on semantic endpoints.
		if auth.HasAnyRole(roles, adminBypassRoles) {
			// admin role can target any tenant_id.
		} else if auth.HasAnyRole(roles, []string{"local_admin"}) {
			if !auth.TenantInRequestAllowed(tenantID, auth.AllowedTenantsFromContext(ctx)) {
				return nil, errTenantAccessDeniedForSemantic
			}
		} else {
			return nil, errTenantAccessDeniedForSemantic
		}
	} else if auth.AuthTypeFromContext(ctx) == "api_key" {
		// If an explicit tenant_id was provided, verify the API key is allowed to target it.
		if apiKeyTenant := auth.TenantIDFromContext(ctx); apiKeyTenant != "" && apiKeyTenant != tenantID {
			return nil, errTenantAccessDeniedForSemantic
		}
	}

	tenant, err := h.cfg.ResolveTenantConfig(ctx, tenantID, h.tenantCache, h.store)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, errTenantNotFoundForSemantic
	}
	return tenant, nil
}

func (h *Handlers) writeSemanticTenantResolveError(w http.ResponseWriter, ctx context.Context, err error) {
	if errors.Is(err, errTenantIDRequiredForSemantic) {
		writeError(w, http.StatusBadRequest, "tenant_id is required", "invalid_request_error")
		return
	}
	if errors.Is(err, errTenantNotFoundForSemantic) {
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}
	if errors.Is(err, errTenantAccessDeniedForSemantic) {
		writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
		return
	}
	h.log.ErrorContext(ctx, "failed to resolve tenant for semantic admin operation", "error", err)
	writeError(w, http.StatusInternalServerError, "failed to resolve tenant", "internal_error")
}

// Healthz is the health check endpoint.
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ListModels returns the models allowed for the authenticated tenant.
func (h *Handlers) ListModels(w http.ResponseWriter, r *http.Request) {
	tenant := auth.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication_error")
		return
	}

	models := h.resolveAllowedModels(r.Context(), tenant)
	now := time.Now().Unix()

	resp := ModelsResponse{Object: "list"}
	for _, m := range models {
		resp.Data = append(resp.Data, ModelObject{
			ID:      m.Name,
			Object:  "model",
			Created: now,
			OwnedBy: m.Provider,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// ChatCompletions handles /v1/chat/completions by delegating to the Orchestrator.
// After Phase 3 this function is <=50 lines (Invariant I-3).
func (h *Handlers) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	tenant := auth.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication_error")
		return
	}

	parsed, err := parseChatCompletionsRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	// ML pass-through handled inside Orchestrator.Run via RawBody.

	// Ensure the orchestrator is wired (tests that construct Handlers directly may not wire it).
	if h.orchestrator == nil {
		h.orchestrator = &Orchestrator{h: h}
	}

	var sink ResponseSink
	if parsed.Stream {
		sink = newSSEResponseSink(w, parsed.BodyModel)
	}

	out := h.orchestrator.Run(r.Context(), OrchestratorInput{
		Req:    parsed,
		Tenant: tenant,
		W:      w,
		R:      r,
		Sink:   sink,
		ChatProviderFor: func(ctx context.Context, modelCfg *config.ModelConfig) (providers.Provider, bool) {
			return h.providerForChatModel(ctx, modelCfg)
		},
		EmbeddingProviderFor: func(ctx context.Context, provider string) (providers.EmbeddingProvider, bool) {
			return h.embeddingProviderFor(ctx, provider)
		},
	})

	// Non-streaming success path: the orchestrator returned a CanonicalResponse.
	// Copy extra headers, then write the JSON body.
	if out.Response != nil {
		for k, v := range out.Response.ExtraHeaders {
			w.Header().Set(k, v)
		}
		// When the upstream provider returned a raw body (OpenAI-compatible providers),
		// write it directly to avoid losing fields like usage.prompt_tokens_details,
		// service_tier, system_fingerprint, message.annotations, etc.
		if len(out.Response.RawPassthroughBody) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(out.Response.RawPassthroughBody) //nolint:errcheck
			return
		}
		writeJSON(w, http.StatusOK, canonicalToChatCompletionResponse(out.Response))
	}
}

func (h *Handlers) logConversationAsync(
	ctx context.Context,
	tenant *config.TenantConfig,
	requestID string,
	promptMessages []string,
	choices []ChatChoice,
	piiRequestDecision, piiResponseDecision *string,
	jwtSub *string,
	customerID *string,
) {
	if tenant == nil {
		return
	}
	// Conversation logging is gated by the static YAML config.
	// Per-tenant behavior is controlled by tenant.Logging.Mode below.
	if !h.cfg.ConversationLogging.Enabled {
		return
	}
	mode := strings.TrimSpace(tenant.Logging.Mode)
	if mode == "" {
		mode = "metadata_only"
	}
	if mode == "disabled" {
		return
	}
	promptFull := strings.TrimSpace(strings.Join(promptMessages, " "))
	responseFull := ""
	if len(choices) > 0 {
		responseFull = strings.TrimSpace(choices[0].Message.TextContent())
	}
	promptRedacted := redactTextPII(promptFull)
	responseRedacted := redactTextPII(responseFull)
	piiDetected := (piiRequestDecision != nil && *piiRequestDecision != "allow") || (piiResponseDecision != nil && *piiResponseDecision != "allow")

	promptPreview := redactConversationPreview(truncateText(promptFull, 400))
	responsePreview := redactConversationPreview(truncateText(responseFull, 400))

	row := storage.ConversationLog{
		ID:              uuid.New(),
		RequestID:       requestID,
		TenantID:        tenant.ID,
		JWTSub:          jwtSub,
		CustomerID:      customerID,
		PromptPreview:   promptPreview,
		ResponsePreview: responsePreview,
		PIIDetected:     piiDetected,
		LoggingMode:     "redacted",
	}
	row.PromptRedacted = &promptRedacted
	row.ResponseRedacted = &responseRedacted

	if h.loggingService == nil {
		h.loggingService = loggingcore.New(h.log, h.store)
	}
	event := loggingcore.LogEvent{
		Type:      loggingcore.EventTypeConversation,
		TenantID:  tenant.ID,
		RequestID: requestID,
		Payload: map[string]interface{}{
			"conversation_log": row,
		},
	}
	if err := h.loggingService.LogWithContext(ctx, event); err != nil {
		h.log.ErrorContext(ctx, "failed to persist conversation log", "request_id", requestID, "error", err)
	}
}

func redactTextPII(s string) string {
	out := emailPattern.ReplaceAllString(s, "[REDACTED-EMAIL]")
	out = phonePattern.ReplaceAllString(out, "[REDACTED-PHONE]")
	out = creditCardPattern.ReplaceAllString(out, "[REDACTED-CC]")
	return out
}

// redactConversationPreview applies PII redaction to a conversation preview string.
func redactConversationPreview(s string) string {
	return redactTextPII(s)
}

func truncateText(s string, n int) string {
	runes := []rune(s)
	if n <= 0 || len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// validateMetadata validates shape, sizes, and serializes metadata to JSON.
// Returns (nil, nil) when metadata is nil (field omitted — backward compatible).
func validateMetadata(raw map[string]interface{}) (json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	if len(raw) > 20 {
		return nil, fmt.Errorf("metadata: too many keys (max 20)")
	}
	for k, v := range raw {
		if len(k) > 64 {
			return nil, fmt.Errorf("metadata: key %q exceeds 64 chars", k)
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("metadata: value for key %q must be a string", k)
		}
		if len(s) > 256 {
			return nil, fmt.Errorf("metadata: value for key %q exceeds 256 chars", k)
		}
	}
	b, _ := json.Marshal(raw)
	if len(b) > 4096 {
		return nil, fmt.Errorf("metadata: serialized size exceeds 4KB")
	}
	return json.RawMessage(b), nil
}

// logRequestAsync persists a request log entry. Errors are logged, not propagated.
// Uses context.Background() for the DB write so that request-context cancellation
// (e.g. client disconnect, per-attempt timeout expiry) never silently drops log rows.
func (h *Handlers) logRequestAsync(ctx context.Context, rl storage.RequestLog) {
	if h.loggingService == nil {
		h.loggingService = loggingcore.New(h.log, h.store)
	}
	event := loggingcore.LogEvent{
		Type:      loggingcore.EventTypeConversation,
		TenantID:  rl.TenantID,
		RequestID: rl.RequestID,
		Payload: map[string]interface{}{
			"request_log": rl,
		},
	}
	if err := h.loggingService.LogWithContext(context.Background(), event); err != nil {
		h.log.ErrorContext(ctx, "failed to persist request log",
			"request_id", rl.ID, "error", err)
	}
}

// releaseReservationAsync deletes the budget reservation row after a successful request.
// No-op when reservationID is uuid.Nil (no reservation was made).
func (h *Handlers) releaseReservationAsync(ctx context.Context, reservationID uuid.UUID) {
	if reservationID == uuid.Nil {
		return
	}
	if err := h.store.ReleaseReservation(context.Background(), reservationID); err != nil {
		h.log.WarnContext(ctx, "failed to release budget reservation",
			"reservation_id", reservationID, "error", err)
	}
}

// logInternalEmbeddingAsync records one usage row and one request_log row for all internal
// embedding calls made during a single request (semantic routing, cache, tool routing).
// tokens is what the provider returned; chars is the total length of all embedded texts
// used as fallback when the provider returns tokens=0 (estimate: chars/4).
// Runs fire-and-forget; errors are suppressed.
func (h *Handlers) logInternalEmbeddingAsync(
	ctx context.Context,
	requestID string,
	tenant *config.TenantConfig,
	model *config.ModelConfig,
	tokens int,
	chars int,
) {
	h.asyncWg.Add(1)
	go func() {
		defer h.asyncWg.Done()
		effectiveTokens := tokens
		if effectiveTokens == 0 && chars > 0 {
			effectiveTokens = chars / 4 // chars-to-tokens heuristic (same as /v1/embeddings fallback)
		}
		// Use the resolved model's pricing; if the dynamic global config was seeded before
		// pricing was configured (PromptPer1M == 0), fall back to the YAML config's pricing
		// for the same model so cost is always attributed correctly.
		pricing := model.Pricing
		if pricing.PromptPer1M == 0 && pricing.CompletionPer1M == 0 {
			if yamlModel := h.cfg.ModelByName(model.Name); yamlModel != nil {
				pricing = yamlModel.Pricing
			}
		}
		costUSD := float64(effectiveTokens) / 1_000_000 * pricing.PromptPer1M
		apiKeyID, apiKeyName := auth.APIKeyAttributionFromContext(ctx)
		jwtSub := auth.JWTSubFromContext(ctx)
		now := time.Now().UTC()

		_ = h.store.SaveUsage(context.Background(), storage.UsageRecord{
			ID:           uuid.New(),
			Timestamp:    now,
			TenantID:     tenant.ID,
			Model:        model.Name,
			Provider:     model.Provider,
			PromptTokens: effectiveTokens,
			TotalTokens:  effectiveTokens,
			CostUSD:      costUSD,
			RequestID:    requestID,
			APIKeyID:     apiKeyID,
			APIKeyName:   apiKeyName,
			JWTSub:       jwtSub,
		})

		_ = h.store.LogRequest(context.Background(), storage.RequestLog{
			ID:         uuid.New(),
			RequestID:  requestID,
			Attempt:    1,
			Timestamp:  now,
			TenantID:   tenant.ID,
			Model:      model.Name,
			Provider:   model.Provider,
			Strategy:   "embedding",
			Status:     "ok",
			APIKeyID:   apiKeyID,
			APIKeyName: apiKeyName,
			JWTSub:     jwtSub,
		})
	}()
}

// saveUsageAsync persists a usage record. Errors are logged, not propagated.
func (h *Handlers) saveUsageAsync(ctx context.Context, u storage.UsageRecord) {
	// Use context.Background() for the DB write so that request-context cancellation
	// (e.g. client disconnect after a slow LLM call) never silently drops usage rows.
	if err := h.store.SaveUsage(context.Background(), u); err != nil {
		h.log.ErrorContext(ctx, "failed to persist usage record",
			"request_id", u.RequestID, "error", err)
		return
	}

}

// checkBudget checks the tenant's monthly budget. Returns (blocked, reservationID).
// reservationID is non-nil (uuid.Nil == zero) when a reservation was successfully inserted.
func (h *Handlers) checkBudget(ctx context.Context, span trace.Span, tenant *config.TenantConfig) (bool, uuid.UUID) {
	loc := time.UTC
	if tenant.Budgets.Timezone != "" {
		parsed, err := time.LoadLocation(tenant.Budgets.Timezone)
		if err != nil {
			h.log.WarnContext(ctx, "invalid budget timezone, using UTC",
				"tenant", tenant.ID, "timezone", tenant.Budgets.Timezone, "error", err)
		} else {
			loc = parsed
		}
	}

	now := time.Now().In(loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)

	// Use a small estimated cost for the reservation (we don't know actual cost yet).
	// This prevents two concurrent requests from both passing when only one fits.
	// The reservation is conservative — actual cost is tracked via the usage table.
	const estimatedCostUSD = 0.01

	check, err := h.store.CheckAndReserveBudget(ctx, tenant.ID, monthStart.UTC(), monthEnd.UTC(), tenant.Budgets.MonthlyUSD, estimatedCostUSD)

	span.AddEvent("budget_check", trace.WithAttributes(
		attribute.String("tenant.id", tenant.ID),
		attribute.Float64("budget.limit_usd", tenant.Budgets.MonthlyUSD),
		attribute.Float64("budget.month_spend_usd", check.MonthSpendUSD),
	))

	if err != nil {
		if errors.Is(err, storage.ErrBudgetExceeded) {
			h.log.WarnContext(ctx, "budget exceeded, blocking request",
				"tenant", tenant.ID,
				"budget_limit", tenant.Budgets.MonthlyUSD,
				"month_spend", check.MonthSpendUSD,
			)
			return true, uuid.Nil
		}
		// DB error — log but allow the request through (fail open)
		h.log.ErrorContext(ctx, "budget check failed, allowing request",
			"tenant", tenant.ID, "error", err)
		gatewayotel.BudgetCheckFailTotal.WithLabelValues(tenant.ID).Inc()
	}

	return false, check.ReservationID
}

// applyBudgetEnforcement implements the advanced budget enforcement logic (mode: report_only|block|degrade).
// Returns (degradeGroup, blocked, httpStatus, msg, reservationID).
// reservationID is uuid.Nil when no reservation was inserted (report_only, fail-open, or blocked).
// On DB error, always returns fail-open (no block, no degrade).
func (h *Handlers) applyBudgetEnforcement(
	ctx context.Context, span trace.Span,
	tenant *config.TenantConfig,
	metadata map[string]interface{},
) (degradeGroup string, blocked bool, httpStatus int, msg string, reservationID uuid.UUID) {
	enf := &tenant.BudgetEnforcement

	// Default helpers
	warnPct := enf.Thresholds.WarnPct
	if warnPct <= 0 {
		warnPct = 0.80
	}
	hardPct := enf.Thresholds.HardPct
	if hardPct <= 0 {
		hardPct = 1.00
	}
	blockStatus := enf.BlockStatus
	if blockStatus == 0 {
		blockStatus = http.StatusPaymentRequired // 402
	}

	// Compute month window (same as checkBudget)
	loc := time.UTC
	if tenant.Budgets.Timezone != "" {
		parsed, err := time.LoadLocation(tenant.Budgets.Timezone)
		if err != nil {
			h.log.WarnContext(ctx, "invalid budget timezone, using UTC",
				"tenant", tenant.ID, "timezone", tenant.Budgets.Timezone, "error", err)
		} else {
			loc = parsed
		}
	}
	now := time.Now().In(loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)

	budget := tenant.Budgets.MonthlyUSD
	if budget <= 0 {
		// No budget configured — nothing to enforce
		return "", false, 0, "", uuid.Nil
	}

	// --- Tenant-level spend check ---
	var spend float64
	var hardBreached bool
	var resID uuid.UUID // reservation inserted by CheckAndReserveBudget; uuid.Nil if none

	if enf.Mode == "report_only" {
		var err error
		spend, err = h.store.GetMonthlySpend(ctx, tenant.ID, monthStart.UTC(), monthEnd.UTC())
		if err != nil {
			h.log.ErrorContext(ctx, "budget enforcement: get monthly spend failed, allowing request",
				"tenant", tenant.ID, "error", err)
			gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "error", "tenant").Inc()
			gatewayotel.BudgetCheckFailTotal.WithLabelValues(tenant.ID).Inc()
			return "", false, 0, "", uuid.Nil
		}
		hardBreached = spend/budget >= hardPct
	} else {
		// block or degrade — use CheckAndReserveBudget for atomic enforcement
		const estimatedCostUSD = 0.01
		check, err := h.store.CheckAndReserveBudget(ctx, tenant.ID, monthStart.UTC(), monthEnd.UTC(), budget*hardPct, estimatedCostUSD)
		spend = check.MonthSpendUSD
		resID = check.ReservationID // uuid.Nil when ErrBudgetExceeded (no row inserted)
		if err != nil {
			if errors.Is(err, storage.ErrBudgetExceeded) {
				hardBreached = true
			} else {
				h.log.ErrorContext(ctx, "budget enforcement: reserve failed, allowing request",
					"tenant", tenant.ID, "error", err)
				gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "error", "tenant").Inc()
				gatewayotel.BudgetCheckFailTotal.WithLabelValues(tenant.ID).Inc()
				return "", false, 0, "", uuid.Nil
			}
		}
	}

	span.AddEvent("budget_enforcement_check", trace.WithAttributes(
		attribute.String("tenant.id", tenant.ID),
		attribute.Float64("budget.limit_usd", budget),
		attribute.Float64("budget.month_spend_usd", spend),
		attribute.String("budget.mode", enf.Mode),
	))

	pct := spend / budget
	tenantMsg := fmt.Sprintf("monthly budget exceeded for tenant '%s'", tenant.ID)
	if enf.IncludeCostInError {
		tenantMsg = fmt.Sprintf("monthly budget exceeded for tenant '%s' (spend: $%.4f / $%.4f)", tenant.ID, spend, budget)
	}

	if hardBreached {
		gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "hard", "tenant").Inc()
		h.log.WarnContext(ctx, "budget enforcement: hard threshold reached",
			"tenant", tenant.ID, "spend", spend, "budget", budget, "mode", enf.Mode)
		switch enf.Mode {
		case "report_only":
			// observe only — continue normally
		case "block":
			gatewayotel.BudgetBlockCounter.WithLabelValues(tenant.ID, "tenant").Inc()
			// Request is blocked — not served — reservation stays (conservative).
			return "", true, blockStatus, tenantMsg, uuid.Nil
		case "degrade":
			gatewayotel.BudgetDegradeCounter.WithLabelValues(tenant.ID, "tenant").Inc()
			// Request proceeds with degraded routing; propagate resID so it can be released on success.
			return enf.DegradeRouteGroup, false, 0, "", resID
		}
	} else if pct >= warnPct {
		gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "warn", "tenant").Inc()
		h.log.WarnContext(ctx, "budget enforcement: warn threshold reached",
			"tenant", tenant.ID, "spend", spend, "budget", budget, "pct", pct)
		h.budgetEmitter.EmitBudgetWarn(events.BudgetWarnPayload{
			TenantID:  tenant.ID,
			Level:     "tenant",
			SpendUSD:  spend,
			BudgetUSD: budget,
			Pct:       pct,
			WarnPct:   warnPct,
		})
	} else {
		gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "ok", "tenant").Inc()
	}

	// --- Tag-level budget check ---
	if enf.TagBudgets.Enabled && len(enf.TagBudgets.Keys) > 0 && len(metadata) > 0 {
		for _, key := range enf.TagBudgets.Keys {
			val, ok := metadata[key].(string)
			if !ok || val == "" {
				continue
			}
			tagByVal, exists := enf.TagBudgets.MonthlyUSDByTag[key]
			if !exists {
				continue
			}
			tagLimit, exists := tagByVal[val]
			if !exists || tagLimit <= 0 {
				continue
			}
			tagSpend, err := h.store.GetTagMonthlySpend(ctx, tenant.ID, key, val, monthStart.UTC(), monthEnd.UTC())
			if err != nil {
				h.log.ErrorContext(ctx, "budget enforcement: get tag spend failed, allowing request",
					"tenant", tenant.ID, "tag_key", key, "tag_val", val, "error", err)
				gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "error", "tag").Inc()
				gatewayotel.BudgetCheckFailTotal.WithLabelValues(tenant.ID).Inc()
				continue // fail open for this tag
			}

			tagPct := tagSpend / tagLimit
			tagMsg := fmt.Sprintf("monthly budget exceeded for %s '%s' (tenant '%s')", key, val, tenant.ID)
			if enf.IncludeCostInError {
				tagMsg = fmt.Sprintf("monthly budget exceeded for %s '%s' (tenant '%s', spend: $%.4f / $%.4f)", key, val, tenant.ID, tagSpend, tagLimit)
			}

			if tagPct >= hardPct {
				gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "hard", "tag").Inc()
				h.log.WarnContext(ctx, "budget enforcement: tag hard threshold reached",
					"tenant", tenant.ID, "tag_key", key, "tag_val", val, "spend", tagSpend, "limit", tagLimit, "mode", enf.Mode)
				switch enf.Mode {
				case "report_only":
					// observe only — continue normally
				case "block":
					gatewayotel.BudgetBlockCounter.WithLabelValues(tenant.ID, "tag").Inc()
					// Request blocked by tag budget — not served — reservation stays.
					return "", true, blockStatus, tagMsg, uuid.Nil
				case "degrade":
					gatewayotel.BudgetDegradeCounter.WithLabelValues(tenant.ID, "tag").Inc()
					return enf.DegradeRouteGroup, false, 0, "", resID
				}
			} else if tagPct >= warnPct {
				gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "warn", "tag").Inc()
				h.log.WarnContext(ctx, "budget enforcement: tag warn threshold reached",
					"tenant", tenant.ID, "tag_key", key, "tag_val", val, "pct", tagPct)
				h.budgetEmitter.EmitBudgetWarn(events.BudgetWarnPayload{
					TenantID:  tenant.ID,
					Level:     "tag",
					TagKey:    key,
					TagValue:  val,
					SpendUSD:  tagSpend,
					BudgetUSD: tagLimit,
					Pct:       tagPct,
					WarnPct:   warnPct,
				})
			} else {
				gatewayotel.BudgetCheckCounter.WithLabelValues(tenant.ID, "ok", "tag").Inc()
			}
		}
	}

	return "", false, 0, "", resID
}

// newTextMessage constructs a ChatMessage whose Content is a JSON string literal.
func newTextMessage(role, text string) ChatMessage {
	b, _ := json.Marshal(text)
	return ChatMessage{Role: role, Content: json.RawMessage(b)}
}

// mapProviderMessage converts a providers.ChatMessage to httpapi.ChatMessage,
// preserving tool_calls when the provider response uses function calling.
func mapProviderMessage(m providers.ChatMessage) ChatMessage {
	if len(m.ToolCalls) > 0 {
		return ChatMessage{
			Role:      m.Role,
			Content:   json.RawMessage("null"),
			ToolCalls: m.ToolCalls,
		}
	}
	b, _ := json.Marshal(m.Content)
	return ChatMessage{Role: m.Role, Content: json.RawMessage(b)}
}

func toProviderMessages(msgs []ChatMessage) []providers.ChatMessage {
	out := make([]providers.ChatMessage, len(msgs))
	for i, m := range msgs {
		pm := providers.ChatMessage{
			Role:       m.Role,
			Content:    m.TextContent(),
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
		}
		if urls := m.ImageURLs(); len(urls) > 0 {
			// Include text block first (if any), then one block per image URL.
			if text := m.TextContent(); text != "" {
				pm.ContentBlocks = append(pm.ContentBlocks, providers.MessageContentBlock{
					Type: "text",
					Text: text,
				})
			}
			for _, u := range urls {
				pm.ContentBlocks = append(pm.ContentBlocks, providers.MessageContentBlock{
					Type:     "image_url",
					ImageURL: &providers.ImageURLData{URL: u},
				})
			}
		}
		out[i] = pm
	}
	return out
}

// computeCost calculates the token cost for a request using the full pricing model:
// - Cached reads are billed at cached_input_per_1m (cheaper than regular input)
// - Anthropic cache writes are billed at cache_write_5m_per_1m / cache_write_1h_per_1m
// - Long context applies when prompt_tokens >= long_context_start_tokens
// - GeoMultiplierUS is applied when usage.InferenceGeo == "us"
// - Tool calls are billed per call using tool_catalog prices
func computeCost(pricing config.Pricing, usage providers.Usage, toolPricing map[string]config.ToolPricingEntry) float64 {
	// Tokens billed at the base input rate = total input minus cache reads and cache writes
	cacheWriteTotal := usage.CacheWrite5mTokens + usage.CacheWrite1hTokens
	nonCachedPrompt := usage.PromptTokens - usage.CachedInputTokens - cacheWriteTotal
	if nonCachedPrompt < 0 {
		nonCachedPrompt = 0
	}

	var promptRate, cachedRate, completionRate float64
	if pricing.LongContext && usage.PromptTokens >= pricing.LongContextStartTokens {
		promptRate = pricing.LongContextPromptPer1M
		cachedRate = pricing.LongContextCachedInputPer1M
		completionRate = pricing.LongContextCompletionPer1M
	} else {
		promptRate = pricing.PromptPer1M
		cachedRate = pricing.CachedInputPer1M
		completionRate = pricing.CompletionPer1M
	}

	tokenCost := float64(nonCachedPrompt)*promptRate/1_000_000 +
		float64(usage.CachedInputTokens)*cachedRate/1_000_000 +
		float64(usage.CacheWrite5mTokens)*pricing.CacheWrite5mPer1M/1_000_000 +
		float64(usage.CacheWrite1hTokens)*pricing.CacheWrite1hPer1M/1_000_000 +
		float64(usage.CompletionTokens)*completionRate/1_000_000

	// Apply geo multiplier when Anthropic reports "us" inference.
	if usage.InferenceGeo == "us" && pricing.GeoMultiplierUS > 1.0 {
		tokenCost *= pricing.GeoMultiplierUS
	}

	// Tool costs
	toolCost := 0.0
	for toolType, count := range usage.ToolCallsUsed {
		for _, entry := range toolPricing {
			if entry.ToolType == toolType {
				toolCost += float64(count) * entry.PricePerUnit
				break
			}
		}
	}

	return tokenCost + toolCost
}

// computeToolCost returns only the tool-call portion of the total cost.
func computeToolCost(usage providers.Usage, toolPricing map[string]config.ToolPricingEntry) float64 {
	toolCost := 0.0
	for toolType, count := range usage.ToolCallsUsed {
		for _, entry := range toolPricing {
			if entry.ToolType == toolType {
				toolCost += float64(count) * entry.PricePerUnit
				break
			}
		}
	}
	return toolCost
}

func withTimeoutCtx(parent context.Context, timeoutMs int) (context.Context, func()) {
	return context.WithTimeout(parent, time.Duration(timeoutMs)*time.Millisecond)
}

// cacheTTLSeconds returns ttl if positive, otherwise the 24-hour default.
// Needed because dynamic tenant configs loaded from DB may have ttl_seconds=0
// when the field was never explicitly set.
func cacheTTLSeconds(ttl int) int {
	if ttl > 0 {
		return ttl
	}
	return 86400
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	if ue, ok := err.(*providers.UpstreamError); ok {
		status := ue.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		writeError(w, status, sanitizeUpstreamMessage(ue.StatusCode), "upstream_error")
		return
	}
	// Never forward raw internal error strings (may contain URLs with API keys, etc.)
	writeError(w, http.StatusBadGateway, "upstream provider error", "upstream_error")
}

// sanitizeUpstreamMessage returns a safe, normalized message for upstream errors.
// Raw provider error bodies are never forwarded to clients to prevent info leakage.
func sanitizeUpstreamMessage(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "upstream provider rate limit exceeded"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "upstream provider authentication error"
	case status == http.StatusServiceUnavailable:
		return "upstream provider unavailable"
	case status >= 500:
		return "upstream provider error"
	case status >= 400:
		return "upstream provider rejected request"
	default:
		return "upstream provider error"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message, errType string) {
	writeJSON(w, status, ErrorResponse{
		Error: ErrorDetail{Message: message, Type: errType},
	})
}

// routerErrMessage extracts the loggable message and error type from a router error,
// mirroring the same logic used by writeRouterError for the HTTP response.
func routerErrMessage(err error) (message, errType string) {
	switch e := err.(type) {
	case *router.ErrBlockedBySmartStage:
		if e.IsPromptLength && !e.HasCustomReason {
			return "prompt length exceeds allowed limit", "invalid_request_error"
		}
		return e.Error(), "content_policy_violation"
	case *router.ErrNoCandidatesAfterSmartStages:
		return e.Error(), "no_candidates_error"
	case *router.ErrNoAllowedModels:
		return e.Error(), "permission_error"
	case *router.ErrModelNotAllowed:
		return e.Error(), "permission_error"
	case *router.ErrModelNotInRouteGroup:
		return e.Error(), "invalid_request_error"
	case *router.ErrRouteGroupNotFound:
		return e.Error(), "invalid_request_error"
	case *router.ErrDefaultRouteGroupEmpty:
		return e.Error(), "invalid_configuration"
	case *router.ErrAllCandidatesCircuitBroken:
		return e.Error(), "circuit_breaker_error"
	default:
		return err.Error(), "routing_error"
	}
}

// logRoutingError persists a request_log row for a routing-phase error (e.g. ErrBlockedBySmartStage,
// ErrModelNotAllowed). sm is the SmartStageResult returned in Result even on error (SPEC_150 pattern);
// it may be nil when routing failed before smart stage evaluation (e.g. precedence errors).
// Errors from the store are suppressed (fire-and-forget).
func (h *Handlers) logRoutingError(
	ctx context.Context,
	tenant *config.TenantConfig,
	requestID uuid.UUID,
	requestStart time.Time,
	prec router.PrecedenceDecision,
	sm *router.SmartStageResult,
	routingErr error,
) {
	msg, errType := routerErrMessage(routingErr)

	var sd *router.SmartDecision
	if sm != nil && (sm.Blocked || len(sm.StagesEvaluated) > 0 || sm.SemanticAnchor != "") {
		sd = &router.SmartDecision{
			StagesEvaluated:    sm.StagesEvaluated,
			PreferredModels:    sm.PreferredModels,
			BannedModels:       sm.BannedModels,
			Blocked:            sm.Blocked,
			BlockReason:        sm.BlockReason,
			PromptLength:       sm.PromptLength,
			SemanticAnchor:     sm.SemanticAnchor,
			SemanticSimilarity: sm.SemanticSimilarity,
			SemanticDistance:   sm.SemanticDistance,
			AnchorRouteGroup:   sm.AnchorRouteGroup,
		}
		if len(sm.RulesMatched) > 0 {
			sd.SmartEvaluation = &router.SmartEvaluationSnapshot{
				StagesEvaluated: sm.StagesEvaluated,
				RulesMatched:    sm.RulesMatched,
			}
		}
	}

	snap := router.DecisionSnapshot{
		Precedence: prec,
		Routing:    router.RoutingDecision{Strategy: tenant.Routing.Strategy},
		Smart:      sd,
	}
	snapJSON, _ := snap.ToJSON()

	apiKeyID, apiKeyName := auth.APIKeyAttributionFromContext(ctx)
	jwtSub := auth.JWTSubFromContext(ctx)

	_ = h.store.LogRequest(ctx, storage.RequestLog{
		RequestID:        requestID.String(),
		Attempt:          1,
		Timestamp:        requestStart.UTC(),
		TenantID:         tenant.ID,
		Strategy:         tenant.Routing.Strategy,
		Status:           "error",
		Error:            msg,
		ErrorType:        errType,
		DecisionSnapshot: snapJSON,
		APIKeyID:         apiKeyID,
		APIKeyName:       apiKeyName,
		JWTSub:           jwtSub,
	})
}

// writeRouterError maps router errors to appropriate HTTP status codes
func writeRouterError(w http.ResponseWriter, err error) {
	switch e := err.(type) {
	case *router.ErrBlockedBySmartStage:
		if e.IsPromptLength && !e.HasCustomReason {
			// No custom reason configured: use generic prompt-length validation error.
			writeError(w, http.StatusBadRequest, "prompt length exceeds allowed limit", "invalid_request_error")
		} else {
			// Custom reason (or non-prompt-length block): use the stage's configured reason.
			writeError(w, http.StatusForbidden, e.Error(), "content_policy_violation")
		}
	case *router.ErrNoCandidatesAfterSmartStages:
		writeError(w, http.StatusBadRequest, e.Error(), "no_candidates_error")
	case *router.ErrNoAllowedModels:
		writeError(w, http.StatusBadRequest, e.Error(), "permission_error")
	case *router.ErrModelNotAllowed:
		writeError(w, http.StatusForbidden, e.Error(), "permission_error")
	case *router.ErrModelNotInRouteGroup:
		writeError(w, http.StatusBadRequest, e.Error(), "invalid_request_error")
	case *router.ErrRouteGroupNotFound:
		writeError(w, http.StatusBadRequest, e.Error(), "invalid_request_error")
	case *router.ErrDefaultRouteGroupEmpty:
		writeError(w, http.StatusBadRequest, e.Error(), "invalid_configuration")
	case *router.ErrAllCandidatesCircuitBroken:
		writeError(w, http.StatusServiceUnavailable, e.Error(), "circuit_breaker_error")
	case *router.ErrAllAttemptsFailed:
		writeError(w, http.StatusBadGateway, e.Error(), "upstream_error")
	default:
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
	}
}

// isCBFailure returns true for errors that count as circuit-breaker failures per spec.
// Failures: upstream 5xx, timeout, network errors, and unclassified errors.
// Not failures: HTTP 429 (rate_limited) and HTTP 4xx (auth, invalid_request).
func isCBFailure(errType router.ErrorType) bool {
	switch errType {
	case router.ErrorTypeRateLimited, router.ErrorTypeAuth, router.ErrorTypeInvalidRequest:
		return false
	default:
		// Upstream5xx, Timeout, Network, and Unknown all count as CB failures.
		// Unknown is treated as a failure to avoid silently ignoring unclassified upstream errors.
		return true
	}
}

// hasSemanticRule reports whether any stage in cfg contains a semantic_similarity condition.
func hasSemanticRule(cfg *config.SmartConfig) bool {
	for _, stage := range cfg.Stages {
		for _, rule := range stage.Rules {
			if rule.When.SemanticSimilarity != nil {
				return true
			}
		}
	}
	return false
}

// checkToolRoutes is the first-class tool/action selection stage.
// It computes a prompt embedding and checks for a matching semantic route.
// Returns (match, true) if a route is found with similarity >= its threshold.
// Fail-open: on any error returns (zero, false).
// Uses the tenant's configured embedding model (same model used when creating utterance embeddings)
// so the query vector is always comparable to stored vectors.
func (h *Handlers) checkToolRoutes(ctx context.Context, messages []string, tenant *config.TenantConfig, addEmb func(tokens, chars int, model *config.ModelConfig)) (storage.SemanticRouteMatch, bool, toolRouteBreakdown) {
	var br toolRouteBreakdown
	stageAStart := time.Now()
	embModel, embErr := h.embeddingModelForModality(ctx, tenant, "text")
	stageAMS := int(time.Since(stageAStart).Milliseconds())
	br.embeddingModelMS = &stageAMS
	if embModel == nil {
		h.log.WarnContext(ctx, "tool route: no embedding model, skipping",
			"tenant", tenant.ID,
			"semantic_embedding_model", tenant.Routing.Semantic.EmbeddingModel,
			"error", embErr)
		return storage.SemanticRouteMatch{}, false, br
	}
	var ep providers.EmbeddingProvider
	if embModel.Mock.Enabled {
		ep = providers.NewMockProvider(embModel.Mock, embModel.Name, tenant.ID, embModel.Pricing, nil)
	} else {
		var ok bool
		ep, ok = h.embeddingProviderFor(ctx, embModel.Provider)
		if !ok {
			h.log.WarnContext(ctx, "tool route: embedding provider not available, skipping",
				"tenant", tenant.ID, "provider", embModel.Provider, "model", embModel.Name)
			return storage.SemanticRouteMatch{}, false, br
		}
	}

	text := strings.Join(messages, " ")
	stageBStart := time.Now()
	resp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{Input: []string{text}, Model: embModel.Name})
	stageBMS := int(time.Since(stageBStart).Milliseconds())
	br.embeddingGenerateMS = &stageBMS
	if err != nil || len(resp.Data) == 0 {
		h.log.WarnContext(ctx, "tool route: embedding failed, skipping",
			"tenant", tenant.ID, "model", embModel.Name, "error", err)
		return storage.SemanticRouteMatch{}, false, br
	}
	addEmb(resp.Usage.PromptTokens, len(text), embModel)

	stageCStart := time.Now()
	match, found, err := h.store.GetNearestSemanticRoute(ctx, tenant.ID, resp.Data[0].Embedding)
	stageCMS := int(time.Since(stageCStart).Milliseconds())
	br.semanticDBMS = &stageCMS
	if err != nil {
		h.log.WarnContext(ctx, "tool route: db lookup failed, skipping",
			"tenant", tenant.ID, "error", err)
		return storage.SemanticRouteMatch{}, false, br
	}
	stageDStart := time.Now()
	if !found {
		h.log.InfoContext(ctx, "tool route: no routes found in DB for tenant", "tenant", tenant.ID)
		stageDMS := int(time.Since(stageDStart).Milliseconds())
		br.matchEvalMS = &stageDMS
		return storage.SemanticRouteMatch{}, false, br
	}
	if match.Similarity < match.Threshold {
		h.log.InfoContext(ctx, "tool route: below threshold",
			"tenant", tenant.ID, "route", match.Name,
			"similarity", match.Similarity, "threshold", match.Threshold)
		stageDMS := int(time.Since(stageDStart).Milliseconds())
		br.matchEvalMS = &stageDMS
		return storage.SemanticRouteMatch{}, false, br
	}
	stageDMS := int(time.Since(stageDStart).Milliseconds())
	br.matchEvalMS = &stageDMS
	h.log.InfoContext(ctx, "tool route: matched",
		"tenant", tenant.ID, "route", match.Name, "action", match.Action,
		"similarity", match.Similarity, "threshold", match.Threshold)
	return match, true, br
}

// isToolRoutingEnabledForTenant resolves tool-routing effective state for this request tenant.
// Defaults to enabled unless tenant explicitly disables it.
func (h *Handlers) isToolRoutingEnabledForTenant(tenant *config.TenantConfig) bool {
	if tenant == nil {
		return true
	}
	if tenant.ToolRoutingEnabled == nil {
		return true
	}
	return *tenant.ToolRoutingEnabled
}

// computeBudgetPressure returns the fraction of the monthly budget already consumed
// (current_spend / monthly_usd). Returns 0 when no budget is configured or on error (fail-open).
func (h *Handlers) computeBudgetPressure(ctx context.Context, tenant *config.TenantConfig) float64 {
	if tenant == nil || tenant.Budgets.MonthlyUSD <= 0 {
		return 0
	}
	loc := time.UTC
	if tenant.Budgets.Timezone != "" {
		if l, err := time.LoadLocation(tenant.Budgets.Timezone); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)
	spend, err := h.store.GetMonthlySpend(ctx, tenant.ID, monthStart.UTC(), monthEnd.UTC())
	if err != nil {
		h.log.WarnContext(ctx, "cost optimizer: failed to get monthly spend, using 0 pressure", "error", err)
		return 0
	}
	return spend / tenant.Budgets.MonthlyUSD
}

// embeddingModelForModality returns the configured embedding model for the given modality.
// Resolution order for "text":
//  1. tenant.SemanticModalities["text"].EmbeddingModel (per-modality explicit override)
//  2. tenant.Routing.Semantic.EmbeddingModel (explicit per-tenant config, Phase 2 strict)
//
// For non-text modalities, only step 1 applies; returns (nil, nil) if not configured.
// Returns a non-nil error when configuration is present but invalid (missing model or wrong type).
func (h *Handlers) embeddingModelForModality(ctx context.Context, tenant *config.TenantConfig, modality string) (*config.ModelConfig, error) {
	if tenant != nil && tenant.SemanticModalities != nil {
		if mc, ok := tenant.SemanticModalities[modality]; ok && mc.EmbeddingModel != "" {
			gc := h.resolveGlobalConfig(ctx)
			if gc != nil {
				if m := gc.ModelByName(mc.EmbeddingModel); m != nil {
					return m, nil
				}
			}
			// fallback to YAML config
			for i := range h.cfg.Models {
				if h.cfg.Models[i].Name == mc.EmbeddingModel {
					return &h.cfg.Models[i], nil
				}
			}
			// Explicitly configured but not found — hard error (Phase 2).
			return nil, errors.New("invalid embedding model")
		}
	}
	if modality == "text" {
		return h.resolveTextEmbeddingModel(ctx, tenant)
	}
	return nil, nil
}

// resolveTextEmbeddingModel resolves the embedding model for text modality.
// Phase 2 strict mode — no global fallback:
//  1. tenant.Routing.Semantic.EmbeddingModel must be explicitly set
//  2. The configured model must exist in config and have type == "embedding"
//
// Returns (nil, error) on any misconfiguration.
func (h *Handlers) resolveTextEmbeddingModel(ctx context.Context, tenant *config.TenantConfig) (*config.ModelConfig, error) {
	if tenant == nil || tenant.Routing.Semantic.EmbeddingModel == "" {
		return nil, errors.New("embedding_model not configured for tenant")
	}
	name := tenant.Routing.Semantic.EmbeddingModel
	m := h.resolveModelByName(ctx, name)
	if m == nil || m.Type != "embedding" {
		return nil, errors.New("invalid embedding model")
	}
	return m, nil
}

// findBestMultimodalAnchor evaluates text and image modalities independently and returns
// the anchor with the highest similarity score across all modalities.
// Returns (nil, nil) if no anchor meets the threshold (fail-open).
func (h *Handlers) findBestMultimodalAnchor(
	ctx context.Context,
	tenant *config.TenantConfig,
	textParts []string,
	imageURLs []string,
) (*router.SemanticAnchorResult, []float64) {
	type candidate struct {
		result    router.SemanticAnchorResult
		embedding []float64
	}
	var best *candidate

	evalModality := func(modality, input string) {
		embModel, _ := h.embeddingModelForModality(ctx, tenant, modality)
		if embModel == nil {
			return
		}
		var ep providers.EmbeddingProvider
		if embModel.Mock.Enabled {
			ep = providers.NewMockProvider(embModel.Mock, embModel.Name, tenant.ID, embModel.Pricing, nil)
		} else {
			var ok bool
			ep, ok = h.embeddingProviderFor(ctx, embModel.Provider)
			if !ok {
				return
			}
		}
		resp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{Input: []string{input}, Model: embModel.Name})
		if err != nil || len(resp.Data) == 0 {
			return
		}
		emb := resp.Data[0].Embedding
		name, rg, pm, dist, found, err := h.store.GetNearestSemanticAnchor(ctx, tenant.ID, emb, modality)
		if err != nil || !found {
			return
		}
		similarity := 1.0 - dist
		if best == nil || similarity > (1.0-best.result.Distance) {
			best = &candidate{
				result:    router.SemanticAnchorResult{Name: name, RouteGroup: rg, PreferredModels: pm, Distance: dist},
				embedding: emb,
			}
		}
	}

	// Evaluate text modality (concatenate all text parts)
	if len(textParts) > 0 {
		evalModality("text", strings.Join(textParts, " "))
	}
	// Evaluate image modality (use first image URL)
	if len(imageURLs) > 0 {
		evalModality("image", imageURLs[0])
	}

	if best == nil {
		return nil, nil
	}
	return &best.result, best.embedding
}

// makeCacheEmbedFn returns an embedding function for semantic cache operations.
// Resolution order (Phase 2 strict):
//  1. tenant.SemanticCache.EmbeddingModel (explicit cache-level override)
//  2. tenant.Routing.Semantic.EmbeddingModel (tenant-level semantic embedding model)
//
// Returns nil if no embedding model is configured or the provider is unavailable.
// No global fallback — missing config means cache embedding is unavailable for this tenant.
func (h *Handlers) makeCacheEmbedFn(ctx context.Context, tenant *config.TenantConfig, addEmb func(tokens, chars int, model *config.ModelConfig)) func(context.Context, string) ([]float64, error) {
	var embModel *config.ModelConfig
	if name := tenant.SemanticCache.EmbeddingModel; name != "" {
		embModel = h.resolveModelByName(ctx, name)
	} else {
		// Fall through to tenant-level routing.semantic.embedding_model (Phase 2: no global fallback).
		embModel, _ = h.resolveTextEmbeddingModel(ctx, tenant)
	}
	if embModel == nil {
		return nil
	}
	ep, ok := h.embeddingProviderFor(ctx, embModel.Provider)
	if !ok {
		return nil
	}
	mn := embModel.Name
	capturedEmbModel := embModel
	return func(eCtx context.Context, text string) ([]float64, error) {
		resp, err := ep.CreateEmbedding(eCtx, providers.EmbeddingRequest{Input: []string{text}, Model: mn})
		if err != nil {
			return nil, err
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("no embedding data returned")
		}
		addEmb(resp.Usage.PromptTokens, len(text), capturedEmbModel)
		return resp.Data[0].Embedding, nil
	}
}

// classifyRouterError maps router errors to error types for logging
func classifyRouterError(err error) string {
	switch err.(type) {
	case *router.ErrBlockedBySmartStage:
		return "blocked_by_stage"
	case *router.ErrNoCandidatesAfterSmartStages:
		return "no_candidates_after_stages"
	case *router.ErrNoAllowedModels:
		return "no_allowed_models"
	case *router.ErrModelNotAllowed:
		return "model_not_allowed"
	case *router.ErrModelNotInRouteGroup:
		return "model_not_in_group"
	case *router.ErrRouteGroupNotFound:
		return "route_group_not_found"
	case *router.ErrAllCandidatesCircuitBroken:
		return "circuit_breaker"
	case *router.ErrAllAttemptsFailed:
		return "all_attempts_failed"
	default:
		return "routing_error"
	}
}
