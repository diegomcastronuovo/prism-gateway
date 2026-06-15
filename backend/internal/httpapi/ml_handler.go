package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// handleMLRequest processes a request for an ML model (X-Model-Type: ml).
// It bypasses the LLM pipeline entirely:
//  1. Resolves model from X-Model-Name header
//  2. Validates it is type "ml" with an execution endpoint
//  3. Forwards the raw body as-is via HTTP POST to Execution.Endpoint
//  4. Extracts configured observable fields from request + response and logs them
//  5. Returns the upstream response body as-is (no OpenAI wrapping)
func (h *Handlers) handleMLRequest(w http.ResponseWriter, r *http.Request, rawBody []byte) {
	ctx := r.Context()
	mlStart := time.Now()

	modelName := r.Header.Get("X-Model-Name")
	if modelName == "" {
		h.recordPrometheusMLPreUpstreamError(ctx, "", "unknown", "unknown", "invalid_request_error", mlStart)
		writeError(w, http.StatusBadRequest, "X-Model-Name header is required for ml requests", "invalid_request_error")
		return
	}

	modelCfg := h.resolveModelByName(ctx, modelName)
	if modelCfg == nil {
		h.recordPrometheusMLPreUpstreamError(ctx, "", modelName, "unknown", "not_found", mlStart)
		writeError(w, http.StatusNotFound, "model not found: "+modelName, "not_found")
		return
	}
	if !modelCfg.IsEnabled() {
		h.recordPrometheusMLPreUpstreamError(ctx, "", modelName, modelCfg.Provider, "model_disabled", mlStart)
		writeError(w, http.StatusForbidden, "model is disabled", "model_disabled")
		return
	}
	if modelCfg.Type != "ml" {
		h.recordPrometheusMLPreUpstreamError(ctx, "", modelName, modelCfg.Provider, "invalid_request_error", mlStart)
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("model %q has type %q, not ml", modelName, modelCfg.Type),
			"invalid_request_error")
		return
	}
	if modelCfg.Execution == nil || modelCfg.Execution.Endpoint == "" {
		h.recordPrometheusMLPreUpstreamError(ctx, "", modelName, modelCfg.Provider, "invalid_request_error", mlStart)
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("model %q has no execution endpoint configured", modelName),
			"invalid_request_error")
		return
	}

	tenant := auth.TenantFromContext(ctx)
	tenantID := ""
	if tenant != nil {
		tenantID = tenant.ID
	}

	// Enforce tenant-level model authorization.
	if !mlModelAllowedForTenant(modelName, tenant) {
		h.recordPrometheusMLPreUpstreamError(ctx, tenantID, modelName, modelCfg.Provider, "authorization_error", mlStart)
		writeError(w, http.StatusForbidden, "model not allowed for tenant", "authorization_error")
		return
	}

	// Extract caller identity from auth context (same pattern as LLM path).
	apiKeyID, apiKeyName := auth.APIKeyAttributionFromContext(ctx)
	jwtSub := auth.JWTSubFromContext(ctx)

	requestID := uuid.New().String()
	decisionReason := "explicit_ml_header|model:" + modelName

	// Build minimal routing snapshot for ML.
	routingSnapshotJSON, _ := json.Marshal(map[string]interface{}{
		"kind":               "ml",
		"model":              modelName,
		"provider":           modelCfg.Provider,
		"execution_endpoint": modelCfg.Execution.Endpoint,
	})

	// Call the ML execution endpoint.
	upstreamStart := time.Now()
	respBody, statusCode, err := callMLEndpoint(ctx, modelCfg.Execution.Endpoint, rawBody)
	latencyMs := int(time.Since(upstreamStart).Milliseconds())
	if err != nil {
		h.log.ErrorContext(ctx, "ml upstream call failed",
			slog.String("model", modelName),
			slog.String("tenant", tenantID),
			slog.String("endpoint", modelCfg.Execution.Endpoint),
			slog.Int("latency_ms", latencyMs),
			slog.String("error", err.Error()),
		)

		// Persist failure row — observable fields from request side only.
		metadataJSON := buildMLObservableMetadata(modelCfg, rawBody, nil)
		h.logRequestAsync(ctx, storage.RequestLog{
			ID:               uuid.New(),
			RequestID:        requestID,
			Attempt:          1,
			TenantID:         tenantID,
			Model:            modelName,
			Provider:         modelCfg.Provider,
			Strategy:         "ml",
			Status:           "error",
			LatencyMs:        latencyMs,
			Error:            err.Error(),
			DecisionReason:   decisionReason,
			Metadata:         metadataJSON,
			RoutingSnapshot:  routingSnapshotJSON,
			APIKeyID:         apiKeyID,
			APIKeyName:       apiKeyName,
			JWTSub:           jwtSub,
		})

		totalMs := int(time.Since(mlStart).Milliseconds())
		h.recordPrometheusMLOutcome(ctx, tenantID, modelName, modelCfg.Provider, "error", totalMs, err, 0, 0)

		writeError(w, http.StatusBadGateway, "ml execution failed: "+err.Error(), "upstream_error")
		return
	}

	// Persist success row with observable metadata from request + response.
	metadataJSON := buildMLObservableMetadata(modelCfg, rawBody, respBody)
	h.logRequestAsync(ctx, storage.RequestLog{
		ID:              uuid.New(),
		RequestID:       requestID,
		Attempt:         1,
		TenantID:        tenantID,
		Model:           modelName,
		Provider:        modelCfg.Provider,
		Strategy:        "ml",
		Status:          "ok",
		LatencyMs:       latencyMs,
		DecisionReason:  decisionReason,
		Metadata:        metadataJSON,
		RoutingSnapshot: routingSnapshotJSON,
		APIKeyID:        apiKeyID,
		APIKeyName:      apiKeyName,
		JWTSub:          jwtSub,
	})

	// Persist usage record so ML requests appear in FinOps aggregation.
	// ML models have zero token cost; effective cost is derived from infra allocation.
	h.saveUsageAsync(ctx, storage.UsageRecord{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Model:     modelName,
		Provider:  modelCfg.Provider,
		RequestID: requestID,
		APIKeyID:  apiKeyID,
		APIKeyName: apiKeyName,
		JWTSub:    jwtSub,
	})

	// Log observable fields to slog as well (informational).
	if modelCfg.Observable != nil && len(modelCfg.Observable.Fields) > 0 {
		logMLObservable(ctx, h.log, modelCfg, tenantID, rawBody, respBody)
	}

	h.log.InfoContext(ctx, "ml request completed",
		slog.String("model", modelName),
		slog.String("tenant", tenantID),
		slog.Int("latency_ms", latencyMs),
		slog.Int("status", statusCode),
	)

	obsFieldCount := 0
	if modelCfg.Observable != nil {
		obsFieldCount = len(modelCfg.Observable.Fields)
	}
	totalMs := int(time.Since(mlStart).Milliseconds())
	h.recordPrometheusMLOutcome(ctx, tenantID, modelName, modelCfg.Provider, "ok", totalMs, nil, 0, obsFieldCount)

	// Return the upstream response as-is.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Model-Name", modelName)
	w.WriteHeader(statusCode)
	w.Write(respBody) //nolint:errcheck
}

// buildMLObservableMetadata extracts configured observable fields from request and
// response bodies and returns them serialized as JSON metadata.
// Only explicitly configured fields are included — never the full payload.
// If no fields are configured or extraction fails, returns nil.
func buildMLObservableMetadata(m *config.ModelConfig, reqBody, respBody []byte) json.RawMessage {
	if m.Observable == nil || len(m.Observable.Fields) == 0 {
		return nil
	}

	var reqMap, respMap map[string]interface{}
	_ = json.Unmarshal(reqBody, &reqMap)
	if respBody != nil {
		_ = json.Unmarshal(respBody, &respMap)
	}

	obs := make(map[string]interface{})
	for _, field := range m.Observable.Fields {
		var src map[string]interface{}
		switch field.Role {
		case "input":
			src = reqMap
		case "output":
			src = respMap
		default:
			continue
		}
		val := extractJSONPath(src, field.Path)
		if val == nil {
			continue
		}
		obs[field.Path] = val
	}

	if len(obs) == 0 {
		return nil
	}
	b, err := json.Marshal(map[string]interface{}{"observable": obs})
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

// callMLEndpoint performs an HTTP POST to the ML execution endpoint and returns the response body.
func callMLEndpoint(ctx context.Context, endpoint string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("upstream call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// logMLObservable extracts and logs the observable fields defined in the model config.
// Only the explicitly configured fields are logged — never the full payload.
func logMLObservable(
	ctx context.Context,
	log *slog.Logger,
	m *config.ModelConfig,
	tenantID string,
	reqBody []byte,
	respBody []byte,
) {
	// Parse both bodies lazily (only if fields reference them).
	var reqMap, respMap map[string]interface{}
	if err := json.Unmarshal(reqBody, &reqMap); err != nil {
		reqMap = nil
	}
	if err := json.Unmarshal(respBody, &respMap); err != nil {
		respMap = nil
	}

	attrs := []interface{}{
		slog.String("model", m.Name),
		slog.String("tenant", tenantID),
	}
	for _, field := range m.Observable.Fields {
		var src map[string]interface{}
		switch field.Role {
		case "input":
			src = reqMap
		case "output":
			src = respMap
		default:
			continue
		}
		val := extractJSONPath(src, field.Path)
		if val == nil {
			continue
		}
		attrs = append(attrs, slog.Any("observable."+field.Path, val))
	}

	log.InfoContext(ctx, "ml observable fields", attrs...)
}

// mlModelAllowedForTenant returns true only if modelName is explicitly listed in
// tenant.AllowedModels. Nil tenant, nil/empty AllowedModels, or missing model all return false.
func mlModelAllowedForTenant(modelName string, tenant *config.TenantConfig) bool {
	if tenant == nil || len(tenant.AllowedModels) == 0 {
		return false
	}
	for _, m := range tenant.AllowedModels {
		if m == modelName {
			return true
		}
	}
	return false
}

// extractJSONPath navigates a dot-separated path through a JSON object map.
// Returns nil if any segment is missing or the value is not navigable.
// Example: extractJSONPath({"input":{"features":{"x":1}}}, "input.features") → {"x":1}
func extractJSONPath(data map[string]interface{}, path string) interface{} {
	if data == nil {
		return nil
	}
	parts := strings.SplitN(path, ".", 2)
	val, ok := data[parts[0]]
	if !ok {
		return nil
	}
	if len(parts) == 1 {
		return val
	}
	nested, ok := val.(map[string]interface{})
	if !ok {
		return nil
	}
	return extractJSONPath(nested, parts[1])
}
