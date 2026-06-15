package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CohereEmbedding implements EmbeddingProvider for the Cohere /v1/embed API.
type CohereEmbedding struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewCohereEmbedding creates a Cohere embedding provider.
func NewCohereEmbedding(baseURL, apiKey string) *CohereEmbedding {
	return &CohereEmbedding{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: newTransport(),
		},
	}
}

// cohereEmbedRequest is the wire format for POST /v1/embed.
type cohereEmbedRequest struct {
	Texts     []string `json:"texts"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"`
}

// cohereEmbedResponse is the response from Cohere /v1/embed.
type cohereEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// CreateEmbedding implements EmbeddingProvider.
func (c *CohereEmbedding) CreateEmbedding(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	payload := cohereEmbedRequest{
		Texts:     req.Input,
		Model:     req.Model,
		InputType: "search_document",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal cohere embedding request: %w", err)
	}

	url := c.baseURL + "/v1/embed"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create cohere embedding request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream cohere embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read cohere embedding response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       truncateBody(string(respBody), 500),
		}
	}

	var embedResp cohereEmbedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("unmarshal cohere embedding response: %w", err)
	}

	// Estimate token count; Cohere /v1/embed doesn't always return usage.
	totalPromptTokens := 0
	for _, text := range req.Input {
		totalPromptTokens += EstimateTokens(text)
	}

	var data []EmbeddingData
	for i, vec := range embedResp.Embeddings {
		data = append(data, EmbeddingData{
			Object:    "embedding",
			Embedding: vec,
			Index:     i,
		})
	}

	return &EmbeddingResponse{
		Object: "list",
		Data:   data,
		Model:  req.Model,
		Usage: EmbeddingUsage{
			PromptTokens: totalPromptTokens,
			TotalTokens:  totalPromptTokens,
		},
	}, nil
}
