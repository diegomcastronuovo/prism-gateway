package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// ExternalPII validates requests/responses via HTTP webhook for PII detection.
type ExternalPII struct {
	config     config.ExternalPIIHookConfig
	httpClient *http.Client
	log        *slog.Logger
}

// NewExternalPII creates a new external PII webhook hook instance.
func NewExternalPII(cfg config.ExternalPIIHookConfig, log *slog.Logger) *ExternalPII {
	return &ExternalPII{
		config: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.TimeoutMs) * time.Millisecond,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
			},
		},
		log: log,
	}
}

func (h *ExternalPII) Name() string {
	return "external_pii"
}

func (h *ExternalPII) PreRequest(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest) (PreResult, error) {
	if !h.config.Request.Enabled {
		return PreResult{Decision: Allow, Request: req}, nil
	}

	ctx, span := otel.Tracer("hooks").Start(ctx, "external_pii.PreRequest")
	defer span.End()

	webhookURL := h.config.BaseURL + h.config.Request.Path
	span.SetAttributes(
		attribute.String("webhook.url", webhookURL),
		attribute.String("webhook.phase", "request"),
		attribute.Int("webhook.timeout_ms", h.config.TimeoutMs),
		attribute.String("webhook.fail_mode", h.config.FailMode),
	)

	h.log.InfoContext(ctx, "calling external PII webhook",
		"tenant_id", tenant.ID,
		"model", req.Model,
		"webhook_url", webhookURL,
		"phase", "pre_request",
		"timeout_ms", h.config.TimeoutMs,
	)

	payload := WebhookRequest{
		Body: struct {
			Messages []providers.ChatMessage `json:"messages,omitempty"`
			Choices  []providers.ChatChoice  `json:"choices,omitempty"`
		}{
			Messages: req.Messages,
		},
	}

	start := time.Now()
	rawResp, err := h.callWebhook(ctx, webhookURL, payload)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		h.log.ErrorContext(ctx, "webhook call failed",
			"tenant_id", tenant.ID,
			"webhook_url", webhookURL,
			"error", err.Error(),
			"fail_mode", h.config.FailMode,
			"latency_ms", latencyMs,
		)
		span.RecordError(err)
		span.SetAttributes(
			attribute.String("webhook.error_type", "network_or_timeout"),
			attribute.Int64("webhook.latency_ms", latencyMs),
		)
		return h.handleWebhookError(ctx, span, err, req)
	}

	action, err := ParsePIIWebhookResponse(rawResp)
	if err != nil {
		h.log.ErrorContext(ctx, "invalid webhook response",
			"tenant_id", tenant.ID,
			"webhook_url", webhookURL,
			"error", err.Error(),
			"fail_mode", h.config.FailMode,
		)
		span.RecordError(err)
		span.SetAttributes(attribute.String("webhook.error_type", "invalid_response"))
		return h.handleWebhookError(ctx, span, err, req)
	}

	result, err := mapNormalizedToPreResult(action, req)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to map webhook action",
			"tenant_id", tenant.ID,
			"webhook_url", webhookURL,
			"error", err.Error(),
		)
		span.RecordError(err)
		return h.handleWebhookError(ctx, span, err, req)
	}

	h.log.InfoContext(ctx, "webhook call succeeded",
		"tenant_id", tenant.ID,
		"webhook_url", webhookURL,
		"action", action.Kind.String(),
		"latency_ms", latencyMs,
		"reason", action.Reason,
	)
	span.SetAttributes(
		attribute.String("webhook.action", action.Kind.String()),
		attribute.Int64("webhook.latency_ms", latencyMs),
	)
	span.SetStatus(codes.Ok, "")

	return result, nil
}

func (h *ExternalPII) PostResponse(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest, resp *providers.ChatResponse) (PostResult, error) {
	if !h.config.Response.Enabled {
		return PostResult{Response: resp}, nil
	}

	ctx, span := otel.Tracer("hooks").Start(ctx, "external_pii.PostResponse")
	defer span.End()

	webhookURL := h.config.BaseURL + h.config.Response.Path
	span.SetAttributes(
		attribute.String("webhook.url", webhookURL),
		attribute.String("webhook.phase", "response"),
		attribute.Int("webhook.timeout_ms", h.config.TimeoutMs),
		attribute.String("webhook.fail_mode", h.config.FailMode),
	)

	h.log.InfoContext(ctx, "calling external PII webhook",
		"tenant_id", tenant.ID,
		"model", req.Model,
		"webhook_url", webhookURL,
		"phase", "post_response",
		"timeout_ms", h.config.TimeoutMs,
	)

	payload := WebhookRequest{
		Body: struct {
			Messages []providers.ChatMessage `json:"messages,omitempty"`
			Choices  []providers.ChatChoice  `json:"choices,omitempty"`
		}{
			Choices: resp.Choices,
		},
	}

	start := time.Now()
	rawResp, err := h.callWebhook(ctx, webhookURL, payload)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		h.log.ErrorContext(ctx, "webhook call failed",
			"tenant_id", tenant.ID,
			"webhook_url", webhookURL,
			"error", err.Error(),
			"fail_mode", h.config.FailMode,
			"latency_ms", latencyMs,
		)
		span.RecordError(err)
		span.SetAttributes(
			attribute.String("webhook.error_type", "network_or_timeout"),
			attribute.Int64("webhook.latency_ms", latencyMs),
		)
		// Post-response: always allow on error (can't block after provider call).
		if h.config.FailMode == "fail_closed" {
			span.SetAttributes(attribute.String("webhook.decision", "allow_on_error"))
			span.SetStatus(codes.Error, "webhook failed but allowing response")
		}
		return PostResult{Response: resp}, nil
	}

	action, err := ParsePIIWebhookResponse(rawResp)
	if err != nil {
		h.log.ErrorContext(ctx, "invalid webhook response",
			"tenant_id", tenant.ID,
			"webhook_url", webhookURL,
			"error", err.Error(),
		)
		span.RecordError(err)
		span.SetAttributes(attribute.String("webhook.error_type", "invalid_response"))
		return PostResult{Response: resp}, nil
	}

	result, err := mapNormalizedToPostResult(action, resp, h.log)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to map webhook action",
			"tenant_id", tenant.ID,
			"webhook_url", webhookURL,
			"error", err.Error(),
		)
		span.RecordError(err)
		return PostResult{Response: resp}, nil
	}

	h.log.InfoContext(ctx, "webhook call succeeded",
		"tenant_id", tenant.ID,
		"webhook_url", webhookURL,
		"action", action.Kind.String(),
		"latency_ms", latencyMs,
		"reason", action.Reason,
	)
	span.SetAttributes(
		attribute.String("webhook.action", action.Kind.String()),
		attribute.Int64("webhook.latency_ms", latencyMs),
	)
	span.SetStatus(codes.Ok, "")

	return result, nil
}

// callWebhook makes an HTTP POST request to the webhook endpoint and returns
// the raw response bytes. Parsing is deferred to ParsePIIWebhookResponse so
// that both Arkana Shield and legacy formats can be handled uniformly.
func (h *ExternalPII) callWebhook(ctx context.Context, url string, payload WebhookRequest) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	if len(body) > h.config.MaxBodyBytes {
		return nil, fmt.Errorf("request body size %d exceeds max_body_bytes %d", len(body), h.config.MaxBodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Add authentication (supports both Auth config and APIKey)
	if err := h.addAuthHeader(httpReq); err != nil {
		return nil, fmt.Errorf("add auth header: %w", err)
	}

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, int64(h.config.MaxBodyBytes))
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// Detect truncation: if we read exactly MaxBodyBytes there may be more.
	if len(respBody) == h.config.MaxBodyBytes {
		extra := make([]byte, 1)
		if n, _ := resp.Body.Read(extra); n > 0 {
			return nil, fmt.Errorf("response body exceeds max_body_bytes %d", h.config.MaxBodyBytes)
		}
	}

	return respBody, nil
}

// addAuthHeader adds authentication header to HTTP request based on config.
// Prioritizes APIKey field if set, then falls back to Auth config.
func (h *ExternalPII) addAuthHeader(req *http.Request) error {
	// If APIKey is configured, use it as X-API-Key header
	if h.config.APIKey != "" {
		req.Header.Set("X-API-Key", h.config.APIKey)
		return nil
	}

	// Fall back to Auth config for backwards compatibility
	switch h.config.Auth.Type {
	case "none", "":
		// No authentication
		return nil
	case "bearer":
		if h.config.Auth.Token == "" {
			return fmt.Errorf("bearer auth requires token")
		}
		req.Header.Set("Authorization", "Bearer "+h.config.Auth.Token)
	case "api_key":
		if h.config.Auth.Token == "" {
			return fmt.Errorf("api_key auth requires token")
		}
		header := h.config.Auth.Header
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, h.config.Auth.Token)
	default:
		return fmt.Errorf("unknown auth type: %s", h.config.Auth.Type)
	}
	return nil
}

// mapNormalizedToPreResult converts a NormalizedPIIAction to a PreResult for the
// pre-request hook phase.
func mapNormalizedToPreResult(action NormalizedPIIAction, req providers.ChatRequest) (PreResult, error) {
	switch action.Kind {
	case NormalizedAllow:
		return PreResult{Decision: Allow, Request: req}, nil

	case NormalizedAllowPII:
		return PreResult{
			Decision: AllowPII,
			Request:  req,
			Reason:   "PII service requests routing to PII-safe model",
		}, nil

	case NormalizedReject:
		reason := action.RejectBody
		if reason == "" {
			reason = action.Reason
		}
		return PreResult{
			Decision:   Block,
			Request:    req,
			Reason:     reason,
			StatusCode: action.RejectStatusCode,
		}, nil

	case NormalizedModify:
		if len(action.ModifiedMessages) == 0 {
			return PreResult{}, fmt.Errorf("pre-request modify requires messages")
		}
		modReq := req
		modReq.Messages = action.ModifiedMessages
		reason := action.Reason
		if reason == "" {
			reason = "content modified by webhook"
		}
		return PreResult{Decision: Redact, Request: modReq, Reason: reason}, nil
	}
	return PreResult{}, fmt.Errorf("unknown normalized action kind: %d", action.Kind)
}

// mapNormalizedToPostResult converts a NormalizedPIIAction to a PostResult for the
// post-response hook phase. Reject is logged and ignored (can't block after provider call).
func mapNormalizedToPostResult(action NormalizedPIIAction, resp *providers.ChatResponse, log *slog.Logger) (PostResult, error) {
	switch action.Kind {
	case NormalizedAllow, NormalizedAllowPII:
		return PostResult{Response: resp}, nil

	case NormalizedReject:
		log.Warn("webhook attempted to reject post-response (ignoring)", "reason", action.RejectBody)
		return PostResult{Response: resp}, nil

	case NormalizedModify:
		if len(action.ModifiedChoices) > 0 {
			modifiedResp := *resp
			modifiedResp.Choices = action.ModifiedChoices
			return PostResult{Response: &modifiedResp}, nil
		}
		return PostResult{Response: resp}, fmt.Errorf("post-response modify requires choices")
	}
	return PostResult{Response: resp}, fmt.Errorf("unknown normalized action kind: %d", action.Kind)
}

// handleWebhookError applies fail mode logic and returns appropriate decision.
func (h *ExternalPII) handleWebhookError(ctx context.Context, span interface{ SetAttributes(...attribute.KeyValue) }, err error, req providers.ChatRequest) (PreResult, error) {
	if h.config.FailMode == "fail_closed" {
		span.SetAttributes(attribute.String("webhook.decision", "block"))
		return PreResult{
			Decision: Block,
			Request:  req,
			Reason:   "PII webhook unavailable",
		}, nil
	}

	// fail_open: allow request to proceed
	span.SetAttributes(attribute.String("webhook.decision", "allow"))
	return PreResult{
		Decision: Allow,
		Request:  req,
	}, nil
}

// getActionType returns string representation of webhook action for logging.
func (h *ExternalPII) getActionType(resp *WebhookResponse) string {
	if resp.Action.Allow != nil {
		return "allow"
	}
	if resp.Action.Reject != nil {
		return "reject"
	}
	if resp.Action.Body != nil {
		return "modify"
	}
	if resp.Action.AllowPII != nil {
		return "allow_pii"
	}
	return "unknown"
}

// Webhook request/response data structures

// WebhookRequest is the payload sent to the webhook endpoint.
type WebhookRequest struct {
	Body struct {
		Messages []providers.ChatMessage `json:"messages,omitempty"`
		Choices  []providers.ChatChoice  `json:"choices,omitempty"`
	} `json:"body"`
}

// WebhookResponse is the expected response from the webhook endpoint.
type WebhookResponse struct {
	Action WebhookAction `json:"action"`
}

// WebhookAction represents the action returned by webhook (exactly one field should be set).
type WebhookAction struct {
	Allow    *WebhookAllow    `json:"allow,omitempty"`
	Reject   *WebhookReject   `json:"reject,omitempty"`
	Body     *WebhookBody     `json:"body,omitempty"`
	AllowPII *WebhookAllowPII `json:"allow_pii,omitempty"`
	Reason   string           `json:"reason,omitempty"`
}

// WebhookAllow indicates the request should proceed unchanged.
type WebhookAllow struct{}

// WebhookAllowPII indicates the request should be rerouted to a PII-safe model.
type WebhookAllowPII struct{}

// WebhookReject indicates the request should be blocked.
type WebhookReject struct {
	Response struct {
		Body string `json:"body"`
	} `json:"response"`
}

// WebhookBody contains modified content (messages for pre-request, choices for post-response).
type WebhookBody struct {
	Messages []providers.ChatMessage `json:"messages,omitempty"`
	Choices  []providers.ChatChoice  `json:"choices,omitempty"`
}
