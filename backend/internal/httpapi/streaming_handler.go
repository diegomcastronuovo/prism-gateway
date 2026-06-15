package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type streamChunkPayload struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type openAICompatSSEChunk struct {
	ID      string `json:"id,omitempty"`
	Object  string `json:"object,omitempty"`
	Created int64  `json:"created,omitempty"`
	Model   string `json:"model,omitempty"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content   string          `json:"content,omitempty"`
			ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func estimateStreamingTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / 4.0))
}

func streamingInputText(messages []providers.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range messages {
		if m.Content != "" {
			b.WriteString(m.Content)
			b.WriteByte('\n')
		}
		for _, block := range m.ContentBlocks {
			if block.Type == "text" && block.Text != "" {
				b.WriteString(block.Text)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func appendStreamDeltaContent(out *strings.Builder, data []byte) {
	var payload streamChunkPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	for _, c := range payload.Choices {
		if c.Delta.Content != "" {
			out.WriteString(c.Delta.Content)
		}
	}
}

func streamEventToSSEPayload(event providers.StreamEvent) ([]byte, bool) {
	switch event.Type {
	case "done":
		return nil, true
	case "delta":
		var c openAICompatSSEChunk
		c.Object = "chat.completion.chunk"
		c.Choices = []struct {
			Index int `json:"index"`
			Delta struct {
				Content   string          `json:"content,omitempty"`
				ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}{{
			Index: 0,
			Delta: struct {
				Content   string          `json:"content,omitempty"`
				ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
			}{Content: event.Content, ToolCalls: event.ToolCallsDelta},
			FinishReason: event.FinishReason,
		}}
		b, _ := json.Marshal(c)
		return b, false
	default:
		return nil, false
	}
}

// classifyStreamError derives a stream_error_type string from the context error
// and/or the upstream error. The returned value is used in metadata and logs.
//
//   - "timeout"           — request/context deadline exceeded
//   - "client_disconnect" — client cancelled the request
//   - "upstream_disconnect" — remote connection closed unexpectedly (EOF/reset)
//   - "provider_error"   — other upstream error
func classifyStreamError(ctxErr, streamErr error) string {
	if errors.Is(ctxErr, context.DeadlineExceeded) || errors.Is(streamErr, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(ctxErr, context.Canceled) || errors.Is(streamErr, context.Canceled) {
		return "client_disconnect"
	}
	if streamErr != nil {
		msg := strings.ToLower(streamErr.Error())
		if strings.Contains(msg, "eof") ||
			strings.Contains(msg, "connection reset") ||
			strings.Contains(msg, "broken pipe") ||
			strings.Contains(msg, "use of closed network connection") {
			return "upstream_disconnect"
		}
		return "provider_error"
	}
	// No error info available — treat as client disconnect.
	return "client_disconnect"
}

// writeSSEStream forwards a streaming response from the upstream provider to
// the client as OpenAI-compatible Server-Sent Events.
// TODO(SPEC-175-phase5): reduce the 23-parameter signature by grouping logging
// params into a value type or by passing OrchestratorInput/SSEResponseSink.
//
// Stream telemetry (streaming_enabled, first_token_latency_ms, stream_duration_ms,
// chunk_count, stream_completed, stream_error_type) is persisted in request_log.metadata.
//
// The method is intentionally synchronous: it blocks until the stream is
// complete, the client disconnects, or the upstream closes the connection.
func (h *Handlers) writeSSEStream(
	ctx context.Context,
	w http.ResponseWriter,
	streamResp *providers.StreamResponse,
	provReq providers.ChatRequest,
	requestID string,
	tenant *config.TenantConfig,
	modelName string,
	modelCfg *config.ModelConfig,
	requestStart time.Time,
	decisionReason string,
	requestSnapshotJSON json.RawMessage,
	baseMetadataJSON json.RawMessage,
	attempt int,
	apiKeyID *uuid.UUID,
	apiKeyName *string,
	jwtSub *string,
	routerPreMSVal int,
	budgetReservationID uuid.UUID,
	// conversation logging params
	promptMessages     []string,
	piiRequestDecision  *string,
	piiResponseDecision *string,
	customerID         *string,
	// extended context headers (migration 051)
	ctxChannel         *string,
	ctxInteractionType *string,
	ctxAgentID         *string,
	ctxDepartment      *string,
	ctxTicketID        *string,
	ctxCustomerSegment *string,
	ctxLanguage        *string,
	ctxIntent          *string,
	ctxExperimentID    *string,
	ctxAutonomyLevel   *string,
	ctxPolicyID        *string,
	ctxRiskLevel       *string,
	ctxRevenueImpact   *string,
	ctxCurrency        *string,
) {
	// SSE headers must be written before any data.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Selected-Model", modelName)
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	streamStart := time.Now()
	var firstTokenLatencyMs *int
	chunkCount := 0
	var streamOutput strings.Builder
	inputTokens := estimateStreamingTokens(streamingInputText(provReq.Messages))
	// providerUsage holds usage reported by the provider (e.g. Bedrock stream metadata).
	// When non-nil it takes precedence over character-based estimation in logStreamRequest.
	var providerUsage *providers.Usage

	for {
		select {
		case <-ctx.Done():
			ctxErr := ctx.Err()
			errType := classifyStreamError(ctxErr, nil)
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				h.log.WarnContext(ctx, "stream timed out",
					slog.String("model", modelName),
					slog.String("tenant", tenant.ID),
					slog.Int("chunks_sent", chunkCount),
					slog.String("stream_error_type", errType),
				)
			} else {
				h.log.InfoContext(ctx, "stream cancelled by client",
					slog.String("model", modelName),
					slog.String("tenant", tenant.ID),
					slog.Int("chunks_sent", chunkCount),
					slog.String("stream_error_type", errType),
				)
			}
			h.logStreamRequest(ctx, requestID, tenant, modelName, modelCfg, requestStart,
				streamStart, firstTokenLatencyMs, chunkCount, false, ctxErr,
				decisionReason, requestSnapshotJSON, baseMetadataJSON,
				attempt, routerPreMSVal, apiKeyID, apiKeyName, jwtSub, inputTokens, streamOutput.String(), nil,
				customerID, ctxChannel, ctxInteractionType, ctxAgentID, ctxDepartment, ctxTicketID,
				ctxCustomerSegment, ctxLanguage, ctxIntent, ctxExperimentID, ctxAutonomyLevel,
				ctxPolicyID, ctxRiskLevel, ctxRevenueImpact, ctxCurrency)
			return

		case event, ok := <-streamResp.Events:
			if !ok {
				// Channel closed without [DONE] — treat as completed.
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if canFlush {
					flusher.Flush()
				}
				h.logStreamRequest(ctx, requestID, tenant, modelName, modelCfg, requestStart,
					streamStart, firstTokenLatencyMs, chunkCount, true, nil,
					decisionReason, requestSnapshotJSON, baseMetadataJSON,
					attempt, routerPreMSVal, apiKeyID, apiKeyName, jwtSub, inputTokens, streamOutput.String(), nil,
					customerID, ctxChannel, ctxInteractionType, ctxAgentID, ctxDepartment, ctxTicketID,
					ctxCustomerSegment, ctxLanguage, ctxIntent, ctxExperimentID, ctxAutonomyLevel,
					ctxPolicyID, ctxRiskLevel, ctxRevenueImpact, ctxCurrency)
				h.releaseReservationAsync(ctx, budgetReservationID)
				h.logConversationAsync(ctx, tenant, requestID, promptMessages,
					[]ChatChoice{{Index: 0, Message: newTextMessage("assistant", streamOutput.String()), FinishReason: "stop"}},
					piiRequestDecision, piiResponseDecision, jwtSub, customerID)
				return
			}

			if event.Type == "error" && event.Error != nil {
				errType := classifyStreamError(nil, event.Error)
				h.log.ErrorContext(ctx, "stream upstream error",
					slog.String("model", modelName),
					slog.String("tenant", tenant.ID),
					slog.Int("chunks_sent", chunkCount),
					slog.String("stream_error_type", errType),
					slog.String("error", event.Error.Error()),
				)
				h.logStreamRequest(ctx, requestID, tenant, modelName, modelCfg, requestStart,
					streamStart, firstTokenLatencyMs, chunkCount, false, event.Error,
					decisionReason, requestSnapshotJSON, baseMetadataJSON,
					attempt, routerPreMSVal, apiKeyID, apiKeyName, jwtSub, inputTokens, streamOutput.String(), nil,
					customerID, ctxChannel, ctxInteractionType, ctxAgentID, ctxDepartment, ctxTicketID,
					ctxCustomerSegment, ctxLanguage, ctxIntent, ctxExperimentID, ctxAutonomyLevel,
					ctxPolicyID, ctxRiskLevel, ctxRevenueImpact, ctxCurrency)
				return
			}

			// Capture provider-reported usage from done events before converting to SSE payload.
			if event.Type == "done" && event.Usage != nil {
				providerUsage = event.Usage
			}

			payload, done := streamEventToSSEPayload(event)
			if done {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if canFlush {
					flusher.Flush()
				}
				h.logStreamRequest(ctx, requestID, tenant, modelName, modelCfg, requestStart,
					streamStart, firstTokenLatencyMs, chunkCount, true, nil,
					decisionReason, requestSnapshotJSON, baseMetadataJSON,
					attempt, routerPreMSVal, apiKeyID, apiKeyName, jwtSub, inputTokens, streamOutput.String(), providerUsage,
					customerID, ctxChannel, ctxInteractionType, ctxAgentID, ctxDepartment, ctxTicketID,
					ctxCustomerSegment, ctxLanguage, ctxIntent, ctxExperimentID, ctxAutonomyLevel,
					ctxPolicyID, ctxRiskLevel, ctxRevenueImpact, ctxCurrency)
				h.releaseReservationAsync(ctx, budgetReservationID)
				h.logConversationAsync(ctx, tenant, requestID, promptMessages,
					[]ChatChoice{{Index: 0, Message: newTextMessage("assistant", streamOutput.String()), FinishReason: "stop"}},
					piiRequestDecision, piiResponseDecision, jwtSub, customerID)
				return
			}

			if len(payload) == 0 {
				continue
			}

			// Regular data chunk.
			if firstTokenLatencyMs == nil {
				ms := int(time.Since(streamStart).Milliseconds())
				firstTokenLatencyMs = &ms
			}
			chunkCount++
			if streamOutput.Len() < 4*1024 {
				appendStreamDeltaContent(&streamOutput, payload)
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

// logStreamRequest persists a request_log row for a streamed request.
// It stores stream telemetry in metadata:
//   - streaming_enabled, first_token_latency_ms, stream_duration_ms,
//     chunk_count, stream_completed, stream_error_type (when incomplete)
func (h *Handlers) logStreamRequest(
	ctx context.Context,
	requestID string,
	tenant *config.TenantConfig,
	modelName string,
	modelCfg *config.ModelConfig,
	requestStart time.Time,
	streamStart time.Time,
	firstTokenLatencyMs *int,
	chunkCount int,
	streamCompleted bool,
	streamErr error,
	decisionReason string,
	requestSnapshotJSON json.RawMessage,
	baseMetadataJSON json.RawMessage,
	attempt int,
	routerPreMSVal int,
	apiKeyID *uuid.UUID,
	apiKeyName *string,
	jwtSub *string,
	inputTokens int,
	streamOutput string,
	// providerUsage holds exact token counts reported by the provider (e.g. Bedrock metadata).
	// When non-nil, it takes precedence over character-based estimation.
	providerUsage *providers.Usage,
	// context headers (migration 050/051)
	ctxCustomerID      *string,
	ctxChannel         *string,
	ctxInteractionType *string,
	ctxAgentID         *string,
	ctxDepartment      *string,
	ctxTicketID        *string,
	ctxCustomerSegment *string,
	ctxLanguage        *string,
	ctxIntent          *string,
	ctxExperimentID    *string,
	ctxAutonomyLevel   *string,
	ctxPolicyID        *string,
	ctxRiskLevel       *string,
	ctxRevenueImpact   *string,
	ctxCurrency        *string,
) {
	streamDurationMs := int(time.Since(streamStart).Milliseconds())
	totalLatencyMs := int(time.Since(requestStart).Milliseconds())

	// Build stream metadata.
	streamMeta := map[string]interface{}{
		"streaming_enabled":  true,
		"stream_duration_ms": streamDurationMs,
		"chunk_count":        chunkCount,
		"stream_completed":   streamCompleted,
	}
	if firstTokenLatencyMs != nil {
		streamMeta["first_token_latency_ms"] = *firstTokenLatencyMs
	}

	// Determine status and error type.
	status := "ok"
	errStr := ""
	if !streamCompleted {
		errType := classifyStreamError(ctx.Err(), streamErr)
		streamMeta["stream_error_type"] = errType

		switch errType {
		case "client_disconnect":
			status = "cancelled"
		default:
			// timeout, upstream_disconnect, provider_error
			status = "error"
		}

		if streamErr != nil {
			errStr = streamErr.Error()
		} else if ctx.Err() != nil {
			errStr = ctx.Err().Error()
		}
	}

	// Merge stream metadata with any pre-existing metadata (e.g. user-provided tags).
	metaMap := make(map[string]interface{})
	if baseMetadataJSON != nil {
		_ = json.Unmarshal(baseMetadataJSON, &metaMap)
	}
	for k, v := range streamMeta {
		metaMap[k] = v
	}
	metadataJSON, marshalErr := json.Marshal(metaMap)
	if marshalErr != nil {
		h.log.WarnContext(ctx, "logStreamRequest: failed to marshal stream metadata",
			slog.String("error", marshalErr.Error()),
		)
		metadataJSON = nil
	}

	// Use context.Background() — not the request ctx — because the request context
	// may already be cancelled (client disconnect, timeout) by the time we reach
	// this cleanup write. ExecContext on a cancelled context is a no-op, which
	// would silently drop the log row.
	h.logRequestAsync(context.Background(), storage.RequestLog{
		ID:               uuid.New(),
		RequestID:        requestID,
		Attempt:          attempt + 1,
		TenantID:         tenant.ID,
		Model:            modelName,
		Provider:         modelCfg.Provider,
		Strategy:         tenant.Routing.Strategy,
		Status:           status,
		LatencyMs:        totalLatencyMs,
		Error:            errStr,
		FallbackUsed:     attempt > 0,
		DecisionReason:   decisionReason,
		DecisionSnapshot: requestSnapshotJSON,
		Metadata:         json.RawMessage(metadataJSON),
		RouterPreMS:      &routerPreMSVal,
		APIKeyID:         apiKeyID,
		APIKeyName:       apiKeyName,
		JWTSub:           jwtSub,
		CustomerID:       ctxCustomerID,
		Channel:          ctxChannel,
		InteractionType:  ctxInteractionType,
		AgentID:          ctxAgentID,
		Department:       ctxDepartment,
		TicketID:         ctxTicketID,
		CustomerSegment:  ctxCustomerSegment,
		Language:         ctxLanguage,
		Intent:           ctxIntent,
		ExperimentID:     ctxExperimentID,
		AutonomyLevel:    ctxAutonomyLevel,
		PolicyID:         ctxPolicyID,
		RiskLevel:        ctxRiskLevel,
		RevenueImpact:    ctxRevenueImpact,
		Currency:         ctxCurrency,
	})

	// Streaming-only usage accounting.
	// Prefer provider-reported usage (e.g. from Bedrock stream metadata) when available;
	// fall back to character-based estimation (ceil(chars/4)) otherwise.
	var completionTokens, totalTokens int
	if providerUsage != nil {
		inputTokens = providerUsage.PromptTokens
		completionTokens = providerUsage.CompletionTokens
		totalTokens = providerUsage.TotalTokens
	} else {
		completionTokens = estimateStreamingTokens(streamOutput)
		totalTokens = inputTokens + completionTokens
	}
	streamUsage := providers.Usage{
		PromptTokens:     inputTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
	if providerUsage != nil {
		streamUsage.CachedInputTokens = providerUsage.CachedInputTokens
		streamUsage.ToolCallsUsed = providerUsage.ToolCallsUsed
	}
	gc := h.resolveGlobalConfig(context.Background())
	costUSD := computeCost(modelCfg.Pricing, streamUsage, gc.ToolPricing)
	h.saveUsageAsync(context.Background(), storage.UsageRecord{
		ID:               uuid.New(),
		TenantID:         tenant.ID,
		Model:            modelName,
		Provider:         modelCfg.Provider,
		PromptTokens:     inputTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		CostUSD:          costUSD,
		RequestID:        requestID,
		APIKeyID:         apiKeyID,
		APIKeyName:       apiKeyName,
		JWTSub:           jwtSub,
	})
}
