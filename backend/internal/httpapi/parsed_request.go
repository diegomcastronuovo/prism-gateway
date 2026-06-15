package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

// APIStyle constants identify which public API style a ParsedRequest originated
// from. Set by every parser; read by future response serializers.
// The orchestrator MUST NOT use APIStyle for routing decisions.
const (
	APIStyleOpenAIChat      = "openai_chat"
	APIStyleOpenAIResponses = "openai_responses"
	APIStyleAnthropicMessages = "anthropic_messages"
	APIStyleGemini          = "gemini_generate_content"
	APIStyleML              = "ml"
)

// ParsedRequest is the normalised representation of any chat request after the
// endpoint handler has parsed it from its wire format.
// It is the contract between Layer 1 (endpoint handlers) and Layer 2 (the
// orchestration core). The core MUST NOT access *http.Request directly.
type ParsedRequest struct {
	// Tenant identity and requested model as received from the request.
	TenantID  string
	BodyModel string // empty when the endpoint does not specify a model in the body

	// Normalised messages — always in the internal format.
	Messages []ChatMessage // re-uses the existing type from types.go

	// Inference parameters.
	MaxTokens   *int
	Temperature *float64
	TopP        *float64
	Stream      bool

	// Tools support (function calling).
	Tools      json.RawMessage
	ToolChoice json.RawMessage

	// Cost allocation metadata from the request.
	Metadata map[string]interface{}

	// Original HTTP headers, copied at parse time so the core can read routing
	// headers (X-Route-Group, X-Force-Model, X-Workflow-Id, X-Conversation-Id,
	// X-API-Key, X-Debug-Seed, etc.) from this copy.
	// The core MUST read headers from Headers, NOT from *http.Request.
	Headers http.Header

	// Pre-extracted fields that the core needs for smart routing and cache.
	// The handler extracts these once during parsing.
	MessageTexts     []string // TextContent() of each message
	MessageImageURLs []string // ImageURLs() of each message (multimodal)

	// Raw body (retained for ML pass-through path; nil for normal LLM paths).
	RawBody []byte

	// API style discriminator — set by every parser, read by serializers.
	// Empty string is a programming error; the orchestrator treats it as
	// APIStyleOpenAIChat for backward compatibility.
	APIStyle string

	// SystemPrompt carries a top-level system instruction when the wire format
	// separates it from the messages array (Anthropic, Gemini). For OpenAI Chat
	// the system role message stays in Messages and this field is empty.
	SystemPrompt string

	// Pass-through fields — the orchestrator does not parse these.
	// Provider adapters receive them unchanged. Nil means the field was absent.
	ResponseFormat  json.RawMessage
	Reasoning       json.RawMessage
	SafetySettings  json.RawMessage
	ProviderOptions map[string]json.RawMessage
}

// parseChatCompletionsRequest parses a /v1/chat/completions HTTP request into a
// ParsedRequest. After this call the caller MUST NOT access r.Header or r.Body
// directly — all routing headers are available via ParsedRequest.Headers.
func parseChatCompletionsRequest(r *http.Request) (ParsedRequest, error) {
	// ML pass-through path: body is consumed and stored as raw bytes.
	if r.Header.Get("X-Model-Type") == "ml" {
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			return ParsedRequest{}, err
		}
		pr := ParsedRequest{
			TenantID: auth.TenantIDFromContext(r.Context()),
			Headers:  r.Header,
			RawBody:  rawBody,
			APIStyle: APIStyleML,
		}
		return pr, nil
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return ParsedRequest{}, err
	}

	pr := ParsedRequest{
		TenantID:    auth.TenantIDFromContext(r.Context()),
		BodyModel:   req.Model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		Metadata:    req.Metadata,
		Headers:     r.Header,
		APIStyle:    APIStyleOpenAIChat,
	}

	// Pre-extract message texts and image URLs for smart routing / semantic cache.
	for _, msg := range req.Messages {
		pr.MessageTexts = append(pr.MessageTexts, msg.TextContent())
		pr.MessageImageURLs = append(pr.MessageImageURLs, msg.ImageURLs()...)
	}

	return pr, nil
}

