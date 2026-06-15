package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/circuitbreaker"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

const maxEmbeddingBatch = 128

// embeddingAPIRequest is the incoming JSON body for POST /v1/embeddings.
// Input accepts string | []string (OpenAI spec).
type embeddingAPIRequest struct {
	Model string          `json:"model"`
	Input json.RawMessage `json:"input"` // string or []string — normalized below
	User  string          `json:"user,omitempty"`
}

// normalizeInput converts a JSON string or []string into a []string.
func normalizeInput(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("input is required")
	}
	// Try []string first.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	// Try single string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []string{s}, nil
	}
	return nil, fmt.Errorf("input must be a string or array of strings")
}

// filterEmbeddingCandidates returns only models with Type == "embedding".
func filterEmbeddingCandidates(candidates []config.ModelConfig) []config.ModelConfig {
	var out []config.ModelConfig
	for _, c := range candidates {
		if c.Type == "embedding" {
			out = append(out, c)
		}
	}
	return out
}

// computeEmbeddingCost calculates cost from prompt tokens only (no completion_tokens for embeddings).
// When the provider returns PromptTokens == 0, falls back to chars/4 as a token estimate.
func computeEmbeddingCost(pricing config.Pricing, usage providers.EmbeddingUsage, inputChars int) float64 {
	tokens := usage.PromptTokens
	if tokens == 0 && inputChars > 0 {
		tokens = inputChars / 4
	}
	return float64(tokens) * pricing.PromptPer1M / 1_000_000
}

// effectiveEmbeddingTokens returns the token count reported by the provider, or chars/4
// when the provider returns 0 (some providers omit token counts in embedding responses).
func effectiveEmbeddingTokens(usage providers.EmbeddingUsage, inputChars int) int {
	if usage.PromptTokens > 0 {
		return usage.PromptTokens
	}
	if inputChars > 0 {
		return inputChars / 4
	}
	return 0
}

// Embeddings handles POST /v1/embeddings.
func (h *Handlers) Embeddings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := gatewayotel.Tracer().Start(ctx, "Embeddings")
	defer span.End()

	requestID := uuid.New()
	requestStart := time.Now()
	var (
		preDecodeMS       *int
		preAuthzMS        *int
		preTenantConfigMS *int
		prePIIMS          *int
		preRateLimitMS    *int
		preModelFilterMS  *int
		preRoutingMS      *int
		cfgModelResolutionMS *int
	)

	tenant := auth.TenantFromContext(ctx)
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication_error")
		return
	}
	span.SetAttributes(gatewayotel.AttrTenant(tenant.ID))

	st := newGatewayPrometheusState(tenant.ID, requestStart)
	st.modelType = "embedding"
	defer st.flush(h, ctx)

	apiKeyID, apiKeyName := auth.APIKeyAttributionFromContext(ctx)
	jwtSub := auth.JWTSubFromContext(ctx)

	// Decode request body.
	decodeStart := time.Now()
	var req embeddingAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		st.setError("", "", "invalid_request_error")
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	inputs, err := normalizeInput(req.Input)
	if err != nil {
		st.setError("", "", "invalid_request_error")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if len(inputs) == 0 {
		st.setError("", "", "invalid_request_error")
		writeError(w, http.StatusBadRequest, "input must not be empty", "invalid_request_error")
		return
	}
	if len(inputs) > maxEmbeddingBatch {
		st.setError("", "", "invalid_request_error")
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("input batch size %d exceeds maximum of %d", len(inputs), maxEmbeddingBatch),
			"invalid_request_error")
		return
	}
	decodeMSVal := int(time.Since(decodeStart).Milliseconds())
	preDecodeMS = &decodeMSVal
	authzMSVal := 0
	preAuthzMS = &authzMSVal

	// Precedence resolution — same as ChatCompletions.
	bodyModel := req.Model

	headerKey := tenant.Selection.HeaderModelKey
	if headerKey == "" {
		headerKey = "X-Model"
	}
	headerModel := r.Header.Get(headerKey)

	routeHeaderKey := tenant.Selection.HeaderRouteKey
	if routeHeaderKey == "" {
		routeHeaderKey = "X-Route-Group"
	}
	routeGroup := r.Header.Get(routeHeaderKey)

	// Use the dynamically resolved global models so that models added via DB
	// are visible here, and use the embedding-mode resolver so that P0 includes
	// type=="embedding" models (which the chat-mode resolver intentionally excludes).
	tenantCfgStart := time.Now()
	globalModels := h.resolveGlobalConfig(ctx).Models
	tenantCfgMSVal := int(time.Since(tenantCfgStart).Milliseconds())
	preTenantConfigMS = &tenantCfgMSVal
	precedenceResolver := router.NewEmbeddingPrecedenceResolver(tenant, globalModels)
	precedenceResult, err := precedenceResolver.Resolve(bodyModel, headerModel, routeGroup)
	if err != nil {
		st.setError("", "", "routing_error")
		writeRouterError(w, err)
		routerPreMSVal := int(time.Since(requestStart).Milliseconds())
		h.logRequestAsync(ctx, storage.RequestLog{
			ID:             uuid.New(),
			RequestID:      requestID.String(),
			Attempt:        0,
			TenantID:       tenant.ID,
			Strategy:       tenant.Routing.Strategy,
			Status:         "error",
			Error:          err.Error(),
			DecisionReason: "precedence_error",
			ErrorType:      "invalid_request",
			RouterPreMS:    &routerPreMSVal,
			PreDecodeMS:    preDecodeMS,
			PreAuthzMS:     preAuthzMS,
			PreTenantConfigMS: preTenantConfigMS,
			APIKeyID:       apiKeyID,
			APIKeyName:     apiKeyName,
			JWTSub:         jwtSub,
		})
		return
	}

	// Filter candidates to embedding-type models only.
	modelFilterStart := time.Now()
	allCandidates := precedenceResult.Candidates
	embeddingCandidates := filterEmbeddingCandidates(allCandidates)
	modelFilterMSVal := int(time.Since(modelFilterStart).Milliseconds())
	preModelFilterMS = &modelFilterMSVal
	if len(embeddingCandidates) == 0 {
		st.setError("", "", "invalid_request_error")
		writeError(w, http.StatusBadRequest,
			"no embedding models available; ensure model type is set to 'embedding' in config",
			"invalid_request_error")
		return
	}

	// Route using embedding candidates.
	routeReq := router.Request{
		TenantID:    tenant.ID,
		Strategy:    tenant.Routing.Strategy,
		Candidates:  embeddingCandidates,
		ForcedModel: precedenceResult.ForcedModel,
		RouteGroup:  routeGroup,
		RouteGroups: tenant.Selection.RouteGroups,
		SmartConfig: &tenant.Routing.Smart,
		Messages:    []string{},
	}

	routingStart := time.Now()
	result, err := h.router.Select(routeReq)
	routingMSVal := int(time.Since(routingStart).Milliseconds())
	preRoutingMS = &routingMSVal
	if err != nil {
		st.setError("", "", "routing_error")
		writeRouterError(w, err)
		routerPreMSVal := int(time.Since(requestStart).Milliseconds())
		h.logRequestAsync(ctx, storage.RequestLog{
			ID:             uuid.New(),
			RequestID:      requestID.String(),
			Attempt:        0,
			TenantID:       tenant.ID,
			Strategy:       tenant.Routing.Strategy,
			Status:         "error",
			Error:          err.Error(),
			DecisionReason: precedenceResult.DecisionReason,
			ErrorType:      classifyRouterError(err),
			RouterPreMS:    &routerPreMSVal,
			PreDecodeMS:    preDecodeMS,
			PreAuthzMS:     preAuthzMS,
			PreTenantConfigMS: preTenantConfigMS,
			PreModelFilterMS:  preModelFilterMS,
			PreRoutingMS:      preRoutingMS,
			APIKeyID:       apiKeyID,
			APIKeyName:     apiKeyName,
			JWTSub:         jwtSub,
		})
		return
	}

	// Budget check (embeddings: reservation is not released on success — cost is negligible).
	if tenant.Budgets.MonthlyUSD > 0 {
		budgetBlocked, _ := h.checkBudget(ctx, span, tenant)
		if budgetBlocked {
			st.setError("", "", "budget_exceeded")
			writeError(w, http.StatusTooManyRequests,
				fmt.Sprintf("monthly budget exceeded for tenant '%s'", tenant.ID),
				"budget_exceeded")
			return
		}
	}
	// Embeddings path currently does not execute PII hooks or in-handler rate limiting.
	piiZero := 0
	prePIIMS = &piiZero
	rateLimitZero := 0
	preRateLimitMS = &rateLimitZero

	// Determine max attempts.
	maxAttempts := len(result.Candidates)
	if tenant.Routing.Fallback.Enabled && tenant.Routing.Fallback.MaxAttempts > 0 {
		if tenant.Routing.Fallback.MaxAttempts < maxAttempts {
			maxAttempts = tenant.Routing.Fallback.MaxAttempts
		}
	} else if !tenant.Routing.Fallback.Enabled {
		maxAttempts = 1
	}

	// Build routing snapshot for logging.
	requestSnapshot := router.DecisionSnapshot{
		Precedence: router.PrecedenceDecision{
			RequestedSource: precedenceResult.RequestedSource,
			RequestedModel:  precedenceResult.RequestedModel,
			RouteGroup:      precedenceResult.RouteGroupUsed,
			PoolSize:        len(result.Candidates),
		},
		Fallback: &router.FallbackDecision{
			Enabled:     tenant.Routing.Fallback.Enabled,
			MaxAttempts: maxAttempts,
		},
		Routing: router.RoutingDecision{
			Strategy: tenant.Routing.Strategy,
		},
		Plan: result.Candidates,
	}
	requestSnapshotJSON, _ := requestSnapshot.ToJSON()

	totalInputChars := 0
	for _, s := range inputs {
		totalInputChars += len(s)
	}

	embReq := providers.EmbeddingRequest{
		Input: inputs,
		User:  req.User,
	}

	var lastErr error
	var attemptCount int
	var lastModelName, lastProvider string

	for attempt := 0; attempt < maxAttempts && attempt < len(result.Candidates); attempt++ {
		attemptCount = attempt + 1
		modelName := result.Candidates[attempt]
		modelResolveStart := time.Now()
		modelCfg := h.resolveModelByName(ctx, modelName)
		modelResolveMSVal := int(time.Since(modelResolveStart).Milliseconds())
		if cfgModelResolutionMS == nil {
			v := modelResolveMSVal
			cfgModelResolutionMS = &v
		} else {
			v := *cfgModelResolutionMS + modelResolveMSVal
			cfgModelResolutionMS = &v
		}
		if preTenantConfigMS != nil {
			v := *preTenantConfigMS + modelResolveMSVal
			preTenantConfigMS = &v
		}
		if modelCfg == nil {
			continue
		}
		if !modelCfg.IsEnabled() {
			writeError(w, http.StatusForbidden, "model is disabled", "model_disabled")
			return
		}
		lastModelName = modelName
		lastProvider = modelCfg.Provider

		embReq.Model = modelName

		// Resolve embedding provider.
		var ep providers.EmbeddingProvider
		var isMock bool

		if modelCfg.Mock.Enabled {
			// Parse X-Debug-Seed for deterministic testing.
			var seed *int64
			if seedStr := r.Header.Get("X-Debug-Seed"); seedStr != "" {
				if s, err2 := strconv.ParseInt(seedStr, 10, 64); err2 == nil {
					seed = &s
				}
			}
			ep = providers.NewMockProvider(modelCfg.Mock, modelName, tenant.ID, modelCfg.Pricing, seed)
			isMock = true
		} else {
			var ok bool
			ep, ok = h.embeddingProviderFor(ctx, modelCfg.Provider)
			if !ok {
				h.log.WarnContext(ctx, "embedding provider not found, skipping",
					"provider", modelCfg.Provider, "model", modelName)
				continue
			}
		}
		preRequestBuildStart := time.Now()

		// Per-attempt timeout.
		attemptCtx := ctx
		var cancel func()
		if tenant.Routing.Fallback.Enabled && tenant.Routing.Fallback.TimeoutMs > 0 {
			attemptCtx, cancel = withTimeoutCtx(ctx, tenant.Routing.Fallback.TimeoutMs)
		}

		// Circuit breaker gate.
		cbAllowed, isProbe, _ := h.breaker.Allow(attemptCtx, modelCfg.Provider)
		if !cbAllowed {
			if cancel != nil {
				cancel()
			}
			h.log.WarnContext(ctx, "circuit breaker open",
				"provider", modelCfg.Provider, "model", modelName)
			continue
		}
		preRequestBuildMSVal := int(time.Since(preRequestBuildStart).Milliseconds())

		attemptStart := time.Now()
		attemptCtx, attemptSpan := gatewayotel.Tracer().Start(attemptCtx, "upstream_embedding_call")
		attemptSpan.SetAttributes(
			gatewayotel.AttrModel(modelName),
			gatewayotel.AttrProvider(modelCfg.Provider),
			gatewayotel.AttrAttempt(attempt+1),
			gatewayotel.AttrTenant(tenant.ID),
		)

		upstreamStart := time.Now()
		routerPreMSVal := int(upstreamStart.Sub(requestStart).Milliseconds())
		resp, err := ep.CreateEmbedding(attemptCtx, embReq)
		upstreamDone := time.Now()
		llmLatencyMSVal := int(upstreamDone.Sub(upstreamStart).Milliseconds())
		latencyMs := float64(time.Since(attemptStart).Milliseconds())

		if cancel != nil {
			cancel()
		}

		if err != nil {
			errType := h.errorClassifier.Classify(err)

			cbOutcome := circuitbreaker.OutcomeSuccess
			if isCBFailure(errType) {
				cbOutcome = circuitbreaker.OutcomeFailure
			}
			_ = h.breaker.Report(ctx, modelCfg.Provider, cbOutcome, isProbe)

			attemptSpan.SetStatus(codes.Error, err.Error())
			attemptSpan.SetAttributes(
				attribute.String("error", err.Error()),
				attribute.String("error_type", string(errType)),
			)
			attemptSpan.End()

			h.logRequestAsync(ctx, storage.RequestLog{
				ID:               uuid.New(),
				RequestID:        requestID.String(),
				Attempt:          attempt + 1,
				TenantID:         tenant.ID,
				Model:            modelName,
				Provider:         modelCfg.Provider,
				Strategy:         tenant.Routing.Strategy,
				Status:           "error",
				LatencyMs:        int(latencyMs),
				Error:            err.Error(),
				FallbackUsed:     attempt > 0,
				DecisionReason:   precedenceResult.DecisionReason,
				ErrorType:        string(errType),
				DecisionSnapshot: requestSnapshotJSON,
				RouterPreMS:      &routerPreMSVal,
				LLMLatencyMS:     &llmLatencyMSVal,
				PreDecodeMS:      preDecodeMS,
				PreAuthzMS:       preAuthzMS,
				PreTenantConfigMS: preTenantConfigMS,
				PrePIIMS:         prePIIMS,
				PreRateLimitMS:   preRateLimitMS,
				PreModelFilterMS: preModelFilterMS,
				PreRoutingMS:     preRoutingMS,
				PreRequestBuildMS: &preRequestBuildMSVal,
				CfgModelResolutionMS: cfgModelResolutionMS,
				APIKeyID:         apiKeyID,
				APIKeyName:       apiKeyName,
				JWTSub:           jwtSub,
			})

			h.router.UpdateModelStats(tenant.ID, modelName, false)

			dateUTC := time.Now().UTC().Truncate(24 * time.Hour)
			h.statsDispatcher.Submit(storage.ModelStatDaily{
				Date:         dateUTC,
				TenantID:     tenant.ID,
				Model:        modelName,
				ErrorCount:   1,
				AvgLatencyMs: latencyMs,
			})

			lastErr = err
			if !h.errorClassifier.IsRetryable(errType) {
				st.setError(modelName, modelCfg.Provider, string(errType))
				writeUpstreamError(w, err)
				return
			}
			continue
		}

		// Success.
		attemptSpan.SetStatus(codes.Ok, "")
		attemptSpan.SetAttributes(gatewayotel.AttrStatus(200))
		attemptSpan.End()

		h.router.RecordLatency(tenant.ID, modelName, latencyMs)
		_ = h.breaker.Report(ctx, modelCfg.Provider, circuitbreaker.OutcomeSuccess, isProbe)

		effectiveTokens := effectiveEmbeddingTokens(resp.Usage, totalInputChars)
		costUSD := computeEmbeddingCost(modelCfg.Pricing, resp.Usage, totalInputChars)
		h.router.UpdateModelStats(tenant.ID, modelName, true)

		dateUTC := time.Now().UTC().Truncate(24 * time.Hour)
		h.statsDispatcher.Submit(storage.ModelStatDaily{
			Date:         dateUTC,
			TenantID:     tenant.ID,
			Model:        modelName,
			SuccessCount: 1,
			AvgLatencyMs: latencyMs,
			TotalCostUSD: costUSD,
		})

		totalLatencyMs := int(time.Since(requestStart).Milliseconds())

		h.saveUsageAsync(ctx, storage.UsageRecord{
			ID:           uuid.New(),
			TenantID:     tenant.ID,
			Model:        modelName,
			Provider:     modelCfg.Provider,
			PromptTokens: effectiveTokens,
			TotalTokens:  effectiveTokens,
			CostUSD:      costUSD,
			RequestID:    requestID.String(),
			APIKeyID:     apiKeyID,
			APIKeyName:   apiKeyName,
			JWTSub:       jwtSub,
		})
		routerPostMSVal := int(time.Since(upstreamDone).Milliseconds())

		h.logRequestAsync(ctx, storage.RequestLog{
			ID:               uuid.New(),
			RequestID:        requestID.String(),
			Attempt:          attempt + 1,
			TenantID:         tenant.ID,
			Model:            modelName,
			Provider:         modelCfg.Provider,
			Strategy:         tenant.Routing.Strategy,
			Status:           "ok",
			LatencyMs:        totalLatencyMs,
			FallbackUsed:     attempt > 0,
			DecisionReason:   precedenceResult.DecisionReason,
			DecisionSnapshot: requestSnapshotJSON,
			RouterPreMS:      &routerPreMSVal,
			LLMLatencyMS:     &llmLatencyMSVal,
			RouterPostMS:     &routerPostMSVal,
			PreDecodeMS:      preDecodeMS,
			PreAuthzMS:       preAuthzMS,
			PreTenantConfigMS: preTenantConfigMS,
			PrePIIMS:         prePIIMS,
			PreRateLimitMS:   preRateLimitMS,
			PreModelFilterMS: preModelFilterMS,
			PreRoutingMS:     preRoutingMS,
			PreRequestBuildMS: &preRequestBuildMSVal,
			CfgModelResolutionMS: cfgModelResolutionMS,
			APIKeyID:         apiKeyID,
			APIKeyName:       apiKeyName,
			JWTSub:           jwtSub,
		})

		if isMock {
			w.Header().Set("X-Mock-Response", "true")
		}
		w.Header().Set("X-Selected-Model", modelName)
		span.SetAttributes(gatewayotel.AttrModel(modelName))
		span.SetStatus(codes.Ok, "")
		st.setOk(modelName, modelCfg.Provider, true, costUSD, resp.Usage.PromptTokens, 0)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// All attempts exhausted.
	span.SetStatus(codes.Error, "all embedding candidates failed")
	h.log.ErrorContext(ctx, "all upstream embedding attempts failed",
		"tenant", tenant.ID,
		"candidates", result.Candidates,
		"max_attempts", maxAttempts,
	)

	allFailedSnapshot := router.DecisionSnapshot{
		Precedence: requestSnapshot.Precedence,
		Fallback: &router.FallbackDecision{
			Enabled:        tenant.Routing.Fallback.Enabled,
			MaxAttempts:    maxAttempts,
			ActualAttempts: attemptCount,
			ErrorTypes:     []string{},
		},
		Routing: router.RoutingDecision{
			Strategy: tenant.Routing.Strategy,
		},
		Plan: result.Candidates,
	}
	snapshotJSON, _ := allFailedSnapshot.ToJSON()

	h.logRequestAsync(ctx, storage.RequestLog{
		ID:               uuid.New(),
		RequestID:        requestID.String(),
		Attempt:          0,
		TenantID:         tenant.ID,
		Strategy:         tenant.Routing.Strategy,
		Status:           "error",
		Error:            "all embedding attempts failed",
		DecisionReason:   precedenceResult.DecisionReason + "|all_failed",
		ErrorType:        "all_attempts_failed",
		DecisionSnapshot: snapshotJSON,
		APIKeyID:         apiKeyID,
		APIKeyName:       apiKeyName,
		JWTSub:           jwtSub,
	})

	errLabel := "all_attempts_failed"
	if lastErr != nil {
		errLabel = string(h.errorClassifier.Classify(lastErr))
	}
	st.setError(lastModelName, lastProvider, errLabel)

	if lastErr != nil {
		writeUpstreamError(w, lastErr)
	} else {
		writeError(w, http.StatusBadGateway, "all upstream embedding providers failed", "upstream_error")
	}
}
