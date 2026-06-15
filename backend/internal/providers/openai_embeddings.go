package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// openaiEmbedRequest is the wire format for POST /embeddings.
type openaiEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
	User  string   `json:"user,omitempty"`
}

// openaiEmbedData is one vector in the OpenAI embed response.
type openaiEmbedData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// openaiEmbedUsage is the usage block in the OpenAI embed response.
type openaiEmbedUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// openaiEmbedResponse is the full response from POST /embeddings.
type openaiEmbedResponse struct {
	Object string            `json:"object"`
	Data   []openaiEmbedData `json:"data"`
	Model  string            `json:"model"`
	Usage  openaiEmbedUsage  `json:"usage"`
}

// CreateEmbedding implements EmbeddingProvider for *OpenAI (and openai-compat providers).
func (o *OpenAI) CreateEmbedding(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	payload := openaiEmbedRequest{
		Model: req.Model,
		Input: req.Input,
		User:  req.User,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	url := o.openAICompat.baseURL + "/embeddings"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if o.openAICompat.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.openAICompat.apiKey)
	}

	resp, err := o.openAICompat.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       truncateBody(string(respBody), 500),
		}
	}

	var embedResp openaiEmbedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}

	out := &EmbeddingResponse{
		Object: embedResp.Object,
		Model:  embedResp.Model,
		Usage: EmbeddingUsage{
			PromptTokens: embedResp.Usage.PromptTokens,
			TotalTokens:  embedResp.Usage.TotalTokens,
		},
	}
	for _, d := range embedResp.Data {
		out.Data = append(out.Data, EmbeddingData{
			Object:    d.Object,
			Embedding: d.Embedding,
			Index:     d.Index,
		})
	}
	return out, nil
}
