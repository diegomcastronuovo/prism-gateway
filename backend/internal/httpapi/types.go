package httpapi

import (
	"encoding/json"
	"strings"
	"time"
)

// OpenAI-compatible request/response types (subset).

// ChatMessage supports both legacy string content and multimodal content blocks.
//
//	Legacy:     "content": "hello"
//	Multimodal: "content": [{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"..."}}]
type ChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// TextContent returns the plain-text representation of the message content.
// For a string literal it returns the string; for a block array it concatenates
// all "text" block values joined by a space.
func (m ChatMessage) TextContent() string {
	if len(m.Content) == 0 {
		return ""
	}
	// Try string literal first.
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	// Fall back to block array.
	var blocks []ContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

// ImageURLs returns every image URL found in multimodal content blocks.
// Returns nil for string-literal content.
func (m ChatMessage) ImageURLs() []string {
	var blocks []ContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil
	}
	var urls []string
	for _, b := range blocks {
		if b.Type == "image_url" && b.ImageURL != nil && b.ImageURL.URL != "" {
			urls = append(urls, b.ImageURL.URL)
		}
	}
	return urls
}

type ChatCompletionRequest struct {
	Model       string                 `json:"model"`
	Messages    []ChatMessage          `json:"messages"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Temperature *float64               `json:"temperature,omitempty"`
	TopP        *float64               `json:"top_p,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Tools       json.RawMessage        `json:"tools,omitempty"`
	ToolChoice  json.RawMessage        `json:"tool_choice,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"` // optional cost-allocation tags
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
}

type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

type ErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Code    *string `json:"code,omitempty"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ContentBlock is one element of a multimodal message content array.
type ContentBlock struct {
	Type     string        `json:"type"` // "text" | "image_url"
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLData `json:"image_url,omitempty"`
}

// ImageURLData holds the URL of an image in a content block.
type ImageURLData struct {
	URL string `json:"url"`
}

// Smart routing impact response types

type SmartImpactResponse struct {
	TenantID   string            `json:"tenant_id"`
	WindowDays int               `json:"window_days"`
	Period     PeriodInfo        `json:"period"`
	Actual     ActualMetrics     `json:"actual"`
	Baseline   BaselineMetrics   `json:"baseline"`
	Impact     ImpactCalculation `json:"impact"`
}

type PeriodInfo struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type ActualMetrics struct {
	Requests     int     `json:"requests"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	AvgCostUSD   float64 `json:"avg_cost_usd"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

type BaselineMetrics struct {
	Type         string  `json:"type"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	AvgCostUSD   float64 `json:"avg_cost_usd"`
}

type ImpactCalculation struct {
	SavingsUSD float64  `json:"savings_usd"`
	SavingsPct float64  `json:"savings_pct"`
	Notes      []string `json:"notes"`
}
