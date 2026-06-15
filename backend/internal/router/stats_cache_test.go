package router

import (
	"context"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type fakeStatsStorage struct {
	storage.NopStorage
	stats []storage.ModelStatDaily
}

func (s *fakeStatsStorage) GetModelStats(ctx context.Context, tenantID string, windowDays int) ([]storage.ModelStatDaily, error) {
	return s.stats, nil
}

func TestModelStatsCache_CachesResults(t *testing.T) {
	store := &fakeStatsStorage{
		stats: []storage.ModelStatDaily{
			{
				Date:         time.Now().UTC(),
				TenantID:     "t1",
				Model:        "model-a",
				RequestCount: 100,
				SuccessCount: 95,
				ErrorCount:   5,
				AvgLatencyMs: 150.0,
			},
			{
				Date:         time.Now().UTC().AddDate(0, 0, -1),
				TenantID:     "t1",
				Model:        "model-a",
				RequestCount: 50,
				SuccessCount: 48,
				ErrorCount:   2,
				AvgLatencyMs: 140.0,
			},
		},
	}

	cache := NewModelStatsCache(store, 1*time.Second, 7)

	// First call - miss
	stats1, err := cache.GetStats(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats1) != 1 {
		t.Fatalf("expected 1 aggregated model, got %d", len(stats1))
	}

	agg := stats1["model-a"]
	if agg == nil {
		t.Fatal("expected model-a stats")
	}

	// Check aggregation
	if agg.RequestCount != 150 {
		t.Errorf("expected 150 total requests, got %d", agg.RequestCount)
	}

	expectedErrorRate := 7.0 / 150.0
	if agg.ErrorRate < expectedErrorRate-0.001 || agg.ErrorRate > expectedErrorRate+0.001 {
		t.Errorf("expected error rate ~%f, got %f", expectedErrorRate, agg.ErrorRate)
	}

	// Second call - should hit cache (same results)
	stats2, err := cache.GetStats(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats2) != len(stats1) {
		t.Error("cache should return same results")
	}
}

func TestModelStatsCache_Expiration(t *testing.T) {
	store := &fakeStatsStorage{
		stats: []storage.ModelStatDaily{
			{
				Date:         time.Now().UTC(),
				TenantID:     "t1",
				Model:        "model-a",
				RequestCount: 100,
				SuccessCount: 100,
				ErrorCount:   0,
				AvgLatencyMs: 150.0,
			},
		},
	}

	cache := NewModelStatsCache(store, 50*time.Millisecond, 7)

	// First call
	stats1, err := cache.GetStats(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats1) != 1 {
		t.Fatal("expected 1 model")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should re-fetch from DB
	stats2, err := cache.GetStats(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats2) != 1 {
		t.Fatal("expected 1 model after expiration")
	}
}

func TestModelStatsCache_Invalidate(t *testing.T) {
	store := &fakeStatsStorage{
		stats: []storage.ModelStatDaily{
			{
				Date:         time.Now().UTC(),
				TenantID:     "t1",
				Model:        "model-a",
				RequestCount: 100,
				SuccessCount: 100,
				ErrorCount:   0,
				AvgLatencyMs: 150.0,
			},
		},
	}

	cache := NewModelStatsCache(store, 1*time.Minute, 7)

	// Prime cache
	_, err := cache.GetStats(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	// Invalidate
	cache.Invalidate("t1")

	// Next call should re-fetch
	stats, err := cache.GetStats(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 1 {
		t.Fatal("expected stats after invalidation")
	}
}

func TestModelStatsCache_EmptyStats(t *testing.T) {
	store := &fakeStatsStorage{
		stats: []storage.ModelStatDaily{},
	}

	cache := NewModelStatsCache(store, 1*time.Second, 7)

	stats, err := cache.GetStats(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 0 {
		t.Errorf("expected empty stats, got %d", len(stats))
	}
}
