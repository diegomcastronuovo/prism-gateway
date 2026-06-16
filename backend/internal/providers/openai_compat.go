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

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string          `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// openAICompat is the shared implementation for OpenAI-compatible APIs (OpenAI, xAI, local, etc.).
type openAICompat struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	streamClient *http.Client
}

func newOpenAICompat(baseURL, apiKey string) openAICompat {
	t := newTransport()
	return openAICompat{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: t,
		},
		streamClient: &http.Client{Transport: t},
	}
}

func (o *openAICompat) setHTTPClient(c *http.Client) {
	o.httpClient = c
	o.streamClient = &http.Client{Transport: c.Transport}
}

// responsesAPIBody is the wire format for POST /v1/responses (OpenAI Responses API).
type responsesAPIBody struct {
	Model           string          `json:"model"`
	Input           interface{}     `json:"input"`
	Tools           json.RawMessage `json:"tools,omitempty"`
	ToolChoice      json.RawMessage `json:"tool_choice,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
}

// responsesAPIRaw is the non-streaming response shape for POST /v1/responses.
type responsesAPIRaw struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

func mapResponsesRawToChat(raw responsesAPIRaw) *ChatResponse {
	var choices []ChatChoice
	for _, item := range raw.Output {
		if item.Type != "message" {
			continue
		}
		text := ""
		for _, c := range item.Content {
			if c.Type == "output_text" {
				text += c.Text
			}
		}
		choices = append(choices, ChatChoice{
			Message:      ChatMessage{Role: "assistant", Content: text},
			FinishReason: "stop",
		})
	}
	if choices == nil {
		choices = []ChatChoice{}
	}

	// Count tool usage from Responses API output items.
	toolCounts := make(map[string]int)
	for _, item := range raw.Output {
		switch item.Type {
		case "web_search_call":
			toolCounts["web_search"]++
		case "file_search_call":
			toolCounts["file_search"]++
		case "code_interpreter_call", "container_call":
			toolCounts["container"]++
		case "function_call":
			toolCounts["function"]++
		case "mcp_call":
			toolCounts["mcp"]++
		}
	}

	u := Usage{
		PromptTokens:      raw.Usage.InputTokens,
		CompletionTokens:  raw.Usage.OutputTokens,
		TotalTokens:       raw.Usage.TotalTokens,
		CachedInputTokens: raw.Usage.InputTokensDetails.CachedTokens,
	}
	if len(toolCounts) > 0 {
		u.ToolCallsUsed = toolCounts
	}

	return &ChatResponse{
		ID:      raw.ID,
		Object:  "chat.completion",
		Model:   raw.Model,
		Choices: choices,
		Usage:   u,
	}
}

func (o *openAICompat) responsesAPICompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(responsesAPIBody{
		Model:           req.Model,
		Input:           req.Messages,
		Tools:           req.Tools,
		ToolChoice:      req.ToolChoice,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal responses request: %w", err)
	}
	url := o.baseURL + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create responses request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream responses request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read responses body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: truncateBody(string(respBody), 500)}
	}
	var raw responsesAPIRaw
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal responses response: %w", err)
	}
	cr := mapResponsesRawToChat(raw)
	cr.RawBody = respBody
	return cr, nil
}

func (o *openAICompat) responsesAPIStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	body, err := json.Marshal(responsesAPIBody{
		Model:           req.Model,
		Input:           req.Messages,
		Tools:           req.Tools,
		ToolChoice:      req.ToolChoice,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal responses stream request: %w", err)
	}
	url := o.baseURL + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create responses stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	streamClient := o.streamClient
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream responses stream failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: truncateBody(string(errBody), 500)}
	}
	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		var currentEvent string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				select {
				case ch <- StreamEvent{Type: "done"}:
				case <-ctx.Done():
				}
				return
			}
			switch currentEvent {
			case "response.output_text.delta":
				var d struct {
					Delta string `json:"delta"`
				}
				if err := json.Unmarshal([]byte(data), &d); err == nil {
					select {
					case ch <- StreamEvent{Type: "delta", Content: d.Delta}:
					case <-ctx.Done():
						return
					}
				}
			case "response.completed":
				var completed struct {
					Response struct {
						Usage struct {
							InputTokens  int `json:"input_tokens"`
							OutputTokens int `json:"output_tokens"`
							TotalTokens  int `json:"total_tokens"`
							InputTokensDetails struct {
								CachedTokens int `json:"cached_tokens"`
							} `json:"input_tokens_details"`
						} `json:"usage"`
					} `json:"response"`
				}
				if err := json.Unmarshal([]byte(data), &completed); err == nil {
					u := &Usage{
						PromptTokens:      completed.Response.Usage.InputTokens,
						CompletionTokens:  completed.Response.Usage.OutputTokens,
						TotalTokens:       completed.Response.Usage.TotalTokens,
						CachedInputTokens: completed.Response.Usage.InputTokensDetails.CachedTokens,
					}
					select {
					case ch <- StreamEvent{Type: "done", Usage: u}:
					case <-ctx.Done():
					}
				}
				return
			}
		}
	}()
	return &StreamResponse{Events: ch}, nil
}

// chatCompletionStream sends a streaming chat completion request and returns
// chunks via a channel. The caller must drain the channel until it is closed.
// The upstream HTTP connection is kept open and read in a background goroutine;
// context cancellation will abort it.
func (o *openAICompat) chatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	if req.UseResponsesAPI {
		return o.responsesAPIStream(ctx, req)
	}
	req.Stream = true
	if req.ProviderModelID != "" {
		req.Model = req.ProviderModelID
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	// Use a transport-sharing client with no hard timeout; context controls deadline.
	streamClient := o.streamClient
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
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				select {
				case ch <- StreamEvent{Type: "done"}:
				case <-ctx.Done():
				}
				return
			}
			var raw openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &raw); err != nil {
				select {
				case ch <- StreamEvent{Type: "error", Error: err}:
				case <-ctx.Done():
				}
				return
			}
			ev := StreamEvent{Type: "delta"}
			if len(raw.Choices) > 0 {
				ev.Content = raw.Choices[0].Delta.Content
				ev.ToolCallsDelta = raw.Choices[0].Delta.ToolCalls
				ev.FinishReason = raw.Choices[0].FinishReason
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
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

func (o *openAICompat) chatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.UseResponsesAPI {
		return o.responsesAPICompletion(ctx, req)
	}
	req.Stream = false
	if req.ProviderModelID != "" {
		req.Model = req.ProviderModelID
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.httpClient.Do(httpReq)
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

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Parse cached input tokens from prompt_tokens_details if present.
	var tokDetails struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err2 := json.Unmarshal(respBody, &tokDetails); err2 == nil {
		chatResp.Usage.CachedInputTokens = tokDetails.Usage.PromptTokensDetails.CachedTokens
	}

	// Parse tool calls used (function calls from choices[0].message.tool_calls).
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		var toolCallsRaw []struct {
			Type string `json:"type"`
		}
		if err2 := json.Unmarshal(chatResp.Choices[0].Message.ToolCalls, &toolCallsRaw); err2 == nil {
			counts := make(map[string]int)
			for _, tc := range toolCallsRaw {
				t := tc.Type
				if t == "" {
					t = "function"
				}
				counts[t]++
			}
			if len(counts) > 0 {
				chatResp.Usage.ToolCallsUsed = counts
			}
		}
	}

	chatResp.RawBody = respBody
	return &chatResp, nil
}
