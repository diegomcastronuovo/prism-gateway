package providers

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// Mock implementations for testing.
type mockSleeper struct {
	mu    sync.Mutex
	calls []time.Duration
}

func (m *mockSleeper) Sleep(d time.Duration) {
	m.mu.Lock()
	m.calls = append(m.calls, d)
	m.mu.Unlock()
}

func (m *mockSleeper) getCalls() []time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Duration(nil), m.calls...)
}

type mockRand struct {
	mu      sync.Mutex
	intIdx  int
	floatIdx int
	intVals  []int
	floatVals []float64
}

func (m *mockRand) Intn(n int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.intIdx >= len(m.intVals) {
		return 0
	}
	val := m.intVals[m.intIdx]
	m.intIdx++
	return val
}

func (m *mockRand) Float64() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.floatIdx >= len(m.floatVals) {
		return 0.0
	}
	val := m.floatVals[m.floatIdx]
	m.floatIdx++
	return val
}

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func TestMockProvider_DelayInRange(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 50,
		DelayMaxMs: 150,
	}

	sleeper := &mockSleeper{}
	// Return value that will give us 100ms delay: min(50) + rand(0..100)
	randSrc := &mockRand{intVals: []int{50}}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(
		cfg, "test-model", "test-tenant",
		config.Pricing{PromptPer1M: 1.0, CompletionPer1M: 2.0},
		sleeper, randSrc, clock,
	)

	req := ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	_, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := sleeper.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 sleep call, got %d", len(calls))
	}

	// Expected: 50 (min) + 50 (rand) = 100ms
	expected := 100 * time.Millisecond
	if calls[0] != expected {
		t.Errorf("expected sleep %v, got %v", expected, calls[0])
	}

	if provider.GetActualDelay() != 100 {
		t.Errorf("expected actualDelayMs=100, got %d", provider.GetActualDelay())
	}
}

func TestMockProvider_DeterministicWithSeed(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 10,
		DelayMaxMs: 100,
	}

	seed := int64(12345)
	provider1 := NewMockProvider(cfg, "model", "tenant", config.Pricing{}, &seed)
	provider2 := NewMockProvider(cfg, "model", "tenant", config.Pricing{}, &seed)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	resp1, err1 := provider1.ChatCompletion(context.Background(), req)
	resp2, err2 := provider2.ChatCompletion(context.Background(), req)

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}

	// Same seed should produce same delay
	if provider1.GetActualDelay() != provider2.GetActualDelay() {
		t.Errorf("same seed should produce same delay: %d vs %d",
			provider1.GetActualDelay(), provider2.GetActualDelay())
	}

	// Content includes delay, so should be identical
	content1 := resp1.Choices[0].Message.Content
	content2 := resp2.Choices[0].Message.Content
	if content1 != content2 {
		t.Errorf("same seed should produce identical content:\n%s\nvs\n%s", content1, content2)
	}
}

func TestMockProvider_ErrorRate(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:      true,
		DelayMinMs:   1,
		DelayMaxMs:   1,
		ErrorRate:    0.5,
		ErrorStatus:  503,
		ErrorMessage: "test error",
	}

	sleeper := &mockSleeper{}
	// First call: error (0.3 < 0.5), second call: success (0.7 >= 0.5)
	randSrc := &mockRand{
		intVals:   []int{0, 0},
		floatVals: []float64{0.3, 0.7},
	}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}

	// First call should error
	_, err := provider.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("expected UpstreamError, got %T", err)
	}
	if ue.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", ue.StatusCode)
	}
	if !strings.Contains(ue.Body, "test error") {
		t.Errorf("expected error message in body, got: %s", ue.Body)
	}

	// Second call should succeed
	resp, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

func TestMockProvider_FixedResponse(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:       true,
		DelayMinMs:    1,
		DelayMaxMs:    1,
		FixedResponse: "This is a fixed test response",
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{intVals: []int{0}}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	resp, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := resp.Choices[0].Message.Content
	if content != "This is a fixed test response" {
		t.Errorf("expected fixed response, got: %s", content)
	}
}

func TestMockProvider_DynamicResponseFormat(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 75,
		DelayMaxMs: 75,
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{intVals: []int{0}}
	clock := &mockClock{now: time.Date(2025, 1, 15, 14, 23, 45, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "gpt-4o-mini", "my-tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	}

	resp, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := resp.Choices[0].Message.Content
	if !strings.Contains(content, "Respuesta Mock del modelo gpt-4o-mini") {
		t.Errorf("content should include model name, got: %s", content)
	}
	if !strings.Contains(content, "tenant my-tenant") {
		t.Errorf("content should include tenant ID, got: %s", content)
	}
	if !strings.Contains(content, "2025-01-15T14:23:45Z") {
		t.Errorf("content should include timestamp, got: %s", content)
	}
	if !strings.Contains(content, "delay=75ms") {
		t.Errorf("content should include delay, got: %s", content)
	}
}

func TestMockProvider_TokenEstimation(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 1,
		DelayMaxMs: 1,
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{intVals: []int{0}}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model: "model",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are helpful"},      // ~4 words = ~4 tokens
			{Role: "user", Content: "What is the meaning of life?"}, // ~6 words = ~6 tokens
		},
	}

	resp, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Usage.PromptTokens == 0 {
		t.Error("prompt_tokens should be estimated")
	}
	if resp.Usage.CompletionTokens == 0 {
		t.Error("completion_tokens should be estimated")
	}
	if resp.Usage.TotalTokens != resp.Usage.PromptTokens+resp.Usage.CompletionTokens {
		t.Errorf("total_tokens (%d) should equal prompt (%d) + completion (%d)",
			resp.Usage.TotalTokens, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
}

func TestMockProvider_CostComputation(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 1,
		DelayMaxMs: 1,
	}

	pricing := config.Pricing{
		PromptPer1M:     0.15,
		CompletionPer1M: 0.60,
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{intVals: []int{0}}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", pricing, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test message here"}},
	}

	resp, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify we have non-zero token counts (cost computation happens in handler)
	if resp.Usage.PromptTokens == 0 || resp.Usage.CompletionTokens == 0 {
		t.Error("usage should have estimated tokens for cost computation")
	}

	// Verify response structure is complete
	if resp.ID == "" || resp.Model != "model" {
		t.Error("response should have proper metadata")
	}
}

func TestMockProvider_StreamingRejected(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 1,
		DelayMaxMs: 1,
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{intVals: []int{0}}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Stream:   true,
	}

	_, err := provider.ChatCompletionStream(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for streaming, got nil")
	}

	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("expected UpstreamError, got %T", err)
	}
	if ue.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", ue.StatusCode)
	}
	if !strings.Contains(ue.Body, "streaming_not_supported_for_mock") {
		t.Errorf("expected streaming error type in body, got: %s", ue.Body)
	}
}

func TestMockProvider_NoSleep(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 42,
		DelayMaxMs: 42,
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{intVals: []int{0}}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	_, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := sleeper.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 sleep call, got %d", len(calls))
	}
	if calls[0] != 42*time.Millisecond {
		t.Errorf("expected sleep(42ms), got sleep(%v)", calls[0])
	}
}

func TestMockProvider_Concurrency(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 1,
		DelayMaxMs: 10,
	}

	seed := int64(999)
	provider := NewMockProvider(cfg, "model", "tenant", config.Pricing{}, &seed)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := provider.ChatCompletion(context.Background(), req)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
}

func TestMockProvider_ErrorRateZero(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 1,
		DelayMaxMs: 1,
		ErrorRate:  0.0, // No errors
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{
		intVals:   []int{0},
		floatVals: []float64{0.5}, // Doesn't matter, ErrorRate is 0
	}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	resp, err := provider.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error with ErrorRate=0, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

func TestMockProvider_DefaultErrorStatus(t *testing.T) {
	cfg := config.MockConfig{
		Enabled:    true,
		DelayMinMs: 1,
		DelayMaxMs: 1,
		ErrorRate:  1.0, // Always error
		// ErrorStatus: 0 (not set, should default to 500)
	}

	sleeper := &mockSleeper{}
	randSrc := &mockRand{
		intVals:   []int{0},
		floatVals: []float64{0.0}, // < 1.0, triggers error
	}
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	provider := NewMockProviderWithDeps(cfg, "model", "tenant", config.Pricing{}, sleeper, randSrc, clock)

	req := ChatRequest{
		Model:    "model",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	_, err := provider.ChatCompletion(context.Background(), req)
	ue, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("expected UpstreamError, got %T", err)
	}
	if ue.StatusCode != 500 {
		t.Errorf("expected default status 500, got %d", ue.StatusCode)
	}
}
