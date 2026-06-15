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

// Gemini implements the Provider interface for Google's Gemini (Generative Language) API.
type Gemini struct {
	baseURL      string
	httpClient   *http.Client
	streamClient *http.Client
}

const defaultGeminiMaxTokens = 4096

// geminiKeyTransport injects the API key as x-goog-api-key header instead of
// embedding it in the URL where it would appear in error messages and proxy logs.
type geminiKeyTransport struct {
	base   http.RoundTripper
	apiKey string
}

func (t *geminiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("x-goog-api-key", t.apiKey)
	return t.base.RoundTrip(req)
}

func NewGemini(baseURL, apiKey string) *Gemini {
	base := newTransport()
	var transport http.RoundTripper = base
	if apiKey != "" {
		transport = &geminiKeyTransport{base: base, apiKey: apiKey}
	}
	return &Gemini{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
		streamClient: &http.Client{Transport: transport},
	}
}

// SetHTTPClient allows overriding the default HTTP client (for testing).
func (g *Gemini) SetHTTPClient(c *http.Client) {
	g.httpClient = c
	g.streamClient = &http.Client{Transport: c.Transport}
}

// Gemini generateContent request types.

// geminiFileData carries an image URL in a Gemini content part.
type geminiFileData struct {
	FileURI  string `json:"file_uri"`
	MimeType string `json:"mime_type,omitempty"`
}

type geminiPart struct {
	Text     string          `json:"text,omitempty"`
	FileData *geminiFileData `json:"file_data,omitempty"` // for image URL parts
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig,omitempty"`
}

// Gemini generateContent response types.
type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

func (g *Gemini) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	gemReq := g.translateRequest(req)

	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	url := g.baseURL + "/models/" + req.Model + ":generateContent"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       truncateBody(string(respBody), 500),
		}
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini response: %w", err)
	}

	return g.translateResponse(req.Model, gemResp), nil
}

func (g *Gemini) ChatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	gemReq := g.translateRequest(req)
	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini stream request: %w", err)
	}

	url := g.baseURL + "/models/" + req.Model + ":streamGenerateContent?alt=sse"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := g.streamClient.Do(httpReq)
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
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var ev geminiResponse
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				select {
				case ch <- StreamEvent{Type: "error", Error: err}:
				case <-ctx.Done():
				}
				return
			}
			if len(ev.Candidates) == 0 {
				continue
			}
			candidate := ev.Candidates[0]
			var content string
			for _, part := range candidate.Content.Parts {
				content += part.Text
			}
			if content != "" {
				select {
				case ch <- StreamEvent{Type: "delta", Content: content}:
				case <-ctx.Done():
					return
				}
			}
			if candidate.FinishReason != "" {
				fr := mapGeminiFinishReason(candidate.FinishReason)
				select {
				case ch <- StreamEvent{Type: "done", FinishReason: &fr}:
				case <-ctx.Done():
				}
				return
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

// translateRequest converts an OpenAI-style ChatRequest to a Gemini generateContent request.
func (g *Gemini) translateRequest(req ChatRequest) geminiRequest {
	var systemParts []geminiPart
	var contents []geminiContent

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			systemParts = append(systemParts, geminiPart{Text: msg.Content})
		case "assistant":
			contents = append(contents, geminiContent{
				Role:  "model",
				Parts: geminiParts(msg),
			})
		case "user":
			contents = append(contents, geminiContent{
				Role:  "user",
				Parts: geminiParts(msg),
			})
		default:
			contents = append(contents, geminiContent{
				Role:  "user",
				Parts: geminiParts(msg),
			})
		}
	}

	// Gemini requires at least one content entry
	if len(contents) == 0 {
		contents = append(contents, geminiContent{
			Role:  "user",
			Parts: []geminiPart{{Text: ""}},
		})
	}

	gemReq := geminiRequest{
		Contents: contents,
	}

	if len(systemParts) > 0 {
		gemReq.SystemInstruction = &geminiContent{
			Role:  "user",
			Parts: systemParts,
		}
	}

	maxTokens := defaultGeminiMaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	gemReq.GenerationConfig.MaxOutputTokens = maxTokens

	if req.Temperature != nil {
		gemReq.GenerationConfig.Temperature = req.Temperature
	}

	return gemReq
}

// geminiParts builds the []geminiPart slice for a single ChatMessage.
// For text-only messages it returns a single text part.
// For multimodal messages it maps text blocks → text parts and
// image_url blocks → file_data parts (using the image URL as the file URI).
func geminiParts(msg ChatMessage) []geminiPart {
	if len(msg.ContentBlocks) == 0 {
		return []geminiPart{{Text: msg.Content}}
	}
	var parts []geminiPart
	for _, cb := range msg.ContentBlocks {
		switch cb.Type {
		case "text":
			if cb.Text != "" {
				parts = append(parts, geminiPart{Text: cb.Text})
			}
		case "image_url":
			if cb.ImageURL != nil && cb.ImageURL.URL != "" {
				parts = append(parts, geminiPart{
					FileData: &geminiFileData{FileURI: cb.ImageURL.URL},
				})
			}
		}
	}
	if len(parts) == 0 {
		parts = []geminiPart{{Text: ""}}
	}
	return parts
}

// translateResponse converts a Gemini generateContent response to an OpenAI-style ChatResponse.
func (g *Gemini) translateResponse(model string, resp geminiResponse) *ChatResponse {
	var content string
	finishReason := "stop"

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		for _, part := range candidate.Content.Parts {
			content += part.Text
		}

		// Map Gemini finishReason to OpenAI finish_reason
		switch candidate.FinishReason {
		case "STOP":
			finishReason = "stop"
		case "MAX_TOKENS":
			finishReason = "length"
		case "SAFETY":
			finishReason = "content_filter"
		default:
			finishReason = "stop"
		}
	}

	// Use token counts from response if available, otherwise estimate
	promptTokens := resp.UsageMetadata.PromptTokenCount
	completionTokens := resp.UsageMetadata.CandidatesTokenCount
	totalTokens := resp.UsageMetadata.TotalTokenCount

	if totalTokens == 0 && (promptTokens > 0 || completionTokens > 0) {
		totalTokens = promptTokens + completionTokens
	}
	if totalTokens == 0 {
		// Estimate: ~4 chars per token
		promptTokens = len(content) / 4
		completionTokens = len(content) / 4
		totalTokens = promptTokens + completionTokens
	}

	return &ChatResponse{
		ID:      fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatChoice{
			{
				Index:        0,
				Message:      ChatMessage{Role: "assistant", Content: content},
				FinishReason: finishReason,
			},
		},
		Usage: Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		},
	}
}

func mapGeminiFinishReason(r string) string {
	switch r {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	default:
		return "stop"
	}
}
