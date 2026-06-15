package httpapi

import "encoding/json"

// AnthropicMessagesRequest is the wire format for POST /v1/messages (Anthropic Messages API).
// Fields are mapped to ParsedRequest by parseAnthropicMessagesRequest.
type AnthropicMessagesRequest struct {
	Model         string                 `json:"model"`
	Messages      []AnthropicInputMessage `json:"messages"`
	System        string                 `json:"system"`
	MaxTokens     *int                   `json:"max_tokens"`
	Tools         json.RawMessage        `json:"tools,omitempty"`
	ToolChoice    json.RawMessage        `json:"tool_choice,omitempty"`
	Temperature   *float64               `json:"temperature,omitempty"`
	TopP          *float64               `json:"top_p,omitempty"`
	Stream        bool                   `json:"stream"`
	Thinking      json.RawMessage        `json:"thinking,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	StopSequences []string               `json:"stop_sequences,omitempty"`
}

// AnthropicInputMessage is one element of the messages array.
// Content may be a plain string or an array of content blocks.
type AnthropicInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string | []AnthropicInputContentBlock
}

// AnthropicInputContentBlock is one element of a content block array.
type AnthropicInputContentBlock struct {
	Type   string          `json:"type"`
	Text   string          `json:"text,omitempty"`
	Source json.RawMessage `json:"source,omitempty"` // for image blocks
}

// AnthropicMessagesResponse is the non-streaming wire response for POST /v1/messages.
type AnthropicMessagesResponse struct {
	ID           string                          `json:"id"`
	Type         string                          `json:"type"`
	Role         string                          `json:"role"`
	Content      []AnthropicResponseContentBlock `json:"content"`
	Model        string                          `json:"model"`
	StopReason   string                          `json:"stop_reason"`
	StopSequence *string                         `json:"stop_sequence"`
	Usage        AnthropicUsage                  `json:"usage"`
}

// AnthropicResponseContentBlock is one content block in the response.
type AnthropicResponseContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// AnthropicUsage holds token counts using the Anthropic Messages API field names.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
