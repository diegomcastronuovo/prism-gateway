package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// ChatRequest is the provider-agnostic chat completion request.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	// Tools and ToolChoice are forwarded opaquely to OpenAI-compatible providers.
	Tools      json.RawMessage `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	// ProviderModelID is the upstream native model id when it differs from Model (gateway-visible name).
	ProviderModelID string `json:"-"`
	// UseResponsesAPI signals that the caller wants /v1/responses instead of /v1/chat/completions.
	// Only honoured by the OpenAI provider. Other providers ignore it.
	UseResponsesAPI bool `json:"-"`
}

// MessageContentBlock is one element of a multimodal message (OpenAI wire format).
// Used when ContentBlocks is populated on a ChatMessage.
type MessageContentBlock struct {
	Type     string        `json:"type"` // "text" | "image_url"
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLData `json:"image_url,omitempty"`
}

// ImageURLData holds the URL reference inside an image_url block.
type ImageURLData struct {
	URL string `json:"url"`
}

// ChatMessage is the provider-agnostic message type.
// Content holds the plain-text value for text-only messages.
// When ContentBlocks is non-empty the message is multimodal: MarshalJSON emits
// "content" as a JSON array (OpenAI wire format). Anthropic and Gemini translators
// inspect ContentBlocks directly and build their own provider-specific formats.
// ToolCalls is populated when deserializing a provider response that used function calling.
type ChatMessage struct {
	Role          string                `json:"role"`
	Content       string                `json:"content,omitempty"`
	ToolCalls     json.RawMessage       `json:"tool_calls,omitempty"`
	ToolCallID    string                `json:"tool_call_id,omitempty"`
	ContentBlocks []MessageContentBlock `json:"-"` // multimodal; not serialised as a field
}

// MarshalJSON encodes ChatMessage for OpenAI-compatible providers.
//   - text-only       → {"role":"...","content":"..."}
//   - multimodal      → {"role":"...","content":[...blocks...]}
//   - assistant+tools → {"role":"assistant","content":null,"tool_calls":[...]}
//   - tool result     → {"role":"tool","content":"...","tool_call_id":"..."}
func (m ChatMessage) MarshalJSON() ([]byte, error) {
	if len(m.ContentBlocks) > 0 {
		return json.Marshal(struct {
			Role    string                `json:"role"`
			Content []MessageContentBlock `json:"content"`
		}{Role: m.Role, Content: m.ContentBlocks})
	}
	if len(m.ToolCalls) > 0 {
		return json.Marshal(struct {
			Role      string          `json:"role"`
			Content   *string         `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls"`
		}{Role: m.Role, Content: nil, ToolCalls: m.ToolCalls})
	}
	if m.ToolCallID != "" {
		return json.Marshal(struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
		}{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID})
	}
	return json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: m.Role, Content: m.Content})
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// CachedInputTokens is the subset of PromptTokens served from the provider's
	// prompt cache (cache reads). Populated by OpenAI-compatible and Anthropic providers.
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	// ToolCallsUsed maps tool_type to call count for this request.
	// e.g. {"web_search": 2} means 2 web search calls were made.
	ToolCallsUsed map[string]int `json:"tool_calls_used,omitempty"`
	// Anthropic cache-write token counts (ephemeral cache, billed at write price).
	// CacheWrite5mTokens: tokens written with a 5-minute TTL.
	// CacheWrite1hTokens: tokens written with a 1-hour TTL.
	CacheWrite5mTokens int `json:"cache_write_5m_tokens,omitempty"`
	CacheWrite1hTokens int `json:"cache_write_1h_tokens,omitempty"`
	// InferenceGeo is the geographic region where inference ran, as reported by Anthropic
	// ("us" | "global"). Used to apply GeoMultiplierUS when non-empty.
	InferenceGeo string `json:"inference_geo,omitempty"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
	// RawBody carries the unmodified upstream response body for providers that
	// need lossless passthrough (e.g. Responses API). Excluded from JSON marshaling.
	RawBody json.RawMessage `json:"-"`
}

// StreamEvent is the provider-agnostic streaming event.
// Providers must emit this internal format; HTTP/SSE rendering is handled upstream.
type StreamEvent struct {
	Type            string          // "delta" | "done" | "error"
	Content         string          // assistant incremental text for delta
	ToolCallsDelta  json.RawMessage // raw tool_calls delta chunk (OpenAI function calling)
	FinishReason    *string         // optional OpenAI finish_reason equivalent
	Error           error           // only used when Type == "error"
	// Usage holds provider-reported token counts, only set on "done" events when the
	// provider makes them available (e.g. Bedrock stream metadata). nil = use estimation.
	Usage *Usage
}

// StreamResponse holds metadata plus the chunk channel for streaming.
type StreamResponse struct {
	Events <-chan StreamEvent
}

// Provider is the interface every upstream LLM backend must implement.
type Provider interface {
	// ChatCompletion sends a non-streaming chat completion request.
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// ChatCompletionStream sends a streaming request and returns chunks via channel.
	ChatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error)
}

var ErrStreamingNotSupported = fmt.Errorf("streaming is not yet supported for this provider")

// MessagesUsage holds token counts from a raw Anthropic Messages API response.
// Used exclusively by NativeMessagesProvider — no overlap with existing Usage type.
type MessagesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// NativeMessagesProvider is an optional capability implemented by providers that
// support raw passthrough of the Anthropic /v1/messages protocol without translation.
// Implemented by *Anthropic only. Zero impact on existing Provider or EmbeddingProvider.
type NativeMessagesProvider interface {
	// SendMessagesRaw forwards the raw Anthropic Messages request body as-is and
	// returns the raw response body plus best-effort token counts for metrics.
	SendMessagesRaw(ctx context.Context, body json.RawMessage) (json.RawMessage, MessagesUsage, error)

	// SendMessagesRawStream forwards the raw streaming request and returns the
	// upstream HTTP response for SSE passthrough. Caller must close the body.
	SendMessagesRawStream(ctx context.Context, body json.RawMessage) (*http.Response, error)
}

// Registry maps provider names to Provider and EmbeddingProvider instances.
type Registry struct {
	providers               map[string]Provider
	embeddingProviders      map[string]EmbeddingProvider
	nativeMessagesProviders map[string]NativeMessagesProvider
}

func NewRegistry() *Registry {
	return &Registry{
		providers:               make(map[string]Provider),
		embeddingProviders:      make(map[string]EmbeddingProvider),
		nativeMessagesProviders: make(map[string]NativeMessagesProvider),
	}
}

func (r *Registry) Register(name string, p Provider) {
	r.providers[name] = p
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// RegisterEmbedding registers an EmbeddingProvider under the given name.
func (r *Registry) RegisterEmbedding(name string, p EmbeddingProvider) {
	r.embeddingProviders[name] = p
}

// GetEmbedding returns the EmbeddingProvider registered under the given name.
func (r *Registry) GetEmbedding(name string) (EmbeddingProvider, bool) {
	p, ok := r.embeddingProviders[name]
	return p, ok
}

// RegisterNativeMessages registers a NativeMessagesProvider under the given name.
func (r *Registry) RegisterNativeMessages(name string, p NativeMessagesProvider) {
	r.nativeMessagesProviders[name] = p
}

// GetNativeMessages returns the NativeMessagesProvider registered under the given name.
func (r *Registry) GetNativeMessages(name string) (NativeMessagesProvider, bool) {
	p, ok := r.nativeMessagesProviders[name]
	return p, ok
}

// BuildFromConfig creates providers from the config and registers them.
func BuildFromConfig(cfg *config.Config) (*Registry, error) {
	reg := NewRegistry()
	for name, pc := range cfg.Providers {
		if err := RegisterOne(reg, name, pc); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// EstimateTokens provides a simple character-based token estimation.
func EstimateTokens(text string) int {
	// ~4 chars per token is a common rough heuristic
	t := len(text) / 4
	if t == 0 && len(text) > 0 {
		t = 1
	}
	return t
}
