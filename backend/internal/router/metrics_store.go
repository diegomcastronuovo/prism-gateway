package router

import (
	"context"
	"sync"
)

// ErrorStats holds aggregated error counters for a (tenant, model) pair.
type ErrorStats struct {
	RequestCount int
	ErrorCount   int
	ErrorRate    float64
}

// MetricsStore abstracts the storage backend for smart routing metrics
// (EWMA latency + request/error counters) per (tenant, model).
// Implementations: InMemoryMetricsStore (default), RedisMetricsStore.
type MetricsStore interface {
	// UpdateLatencyEWMA applies an EWMA update for the given model's latency.
	UpdateLatencyEWMA(ctx context.Context, tenantID, model string, latencyMs float64) error

	// IncRequest increments the request counter and, if isError is true,
	// also increments the error counter.
	IncRequest(ctx context.Context, tenantID, model string, isError bool) error

	// GetLatencyEWMA returns the current EWMA latency for each requested model.
	// Models with no recorded data are absent from the returned map.
	GetLatencyEWMA(ctx context.Context, tenantID string, models []string) (map[string]float64, error)

	// GetErrorStats returns error statistics for each requested model.
	// Models with no recorded data are absent from the returned map.
	GetErrorStats(ctx context.Context, tenantID string, models []string) (map[string]ErrorStats, error)
}

// ---------- InMemoryMetricsStore ----------

type counterStats struct {
	req int
	err int
}

// InMemoryMetricsStore is the default MetricsStore implementation.
// It is goroutine-safe and lives within a single process.
type InMemoryMetricsStore struct {
	mu    sync.RWMutex
	ewma  map[string]map[string]float64       // tenant -> model -> ewma ms
	stats map[string]map[string]*counterStats // tenant -> model -> counters
	alpha float64
}

// NewInMemoryMetricsStore creates an in-process MetricsStore.
func NewInMemoryMetricsStore() *InMemoryMetricsStore {
	return &InMemoryMetricsStore{
		ewma:  make(map[string]map[string]float64),
		stats: make(map[string]map[string]*counterStats),
		alpha: ewmaAlpha,
	}
}

func (s *InMemoryMetricsStore) UpdateLatencyEWMA(_ context.Context, tenantID, model string, latencyMs float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ewma[tenantID] == nil {
		s.ewma[tenantID] = make(map[string]float64)
	}
	prev, exists := s.ewma[tenantID][model]
	if !exists {
		s.ewma[tenantID][model] = latencyMs
	} else {
		s.ewma[tenantID][model] = s.alpha*latencyMs + (1-s.alpha)*prev
	}
	return nil
}

func (s *InMemoryMetricsStore) IncRequest(_ context.Context, tenantID, model string, isError bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stats[tenantID] == nil {
		s.stats[tenantID] = make(map[string]*counterStats)
	}
	cs := s.stats[tenantID][model]
	if cs == nil {
		cs = &counterStats{}
		s.stats[tenantID][model] = cs
	}

	cs.req++
	if isError {
		cs.err++
	}

	// Decay counters after 100 requests to avoid stale signal.
	if cs.req > 100 {
		cs.req = 50
		cs.err = int(float64(cs.err) * 0.5)
	}
	return nil
}

func (s *InMemoryMetricsStore) GetLatencyEWMA(_ context.Context, tenantID string, models []string) (map[string]float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]float64)
	tenant := s.ewma[tenantID]
	if tenant == nil {
		return result, nil
	}
	for _, model := range models {
		if v, ok := tenant[model]; ok {
			result[model] = v
		}
	}
	return result, nil
}

func (s *InMemoryMetricsStore) GetErrorStats(_ context.Context, tenantID string, models []string) (map[string]ErrorStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]ErrorStats)
	tenant := s.stats[tenantID]
	if tenant == nil {
		return result, nil
	}
	for _, model := range models {
		cs, ok := tenant[model]
		if !ok {
			continue
		}
		errorRate := 0.0
		if cs.req > 0 {
			errorRate = float64(cs.err) / float64(cs.req)
		}
		result[model] = ErrorStats{
			RequestCount: cs.req,
			ErrorCount:   cs.err,
			ErrorRate:    errorRate,
		}
	}
	return result, nil
}
