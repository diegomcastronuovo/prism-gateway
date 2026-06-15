package providers

import (
	"context"
	"fmt"
	"time"
)

const mockEmbeddingDims = 1536

// CreateEmbedding implements EmbeddingProvider for *MockProvider.
// Returns zero vectors of dimension 1536 per input, respecting delay and error_rate.
func (m *MockProvider) CreateEmbedding(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	// 1. Calculate and apply random delay
	delayMs := m.calculateDelay()
	m.actualDelayMs = delayMs
	m.sleeper.Sleep(time.Duration(delayMs) * time.Millisecond)

	// 2. Check for simulated error
	if m.config.ErrorRate > 0 && m.randSource.Float64() < m.config.ErrorRate {
		status := m.config.ErrorStatus
		if status == 0 {
			status = 500
		}
		message := m.config.ErrorMessage
		if message == "" {
			message = "simulated mock error"
		}
		return nil, &UpstreamError{
			StatusCode: status,
			Body:       fmt.Sprintf(`{"error":{"type":"mock_error","message":"%s"}}`, message),
		}
	}

	// 3. Build zero-vector embeddings and count tokens
	totalPromptTokens := 0
	var data []EmbeddingData
	for i, text := range req.Input {
		totalPromptTokens += EstimateTokens(text)
		data = append(data, EmbeddingData{
			Object:    "embedding",
			Embedding: make([]float64, mockEmbeddingDims),
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
