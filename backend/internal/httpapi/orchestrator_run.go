package httpapi

// orchestrator_run.go -- Orchestrator.Run() implementation.
//
// This file MUST NOT read from *http.Request directly (Invariant I-1).
// All request data must come from OrchestratorInput.Req (ParsedRequest).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/circuitbreaker"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/hooks"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/ratelimit"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// orchestratorRun is the package-level implementation of Orchestrator.Run.
// Extracted to a standalone function to allow calling from both the method
// and test helpers without receiver coupling.
func orchestratorRun(ctx context.Context, o *Orchestrator, in OrchestratorInput) OrchestratorOutput {
	h := o.h // delegate all helper methods to Handlers during Phase 3 transition

	ctx, span := gatewayotel.Tracer().Start(ctx, "Orchestrator.Run")
	defer span.End()

	tenant := in.Tenant
	requestID := uuid.New()
	requestStart := time.Now()

	var (
		preDecodeMS                   *int
		preAuthzMS                    *int
		preTenantConfigMS             *int
		prePIIMS                      *int
		preRateLimitMS                *int
		preModelFilterMS              *int
		preRoutingMS                  *int
		preRequestBuildMS             *int
		cfgToolRoutesMS               *int
		cfgDynamicRoutesMS            *int
		cfgBudgetPressureMS           *int
		cfgSemanticMS                 *int
		cfgModelResolutionMS          *int
		toolRoutesEmbeddingModelMS    *int
		toolRoutesEmbeddingGenerateMS *int
		toolRoutesSemanticDBMS        *int
		toolRoutesMatchEvalMS         *int
	)

	span.SetAttributes(gatewayotel.AttrTenant(tenant.ID))

	st := newGatewayPrometheusState(tenant.ID, requestStart)
	defer st.flush(h, ctx)

	// ML pass-through: if body was pre-read as raw bytes with X-Model-Type=ml,
	// bypass the LLM pipeline entirely.
	if in.Req.RawBody != nil && in.Req.Headers.Get("X-Model-Type") == "ml" {
		st.skip = true
		// The ML handler uses in.W and in.R directly (raw body already extracted).
		h.handleMLRequest(in.W, in.R, in.Req.RawBody)
		return OrchestratorOutput{}
	}

	req := ChatCompletionRequest{
		Messages:    in.Req.Messages,
		MaxTokens:   in.Req.MaxTokens,
		Temperature: in.Req.Temperature,
		TopP:        in.Req.TopP,
		Stream:      in.Req.Stream,
		Tools:       in.Req.Tools,
		ToolChoice:  in.Req.ToolChoice,
		Metadata:    in.Req.Metadata,
	}

	if len(req.Messages) == 0 {
		st.setError("", "", "invalid_request_error")
		writeError(in.W, 400, "messages is required and must not be empty", "invalid_request_error")
		return OrchestratorOutput{Err: errors.New("messages is required and must not be empty")}
	}

	metadataJSON, err := validateMetadata(req.Metadata)
	if err != nil {
		st.setError("", "", "invalid_request_error")
		writeError(in.W, 400, err.Error(), "invalid_request_error")
		return OrchestratorOutput{Err: err}
	}
	decodeMSVal := 0 // decoding happened upstream in parseChatCompletionsRequest
	preDecodeMS = &decodeMSVal

	gc := h.resolveGlobalConfig(ctx)

	bodyModel := in.Req.BodyModel

	// SPEC_151: header overrides are opt-in per tenant.
	// Read routing headers from Req.Headers (NOT from in.R.Header -- Invariant I-1).
	var headerModel string
	if tenant.Selection.AllowModelOverride {
		headerKey := tenant.Selection.HeaderModelKey
		if headerKey == "" {
			headerKey = "X-Model"
		}
		headerModel = in.Req.Headers.Get(headerKey)
	}

	var routeGroup string
	if tenant.Selection.AllowRouteGroupOverride {
		routeHeaderKey := tenant.Selection.HeaderRouteKey
		if routeHeaderKey == "" {
			routeHeaderKey = "X-Route-Group"
		}
		routeGroup = in.Req.Headers.Get(routeHeaderKey)
	}

	// Virtual model alias resolution.
	var (
		virtualAliasName   string
		virtualAliasActive bool
		virtualExposeAlias bool
	)
	if bodyModel != "" {
		if aliasCfg, ok := tenant.Selection.VirtualModels[bodyModel]; ok && aliasCfg.Enabled {
			isRealModel := false
			for _, m := range gc.Models {
				if m.Name == bodyModel {
					isRealModel = true
					break
				}
			}
			if !isRealModel {
				virtualAliasName = bodyModel
				virtualAliasActive = true
				virtualExposeAlias = aliasCfg.ExposeAliasInResponse
				bodyModel = ""
				if routeGroup == "" && aliasCfg.RouteGroup != "" {
					routeGroup = aliasCfg.RouteGroup
				}
			}
		}
	}

	// Traffic split (canary routing).
	var (
		trafficSplitApplied    bool
		trafficSplitKey        string
		trafficSplitCandidates []config.TrafficSplitEntry
	)
	if len(tenant.TrafficSplit) > 0 {
		splitKey := bodyModel
		if virtualAliasActive {
			splitKey = virtualAliasName
		}
		if splitKey != "" {
			if entries := tenant.TrafficSplit[splitKey]; len(entries) > 0 {
				if selected, ok := weightedSelectModel(entries); ok {
					bodyModel = selected
					virtualAliasActive = false
					trafficSplitApplied = true
					trafficSplitKey = splitKey
					trafficSplitCandidates = entries
					h.log.InfoContext(ctx, "traffic split applied",
						"key", splitKey, "selected", selected, "tenant", tenant.ID)
				} else {
					h.log.WarnContext(ctx, "traffic split: invalid config, routing normally",
						"key", splitKey, "tenant", tenant.ID)
				}
			}
		}
	}

	// Precedence resolution.
	modelFilterStart := time.Now()
	precedenceResolver := router.NewPrecedenceResolver(tenant, gc.Models)
	precedenceResult, err := precedenceResolver.Resolve(bodyModel, headerModel, routeGroup)
	if err != nil {
		st.setError("", "", "routing_error")
		writeRouterError(in.W, err)
		h.logRoutingError(ctx, tenant, requestID, requestStart,
			router.PrecedenceDecision{},
			nil,
			err,
		)
		return OrchestratorOutput{Err: err}
	}
	modelFilterMSVal := int(time.Since(modelFilterStart).Milliseconds())
	preModelFilterMS = &modelFilterMSVal

	candidates := precedenceResult.Candidates
	forcedModel := precedenceResult.ForcedModel
	decisionReason := precedenceResult.DecisionReason

	// Debug: fail models — admin-only to prevent routing manipulation by tenants.
	failModels := make(map[string]bool)
	if failModel := in.Req.Headers.Get("X-Debug-Fail-Model"); failModel != "" && auth.AuthTypeFromContext(ctx) == "admin_token" {
		failModels[failModel] = true
	}

	messages := in.Req.MessageTexts
	requestImageURLs := in.Req.MessageImageURLs

	// Embedding accumulator (replaces addEmb closure -- Invariant I-6).
	sharedRequestID := requestID.String()
	embAcc := &embeddingAccumulator{}
	defer func() {
		embAcc.flush(ctx, h.logInternalEmbeddingAsync, sharedRequestID, tenant)
	}()

	// Tool routing stage.
	tenantCfgAccumMS := 0
	toolRoutingEnabled := h.isToolRoutingEnabledForTenant(tenant)
	toolRouteStart := time.Now()
	var toolRouteMatch storage.SemanticRouteMatch
	toolRouteFound := false
	toolRouteBr := toolRouteBreakdown{}
	if toolRoutingEnabled {
		toolRouteMatch, toolRouteFound, toolRouteBr = h.checkToolRoutes(ctx, messages, tenant, embAcc.Add)
	} else {
		zero := 0
		toolRouteBr.embeddingModelMS = &zero
		toolRouteBr.embeddingGenerateMS = &zero
		toolRouteBr.semanticDBMS = &zero
		toolRouteBr.matchEvalMS = &zero
	}
	toolRoutesEmbeddingModelMS = toolRouteBr.embeddingModelMS
	toolRoutesEmbeddingGenerateMS = toolRouteBr.embeddingGenerateMS
	toolRoutesSemanticDBMS = toolRouteBr.semanticDBMS
	toolRoutesMatchEvalMS = toolRouteBr.matchEvalMS
	if toolRouteFound {
		st.setOk("tool-route", "internal", false, 0, 0, 0)
		span.AddEvent("tool_route_match", trace.WithAttributes(
			attribute.String("route.name", toolRouteMatch.Name),
			attribute.String("route.action", toolRouteMatch.Action),
			attribute.Float64("route.similarity", toolRouteMatch.Similarity),
		))
		in.W.Header().Set("X-Tool-Route", toolRouteMatch.Name)
		in.W.Header().Set("X-Tool-Action", toolRouteMatch.Action)
		in.W.Header().Set("X-Tool-Route-Similarity", fmt.Sprintf("%.4f", toolRouteMatch.Similarity))
		writeJSON(in.W, 200, ChatCompletionResponse{
			ID:      "chatcmpl-tool-" + uuid.New().String()[:8],
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "tool-route",
			Choices: []ChatChoice{{
				Index:        0,
				Message:      newTextMessage("assistant", fmt.Sprintf(`{"tool":%q,"route":%q,"similarity":%.4f}`, toolRouteMatch.Action, toolRouteMatch.Name, toolRouteMatch.Similarity)),
				FinishReason: "stop",
			}},
		})
		return OrchestratorOutput{}
	}
	toolRouteMSVal := int(time.Since(toolRouteStart).Milliseconds())
	cfgToolRoutesMS = &toolRouteMSVal
	tenantCfgAccumMS += toolRouteMSVal

	// Dynamic route check.
	dynRouteStart := time.Now()
	if toolRoutingEnabled && toolRouteFound {
		st.setOk("dynamic-route", "internal", false, 0, 0, 0)
		in.W.Header().Set("X-Dynamic-Route", toolRouteMatch.Name)
		in.W.Header().Set("X-Dynamic-Action", toolRouteMatch.Action)
		in.W.Header().Set("X-Dynamic-Route-Similarity", fmt.Sprintf("%.4f", toolRouteMatch.Similarity))
		writeJSON(in.W, 200, ChatCompletionResponse{
			ID:      "chatcmpl-route-" + uuid.New().String()[:8],
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "dynamic-route",
			Choices: []ChatChoice{{
				Index:        0,
				Message:      newTextMessage("assistant", fmt.Sprintf(`{"route":%q,"action":%q,"similarity":%.4f}`, toolRouteMatch.Name, toolRouteMatch.Action, toolRouteMatch.Similarity)),
				FinishReason: "stop",
			}},
		})
		return OrchestratorOutput{}
	}
	dynRouteMSVal := int(time.Since(dynRouteStart).Milliseconds())
	if !toolRoutingEnabled {
		dynRouteMSVal = 0
	}
	cfgDynamicRoutesMS = &dynRouteMSVal
	tenantCfgAccumMS += dynRouteMSVal

	// Budget pressure for cost-aware routing.
	budgetPressureStart := time.Now()
	budgetPressure := h.computeBudgetPressure(ctx, tenant)
	budgetPressureMSVal := int(time.Since(budgetPressureStart).Milliseconds())
	cfgBudgetPressureMS = &budgetPressureMSVal
	tenantCfgAccumMS += budgetPressureMSVal

	routeReq := router.Request{
		TenantID:                 tenant.ID,
		Strategy:                 tenant.Routing.Strategy,
		Candidates:               candidates,
		ForcedModel:              forcedModel,
		RouteGroup:               routeGroup,
		DefaultRouteGroup:        tenant.Routing.RouteGroup,
		FailModels:               failModels,
		RouteGroups:              tenant.Selection.RouteGroups,
		SmartConfig:              &tenant.Routing.Smart,
		Messages:                 messages,
		SemanticThresholdDefault: tenant.Routing.Semantic.ThresholdDefault,
		BudgetPressure:           budgetPressure,
		MaxTokens:                req.MaxTokens,
	}

	// Wire semantic routing deps.
	semanticDepsStart := time.Now()
	if hasSemanticRule(&tenant.Routing.Smart) {
		routeReq.Ctx = ctx
		h.log.InfoContext(ctx, "semantic routing: rule detected, wiring deps",
			"tenant", tenant.ID,
			"routing_strategy", tenant.Routing.Strategy)

		hasImages := len(requestImageURLs) > 0

		if !hasImages {
			routeReq.SemanticLookup = func(lCtx context.Context, tID string, emb []float64) (router.SemanticAnchorResult, bool, error) {
				name, rg, pm, dist, found, err := h.store.GetNearestSemanticAnchor(lCtx, tID, emb, "text")
				if err != nil || !found {
					return router.SemanticAnchorResult{}, found, err
				}
				return router.SemanticAnchorResult{Name: name, RouteGroup: rg, PreferredModels: pm, Distance: dist}, true, nil
			}
			embModel, embErr := h.embeddingModelForModality(ctx, tenant, "text")
			if embModel == nil {
				h.log.WarnContext(ctx, "semantic routing: no embedding model configured, semantic stages will be skipped",
					"tenant", tenant.ID,
					"routing_strategy", tenant.Routing.Strategy,
					"semantic_embedding_model", tenant.Routing.Semantic.EmbeddingModel,
					"error", embErr)
			} else if ep, ok := in.EmbeddingProviderFor(ctx, embModel.Provider); ok {
				mn := embModel.Name
				capturedEmbModel := embModel
				routeReq.EmbeddingFunc = func(eCtx context.Context, text string) ([]float64, error) {
					resp, err := ep.CreateEmbedding(eCtx, providers.EmbeddingRequest{
						Input: []string{text},
						Model: mn,
					})
					if err != nil {
						return nil, err
					}
					if len(resp.Data) == 0 {
						return nil, fmt.Errorf("no embedding data returned")
					}
					embAcc.Add(resp.Usage.PromptTokens, len(text), capturedEmbModel)
					return resp.Data[0].Embedding, nil
				}
				h.log.InfoContext(ctx, "semantic routing: embedding deps wired",
					"tenant", tenant.ID,
					"routing_strategy", tenant.Routing.Strategy,
					"embedding_model", mn,
					"provider", embModel.Provider)
			} else {
				h.log.WarnContext(ctx, "semantic routing: embedding provider unavailable, semantic stages will be skipped",
					"tenant", tenant.ID,
					"routing_strategy", tenant.Routing.Strategy,
					"embedding_model", embModel.Name,
					"provider", embModel.Provider)
			}
		} else {
			bestAnchor, bestEmb := h.findBestMultimodalAnchor(ctx, tenant, messages, requestImageURLs)
			if bestAnchor != nil {
				captured := *bestAnchor
				capturedEmb := bestEmb
				routeReq.EmbeddingFunc = func(_ context.Context, _ string) ([]float64, error) {
					return capturedEmb, nil
				}
				routeReq.SemanticLookup = func(_ context.Context, _ string, _ []float64) (router.SemanticAnchorResult, bool, error) {
					return captured, true, nil
				}
			}
		}
	}
	semanticMSVal := int(time.Since(semanticDepsStart).Milliseconds())
	cfgSemanticMS = &semanticMSVal
	tenantCfgAccumMS += semanticMSVal

	routingStart := time.Now()
	routingCtx, routingSpan := gatewayotel.Tracer().Start(ctx, "router.select")
	routingSpan.SetAttributes(
		gatewayotel.AttrTenant(tenant.ID),
		attribute.String("routing.strategy", tenant.Routing.Strategy),
		attribute.Int("routing.candidates", len(candidates)),
	)
	result, err := h.router.Select(routeReq)
	routingMSVal := int(time.Since(routingStart).Milliseconds())
	preRoutingMS = &routingMSVal
	if err != nil {
		routingSpan.RecordError(err)
		routingSpan.SetStatus(codes.Error, err.Error())
		routingSpan.End()
		_ = routingCtx
		st.setError("", "", "routing_error")
		writeRouterError(in.W, err)
		h.logRoutingError(ctx, tenant, requestID, requestStart,
			router.PrecedenceDecision{
				RequestedSource: precedenceResult.RequestedSource,
				RequestedModel:  precedenceResult.RequestedModel,
				RouteGroup:      precedenceResult.RouteGroupUsed,
				PoolSize:        len(precedenceResult.Candidates),
			},
			result.SmartResult,
			err,
		)
		return OrchestratorOutput{Err: err}
	}

	// Build SmartDecision for decision_snapshot.
	var smartDecision *router.SmartDecision
	if result.SmartResult != nil {
		sd := &router.SmartDecision{
			StagesEvaluated:      result.SmartResult.StagesEvaluated,
			PreferredModels:      result.SmartResult.PreferredModels,
			BannedModels:         result.SmartResult.BannedModels,
			Blocked:              result.SmartResult.Blocked,
			BlockReason:          result.SmartResult.BlockReason,
			PromptLength:         result.SmartResult.PromptLength,
			SemanticAnchor:       result.SmartResult.SemanticAnchor,
			SemanticSimilarity:   result.SmartResult.SemanticSimilarity,
			SemanticDistance:     result.SmartResult.SemanticDistance,
			AnchorRouteGroup:     result.SmartResult.AnchorRouteGroup,
			EstimatedCostsUSD:    result.SmartResult.EstimatedCostsUSD,
			BudgetPressure:       result.SmartResult.BudgetPressure,
			CostOptimizerApplied: result.SmartResult.CostOptimizerApplied,
			EffectiveWeights:     result.SmartResult.EffectiveWeights,
			EffectiveCostWeight:  result.SmartResult.EffectiveCostWeight,
			RankingDetails:       result.SmartResult.RankingDetails,
		}
		if len(result.SmartResult.RulesMatched) > 0 {
			sd.SmartEvaluation = &router.SmartEvaluationSnapshot{
				StagesEvaluated: result.SmartResult.StagesEvaluated,
				RulesMatched:    result.SmartResult.RulesMatched,
			}
			for _, rm := range result.SmartResult.RulesMatched {
				h.log.InfoContext(ctx, "SMART_RULE_MATCH",
					"tenant", tenant.ID,
					"stage", rm.Stage,
					"condition", rm.Condition,
					"value", rm.Value,
					"action", rm.Action,
					"reason", rm.Reason,
				)
			}
		}
		smartDecision = sd

		routingSpan.SetAttributes(
			attribute.Bool("routing.smart.blocked", result.SmartResult.Blocked),
			attribute.String("routing.smart.block_reason", result.SmartResult.BlockReason),
			attribute.String("routing.smart.anchor", result.SmartResult.SemanticAnchor),
			attribute.Float64("routing.smart.semantic_similarity", result.SmartResult.SemanticSimilarity),
			attribute.Float64("routing.smart.budget_pressure", result.SmartResult.BudgetPressure),
			attribute.Bool("routing.smart.cost_optimizer_applied", result.SmartResult.CostOptimizerApplied),
		)
		for _, rm := range result.SmartResult.RulesMatched {
			actionJSON, _ := json.Marshal(rm.Action)
			routingSpan.AddEvent("smart_rule_match", trace.WithAttributes(
				attribute.String("stage", rm.Stage),
				attribute.String("condition", rm.Condition),
				attribute.String("action", string(actionJSON)),
				attribute.String("reason", rm.Reason),
			))
		}
	}
	routingSpan.SetAttributes(
		gatewayotel.AttrModel(result.Selected),
		attribute.String("routing.selected_model", result.Selected),
		attribute.Int("routing.latency_ms", routingMSVal),
	)
	routingSpan.SetStatus(codes.Ok, "")
	routingSpan.End()
	_ = routingCtx

	// Debug: simulated latency for EWMA tracking — admin-only to prevent cross-tenant metric poisoning.
	if latStr := in.Req.Headers.Get("X-Debug-Latency-Ms"); latStr != "" && auth.AuthTypeFromContext(ctx) == "admin_token" {
		if latMs, err := strconv.ParseFloat(latStr, 64); err == nil {
			h.router.RecordLatency(tenant.ID, result.Selected, latMs)
		}
	}

	// Build provider request.
	provReq := providers.ChatRequest{
		Model:           "",
		Messages:        toProviderMessages(req.Messages),
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          false,
		Tools:           req.Tools,
		ToolChoice:      req.ToolChoice,
		UseResponsesAPI: in.Req.APIStyle == APIStyleOpenAIResponses,
	}

	// PreRequest hooks.
	var piiRequestDecision *string
	var piiResponseDecision *string
	var piiSnapshot *router.PIISnapshot
	piiStageMSVal := 0

	tenantHooks := h.hooks.ForTenant(tenant)
	for _, hook := range tenantHooks {
		hookStageStart := time.Now()
		hookResult, hookErr := hook.PreRequest(ctx, tenant, provReq)
		hookName := hook.Name()

		if hookName == "external_pii" {
			var decision string
			switch hookResult.Decision {
			case hooks.Allow:
				decision = "allow"
			case hooks.Block:
				decision = "reject"
			case hooks.Redact:
				decision = "modify"
			case hooks.AllowPII:
				decision = "allow_pii"
			default:
				decision = hookResult.Decision.String()
			}
			piiRequestDecision = &decision
			piiStageMSVal += int(time.Since(hookStageStart).Milliseconds())
		}

		span.AddEvent("hook.pre_request", trace.WithAttributes(
			attribute.String("hook.name", hookName),
			attribute.String("hook.decision", hookResult.Decision.String()),
			attribute.String("hook.reason", hookResult.Reason),
		))

		if hookErr != nil {
			h.log.ErrorContext(ctx, "hook error",
				"hook", hookName, "tenant", tenant.ID, "error", hookErr)
			st.setError("", "", "internal_error")
			writeError(in.W, 500, "internal hook error", "internal_error")
			return OrchestratorOutput{Err: hookErr}
		}

		logWithMode(ctx, h.log, LogMode(h.cfg.Server.LogMode), slog.LevelInfo, "hook executed",
			slog.String("hook", hookName),
			slog.String("tenant", tenant.ID),
			slog.String("decision", hookResult.Decision.String()),
			slog.String("reason", hookResult.Reason),
		)

		switch hookResult.Decision {
		case hooks.Block:
			statusCode := 400
			if hookResult.StatusCode != 0 {
				statusCode = hookResult.StatusCode
			}
			blockMsg := "request blocked by hook '" + hookName + "': " + hookResult.Reason
			st.setError("", "", "content_policy_violation")
			writeError(in.W, statusCode, blockMsg, "content_policy_violation")
			{
				hookPIISnap := &router.PIISnapshot{Decision: "reject"}
				hookSnap := router.DecisionSnapshot{
					Routing: router.RoutingDecision{Strategy: tenant.Routing.Strategy},
					PII:     hookPIISnap,
				}
				hookSnapJSON, _ := hookSnap.ToJSON()
				hookAPIKeyID, hookAPIKeyName := auth.APIKeyAttributionFromContext(ctx)
				hookJWTSub := auth.JWTSubFromContext(ctx)
				rejectDecision := "reject"
				_ = h.store.LogRequest(ctx, storage.RequestLog{
					RequestID:                 requestID.String(),
					Attempt:                   1,
					Timestamp:                 requestStart.UTC(),
					TenantID:                  tenant.ID,
					Strategy:                  tenant.Routing.Strategy,
					Status:                    "error",
					Error:                     blockMsg,
					ErrorType:                 "content_policy_violation",
					DecisionSnapshot:          hookSnapJSON,
					PIIWebhookRequestDecision: &rejectDecision,
					APIKeyID:                  hookAPIKeyID,
					APIKeyName:                hookAPIKeyName,
					JWTSub:                    hookJWTSub,
				})
			}
			return OrchestratorOutput{Err: errors.New(blockMsg)}
		case hooks.Redact:
			provReq = hookResult.Request
		case hooks.Warn:
			span.SetAttributes(attribute.String("hook.warning."+hookName, hookResult.Reason))
			logWithMode(ctx, h.log, LogMode(h.cfg.Server.LogMode), slog.LevelWarn, "hook warning",
				slog.String("hook", hookName),
				slog.String("tenant", tenant.ID),
				slog.String("reason", hookResult.Reason))
		case hooks.Allow:
			// no-op
		case hooks.AllowPII:
			if tenant.Hooks.PII == nil || tenant.Hooks.PII.AllowPII == nil || !tenant.Hooks.PII.AllowPII.Enabled {
				st.setError("", "", "content_policy_violation")
				writeError(in.W, 400,
					"request blocked by hook '"+hookName+"': allow_pii triggered but not configured",
					"content_policy_violation")
				return OrchestratorOutput{Err: errors.New("allow_pii not configured")}
			}
			targetModel := tenant.Hooks.PII.AllowPII.Model
			if h.resolveModelByName(ctx, targetModel) == nil {
				st.setError("", "", "content_policy_violation")
				writeError(in.W, 400,
					"request blocked by hook '"+hookName+"': allow_pii model not found: "+targetModel,
					"content_policy_violation")
				return OrchestratorOutput{Err: errors.New("allow_pii model not found")}
			}
			result.Candidates = []string{targetModel}
			piiSnapshot = &router.PIISnapshot{Decision: "allow_pii", TargetModel: targetModel}
		}
	}
	prePIIMS = &piiStageMSVal
	rateLimitZero := 0
	preRateLimitMS = &rateLimitZero

	// Budget enforcement.
	authzStart := time.Now()
	var budgetReservationID uuid.UUID
	if tenant.BudgetEnforcement.Enabled {
		degradeGroup, blocked, httpStatus, msg, resID := h.applyBudgetEnforcement(ctx, span, tenant, req.Metadata)
		budgetReservationID = resID
		if blocked {
			st.setError("", "", "budget_exceeded")
			writeError(in.W, httpStatus, msg, "budget_exceeded")
			return OrchestratorOutput{Err: errors.New(msg)}
		}
		if degradeGroup != "" {
			if groupModels, ok := tenant.Selection.RouteGroups[degradeGroup]; ok && len(groupModels) > 0 {
				allowed := make(map[string]bool, len(groupModels))
				for _, m := range groupModels {
					allowed[m] = true
				}
				var degraded []string
				for _, c := range result.Candidates {
					if allowed[c] {
						degraded = append(degraded, c)
					}
				}
				if len(degraded) > 0 {
					result.Selected = degraded[0]
					result.Candidates = degraded
				}
			}
		}
	} else if tenant.Budgets.MonthlyUSD > 0 {
		budgetBlocked, resID := h.checkBudget(ctx, span, tenant)
		budgetReservationID = resID
		if budgetBlocked {
			st.setError("", "", "budget_exceeded")
			writeError(in.W, 429,
				fmt.Sprintf("monthly budget exceeded for tenant '%s'", tenant.ID),
				"budget_exceeded")
			return OrchestratorOutput{Err: errors.New("budget exceeded")}
		}
	}
	authzMSVal := int(time.Since(authzStart).Milliseconds())
	preAuthzMS = &authzMSVal

	// Fallback loop limits.
	maxAttempts := len(result.Candidates)
	if tenant.Routing.Fallback.Enabled && tenant.Routing.Fallback.MaxAttempts > 0 {
		if tenant.Routing.Fallback.MaxAttempts < maxAttempts {
			maxAttempts = tenant.Routing.Fallback.MaxAttempts
		}
	} else if !tenant.Routing.Fallback.Enabled {
		maxAttempts = 1
	}

	apiKeyID, apiKeyName := auth.APIKeyAttributionFromContext(ctx)
	jwtSub := auth.JWTSubFromContext(ctx)
	if jwtSub != nil && *jwtSub != "" {
		hashed := ratelimit.HashSub(*jwtSub)
		jwtSub = &hashed
	}

	// Extract optional business-context headers (all nullable).
	ctxHeaders := extractAllContextHeaders(in.Req.Headers)

	// Semantic cache lookup.
	var requestEmbedding []float64
	semCacheEnabled := tenant.SemanticCache.Enabled
	if semCacheEnabled {
		semCacheStart := time.Now()
		gatewayotel.SemanticCacheLookupCounter.WithLabelValues(tenant.ID).Inc()
		if embFn := h.makeCacheEmbedFn(ctx, tenant, embAcc.Add); embFn != nil {
			cacheText := strings.Join(messages, " ")
			if cacheEmb, embErr := embFn(ctx, cacheText); embErr == nil {
				requestEmbedding = cacheEmb
				scope := storage.SemanticCacheScopeModel
				if tenant.SemanticCache.Scope == string(storage.SemanticCacheScopeRouteGroup) {
					scope = storage.SemanticCacheScopeRouteGroup
				}
				firstModel := ""
				if len(result.Candidates) > 0 {
					firstModel = result.Candidates[0]
				}
				h.log.DebugContext(ctx, "semantic cache lookup",
					"tenant", tenant.ID, "threshold", tenant.SemanticCache.Threshold,
					"scope", scope, "model", firstModel)
				entry, found, lookupErr := h.store.FindNearestSemanticCache(
					ctx, tenant.ID, cacheEmb, scope, firstModel,
					precedenceResult.RouteGroupUsed, tenant.SemanticCache.Threshold,
				)
				if lookupErr == nil && found {
					h.log.InfoContext(ctx, "semantic cache hit",
						"tenant", tenant.ID, "model", entry.Model,
						"route_group", entry.RouteGroup, "similarity", entry.Similarity)
					gatewayotel.SemanticCacheHitCounter.WithLabelValues(tenant.ID).Inc()
					gatewayotel.SemanticCacheSimilarityHist.WithLabelValues(tenant.ID).Observe(entry.Similarity)
					if h.semCacheAsync.TryAcquire(1) {
						go func() {
							defer h.semCacheAsync.Release(1)
							h.store.TouchSemanticCacheHit(context.Background(), entry.ID, time.Now())
						}()
					} else {
						gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_touch").Inc()
					}
					in.W.Header().Set("X-Semantic-Cache", "HIT")
					in.W.Header().Set("X-Semantic-Cache-Similarity", fmt.Sprintf("%.4f", entry.Similarity))
					in.W.Header().Set("X-Selected-Model", entry.Model)
					var cachedResp ChatCompletionResponse
					if jsonErr := json.Unmarshal(entry.ResponseJSON, &cachedResp); jsonErr == nil {
						cacheProv := "unknown"
						if mcfg := h.resolveModelByName(ctx, entry.Model); mcfg != nil {
							cacheProv = mcfg.Provider
						}
						cacheHitLatencyMs := int(time.Since(requestStart).Milliseconds())
						cacheHitMeta, _ := json.Marshal(map[string]any{
							"semantic_cache": "hit",
							"similarity":     entry.Similarity,
							"route_group":    entry.RouteGroup,
						})
						h.logRequestAsync(ctx, storage.RequestLog{
							ID:              uuid.New(),
							RequestID:       requestID.String(),
							Attempt:         1,
							TenantID:        tenant.ID,
							Model:           entry.Model,
							Provider:        cacheProv,
							Strategy:        "semantic_cache",
							Status:          "ok",
							LatencyMs:       cacheHitLatencyMs,
							Metadata:        cacheHitMeta,
							APIKeyID:        apiKeyID,
							APIKeyName:      apiKeyName,
							JWTSub:          jwtSub,
							CustomerID:      ctxHeaders.CustomerID,
							Channel:         ctxHeaders.Channel,
							InteractionType: ctxHeaders.InteractionType,
							AgentID:         ctxHeaders.AgentID,
							Department:      ctxHeaders.Department,
							TicketID:        ctxHeaders.TicketID,
							CustomerSegment: ctxHeaders.CustomerSegment,
							Language:        ctxHeaders.Language,
							Intent:          ctxHeaders.Intent,
							ExperimentID:    ctxHeaders.ExperimentID,
							AutonomyLevel:   ctxHeaders.AutonomyLevel,
							PolicyID:        ctxHeaders.PolicyID,
							RiskLevel:       ctxHeaders.RiskLevel,
							RevenueImpact:   ctxHeaders.RevenueImpact,
							Currency:        ctxHeaders.Currency,
						})
						st.setOk(entry.Model, cacheProv, false, 0, 0, 0)
						writeJSON(in.W, 200, cachedResp)
					} else {
						h.log.ErrorContext(ctx, "failed to unmarshal cached response", "error", jsonErr)
						st.setError("", "", "internal_error")
						writeError(in.W, 500, "cache error", "internal_error")
					}
					return OrchestratorOutput{}
				}
				if lookupErr != nil {
					h.log.WarnContext(ctx, "semantic cache lookup error", "tenant", tenant.ID, "error", lookupErr)
				}
			}
			gatewayotel.SemanticCacheMissCounter.WithLabelValues(tenant.ID).Inc()
		}
		semCacheMS := int(time.Since(semCacheStart).Milliseconds())
		tenantCfgAccumMS += semCacheMS
		v := *cfgSemanticMS + semCacheMS
		cfgSemanticMS = &v
	}
	preTenantConfigMS = &tenantCfgAccumMS

	// Build routing decision snapshot.
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
		Plan:  result.Candidates,
		PII:   piiSnapshot,
		Smart: smartDecision,
	}
	requestSnapshotJSON, _ := requestSnapshot.ToJSON()

	// Fallback loop.
	var lastErr error
	var lastModelName, lastProvider string
	for attempt := 0; attempt < maxAttempts && attempt < len(result.Candidates); attempt++ {
		preRequestBuildStart := time.Now()
		modelName := result.Candidates[attempt]
		modelResolveStart := time.Now()
		modelCfg := h.resolveModelByName(ctx, modelName)
		modelResolveMS := int(time.Since(modelResolveStart).Milliseconds())
		tenantCfgAccumMS += modelResolveMS
		if cfgModelResolutionMS == nil {
			v := modelResolveMS
			cfgModelResolutionMS = &v
		} else {
			v := *cfgModelResolutionMS + modelResolveMS
			cfgModelResolutionMS = &v
		}
		preTenantConfigMS = &tenantCfgAccumMS
		if modelCfg == nil {
			continue
		}
		if !modelCfg.IsEnabled() {
			writeError(in.W, 403, "model is disabled", "model_disabled")
			return OrchestratorOutput{Err: errors.New("model is disabled")}
		}
		lastModelName = modelName
		lastProvider = modelCfg.Provider

		var prov providers.Provider
		var ok bool
		isMock := false

		if modelCfg.Mock.Enabled {
			// Read X-Debug-Seed from Req.Headers (Invariant I-1).
			var seed *int64
			if seedStr := in.Req.Headers.Get("X-Debug-Seed"); seedStr != "" {
				if s, err := strconv.ParseInt(seedStr, 10, 64); err == nil {
					seed = &s
				}
			}
			prov = providers.NewMockProvider(modelCfg.Mock, modelName, tenant.ID, modelCfg.Pricing, seed)
			ok = true
			isMock = true
			h.log.InfoContext(ctx, "using mock provider", "model", modelName, "tenant", tenant.ID)
		} else {
			if h.resolveProviderConfig(ctx, modelCfg.Provider) == nil {
				h.log.WarnContext(ctx, "provider not found, skipping",
					"provider", modelCfg.Provider, "model", modelName)
				continue
			}
			prov, ok = in.ChatProviderFor(ctx, modelCfg)
			if !ok {
				h.log.WarnContext(ctx, "provider not found, skipping",
					"provider", modelCfg.Provider, "model", modelName)
				continue
			}
		}

		provReq.Model = modelName
		if strings.TrimSpace(modelCfg.ProviderModelID) != "" {
			provReq.ProviderModelID = strings.TrimSpace(modelCfg.ProviderModelID)
		}

		if cap := tenant.EffectiveMaxOutputTokens(); cap > 0 {
			t := modelCfg.Type
			if t == "" || t == "llm" {
				capCopy := cap
				provReq.MaxTokens = &capCopy
			}
		}
		if pc := h.resolveProviderConfig(ctx, modelCfg.Provider); pc != nil && pc.Type == "aws_bedrock" && strings.TrimSpace(modelCfg.ProviderModelID) == "" {
			st.setError(modelName, modelCfg.Provider, "invalid_request_error")
			writeError(in.W, 400, "provider_model_id is required for models using the aws_bedrock provider", "invalid_request_error")
			return OrchestratorOutput{Err: errors.New("provider_model_id required for bedrock")}
		}

		attemptCtx := ctx
		var cancel func()
		if tenant.Routing.Fallback.Enabled && tenant.Routing.Fallback.TimeoutMs > 0 {
			attemptCtx, cancel = withTimeoutCtx(ctx, tenant.Routing.Fallback.TimeoutMs)
		}

		cbKey := fmt.Sprintf("%s:%s", modelCfg.Provider, tenant.ID)
		cbAllowed, isProbe, _ := h.breaker.Allow(attemptCtx, cbKey)
		if !cbAllowed {
			if cancel != nil {
				cancel()
			}
			h.log.WarnContext(ctx, "circuit breaker open",
				slog.String("provider", modelCfg.Provider),
				slog.String("model", modelName))
			continue
		}
		preRequestBuildMSVal := int(time.Since(preRequestBuildStart).Milliseconds())
		preRequestBuildMS = &preRequestBuildMSVal

		attemptStart := time.Now()

		attemptCtx, attemptSpan := gatewayotel.Tracer().Start(attemptCtx, "upstream_call")
		attemptSpan.SetAttributes(
			gatewayotel.AttrModel(modelName),
			gatewayotel.AttrProvider(modelCfg.Provider),
			gatewayotel.AttrAttempt(attempt+1),
			gatewayotel.AttrTenant(tenant.ID),
		)

		upstreamStart := time.Now()
		routerPreMSVal := int(upstreamStart.Sub(requestStart).Milliseconds())

		// Streaming branch.
		if req.Stream {
			if cancel != nil {
				cancel()
			}
			streamResp, streamErr := prov.ChatCompletionStream(ctx, provReq)
			if errors.Is(streamErr, providers.ErrStreamingNotSupported) {
				attemptSpan.End()
				st.setError(modelName, modelCfg.Provider, "invalid_request_error")
				writeError(in.W, 400, "streaming not supported for selected provider", "invalid_request_error")
				return OrchestratorOutput{Err: streamErr}
			}
			if streamErr != nil {
				attemptSpan.End()
				errType := h.errorClassifier.Classify(streamErr)
				if isCBFailure(errType) {
					_ = h.breaker.Report(ctx, cbKey, circuitbreaker.OutcomeFailure, isProbe)
				}
				latencyMs := float64(time.Since(attemptStart).Milliseconds())
				gatewayotel.UpstreamLatencyMs.WithLabelValues(modelCfg.Provider, modelName, "error").Observe(latencyMs)
				h.log.ErrorContext(ctx, "streaming upstream failed",
					slog.String("model", modelName),
					slog.String("tenant", tenant.ID),
					slog.String("error", streamErr.Error()),
				)
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
					Error:            streamErr.Error(),
					FallbackUsed:     attempt > 0,
					DecisionReason:   decisionReason,
					ErrorType:        string(errType),
					DecisionSnapshot: requestSnapshotJSON,
					Metadata:         metadataJSON,
					RouterPreMS:      &routerPreMSVal,
					APIKeyID:         apiKeyID,
					APIKeyName:       apiKeyName,
					JWTSub:           jwtSub,
					CustomerID:       ctxHeaders.CustomerID,
					Channel:          ctxHeaders.Channel,
					InteractionType:  ctxHeaders.InteractionType,
					AgentID:          ctxHeaders.AgentID,
					Department:       ctxHeaders.Department,
					TicketID:         ctxHeaders.TicketID,
					CustomerSegment:  ctxHeaders.CustomerSegment,
					Language:         ctxHeaders.Language,
					Intent:           ctxHeaders.Intent,
					ExperimentID:     ctxHeaders.ExperimentID,
					AutonomyLevel:    ctxHeaders.AutonomyLevel,
					PolicyID:         ctxHeaders.PolicyID,
					RiskLevel:        ctxHeaders.RiskLevel,
					RevenueImpact:    ctxHeaders.RevenueImpact,
					Currency:         ctxHeaders.Currency,
				})
				st.setError(modelName, modelCfg.Provider, string(errType))
				if !h.errorClassifier.IsRetryable(errType) {
					writeUpstreamError(in.W, streamErr)
					return OrchestratorOutput{Err: streamErr}
				}
				lastErr = streamErr
				continue
			}
			// The SSE stream loop (writeSSEStream) uses in.W directly.
			// This is the Phase 3 delegation: Orchestrator calls back into Handlers for the SSE loop.
			h.writeSSEStream(ctx, in.W, streamResp, provReq, requestID.String(), tenant, modelName, modelCfg,
				requestStart, decisionReason, requestSnapshotJSON, metadataJSON,
				attempt, apiKeyID, apiKeyName, jwtSub, routerPreMSVal, budgetReservationID,
				messages, piiRequestDecision, piiResponseDecision,
				ctxHeaders.CustomerID,
				ctxHeaders.Channel, ctxHeaders.InteractionType, ctxHeaders.AgentID, ctxHeaders.Department,
				ctxHeaders.TicketID, ctxHeaders.CustomerSegment, ctxHeaders.Language,
				ctxHeaders.Intent, ctxHeaders.ExperimentID, ctxHeaders.AutonomyLevel,
				ctxHeaders.PolicyID, ctxHeaders.RiskLevel, ctxHeaders.RevenueImpact, ctxHeaders.Currency)
			attemptSpan.End()
			_ = h.breaker.Report(ctx, cbKey, circuitbreaker.OutcomeSuccess, isProbe)
			st.setOk(modelName, modelCfg.Provider, false, 0, 0, 0)
			return OrchestratorOutput{}
		}

		resp, err := prov.ChatCompletion(attemptCtx, provReq)
		upstreamDone := time.Now()
		llmLatencyMSVal := int(upstreamDone.Sub(upstreamStart).Milliseconds())
		latencyMs := float64(time.Since(attemptStart).Milliseconds())
		upstreamStatus := "success"
		if err != nil {
			upstreamStatus = "error"
		}
		gatewayotel.UpstreamLatencyMs.WithLabelValues(modelCfg.Provider, modelName, upstreamStatus).Observe(latencyMs)

		if cancel != nil {
			cancel()
		}

		if err != nil {
			errType := h.errorClassifier.Classify(err)
			cbOutcome := circuitbreaker.OutcomeSuccess
			if isCBFailure(errType) {
				cbOutcome = circuitbreaker.OutcomeFailure
			}
			_ = h.breaker.Report(ctx, cbKey, cbOutcome, isProbe)

			attemptSpan.SetStatus(codes.Error, err.Error())
			attemptSpan.SetAttributes(
				attribute.String("error", err.Error()),
				attribute.String("error_type", string(errType)),
			)
			attemptSpan.End()

			logWithMode(ctx, h.log, LogMode(h.cfg.Server.LogMode), slog.LevelWarn, "upstream attempt failed",
				slog.String("tenant", tenant.ID),
				slog.String("model", modelName),
				slog.String("provider", modelCfg.Provider),
				slog.Int("attempt", attempt+1),
				slog.Int("latency_ms", int(latencyMs)),
				slog.String("status", "error"),
				slog.String("error", err.Error()),
				slog.String("error_type", string(errType)),
			)

			h.logRequestAsync(ctx, storage.RequestLog{
				ID:                            uuid.New(),
				RequestID:                     requestID.String(),
				Attempt:                       attempt + 1,
				TenantID:                      tenant.ID,
				Model:                         modelName,
				Provider:                      modelCfg.Provider,
				Strategy:                      tenant.Routing.Strategy,
				Status:                        "error",
				LatencyMs:                     int(latencyMs),
				Error:                         err.Error(),
				FallbackUsed:                  attempt > 0,
				PIIWebhookRequestDecision:     piiRequestDecision,
				PIIWebhookResponseDecision:    nil,
				DecisionReason:                decisionReason,
				ErrorType:                     string(errType),
				DecisionSnapshot:              requestSnapshotJSON,
				Metadata:                      metadataJSON,
				RouterPreMS:                   &routerPreMSVal,
				LLMLatencyMS:                  &llmLatencyMSVal,
				PreDecodeMS:                   preDecodeMS,
				PreAuthzMS:                    preAuthzMS,
				PreTenantConfigMS:             preTenantConfigMS,
				PrePIIMS:                      prePIIMS,
				PreRateLimitMS:                preRateLimitMS,
				PreModelFilterMS:              preModelFilterMS,
				PreRoutingMS:                  preRoutingMS,
				PreRequestBuildMS:             preRequestBuildMS,
				CfgToolRoutesMS:               cfgToolRoutesMS,
				CfgDynamicRoutesMS:            cfgDynamicRoutesMS,
				CfgBudgetPressureMS:           cfgBudgetPressureMS,
				CfgSemanticMS:                 cfgSemanticMS,
				CfgModelResolutionMS:          cfgModelResolutionMS,
				ToolRoutesEmbeddingModelMS:    toolRoutesEmbeddingModelMS,
				ToolRoutesEmbeddingGenerateMS: toolRoutesEmbeddingGenerateMS,
				ToolRoutesSemanticDBMS:        toolRoutesSemanticDBMS,
				ToolRoutesMatchEvalMS:         toolRoutesMatchEvalMS,
				APIKeyID:                      apiKeyID,
				APIKeyName:                    apiKeyName,
				JWTSub:                        jwtSub,
				CustomerID:                    ctxHeaders.CustomerID,
				Channel:                       ctxHeaders.Channel,
				InteractionType:               ctxHeaders.InteractionType,
				AgentID:                       ctxHeaders.AgentID,
				Department:                    ctxHeaders.Department,
				TicketID:                      ctxHeaders.TicketID,
				CustomerSegment:               ctxHeaders.CustomerSegment,
				Language:                      ctxHeaders.Language,
				Intent:                        ctxHeaders.Intent,
				ExperimentID:                  ctxHeaders.ExperimentID,
				AutonomyLevel:                 ctxHeaders.AutonomyLevel,
				PolicyID:                      ctxHeaders.PolicyID,
				RiskLevel:                     ctxHeaders.RiskLevel,
				RevenueImpact:                 ctxHeaders.RevenueImpact,
				Currency:                      ctxHeaders.Currency,
			})

			h.router.UpdateModelStats(tenant.ID, modelName, false)

			dateUTC := time.Now().UTC().Truncate(24 * time.Hour)
			stat := storage.ModelStatDaily{
				Date:         dateUTC,
				TenantID:     tenant.ID,
				Model:        modelName,
				SuccessCount: 0,
				ErrorCount:   1,
				AvgLatencyMs: latencyMs,
				TotalCostUSD: 0,
			}
			h.statsDispatcher.Submit(stat)

			lastErr = err

			if !h.errorClassifier.IsRetryable(errType) {
				st.setError(modelName, modelCfg.Provider, string(errType))
				writeUpstreamError(in.W, err)
				return OrchestratorOutput{Err: err}
			}
			continue
		}

		// Success path.
		attemptSpan.SetStatus(codes.Ok, "")
		attemptSpan.SetAttributes(gatewayotel.AttrStatus(200))

		if isMock {
			mockProv := prov.(*providers.MockProvider)
			attemptSpan.AddEvent("mock_response", trace.WithAttributes(
				attribute.Bool("mock.enabled", true),
				attribute.Int("mock.delay_ms", mockProv.GetActualDelay()),
				attribute.Float64("mock.error_rate", modelCfg.Mock.ErrorRate),
			))
		}

		attemptSpan.End()

		h.router.RecordLatency(tenant.ID, modelName, latencyMs)
		_ = h.breaker.Report(ctx, cbKey, circuitbreaker.OutcomeSuccess, isProbe)

		costUSD := computeCost(modelCfg.Pricing, resp.Usage, gc.ToolPricing)
		toolCostUSD := computeToolCost(resp.Usage, gc.ToolPricing)
		h.router.UpdateModelStats(tenant.ID, modelName, true)

		dateUTC := time.Now().UTC().Truncate(24 * time.Hour)
		stat := storage.ModelStatDaily{
			Date:         dateUTC,
			TenantID:     tenant.ID,
			Model:        modelName,
			SuccessCount: 1,
			ErrorCount:   0,
			AvgLatencyMs: latencyMs,
			TotalCostUSD: costUSD,
		}
		h.statsDispatcher.Submit(stat)

		totalLatencyMs := int(time.Since(requestStart).Milliseconds())

		logFields := []any{
			"tenant", tenant.ID,
			"model", modelName,
			"provider", modelCfg.Provider,
			"attempt", attempt + 1,
			"latency_ms", latencyMs,
			"status", "ok",
			"prompt_tokens", resp.Usage.PromptTokens,
			"completion_tokens", resp.Usage.CompletionTokens,
			"cost_usd", costUSD,
			"mock_enabled", isMock,
		}
		if isMock {
			mockProv := prov.(*providers.MockProvider)
			logFields = append(logFields, "mock_delay_ms", mockProv.GetActualDelay())
		}
		h.log.InfoContext(ctx, "upstream attempt succeeded", logFields...)

		clientRequestID := resp.ID
		if clientRequestID == "" {
			clientRequestID = requestID.String()
		}
		sharedRequestID = clientRequestID

		fallbackUsed := attempt > 0

		h.saveUsageAsync(ctx, storage.UsageRecord{
			ID:               uuid.New(),
			TenantID:         tenant.ID,
			Model:            modelName,
			Provider:         modelCfg.Provider,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			CostUSD:          costUSD,
			RequestID:        clientRequestID,
			APIKeyID:         apiKeyID,
			APIKeyName:       apiKeyName,
			JWTSub:           jwtSub,
		})
		h.releaseReservationAsync(ctx, budgetReservationID)

		// PostResponse hooks.
		for _, hook := range tenantHooks {
			postResult, postErr := hook.PostResponse(ctx, tenant, provReq, resp)
			hookName := hook.Name()

			if hookName == "external_pii" && postResult.Response != nil && postResult.Response != resp {
				modify := "modify"
				piiResponseDecision = &modify
			}

			span.AddEvent("hook.post_response", trace.WithAttributes(
				attribute.String("hook.name", hookName),
				attribute.String("hook.reason", postResult.Reason),
			))

			if postErr != nil {
				h.log.ErrorContext(ctx, "post-response hook error",
					"hook", hookName, "tenant", tenant.ID, "error", postErr)
				continue
			}

			if postResult.Response != nil {
				resp = postResult.Response
			}
		}

		// Build routing snapshot.
		routingSnapshot := &router.RoutingSnapshot{
			RoutingStrategy:  tenant.Routing.Strategy,
			CandidateModels:  result.Candidates,
			SelectedModel:    modelName,
			Provider:         modelCfg.Provider,
			FallbackAttempts: attempt,
			Timestamp:        requestStart.UTC(),
			RouteGroup:       precedenceResult.RouteGroupUsed,
		}
		if result.SmartResult != nil && result.SmartResult.SemanticAnchor != "" {
			routingSnapshot.SemanticAnchor = result.SmartResult.SemanticAnchor
			routingSnapshot.Similarity = result.SmartResult.SemanticSimilarity
			threshold := tenant.Routing.Semantic.ThresholdDefault
			if threshold == 0 {
				threshold = 0.60
			}
			routingSnapshot.Threshold = threshold
			if result.SmartResult.AnchorRouteGroup != "" {
				routingSnapshot.RouteGroup = result.SmartResult.AnchorRouteGroup
			}
		}
		if result.SmartResult != nil && result.SmartResult.CostOptimizerApplied {
			routingSnapshot.EstimatedCostsUSD = result.SmartResult.EstimatedCostsUSD
			routingSnapshot.BudgetPressure = result.SmartResult.BudgetPressure
			routingSnapshot.CostOptimizerApplied = true
		}
		if trafficSplitApplied {
			routingSnapshot.TrafficSplitApplied = true
			routingSnapshot.TrafficSplitKey = trafficSplitKey
			for _, e := range trafficSplitCandidates {
				routingSnapshot.TrafficSplitCandidates = append(
					routingSnapshot.TrafficSplitCandidates,
					router.TrafficSplitSnapshotEntry{Model: e.Model, Weight: e.Weight},
				)
			}
		}
		routingSnapshotJSON, _ := routingSnapshot.ToJSON()
		routerPostMSVal := int(time.Since(upstreamDone).Milliseconds())

		stepDecisionSnapshotJSON := requestSnapshotJSON

		h.logRequestAsync(ctx, storage.RequestLog{
			ID:                            uuid.New(),
			RequestID:                     clientRequestID,
			Attempt:                       attempt + 1,
			TenantID:                      tenant.ID,
			Model:                         modelName,
			Provider:                      modelCfg.Provider,
			Strategy:                      tenant.Routing.Strategy,
			Status:                        "ok",
			LatencyMs:                     totalLatencyMs,
			FallbackUsed:                  fallbackUsed,
			PIIWebhookRequestDecision:     piiRequestDecision,
			PIIWebhookResponseDecision:    piiResponseDecision,
			DecisionReason:                decisionReason,
			ErrorType:                     "",
			DecisionSnapshot:              stepDecisionSnapshotJSON,
			Metadata:                      metadataJSON,
			RoutingSnapshot:               routingSnapshotJSON,
			RouterPreMS:                   &routerPreMSVal,
			LLMLatencyMS:                  &llmLatencyMSVal,
			RouterPostMS:                  &routerPostMSVal,
			PreDecodeMS:                   preDecodeMS,
			PreAuthzMS:                    preAuthzMS,
			PreTenantConfigMS:             preTenantConfigMS,
			PrePIIMS:                      prePIIMS,
			PreRateLimitMS:                preRateLimitMS,
			PreModelFilterMS:              preModelFilterMS,
			PreRoutingMS:                  preRoutingMS,
			PreRequestBuildMS:             preRequestBuildMS,
			CfgToolRoutesMS:               cfgToolRoutesMS,
			CfgDynamicRoutesMS:            cfgDynamicRoutesMS,
			CfgBudgetPressureMS:           cfgBudgetPressureMS,
			CfgSemanticMS:                 cfgSemanticMS,
			CfgModelResolutionMS:          cfgModelResolutionMS,
			ToolRoutesEmbeddingModelMS:    toolRoutesEmbeddingModelMS,
			ToolRoutesEmbeddingGenerateMS: toolRoutesEmbeddingGenerateMS,
			ToolRoutesSemanticDBMS:        toolRoutesSemanticDBMS,
			ToolRoutesMatchEvalMS:         toolRoutesMatchEvalMS,
			APIKeyID:                      apiKeyID,
			APIKeyName:                    apiKeyName,
			JWTSub:                        jwtSub,
			CustomerID:                    ctxHeaders.CustomerID,
			Channel:                       ctxHeaders.Channel,
			InteractionType:               ctxHeaders.InteractionType,
			AgentID:                       ctxHeaders.AgentID,
			Department:                    ctxHeaders.Department,
			TicketID:                      ctxHeaders.TicketID,
			CustomerSegment:               ctxHeaders.CustomerSegment,
			Language:                      ctxHeaders.Language,
			Intent:                        ctxHeaders.Intent,
			ExperimentID:                  ctxHeaders.ExperimentID,
			AutonomyLevel:                 ctxHeaders.AutonomyLevel,
			PolicyID:                      ctxHeaders.PolicyID,
			RiskLevel:                     ctxHeaders.RiskLevel,
			RevenueImpact:                 ctxHeaders.RevenueImpact,
			Currency:                      ctxHeaders.Currency,
			CachedTokens:                  resp.Usage.CachedInputTokens,
			ToolCostUSD:                   toolCostUSD,
		})

		// Build API response.
		apiResp := ChatCompletionResponse{
			ID:      resp.ID,
			Object:  resp.Object,
			Created: resp.Created,
			Model:   resp.Model,
			Usage: Usage{
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			},
		}
		for _, c := range resp.Choices {
			apiResp.Choices = append(apiResp.Choices, ChatChoice{
				Index:        c.Index,
				Message:      mapProviderMessage(c.Message),
				FinishReason: c.FinishReason,
			})
		}
		h.logConversationAsync(ctx, tenant, clientRequestID, messages, apiResp.Choices, piiRequestDecision, piiResponseDecision, jwtSub, ctxHeaders.CustomerID)

		if virtualAliasActive && virtualExposeAlias {
			apiResp.Model = virtualAliasName
		}
		span.SetAttributes(gatewayotel.AttrModel(modelName))
		span.SetStatus(codes.Ok, "")

		// Build extra headers for the handler to copy to http.ResponseWriter.
		extraHeaders := map[string]string{
			"X-Selected-Model": modelName,
		}
		if isMock {
			extraHeaders["X-Mock-Response"] = "true"
		}
		if trafficSplitApplied {
			extraHeaders["X-Traffic-Split-Applied"] = "true"
			extraHeaders["X-Traffic-Split-Key"] = trafficSplitKey
		}

		// Semantic cache write (fire-and-forget).
		if semCacheEnabled && requestEmbedding != nil {
			apiRespJSON, _ := json.Marshal(apiResp)
			effectiveCacheRouteGroup := precedenceResult.RouteGroupUsed
			if result.SmartResult != nil && result.SmartResult.AnchorRouteGroup != "" {
				effectiveCacheRouteGroup = result.SmartResult.AnchorRouteGroup
			}
			cacheEntry := storage.SemanticCacheInsert{
				TenantID:     tenant.ID,
				Model:        modelName,
				RouteGroup:   effectiveCacheRouteGroup,
				Embedding:    requestEmbedding,
				RequestText:  strings.Join(messages, " "),
				ResponseJSON: apiRespJSON,
				ExpiresAt:    time.Now().Add(time.Duration(cacheTTLSeconds(tenant.SemanticCache.TTLSeconds)) * time.Second),
			}
			if h.semCacheAsync.TryAcquire(1) {
				go func() {
					defer h.semCacheAsync.Release(1)
					if insErr := h.store.InsertSemanticCacheEntry(context.Background(), cacheEntry); insErr != nil {
						h.log.WarnContext(context.Background(), "semantic cache insert failed", slog.String("error", insErr.Error()))
					}
				}()
			} else {
				gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_write").Inc()
			}
			gatewayotel.SemanticCacheWriteCounter.WithLabelValues(tenant.ID).Inc()
			extraHeaders["X-Semantic-Cache"] = "MISS"
		}

		// Build the canonical response. The handler writes this to http.ResponseWriter.
		// Prefer the unmodified upstream body (set by providers that do their own parsing,
		// e.g. Responses API) over a re-marshal of the already-converted ChatResponse.
		rawProvJSON := resp.RawBody
		if rawProvJSON == nil {
			rawProvJSON, _ = json.Marshal(resp)
		}
		cr := CanonicalResponse{
			ID:                  resp.ID,
			Object:              resp.Object,
			Created:             resp.Created,
			Model:               apiResp.Model, // may have been replaced by virtual alias
			Usage:               resp.Usage,
			Choices:             resp.Choices,
			RawProviderResponse: rawProvJSON,
			// RawPassthroughBody is set only when the provider returned raw upstream bytes
			// that must be written verbatim (e.g. Responses API). Endpoint handlers check
			// this field to skip re-serialisation and avoid conversion loss.
			RawPassthroughBody:  resp.RawBody,
			SelectedModel:       modelName,
			Provider:            modelCfg.Provider,
			RouteGroup:          precedenceResult.RouteGroupUsed,
			Latency:             time.Since(requestStart),
			ExtraHeaders:        extraHeaders,
		}

		st.setOk(modelName, modelCfg.Provider, true, costUSD, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
		return OrchestratorOutput{Response: &cr}
	}

	// All attempts exhausted.
	span.SetStatus(codes.Error, "all candidates failed")
	h.log.ErrorContext(ctx, "all upstream attempts failed",
		"tenant", tenant.ID,
		"candidates", result.Candidates,
		"max_attempts", maxAttempts,
	)

	errLabel := "all_attempts_failed"
	if lastErr != nil {
		errLabel = string(h.errorClassifier.Classify(lastErr))
	}
	st.setError(lastModelName, lastProvider, errLabel)

	if lastErr != nil {
		writeUpstreamError(in.W, lastErr)
		return OrchestratorOutput{Err: lastErr}
	}
	writeError(in.W, 502, "all upstream providers failed", "upstream_error")
	return OrchestratorOutput{Err: errors.New("all upstream providers failed")}
}

// ContextHeaders holds all optional business-context headers extracted from an incoming HTTP request.
// All fields are nullable — nil when the corresponding header is absent or empty.
type ContextHeaders struct {
	CustomerID      *string
	Channel         *string
	InteractionType *string
	AgentID         *string
	Department      *string
	TicketID        *string
	CustomerSegment *string
	Language        *string
	Intent          *string
	ExperimentID    *string
	AutonomyLevel   *string
	PolicyID        *string
	RiskLevel       *string
	RevenueImpact   *string
	Currency        *string
}

// extractAllContextHeaders reads all 15 optional business-context headers from an HTTP request.
// Returns a ContextHeaders struct with nil for each header that is absent or empty.
func extractAllContextHeaders(h http.Header) ContextHeaders {
	get := func(key string) *string {
		if v := h.Get(key); v != "" {
			return &v
		}
		return nil
	}
	return ContextHeaders{
		CustomerID:      get("X-Customer-Id"),
		Channel:         get("X-Channel"),
		InteractionType: get("X-Interaction-Type"),
		AgentID:         get("X-Agent-Id"),
		Department:      get("X-Department"),
		TicketID:        get("X-Ticket-Id"),
		CustomerSegment: get("X-Customer-Segment"),
		Language:        get("X-Language"),
		Intent:          get("X-Intent"),
		ExperimentID:    get("X-Experiment-Id"),
		AutonomyLevel:   get("X-Autonomy-Level"),
		PolicyID:        get("X-Policy-Id"),
		RiskLevel:       get("X-Risk-Level"),
		RevenueImpact:   get("X-Revenue-Impact"),
		Currency:        get("X-Currency"),
	}
}
