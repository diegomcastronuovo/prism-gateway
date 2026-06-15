package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// claudeCodeAPIKey returns the API key to use for the Claude Code messages endpoint.
// Prefers CLAUDE_API_KEY; falls back to ANTHROPIC_API_KEY when not set.
// Returns "" when neither env var is set.
func claudeCodeAPIKey() string {
	if key := os.Getenv("CLAUDE_API_KEY"); key != "" {
		return key
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

// AnthropicMessages handles POST /v1/claudecode as a raw passthrough to the Anthropic API.
//
// This handler is completely isolated from /v1/chat/completions:
//   - No routing logic, no hooks, no policy evaluation
//   - No format translation — request and response are forwarded as-is
//   - Provider selected via X-Provider header (default: "anthropic")
//   - Full audit row written to anthropic_message_log (SPEC_154)
//
// Intended for clients that speak the Anthropic Messages protocol natively
// (e.g. Claude Code). Auth is enforced by the standard middleware chain.
func (h *Handlers) AnthropicMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestStart := time.Now()

	// Read the raw body — preserved exactly for passthrough.
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB cap
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request")
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body is required", "invalid_request")
		return
	}

	// License gate: claude_code feature must be active.
	if !h.claudeCodeLicenseCheck(w) {
		return
	}

	// Tenant gate: claude_code must be explicitly enabled for this tenant (SPEC_161).
	// Returns the resolved TenantConfig for use in the budget check below.
	tenant, ok := h.claudeCodeTenantCheck(ctx, w)
	if !ok {
		return
	}

	// Claude Code budget enforcement (SPEC_163).
	if !h.claudeCodeBudgetCheck(ctx, w, tenant) {
		return
	}

	// Resolve provider: X-Provider header, default "anthropic".
	providerName := r.Header.Get("X-Provider")
	if providerName == "" {
		providerName = "anthropic"
	}

	if h.registry == nil {
		writeError(w, http.StatusServiceUnavailable, "provider registry not initialized", "internal_error")
		return
	}
	nmp, ok := h.registry.GetNativeMessages(providerName)
	if !ok {
		writeError(w, http.StatusBadRequest,
			"provider does not support native messages passthrough: "+providerName,
			"provider_not_supported")
		return
	}

	// SPEC_158: inject CLAUDE_API_KEY (fallback ANTHROPIC_API_KEY) so the provider
	// uses the dedicated Claude Code key instead of the generic Anthropic key.
	if claudeKey := claudeCodeAPIKey(); claudeKey != "" {
		ctx = providers.WithMessagesAPIKey(ctx, claudeKey)
	}

	// Attribution for audit log — all sourced from auth middleware context.
	apiKeyID, apiKeyName := auth.APIKeyAttributionFromContext(ctx)
	jwtSub := auth.JWTSubFromContext(ctx)
	tenantID := auth.TenantIDFromContext(ctx)
	apiKeyValue := maskedAPIKeyHeader(r)

	// Resolve attribution IDs as plain strings for audit log.
	var apiKeyIDStr *string
	if apiKeyID != nil {
		s := apiKeyID.String()
		apiKeyIDStr = &s
	}

	requestID := uuid.New().String()

	// Parse the request body for audit fields (best-effort; errors ignored).
	parsedReq := parseAnthropicRequest(body)

	// Build prompt_text from system + messages.
	promptText := buildPromptText(parsedReq)

	streaming := isStreamingMessagesBody(body)

	h.log.InfoContext(ctx, "anthropic messages request received",
		"request_id", requestID,
		"tenant_id", tenantID,
		"provider", providerName,
		"streaming", streaming,
		"model_requested", derefStr(parsedReq.modelRequested),
		"body_bytes", len(body),
	)

	// Detect streaming from the raw body to select the right code path.
	if streaming {
		h.handleAnthropicMessagesStream(w, r, ctx, nmp, body, requestID, providerName,
			requestStart, tenantID, apiKeyIDStr, apiKeyName, apiKeyValue, jwtSub, promptText)
		return
	}

	// ── Non-streaming passthrough ─────────────────────────────────────────────
	respBody, usage, err := nmp.SendMessagesRaw(ctx, json.RawMessage(body))
	latencyMs := int(time.Since(requestStart).Milliseconds())

	if err != nil {
		statusCode := http.StatusBadGateway
		var upErr *providers.UpstreamError
		if errors.As(err, &upErr) {
			statusCode = upErr.StatusCode
		}
		h.log.ErrorContext(ctx, "anthropic messages upstream error",
			"request_id", requestID,
			"tenant_id", tenantID,
			"status_code", statusCode,
			"latency_ms", latencyMs,
			"error", err.Error(),
		)
		errType := "upstream_error"
		errMsg := err.Error()
		sc := statusCode
		h.logAnthropicMessage(ctx, storage.AnthropicMessageLog{
			TenantID:       tenantID,
			RequestID:      requestID,
			APIKeyID:       apiKeyIDStr,
			APIKeyValue:    apiKeyValue,
			APIKeyName:     apiKeyName,
			JwtSub:         jwtSub,
			Provider:       providerName,
			Endpoint:       "/v1/claudecode",
			HTTPMethod:     "POST",
			ModelRequested: parsedReq.modelRequested,
			StatusCode:     &sc,
			Success:        false,
			PromptText:     promptText,
			RawRequestJSON: body,
			ErrorType:      &errType,
			ErrorMessage:   &errMsg,
			LatencyMs:      &latencyMs,
		})
		http.Error(w, err.Error(), statusCode)
		return
	}

	// Parse the response for audit fields.
	parsedResp := parseAnthropicResponse(respBody)
	responseText := buildResponseText(parsedResp)
	sc200 := http.StatusOK

	h.log.InfoContext(ctx, "anthropic messages upstream ok",
		"request_id", requestID,
		"tenant_id", tenantID,
		"model_used", derefStr(parsedResp.model),
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
		"latency_ms", latencyMs,
		"response_bytes", len(respBody),
	)

	h.logAnthropicMessage(ctx, storage.AnthropicMessageLog{
		TenantID:           tenantID,
		RequestID:          requestID,
		APIKeyID:           apiKeyIDStr,
		APIKeyValue:        apiKeyValue,
		APIKeyName:         apiKeyName,
		JwtSub:             jwtSub,
		Provider:           providerName,
		Endpoint:           "/v1/claudecode",
		HTTPMethod:         "POST",
		ModelRequested:     parsedReq.modelRequested,
		ModelUsed:          parsedResp.model,
		AnthropicMessageID: parsedResp.id,
		StatusCode:         &sc200,
		Success:            true,
		InputTokens:        tokenPtr(usage.InputTokens),
		OutputTokens:       tokenPtr(usage.OutputTokens),
		TotalTokens:        tokenPtr(usage.InputTokens + usage.OutputTokens),
		Cost:               h.computeAnthropicCost(ctx, parsedResp.model, parsedReq.modelRequested, usage.InputTokens, usage.OutputTokens),
		StopReason:         parsedResp.stopReason,
		StopSequence:       parsedResp.stopSequence,
		PromptText:         promptText,
		ResponseText:       responseText,
		RawRequestJSON:     body,
		RawResponseJSON:    respBody,
		LatencyMs:          &latencyMs,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody) //nolint:errcheck
}

// handleAnthropicMessagesStream performs a raw SSE passthrough for streaming requests.
// Each upstream SSE line is flushed to the client immediately.
// The stream is also intercepted to reconstruct audit fields for anthropic_message_log.
func (h *Handlers) handleAnthropicMessagesStream(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	nmp providers.NativeMessagesProvider,
	body []byte,
	requestID, providerName string,
	requestStart time.Time,
	tenantID string,
	apiKeyIDStr *string, apiKeyName *string, apiKeyValue *string, jwtSub *string,
	promptText *string,
) {
	parsedReq := parseAnthropicRequest(body)

	upstream, err := nmp.SendMessagesRawStream(ctx, json.RawMessage(body))
	latencyMs := int(time.Since(requestStart).Milliseconds())

	if err != nil {
		statusCode := http.StatusBadGateway
		var upErr *providers.UpstreamError
		if errors.As(err, &upErr) {
			statusCode = upErr.StatusCode
		}
		h.log.ErrorContext(ctx, "anthropic messages stream upstream error",
			"request_id", requestID,
			"tenant_id", tenantID,
			"status_code", statusCode,
			"latency_ms", latencyMs,
			"error", err.Error(),
		)
		errType := "upstream_error"
		errMsg := err.Error()
		sc := statusCode
		h.logAnthropicMessage(ctx, storage.AnthropicMessageLog{
			TenantID:       tenantID,
			RequestID:      requestID,
			APIKeyID:       apiKeyIDStr,
			APIKeyValue:    apiKeyValue,
			APIKeyName:     apiKeyName,
			JwtSub:         jwtSub,
			Provider:       providerName,
			Endpoint:       "/v1/claudecode",
			HTTPMethod:     "POST",
			ModelRequested: parsedReq.modelRequested,
			StatusCode:     &sc,
			Success:        false,
			PromptText:     promptText,
			RawRequestJSON: body,
			ErrorType:      &errType,
			ErrorMessage:   &errMsg,
			LatencyMs:      &latencyMs,
		})
		http.Error(w, err.Error(), statusCode)
		return
	}
	defer upstream.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	// Intercept SSE events while piping them to the client.
	// We accumulate enough data to build the audit row after the stream ends.
	var (
		textBuilder   strings.Builder
		messageID     string
		modelUsed     string
		inputTokens   int
		outputTokens  int
		stopReason    string
		stopSequence  string
		lastEventType string
	)

	scanner := bufio.NewScanner(upstream.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		// Copy line before writing (scanner reuses buffer).
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)

		w.Write(append(lineCopy, '\n')) //nolint:errcheck
		if canFlush {
			flusher.Flush()
		}

		// Parse for audit interception.
		lineStr := string(lineCopy)
		if strings.HasPrefix(lineStr, "event: ") {
			lastEventType = strings.TrimSpace(strings.TrimPrefix(lineStr, "event: "))
			continue
		}
		if !strings.HasPrefix(lineStr, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(lineStr, "data: ")
		interceptAnthropicSSEEvent(payload, lastEventType,
			&messageID, &modelUsed, &inputTokens, &outputTokens,
			&stopReason, &stopSequence, &textBuilder)
	}

	finalLatencyMs := int(time.Since(requestStart).Milliseconds())
	sc200 := http.StatusOK

	var msgIDPtr, modelPtr, stopReasonPtr, stopSeqPtr *string
	if messageID != "" {
		msgIDPtr = &messageID
	}
	if modelUsed != "" {
		modelPtr = &modelUsed
	}
	if stopReason != "" {
		stopReasonPtr = &stopReason
	}
	if stopSequence != "" {
		stopSeqPtr = &stopSequence
	}

	responseText := textBuilder.String()
	var responseTextPtr *string
	if responseText != "" {
		responseTextPtr = &responseText
	}

	h.log.InfoContext(ctx, "anthropic messages stream completed",
		"request_id", requestID,
		"tenant_id", tenantID,
		"model_used", derefStr(modelPtr),
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
		"stop_reason", stopReason,
		"latency_ms", finalLatencyMs,
	)

	h.logAnthropicMessage(ctx, storage.AnthropicMessageLog{
		TenantID:           tenantID,
		RequestID:          requestID,
		APIKeyID:           apiKeyIDStr,
		APIKeyValue:        apiKeyValue,
		APIKeyName:         apiKeyName,
		JwtSub:             jwtSub,
		Provider:           providerName,
		Endpoint:           "/v1/claudecode",
		HTTPMethod:         "POST",
		ModelRequested:     parsedReq.modelRequested,
		ModelUsed:          modelPtr,
		AnthropicMessageID: msgIDPtr,
		StatusCode:         &sc200,
		Success:            true,
		InputTokens:        tokenPtr(inputTokens),
		OutputTokens:       tokenPtr(outputTokens),
		TotalTokens:        tokenPtr(inputTokens + outputTokens),
		Cost:               h.computeAnthropicCost(ctx, modelPtr, parsedReq.modelRequested, inputTokens, outputTokens),
		StopReason:         stopReasonPtr,
		StopSequence:       stopSeqPtr,
		PromptText:         promptText,
		ResponseText:       responseTextPtr,
		RawRequestJSON: body,
		// rawSSE contains raw SSE text (not valid JSON) — store nil; structured
		// fields (model, tokens, stop_reason, response_text) are captured above.
		RawResponseJSON: nil,
		LatencyMs:       &finalLatencyMs,
	})
}

// logAnthropicMessage writes the audit row asynchronously.
// Logs DB errors so they are visible in container logs without failing the request.
func (h *Handlers) logAnthropicMessage(ctx context.Context, row storage.AnthropicMessageLog) {
	h.asyncWg.Add(1)
	go func() {
		defer h.asyncWg.Done()
		row.CreatedAt = time.Now().UTC()
		if err := h.store.InsertAnthropicMessageLog(context.Background(), row); err != nil {
			h.log.ErrorContext(ctx, "anthropic_message_log insert failed",
				"request_id", row.RequestID,
				"tenant_id", row.TenantID,
				"error", err.Error(),
			)
		}
	}()
}

// logNativeMessagesRequest writes a request_log row for a /v1/claudecode call.
// Kept for backward-compatible metrics; the full audit row is written by logAnthropicMessage.
func (h *Handlers) logNativeMessagesRequest(
	ctx context.Context,
	requestID, providerName, status string,
	latencyMs, inputTokens, outputTokens int,
	apiKeyID *uuid.UUID, apiKeyName *string, jwtSub *string,
) {
	h.asyncWg.Add(1)
	go func() {
		defer h.asyncWg.Done()
		rl := storage.RequestLog{
			ID:         uuid.New(),
			RequestID:  requestID,
			Attempt:    1,
			TenantID:   auth.TenantIDFromContext(ctx),
			Model:      "", // no single model — full payload is forwarded
			Provider:   providerName,
			Strategy:   "native_messages",
			Status:     status,
			LatencyMs:  latencyMs,
			APIKeyID:   apiKeyID,
			APIKeyName: apiKeyName,
			JWTSub:     jwtSub,
		}
		_ = h.store.LogRequest(context.Background(), rl)

		if inputTokens > 0 || outputTokens > 0 {
			_ = h.store.SaveUsage(context.Background(), storage.UsageRecord{
				ID:               uuid.New(),
				TenantID:         auth.TenantIDFromContext(ctx),
				Model:            "",
				Provider:         providerName,
				PromptTokens:     inputTokens,
				CompletionTokens: outputTokens,
				TotalTokens:      inputTokens + outputTokens,
				CostUSD:          0,
				RequestID:        requestID,
				APIKeyID:         apiKeyID,
				APIKeyName:       apiKeyName,
				JWTSub:           jwtSub,
			})
		}
	}()
}

// isStreamingMessagesBody returns true when the raw JSON body contains "stream": true.
func isStreamingMessagesBody(body []byte) bool {
	var peek struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &peek)
	return peek.Stream
}

// ── Audit helpers ─────────────────────────────────────────────────────────────

// anthropicReqParsed holds fields extracted from the request body for audit.
type anthropicReqParsed struct {
	modelRequested *string
	system         string
	messages       []anthropicAuditMessage
}

type anthropicAuditMessage struct {
	Role    string
	Content json.RawMessage
}

// parseAnthropicRequest extracts audit-relevant fields from the raw request JSON.
func parseAnthropicRequest(body []byte) anthropicReqParsed {
	var raw struct {
		Model    string                   `json:"model"`
		System   string                   `json:"system"`
		Messages []anthropicAuditMessage  `json:"messages"`
	}
	_ = json.Unmarshal(body, &raw)

	var out anthropicReqParsed
	if raw.Model != "" {
		out.modelRequested = &raw.Model
	}
	out.system = raw.System
	out.messages = raw.Messages
	return out
}

// buildPromptText constructs the human-readable audit representation of the prompt.
// Format: [SYSTEM]\n...\n\n[USER]\n...\n\n[ASSISTANT]\n...
func buildPromptText(req anthropicReqParsed) *string {
	var b strings.Builder
	if req.system != "" {
		b.WriteString("[SYSTEM]\n")
		b.WriteString(req.system)
	}
	for _, msg := range req.messages {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[")
		b.WriteString(strings.ToUpper(msg.Role))
		b.WriteString("]\n")
		b.WriteString(extractTextFromContent(msg.Content))
	}
	if b.Len() == 0 {
		return nil
	}
	s := b.String()
	return &s
}

// extractTextFromContent handles both string content and block arrays.
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try as plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try as content blocks array.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if b.Text != "" {
					parts = append(parts, b.Text)
				}
			default:
				parts = append(parts, "["+b.Type+"]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// anthropicRespParsed holds fields extracted from the non-streaming response.
type anthropicRespParsed struct {
	id           *string
	model        *string
	stopReason   *string
	stopSequence *string
	content      []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
}

// parseAnthropicResponse extracts audit-relevant fields from the raw response JSON.
func parseAnthropicResponse(body []byte) anthropicRespParsed {
	var raw struct {
		ID           string `json:"id"`
		Model        string `json:"model"`
		StopReason   string `json:"stop_reason"`
		StopSequence string `json:"stop_sequence"`
		Content      []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	_ = json.Unmarshal(body, &raw)

	var out anthropicRespParsed
	if raw.ID != "" {
		out.id = &raw.ID
	}
	if raw.Model != "" {
		out.model = &raw.Model
	}
	if raw.StopReason != "" {
		out.stopReason = &raw.StopReason
	}
	if raw.StopSequence != "" {
		out.stopSequence = &raw.StopSequence
	}
	out.content = raw.Content
	return out
}

// buildResponseText concatenates all text content blocks from the response.
func buildResponseText(resp anthropicRespParsed) *string {
	var b strings.Builder
	for _, block := range resp.content {
		switch block.Type {
		case "text":
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(block.Text)
		default:
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString("[" + block.Type + "]")
		}
	}
	if b.Len() == 0 {
		return nil
	}
	s := b.String()
	return &s
}

// interceptAnthropicSSEEvent parses one SSE data payload and updates the
// streaming accumulator variables for the audit row.
func interceptAnthropicSSEEvent(
	payload, lastEventType string,
	messageID, modelUsed *string,
	inputTokens, outputTokens *int,
	stopReason, stopSequence *string,
	textBuilder *strings.Builder,
) {
	var ev map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return
	}

	evType := lastEventType
	if t, ok := ev["type"]; ok {
		_ = json.Unmarshal(t, &evType)
	}

	switch evType {
	case "message_start":
		// {"type":"message_start","message":{"id":"...","model":"...","usage":{"input_tokens":N}}}
		var ms struct {
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &ms); err == nil {
			if ms.Message.ID != "" {
				*messageID = ms.Message.ID
			}
			if ms.Message.Model != "" {
				*modelUsed = ms.Message.Model
			}
			if ms.Message.Usage.InputTokens > 0 {
				*inputTokens = ms.Message.Usage.InputTokens
			}
		}

	case "content_block_delta":
		// {"type":"content_block_delta","delta":{"type":"text_delta","text":"..."}}
		var cbd struct {
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &cbd); err == nil {
			if cbd.Delta.Type == "text_delta" && cbd.Delta.Text != "" {
				textBuilder.WriteString(cbd.Delta.Text)
			}
		}

	case "message_delta":
		// {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":N}}
		var md struct {
			Delta struct {
				StopReason   string  `json:"stop_reason"`
				StopSequence *string `json:"stop_sequence"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &md); err == nil {
			if md.Delta.StopReason != "" {
				*stopReason = md.Delta.StopReason
			}
			if md.Delta.StopSequence != nil && *md.Delta.StopSequence != "" {
				*stopSequence = *md.Delta.StopSequence
			}
			if md.Usage.OutputTokens > 0 {
				*outputTokens = md.Usage.OutputTokens
			}
		}
	}
}

// maskedAPIKeyHeader returns a masked version of the X-API-Key header value.
// Keeps the prefix up to the first dash-separated segment plus "****" and the last 4 chars.
// Returns nil when the header is absent (e.g. JWT auth).
//
// Example: "sk-ant-api03-AbCdEfGhIjKlMnOp" → "sk-ant-****MnOp"
// Security note: we never store the full key. This is sufficient for audit identification.
func maskedAPIKeyHeader(r *http.Request) *string {
	raw := r.Header.Get("X-API-Key")
	if raw == "" {
		return nil
	}
	masked := maskAPIKey(raw)
	return &masked
}

// maskAPIKey masks the middle portion of an API key string.
func maskAPIKey(key string) string {
	const suffixLen = 4
	if len(key) <= suffixLen+8 {
		// Key too short to mask meaningfully — return fully redacted.
		return "****"
	}
	// Find a natural prefix boundary (first two dash-separated segments).
	parts := strings.SplitN(key, "-", 4)
	prefix := ""
	if len(parts) >= 3 {
		prefix = parts[0] + "-" + parts[1] + "-"
	} else {
		prefix = key[:4]
	}
	suffix := key[len(key)-suffixLen:]
	return prefix + "****" + suffix
}

// tokenPtr returns nil when v == 0, otherwise a pointer to v.
// Zero token counts are stored as NULL rather than 0.
func tokenPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// derefStr dereferences a *string for logging; returns "" if nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// computeAnthropicCost calculates the monetary cost of a request.
//
// Resolution order (SPEC_162):
//  1. Look up model in the standard catalog (GlobalConfig.Models) — pricing is per-1M tokens.
//  2. If not found, derive the model family ("haiku"/"sonnet"/"opus") and look up
//     GlobalConfig.ClaudeCodePricing[family] — pricing is USD per 1,000,000 tokens.
//
// Returns nil when pricing is unavailable or tokens are both zero,
// so the DB column stores NULL rather than a misleading 0.
func (h *Handlers) computeAnthropicCost(ctx context.Context, modelUsed, modelRequested *string, inputTokens, outputTokens int) *float64 {
	if inputTokens == 0 && outputTokens == 0 {
		return nil
	}
	name := derefStr(modelUsed)
	if name == "" {
		name = derefStr(modelRequested)
	}
	if name == "" {
		return nil
	}

	// Path 1: standard model catalog (per-million pricing).
	// Only used when the catalog entry has non-zero pricing — otherwise fall through
	// to Path 2 so that ClaudeCodePricing can supply the price (e.g. Sonnet is in
	// the catalog with price=0 but has an explicit ClaudeCode family price).
	mc := h.resolveModelByName(ctx, name)
	if mc != nil && (mc.Pricing.PromptPer1M != 0 || mc.Pricing.CompletionPer1M != 0) {
		p := mc.Pricing
		cost := (float64(inputTokens)/1_000_000.0)*p.PromptPer1M +
			(float64(outputTokens)/1_000_000.0)*p.CompletionPer1M
		return &cost
	}

	// Path 2: Claude Code family pricing (per-million tokens).
	family := claudeCodeModelFamily(name)
	if family == "" {
		return nil
	}
	gc := h.resolveGlobalConfig(ctx)
	if gc == nil || gc.ClaudeCodePricing == nil {
		return nil
	}
	fp, ok := gc.ClaudeCodePricing[family]
	if !ok || (fp.Input == 0 && fp.Output == 0) {
		return nil
	}
	cost := (float64(inputTokens)/1_000_000.0)*fp.Input + (float64(outputTokens)/1_000_000.0)*fp.Output
	return &cost
}

// claudeCodeModelFamily derives the model family from a model name string.
// Returns "haiku", "sonnet", or "opus" via case-insensitive substring match.
// Returns "" when no family can be resolved.
func claudeCodeModelFamily(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "haiku"):
		return "haiku"
	case strings.Contains(lower, "sonnet"):
		return "sonnet"
	case strings.Contains(lower, "opus"):
		return "opus"
	default:
		return ""
	}
}

// claudeCodeTenantCheck returns the resolved TenantConfig and true when the tenant
// has ClaudeCode.Enabled == true. Writes 403 and returns (nil, false) otherwise.
func (h *Handlers) claudeCodeTenantCheck(ctx context.Context, w http.ResponseWriter) (*config.TenantConfig, bool) {
	tenantID := auth.TenantIDFromContext(ctx)
	tenant, err := h.cfg.ResolveTenantConfig(ctx, tenantID, h.tenantCache, h.store)
	if err != nil || tenant == nil || tenant.ClaudeCode == nil || !tenant.ClaudeCode.Enabled {
		writeError(w, http.StatusForbidden, "Claude Code not enabled for tenant", "permission_error")
		return nil, false
	}
	return tenant, true
}

// claudeCodeBudgetCheck enforces the Claude Code monthly budget (SPEC_163).
// It reuses the existing BudgetEnforcement thresholds/mode from the tenant config,
// but evaluates spend against claude_code.monthly_budget using anthropic_message_log.
// Returns false (and writes the error response) when the request should be blocked.
// Degrade mode is treated as report_only since /v1/claudecode has no route-group concept.
func (h *Handlers) claudeCodeBudgetCheck(ctx context.Context, w http.ResponseWriter, tenant *config.TenantConfig) bool {
	cc := tenant.ClaudeCode
	budget := cc.MonthlyBudget
	if budget <= 0 {
		return true // no Claude Code budget configured
	}

	enf := &tenant.BudgetEnforcement
	if !enf.Enabled {
		return true // budget enforcement disabled for this tenant
	}

	// Resolve timezone; default to America/Buenos_Aires.
	tz := cc.Timezone
	if tz == "" {
		tz = "America/Buenos_Aires"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		h.log.WarnContext(ctx, "claude code budget: invalid timezone, falling back to America/Buenos_Aires",
			"tenant", tenant.ID, "timezone", tz, "error", err)
		loc, _ = time.LoadLocation("America/Buenos_Aires")
	}

	now := time.Now().In(loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)

	spend, err := h.store.GetClaudeCodeMonthlySpend(ctx, tenant.ID, monthStart.UTC(), monthEnd.UTC())
	if err != nil {
		h.log.ErrorContext(ctx, "claude code budget: spend query failed, allowing request",
			"tenant", tenant.ID, "error", err)
		return true // fail open
	}

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
		blockStatus = http.StatusPaymentRequired
	}

	pct := spend / budget

	if pct >= hardPct {
		h.log.WarnContext(ctx, "claude code budget: hard threshold reached",
			"tenant", tenant.ID, "spend", spend, "budget", budget, "mode", enf.Mode)
		switch enf.Mode {
		case "block":
			msg := fmt.Sprintf("monthly Claude Code budget exceeded for tenant '%s'", tenant.ID)
			writeError(w, blockStatus, msg, "budget_exceeded")
			return false
		case "degrade":
			// degrade has no meaning for /v1/claudecode (no model routing) — treat as report_only.
			h.log.WarnContext(ctx, "claude code budget: degrade mode not applicable to /v1/claudecode, continuing",
				"tenant", tenant.ID)
		// report_only: log and continue
		}
	} else if pct >= warnPct {
		h.log.WarnContext(ctx, "claude code budget: warn threshold reached",
			"tenant", tenant.ID, "spend", spend, "budget", budget, "pct", pct)
	}

	return true
}

// claudeCodeLicenseCheck always returns true in the community edition (no license gating).
func (h *Handlers) claudeCodeLicenseCheck(_ http.ResponseWriter) bool {
	return true
}
