package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

// extractGeminiModel extracts the model name from a Gemini action-style URL path.
// Expected format: /v1/models/{model}:generateContent or /v1/models/{model}:streamGenerateContent.
// Returns empty string when the path does not match the expected layout.
func extractGeminiModel(path string) string {
	// Strip the /v1/models/ prefix.
	const prefix = "/v1/models/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	// The action suffix starts at the colon; everything before is the model name.
	if idx := strings.IndexByte(rest, ':'); idx >= 0 {
		return rest[:idx]
	}
	return ""
}

// ── Wire input types ──────────────────────────────────────────────────────────

// GeminiGenerateContentRequest is the wire format for POST /v1/models/{model}:generateContent.
type GeminiGenerateContentRequest struct {
	Contents          []GeminiContent         `json:"contents"`
	SystemInstruction *GeminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings    json.RawMessage         `json:"safetySettings,omitempty"`
	Tools             json.RawMessage         `json:"tools,omitempty"`
}

// GeminiContent represents a single turn in the conversation or a system instruction.
type GeminiContent struct {
	Role  string      `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart is one part within a GeminiContent.
type GeminiPart struct {
	Text string `json:"text,omitempty"`
}

// GeminiGenerationConfig holds inference parameter overrides for the Gemini API.
type GeminiGenerationConfig struct {
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
}

// ── Parser ────────────────────────────────────────────────────────────────────

// parseGeminiGenerateContentRequest decodes a POST /v1/models/{model}:generateContent body
// and maps it to a ParsedRequest. The stream flag is provided by the handler based on the
// route path — it is NOT read from the request body.
//
// Mapping rules:
//   - r.PathValue("model")        → BodyModel (required; 400 on empty)
//   - contents[]                  → Messages (role "model" → "assistant", else → "user")
//   - systemInstruction.parts     → SystemPrompt (concatenated with "\n")
//   - generationConfig fields     → MaxTokens / Temperature / TopP
//   - safetySettings              → SafetySettings (raw pass-through)
//   - stream (bool arg)           → Stream
//
// APIStyle is always set to APIStyleGemini.
func parseGeminiGenerateContentRequest(r *http.Request, stream bool) (ParsedRequest, error) {
	model := extractGeminiModel(r.URL.Path)
	if model == "" {
		return ParsedRequest{}, errors.New("model path parameter is required")
	}

	var req GeminiGenerateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return ParsedRequest{}, err
	}

	// Normalise contents[] → []ChatMessage.
	messages := make([]ChatMessage, 0, len(req.Contents))
	for _, c := range req.Contents {
		role := c.Role
		if role == "model" {
			role = "assistant"
		} else {
			role = "user"
		}

		// Concatenate all text parts with a newline separator.
		var parts []string
		for _, p := range c.Parts {
			if p.Text != "" {
				parts = append(parts, p.Text)
			}
		}
		text := strings.Join(parts, "\n")
		textJSON, _ := json.Marshal(text)
		messages = append(messages, ChatMessage{
			Role:    role,
			Content: json.RawMessage(textJSON),
		})
	}

	// Extract system instruction.
	var systemPrompt string
	if req.SystemInstruction != nil {
		var sysParts []string
		for _, p := range req.SystemInstruction.Parts {
			if p.Text != "" {
				sysParts = append(sysParts, p.Text)
			}
		}
		systemPrompt = strings.Join(sysParts, "\n")
	}

	pr := ParsedRequest{
		TenantID:       auth.TenantIDFromContext(r.Context()),
		BodyModel:      model,
		Messages:       messages,
		SystemPrompt:   systemPrompt,
		Stream:         stream,
		SafetySettings: req.SafetySettings,
		Headers:        r.Header,
		APIStyle:       APIStyleGemini,
	}

	if req.GenerationConfig != nil {
		pr.MaxTokens = req.GenerationConfig.MaxOutputTokens
		pr.Temperature = req.GenerationConfig.Temperature
		pr.TopP = req.GenerationConfig.TopP
	}

	// Pre-extract message texts and image URLs for smart routing / semantic cache.
	for _, msg := range messages {
		pr.MessageTexts = append(pr.MessageTexts, msg.TextContent())
		pr.MessageImageURLs = append(pr.MessageImageURLs, msg.ImageURLs()...)
	}

	return pr, nil
}
