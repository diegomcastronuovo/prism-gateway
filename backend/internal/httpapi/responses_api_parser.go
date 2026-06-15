package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

// ResponsesAPIRequest is the wire format for POST /v1/responses (OpenAI Responses API).
// Fields are mapped to ParsedRequest by parseResponsesAPIRequest.
type ResponsesAPIRequest struct {
	Model              string                 `json:"model"`
	Input              json.RawMessage        `json:"input"`
	Instructions       string                 `json:"instructions"`
	Reasoning          json.RawMessage        `json:"reasoning,omitempty"`
	Text               responsesAPITextConfig  `json:"text,omitempty"`
	Tools              json.RawMessage        `json:"tools,omitempty"`
	ToolChoice         json.RawMessage        `json:"tool_choice,omitempty"`
	MaxOutputTokens    *int                   `json:"max_output_tokens,omitempty"`
	Temperature        *float64               `json:"temperature,omitempty"`
	TopP               *float64               `json:"top_p,omitempty"`
	Stream             bool                   `json:"stream,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	PreviousResponseID string                 `json:"previous_response_id,omitempty"`
}

// responsesAPITextConfig holds the text-output configuration block from the Responses API.
// Currently only the Format field is relevant for routing.
type responsesAPITextConfig struct {
	Format json.RawMessage `json:"format,omitempty"`
}

// parseResponsesAPIRequest decodes a POST /v1/responses body and maps it to a
// ParsedRequest. The caller MUST NOT access r.Header or r.Body after this call —
// all routing headers are available via ParsedRequest.Headers.
//
// Mapping rules (Responses API → ParsedRequest):
//   - input          → Messages
//   - instructions   → SystemPrompt (empty string when absent)
//   - reasoning      → Reasoning
//   - text.format    → ResponseFormat
//   - tools          → Tools
//   - tool_choice    → ToolChoice
//   - max_output_tokens → MaxTokens
//   - model          → BodyModel
//   - temperature    → Temperature
//   - top_p          → TopP
//   - stream         → Stream
//   - metadata       → Metadata
//   - previous_response_id → silently ignored
//
// APIStyle is always set to APIStyleOpenAIResponses.
func parseResponsesAPIRequest(r *http.Request) (ParsedRequest, error) {
	var req ResponsesAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return ParsedRequest{}, err
	}

	var messages []ChatMessage
	if len(req.Input) > 0 {
		if req.Input[0] == '"' {
			// string input → single user message
			var text string
			if err := json.Unmarshal(req.Input, &text); err != nil {
				return ParsedRequest{}, err
			}
			rawContent, _ := json.Marshal(text)
			messages = []ChatMessage{{Role: "user", Content: rawContent}}
		} else {
			if err := json.Unmarshal(req.Input, &messages); err != nil {
				return ParsedRequest{}, err
			}
		}
	}

	pr := ParsedRequest{
		TenantID:       auth.TenantIDFromContext(r.Context()),
		BodyModel:      req.Model,
		Messages:       messages,
		SystemPrompt:   req.Instructions,
		Reasoning:      req.Reasoning,
		ResponseFormat: req.Text.Format,
		Tools:          req.Tools,
		ToolChoice:     req.ToolChoice,
		MaxTokens:      req.MaxOutputTokens,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		Stream:         req.Stream,
		Metadata:       req.Metadata,
		Headers:        r.Header,
		APIStyle:       APIStyleOpenAIResponses,
	}

	// Pre-extract message texts and image URLs for smart routing / semantic cache.
	// Mirrors parseChatCompletionsRequest in parsed_request.go.
	for _, msg := range messages {
		pr.MessageTexts = append(pr.MessageTexts, msg.TextContent())
		pr.MessageImageURLs = append(pr.MessageImageURLs, msg.ImageURLs()...)
	}

	return pr, nil
}
