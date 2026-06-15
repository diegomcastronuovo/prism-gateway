package providers

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// Sleeper provides an injectable sleep abstraction (following ratelimit Clock pattern).
type Sleeper interface {
	Sleep(d time.Duration)
}

// RandSource provides injectable randomness for deterministic testing.
type RandSource interface {
	Intn(n int) int
	Float64() float64
}

// Clock provides injectable time for deterministic timestamps.
type Clock interface {
	Now() time.Time
}

// Real implementations for production use.
type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

type realRandSource struct {
	r *rand.Rand
}

func (rr *realRandSource) Intn(n int) int    { return rr.r.Intn(n) }
func (rr *realRandSource) Float64() float64  { return rr.r.Float64() }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// MockProvider implements the Provider interface for load testing without hitting real APIs.
type MockProvider struct {
	config        config.MockConfig
	modelName     string
	tenantID      string
	pricing       config.Pricing
	sleeper       Sleeper
	randSource    RandSource
	clock         Clock
	actualDelayMs int // Store the actual delay used for logging
}

// NewMockProvider creates a mock provider with real dependencies.
// If seed is provided, the random source will be deterministic.
func NewMockProvider(cfg config.MockConfig, modelName, tenantID string, pricing config.Pricing, seed *int64) *MockProvider {
	var randSource RandSource
	if seed != nil {
		randSource = &realRandSource{r: rand.New(rand.NewSource(*seed))}
	} else {
		randSource = &realRandSource{r: rand.New(rand.NewSource(time.Now().UnixNano()))}
	}

	return &MockProvider{
		config:     cfg,
		modelName:  modelName,
		tenantID:   tenantID,
		pricing:    pricing,
		sleeper:    realSleeper{},
		randSource: randSource,
		clock:      realClock{},
	}
}

// NewMockProviderWithDeps creates a mock provider with injectable dependencies (for testing).
func NewMockProviderWithDeps(
	cfg config.MockConfig,
	modelName, tenantID string,
	pricing config.Pricing,
	sleeper Sleeper,
	randSource RandSource,
	clock Clock,
) *MockProvider {
	return &MockProvider{
		config:     cfg,
		modelName:  modelName,
		tenantID:   tenantID,
		pricing:    pricing,
		sleeper:    sleeper,
		randSource: randSource,
		clock:      clock,
	}
}

// GetActualDelay returns the last delay used (for logging/observability).
func (m *MockProvider) GetActualDelay() int {
	return m.actualDelayMs
}

// ChatCompletion implements the Provider interface with mock responses.
func (m *MockProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 1. Calculate and apply random delay
	delayMs := m.calculateDelay()
	m.actualDelayMs = delayMs
	m.sleeper.Sleep(time.Duration(delayMs) * time.Millisecond)

	// 2. Check for simulated error
	if m.config.ErrorRate > 0 && m.randSource.Float64() < m.config.ErrorRate {
		status := m.config.ErrorStatus
		if status == 0 {
			status = 500 // default to 500
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

	// 3. Generate response content
	content := m.generateContent(delayMs)

	// 4. Estimate tokens
	promptTokens := 0
	for _, msg := range req.Messages {
		promptTokens += EstimateTokens(msg.Content)
	}
	completionTokens := EstimateTokens(content)
	totalTokens := promptTokens + completionTokens

	// 5. Build response
	now := m.clock.Now()
	return &ChatResponse{
		ID:      fmt.Sprintf("chatcmpl-mock-%d", now.Unix()),
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   m.modelName,
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		},
	}, nil
}

// ChatCompletionStream is not supported for mock providers.
func (m *MockProvider) ChatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	return nil, &UpstreamError{
		StatusCode: 400,
		Body:       `{"error":{"type":"streaming_not_supported_for_mock","message":"Streaming is not supported for mock providers"}}`,
	}
}

// calculateDelay returns a random delay between DelayMinMs and DelayMaxMs.
func (m *MockProvider) calculateDelay() int {
	if m.config.DelayMinMs >= m.config.DelayMaxMs {
		return m.config.DelayMinMs
	}
	rangeMs := m.config.DelayMaxMs - m.config.DelayMinMs
	return m.config.DelayMinMs + m.randSource.Intn(rangeMs+1)
}

// generateContent creates the mock response content.
func (m *MockProvider) generateContent(delayMs int) string {
	if m.config.FixedResponse != "" {
		return m.config.FixedResponse
	}
	timestamp := m.clock.Now().Format(time.RFC3339)
	return fmt.Sprintf("Respuesta Mock del modelo %s, tenant %s, %s, delay=%dms",
		m.modelName, m.tenantID, timestamp, delayMs)
}
