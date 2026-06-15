package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// geminiEmbedContentRequest is the wire format for POST /models/{model}:embedContent.
type geminiEmbedContentRequest struct {
	Content geminiEmbedContent `json:"content"`
}

type geminiEmbedContent struct {
	Parts []geminiEmbedPart `json:"parts"`
}

type geminiEmbedPart struct {
	Text string `json:"text"`
}

// geminiEmbedContentResponse is the response from Gemini embedContent.
type geminiEmbedContentResponse struct {
	Embedding geminiEmbedValues `json:"embedding"`
}

type geminiEmbedValues struct {
	Values []float64 `json:"values"`
}

// CreateEmbedding implements EmbeddingProvider for *Gemini.
// Gemini embedContent only supports one input per call, so we loop sequentially.
func (g *Gemini) CreateEmbedding(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	var data []EmbeddingData
	totalPromptTokens := 0

	for i, text := range req.Input {
		payload := geminiEmbedContentRequest{
			Content: geminiEmbedContent{
				Parts: []geminiEmbedPart{{Text: text}},
			},
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal gemini embedding request: %w", err)
		}

		url := g.baseURL + "/models/" + req.Model + ":embedContent"

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create gemini embedding request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := g.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("upstream gemini embedding request failed: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read gemini embedding response body: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, &UpstreamError{
				StatusCode: resp.StatusCode,
				Body:       truncateBody(string(respBody), 500),
			}
		}

		var embedResp geminiEmbedContentResponse
		if err := json.Unmarshal(respBody, &embedResp); err != nil {
			return nil, fmt.Errorf("unmarshal gemini embedding response: %w", err)
		}

		data = append(data, EmbeddingData{
			Object:    "embedding",
			Embedding: embedResp.Embedding.Values,
			Index:     i,
		})

		// Gemini embed endpoint doesn't return token counts; estimate from input text.
		totalPromptTokens += EstimateTokens(text)
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
