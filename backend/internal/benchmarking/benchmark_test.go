package benchmarking

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeChatProvider struct {
	mu      sync.Mutex
	calls   int32
	handler func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error)
}

func (f *fakeChatProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.handler(ctx, req)
}

func (f *fakeChatProvider) ChatCompletionStream(_ context.Context, _ providers.ChatRequest) (*providers.StreamResponse, error) {
	return nil, providers.ErrStreamingNotSupported
}

func successProv() *fakeChatProvider {
	return &fakeChatProvider{
		handler: func(_ context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				Model: req.Model,
				Usage: providers.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
			}, nil
		},
	}
}

// fakeStore embeds NopStorage for all no-op methods and overrides only the
// three benchmarking methods that the tests actively exercise.
type fakeStore struct {
	storage.NopStorage
	mu               sync.Mutex
	benchmarks       []storage.ModelBenchmarkRow
	insertErr        error
	aggregates       []storage.BenchmarkAggregate
	deletedRows      int64
	deleteRetainDays int
}

func (s *fakeStore) InsertModelBenchmark(_ context.Context, row storage.ModelBenchmarkRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.insertErr != nil {
		return s.insertErr
	}
	s.benchmarks = append(s.benchmarks, row)
	return nil
}

func (s *fakeStore) GetModelBenchmarkAggregates(_ context.Context, _ int) ([]storage.BenchmarkAggregate, error) {
	return s.aggregates, nil
}

func (s *fakeStore) DeleteOldModelBenchmarks(_ context.Context, retainDays int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteRetainDays = retainDays
	return s.deletedRows, nil
}

// ─── config helpers ───────────────────────────────────────────────────────────

func benchCfg() *config.Config {
	return &config.Config{
		Benchmarking: config.BenchmarkingConfig{
			Enabled:         true,
			IntervalMinutes: 30,
			TimeoutMs:       5000,
			MaxConcurrency:  2,
			FailOpen:        true,
			Request: config.BenchmarkRequestConfig{
				Messages: []config.BenchmarkRequestMessage{
					{Role: "user", Content: "Say hello in one short sentence."},
				},
			},
			Storage: config.BenchmarkStorageConfig{RetainDays: 30},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai", Type: "chat",
				Pricing: config.Pricing{PromptPer1M: 0.15, CompletionPer1M: 0.60}},
			{Name: "embed-model", Provider: "openai", Type: "embedding"},
		},
	}
}

func registryWith(name string, prov providers.Provider) *providers.Registry {
	reg := providers.NewRegistry()
	reg.Register(name, prov)
	return reg
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ─── tests ────────────────────────────────────────────────────────────────────

// Test 1: scheduler starts and immediately runs one round of benchmarks.
func TestScheduler_StartsAndRuns(t *testing.T) {
	store := &fakeStore{}
	prov := successProv()
	reg := registryWith("openai", prov)
	cfg := benchCfg()
	cfg.Benchmarking.IntervalMinutes = 99999 // no repeated ticks during test

	sched := NewScheduler(cfg, store, reg, noopLogger(), nil, nil)
	sched.Start()
	defer sched.Stop()

	// Scheduler fires immediately; wait up to 2 s.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := len(store.benchmarks)
		store.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	store.mu.Lock()
	n := len(store.benchmarks)
	store.mu.Unlock()

	if n == 0 {
		t.Fatal("expected at least one benchmark row stored after scheduler started")
	}
}

// Test 2: benchmark results are stored with correct fields.
func TestRunner_ResultStored(t *testing.T) {
	store := &fakeStore{}
	prov := successProv()
	reg := registryWith("openai", prov)
	cfg := benchCfg()

	r := NewRunner(cfg, store, reg, noopLogger(), nil)
	r.Run(context.Background())

	store.mu.Lock()
	rows := store.benchmarks
	store.mu.Unlock()

	if len(rows) != 1 {
		t.Fatalf("expected 1 benchmark row, got %d", len(rows))
	}
	row := rows[0]
	if row.Model != "gpt-4o-mini" {
		t.Errorf("model: want gpt-4o-mini, got %q", row.Model)
	}
	if !row.Success {
		t.Error("expected success=true")
	}
	if row.LatencyMs < 0 {
		t.Errorf("latency_ms should be >= 0, got %d", row.LatencyMs)
	}
	if row.Provider != "openai" {
		t.Errorf("provider: want openai, got %q", row.Provider)
	}
	if row.PromptTokens != 5 {
		t.Errorf("prompt_tokens: want 5, got %d", row.PromptTokens)
	}
}

// Test 3: failed benchmark rows are stored with success=false and an error_type.
func TestRunner_FailedResultStored(t *testing.T) {
	store := &fakeStore{}
	prov := &fakeChatProvider{
		handler: func(_ context.Context, _ providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, fmt.Errorf("upstream connection refused")
		},
	}
	reg := registryWith("openai", prov)
	cfg := benchCfg()

	r := NewRunner(cfg, store, reg, noopLogger(), nil)
	r.Run(context.Background())

	store.mu.Lock()
	rows := store.benchmarks
	store.mu.Unlock()

	if len(rows) != 1 {
		t.Fatalf("expected 1 benchmark row, got %d", len(rows))
	}
	row := rows[0]
	if row.Success {
		t.Error("expected success=false for failed benchmark")
	}
	if row.ErrorType == "" {
		t.Error("expected non-empty error_type for failed benchmark")
	}
}

// Test 4: embedding-only models are skipped.
func TestRunner_EmbeddingModelsSkipped(t *testing.T) {
	store := &fakeStore{}
	var chatCalls int32
	prov := &fakeChatProvider{
		handler: func(_ context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			atomic.AddInt32(&chatCalls, 1)
			return &providers.ChatResponse{Model: req.Model}, nil
		},
	}
	reg := registryWith("openai", prov)

	cfg := benchCfg()
	cfg.Models = []config.ModelConfig{
		{Name: "embed-only", Provider: "openai", Type: "embedding"},
	}

	r := NewRunner(cfg, store, reg, noopLogger(), nil)
	r.Run(context.Background())

	if atomic.LoadInt32(&chatCalls) != 0 {
		t.Errorf("embedding model should not be benchmarked; got %d chat calls", chatCalls)
	}

	store.mu.Lock()
	n := len(store.benchmarks)
	store.mu.Unlock()
	if n != 0 {
		t.Errorf("expected 0 rows for embedding-only model, got %d", n)
	}
}

// Test 5: routing integration — aggregate data is accessible without breaking routing.
func TestBenchmarkAggregates_Readable(t *testing.T) {
	store := &fakeStore{
		aggregates: []storage.BenchmarkAggregate{
			{Model: "gpt-4o-mini", Provider: "openai", AvgLatencyMs: 350, SuccessRate: 0.99, Samples: 10},
		},
	}

	aggs, err := store.GetModelBenchmarkAggregates(context.Background(), 24)
	if err != nil {
		t.Fatalf("unexpected error from GetModelBenchmarkAggregates: %v", err)
	}
	if len(aggs) != 1 {
		t.Fatalf("expected 1 aggregate, got %d", len(aggs))
	}
	if aggs[0].Model != "gpt-4o-mini" {
		t.Errorf("unexpected model in aggregate: %q", aggs[0].Model)
	}
	if aggs[0].SuccessRate != 0.99 {
		t.Errorf("unexpected success_rate: %f", aggs[0].SuccessRate)
	}
}

// Test 6: benchmark failures fail open — runner does not panic or stop the gateway.
func TestRunner_FailOpen(t *testing.T) {
	store := &fakeStore{}
	prov := &fakeChatProvider{
		handler: func(_ context.Context, _ providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, fmt.Errorf("provider completely broken")
		},
	}
	reg := registryWith("openai", prov)
	cfg := benchCfg()

	r := NewRunner(cfg, store, reg, noopLogger(), nil)

	defer func() {
		if rec := recover(); rec != nil {
			t.Errorf("benchmark runner panicked: %v", rec)
		}
	}()
	r.Run(context.Background())

	// The failure row should be stored (fail-open means routing continues, not that failures are hidden).
	store.mu.Lock()
	n := len(store.benchmarks)
	store.mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 failure row (fail-open), got %d", n)
	}
}

// Test 7: retention cleanup is invoked with the configured retain_days.
func TestScheduler_RetentionCleanup(t *testing.T) {
	store := &fakeStore{deletedRows: 5}
	prov := successProv()
	reg := registryWith("openai", prov)
	cfg := benchCfg()
	cfg.Benchmarking.Storage.RetainDays = 7
	cfg.Benchmarking.IntervalMinutes = 99999

	sched := NewScheduler(cfg, store, reg, noopLogger(), nil, nil)
	sched.Start()
	defer sched.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		rd := store.deleteRetainDays
		store.mu.Unlock()
		if rd != 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	store.mu.Lock()
	rd := store.deleteRetainDays
	store.mu.Unlock()

	if rd != 7 {
		t.Errorf("expected retention cleanup called with retain_days=7, got %d", rd)
	}
}
