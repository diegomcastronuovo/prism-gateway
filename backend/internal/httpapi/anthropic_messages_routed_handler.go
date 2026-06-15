package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── Parser ────────────────────────────────────────────────────────────────────

// parseAnthropicMessagesRequest decodes a POST /v1/messages body and maps it to
// a ParsedRequest. The caller MUST NOT access r.Header or r.Body after this
// call — all routing headers are available via ParsedRequest.Headers.
//
// max_tokens is REQUIRED; returns an error when absent.
// APIStyle is always set to APIStyleAnthropicMessages.
func parseAnthropicMessagesRequest(r *http.Request) (ParsedRequest, error) {
	var req AnthropicMessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return ParsedRequest{}, err
	}

	if req.MaxTokens == nil {
		return ParsedRequest{}, errors.New("max_tokens is required")
	}

	// Normalize each message's content (string or block array).
	var messages []ChatMessage
	for _, m := range req.Messages {
		text := extractTextFromContent(m.Content)
		// ChatMessage.Content is json.RawMessage — marshal the plain string.
		textJSON, _ := json.Marshal(text)
		messages = append(messages, ChatMessage{
			Role:    m.Role,
			Content: json.RawMessage(textJSON),
		})
	}

	pr := ParsedRequest{
		TenantID:     auth.TenantIDFromContext(r.Context()),
		BodyModel:    req.Model,
		Messages:     messages,
		SystemPrompt: req.System,
		MaxTokens:    req.MaxTokens,
		Temperature:  req.Temperature,
		TopP:         req.TopP,
		Stream:       req.Stream,
		Tools:        req.Tools,
		ToolChoice:   req.ToolChoice,
		Reasoning:    req.Thinking,
		Metadata:     req.Metadata,
		Headers:      r.Header,
		APIStyle:     APIStyleAnthropicMessages,
	}

	// Map stop_sequences to ProviderOptions.
	if len(req.StopSequences) > 0 {
		val, err := json.Marshal(req.StopSequences)
		if err == nil {
			if pr.ProviderOptions == nil {
				pr.ProviderOptions = make(map[string]json.RawMessage)
			}
			pr.ProviderOptions["stop_sequences"] = val
		}
	}

	// Pre-extract message texts and image URLs for smart routing / semantic cache.
	for _, msg := range messages {
		pr.MessageTexts = append(pr.MessageTexts, msg.TextContent())
		pr.MessageImageURLs = append(pr.MessageImageURLs, msg.ImageURLs()...)
	}

	return pr, nil
}

// ── Serializer ────────────────────────────────────────────────────────────────

// finishReasonToStopReason maps a provider finish reason to an Anthropic stop_reason.
func finishReasonToStopReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return "end_turn"
	}
}

// canonicalToAnthropicMessagesResponse converts a CanonicalResponse to the
// Anthropic Messages wire format.
func canonicalToAnthropicMessagesResponse(cr *CanonicalResponse) AnthropicMessagesResponse {
	var content []AnthropicResponseContentBlock
	var stopReason string

	if len(cr.Choices) > 0 {
		c := cr.Choices[0]
		text := c.Message.Content
		content = []AnthropicResponseContentBlock{
			{Type: "text", Text: text},
		}
		stopReason = finishReasonToStopReason(c.FinishReason)
	} else {
		content = []AnthropicResponseContentBlock{}
		stopReason = "end_turn"
	}

	return AnthropicMessagesResponse{
		ID:           cr.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      content,
		Model:        cr.Model,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage: AnthropicUsage{
			InputTokens:  cr.Usage.PromptTokens,
			OutputTokens: cr.Usage.CompletionTokens,
		},
	}
}

// ── Handler ───────────────────────────────────────────────────────────────────

// AnthropicMessagesRouted handles POST /v1/messages (Anthropic Messages API).
// It routes requests through the full orchestrator pipeline — routing, rate
// limiting, PII hooks, and semantic caching — then serialises the response
// back to Anthropic wire format.
//
// This handler is distinct from AnthropicMessages (/v1/claudecode), which is
// a raw passthrough with no routing. Invariant I-2: that handler and route
// are untouched.
func (h *Handlers) AnthropicMessagesRouted(w http.ResponseWriter, r *http.Request) {
	tenant := auth.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication_error")
		return
	}

	parsed, err := parseAnthropicMessagesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error(), "invalid_request_error")
		return
	}

	// Ensure the orchestrator is wired (tests that construct Handlers directly may not wire it).
	if h.orchestrator == nil {
		h.orchestrator = &Orchestrator{h: h}
	}

	var sink ResponseSink
	var wrappedW http.ResponseWriter = w
	if parsed.Stream {
		aw := newAnthropicMessagesWriter(w, parsed.BodyModel)
		sink = aw
		wrappedW = aw
	}

	out := h.orchestrator.Run(r.Context(), OrchestratorInput{
		Req:    parsed,
		Tenant: tenant,
		W:      wrappedW,
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
	// Copy extra headers, then write the JSON body in Anthropic wire format.
	if out.Response != nil {
		for k, v := range out.Response.ExtraHeaders {
			w.Header().Set(k, v)
		}
		// When the Anthropic provider returned a raw body, write it verbatim to avoid
		// losing fields like usage.cache_read_input_tokens, multi-block content,
		// stop_sequence, and exact stop_reason values.
		if len(out.Response.RawPassthroughBody) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(out.Response.RawPassthroughBody) //nolint:errcheck
			return
		}
		writeJSON(w, http.StatusOK, canonicalToAnthropicMessagesResponse(out.Response))
	}
}
