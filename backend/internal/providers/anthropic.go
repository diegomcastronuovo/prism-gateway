package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Anthropic implements the Provider interface for the Anthropic Messages API.
type Anthropic struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	streamClient *http.Client
}

const anthropicAPIVersion = "2023-06-01"
const defaultAnthropicMaxTokens = 4096

func NewAnthropic(baseURL, apiKey string) *Anthropic {
	t := newTransport()
	return &Anthropic{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: t,
		},
		streamClient: &http.Client{Transport: t},
	}
}

// SetHTTPClient allows overriding the default HTTP client (for testing).
func (a *Anthropic) SetHTTPClient(c *http.Client) {
	a.httpClient = c
	a.streamClient = &http.Client{Transport: c.Transport}
}

// Anthropic Messages API request types.

// anthropicInputBlock is one element of a multimodal content array (request side).
type anthropicInputBlock struct {
	Type   string                `json:"type"` // "text" | "image"
	Text   string                `json:"text,omitempty"`
	Source *anthropicImageSource `json:"source,omitempty"` // for type="image"
}

// anthropicImageSource describes the image location in an Anthropic request.
type anthropicImageSource struct {
	Type string `json:"type"` // "url"
	URL  string `json:"url"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // JSON string or []anthropicInputBlock
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
}

// Anthropic Messages API response types.
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens             int    `json:"input_tokens"`
	OutputTokens            int    `json:"output_tokens"`
	CacheReadInputTokens    int    `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int   `json:"cache_creation_input_tokens"` // flat sum of 5m+1h writes
	CacheCreation           struct {
		Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	ServiceTier  string `json:"service_tier"`
	InferenceGeo string `json:"inference_geo"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

type anthropicErrorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type anthropicStreamEvent struct {
	Type       string  `json:"type"`
	StopReason *string `json:"stop_reason"`
	Delta      struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

func (a *Anthropic) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Translate OpenAI-style request to Anthropic Messages API format
	anthReq := a.translateRequest(req)

	body, err := json.Marshal(anthReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	url := a.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	if a.apiKey != "" {
		httpReq.Header.Set("x-api-key", a.apiKey)
	}

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Extract short error message if possible
		errBody := truncateBody(string(respBody), 500)
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       errBody,
		}
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
	}

	cr := a.translateResponse(anthResp)
	cr.RawBody = respBody
	return cr, nil
}

func (a *Anthropic) ChatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	anthReq := a.translateRequest(req)
	bodyMap := map[string]interface{}{
		"model":      anthReq.Model,
		"messages":   anthReq.Messages,
		"max_tokens": anthReq.MaxTokens,
		"stream":     true,
	}
	if anthReq.System != "" {
		bodyMap["system"] = anthReq.System
	}
	if anthReq.Temperature != nil {
		bodyMap["temperature"] = anthReq.Temperature
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic stream request: %w", err)
	}

	url := a.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	if a.apiKey != "" {
		httpReq.Header.Set("x-api-key", a.apiKey)
	}

	streamClient := a.streamClient
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       truncateBody(string(errBody), 500),
		}
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		lastEventType := ""
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				lastEventType = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			var ev anthropicStreamEvent
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				select {
				case ch <- StreamEvent{Type: "error", Error: err}:
				case <-ctx.Done():
				}
				return
			}
			evType := ev.Type
			if evType == "" {
				evType = lastEventType
			}
			switch evType {
			case "content_block_delta":
				if ev.Delta.Text == "" {
					continue
				}
				select {
				case ch <- StreamEvent{Type: "delta", Content: ev.Delta.Text}:
				case <-ctx.Done():
					return
				}
			case "message_stop":
				select {
				case ch <- StreamEvent{Type: "done", FinishReason: ev.StopReason}:
				case <-ctx.Done():
				}
				return
			case "message_start", "content_block_start", "content_block_stop", "ping":
				continue
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case ch <- StreamEvent{Type: "error", Error: err}:
			case <-ctx.Done():
			}
		}
	}()
	return &StreamResponse{Events: ch}, nil
}

// translateRequest converts an OpenAI-style ChatRequest to an Anthropic Messages API request.
func (a *Anthropic) translateRequest(req ChatRequest) anthropicRequest {
	var systemPrompt string
	var messages []anthropicMessage

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			// Anthropic uses a dedicated system field
			if systemPrompt != "" {
				systemPrompt += "\n\n"
			}
			systemPrompt += msg.Content
		case "user", "assistant":
			messages = append(messages, anthropicMessage{
				Role:    msg.Role,
				Content: anthropicContent(msg),
			})
		default:
			messages = append(messages, anthropicMessage{
				Role:    "user",
				Content: anthropicContent(msg),
			})
		}
	}

	// Anthropic requires at least one message
	if len(messages) == 0 {
		empty, _ := json.Marshal("")
		messages = append(messages, anthropicMessage{Role: "user", Content: json.RawMessage(empty)})
	}

	maxTokens := defaultAnthropicMaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	anthReq := anthropicRequest{
		Model:     req.Model,
		Messages:  messages,
		MaxTokens: maxTokens,
	}
	if systemPrompt != "" {
		anthReq.System = systemPrompt
	}
	if req.Temperature != nil {
		anthReq.Temperature = req.Temperature
	}

	return anthReq
}

// anthropicContent builds the json.RawMessage value for an Anthropic message's "content" field.
// For text-only messages it produces a JSON string; for multimodal messages it produces a
// JSON array of anthropicInputBlocks (text + image source blocks).
func anthropicContent(msg ChatMessage) json.RawMessage {
	if len(msg.ContentBlocks) == 0 {
		b, _ := json.Marshal(msg.Content)
		return json.RawMessage(b)
	}
	var blocks []anthropicInputBlock
	for _, cb := range msg.ContentBlocks {
		switch cb.Type {
		case "text":
			if cb.Text != "" {
				blocks = append(blocks, anthropicInputBlock{Type: "text", Text: cb.Text})
			}
		case "image_url":
			if cb.ImageURL != nil && cb.ImageURL.URL != "" {
				blocks = append(blocks, anthropicInputBlock{
					Type:   "image",
					Source: &anthropicImageSource{Type: "url", URL: cb.ImageURL.URL},
				})
			}
		}
	}
	b, _ := json.Marshal(blocks)
	return json.RawMessage(b)
}

// translateResponse converts an Anthropic Messages API response to an OpenAI-style ChatResponse.
func (a *Anthropic) translateResponse(resp anthropicResponse) *ChatResponse {
	// Combine all text content blocks
	var content string
	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	// Map Anthropic stop_reason to OpenAI finish_reason
	finishReason := "stop"
	switch resp.StopReason {
	case "end_turn", "stop_sequence":
		finishReason = "stop"
	case "max_tokens":
		finishReason = "length"
	}

	return &ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []ChatChoice{
			{
				Index:        0,
				Message:      ChatMessage{Role: "assistant", Content: content},
				FinishReason: finishReason,
			},
		},
		Usage: Usage{
			PromptTokens:       resp.Usage.InputTokens,
			CompletionTokens:   resp.Usage.OutputTokens,
			TotalTokens:        resp.Usage.InputTokens + resp.Usage.OutputTokens,
			CachedInputTokens:  resp.Usage.CacheReadInputTokens,
			CacheWrite5mTokens: resp.Usage.CacheCreation.Ephemeral5mInputTokens,
			CacheWrite1hTokens: resp.Usage.CacheCreation.Ephemeral1hInputTokens,
			InferenceGeo:       resp.Usage.InferenceGeo,
		},
	}
}

// ── NativeMessagesProvider implementation ────────────────────────────────────
// The two methods below implement the NativeMessagesProvider interface for raw
// Anthropic /v1/messages passthrough. They are completely independent from
// ChatCompletion / ChatCompletionStream and share no logic with them.

// SendMessagesRaw forwards the raw request body to the Anthropic /v1/messages
// endpoint as-is (no translation). Returns the raw response body and best-effort
// token counts for metrics.
func (a *Anthropic) SendMessagesRaw(ctx context.Context, body json.RawMessage) (json.RawMessage, MessagesUsage, error) {
	url := a.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, MessagesUsage{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	if key := a.resolveMessagesAPIKey(ctx); key != "" {
		httpReq.Header.Set("x-api-key", key)
	}

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, MessagesUsage{}, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, MessagesUsage{}, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, MessagesUsage{}, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       truncateBody(string(respBody), 500),
		}
	}

	// Extract token counts for metrics (best-effort; errors are intentionally ignored).
	var partial struct {
		Usage MessagesUsage `json:"usage"`
	}
	_ = json.Unmarshal(respBody, &partial)

	return json.RawMessage(respBody), partial.Usage, nil
}

// SendMessagesRawStream forwards the raw streaming request to Anthropic and returns
// the upstream HTTP response for direct SSE passthrough. Caller is responsible for
// closing the response body.
func (a *Anthropic) SendMessagesRawStream(ctx context.Context, body json.RawMessage) (*http.Response, error) {
	url := a.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	if key := a.resolveMessagesAPIKey(ctx); key != "" {
		httpReq.Header.Set("x-api-key", key)
	}

	// Use a transport-only client (no timeout) to avoid cutting off long streams.
	streamClient := a.streamClient
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       truncateBody(string(errBody), 500),
		}
	}
	return resp, nil
}

// truncateBody truncates a string to maxLen, appending "..." if truncated.
func truncateBody(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ── Per-request API key override (SPEC_158) ───────────────────────────────────
// WithMessagesAPIKey injects an API key override into the context for use by
// SendMessagesRaw and SendMessagesRawStream. Callers (e.g. the messages handler)
// use this to pass CLAUDE_API_KEY without mutating global provider state.
// A zero-length key is ignored (no override injected).
type messagesAPIKeyCtxKey struct{}

func WithMessagesAPIKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, messagesAPIKeyCtxKey{}, key)
}

// messagesAPIKeyFromCtx returns the per-request key injected by WithMessagesAPIKey,
// or "" when none is present.
func messagesAPIKeyFromCtx(ctx context.Context) string {
	key, _ := ctx.Value(messagesAPIKeyCtxKey{}).(string)
	return key
}

// resolveMessagesAPIKey returns the effective API key for a messages request:
// context override → provider-level key → "".
func (a *Anthropic) resolveMessagesAPIKey(ctx context.Context) string {
	if key := messagesAPIKeyFromCtx(ctx); key != "" {
		return key
	}
	return a.apiKey
}
