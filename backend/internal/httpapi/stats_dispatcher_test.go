package httpapi

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type countingStorage struct {
	storage.NopStorage
	upsertCount atomic.Int64
	mu          sync.Mutex
	stats       []storage.ModelStatDaily
}

func (s *countingStorage) UpsertModelStatDaily(ctx context.Context, stat storage.ModelStatDaily) error {
	s.upsertCount.Add(1)
	s.mu.Lock()
	s.stats = append(s.stats, stat)
	s.mu.Unlock()
	return nil
}

func TestStatsDispatcher_HighConcurrency_NoGoroutineLeak(t *testing.T) {
	store := &countingStorage{}
	log := testLogger()

	// Capture initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// Create dispatcher with small queue to test backpressure
	dispatcher := NewStatsDispatcher(store, log, 100, 2)

	// Submit many stats concurrently
	const numStats = 10000
	var wg sync.WaitGroup

	for i := 0; i < numStats; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			stat := storage.ModelStatDaily{
				Date:         time.Now().UTC(),
				TenantID:     "test-tenant",
				Model:        "test-model",
				SuccessCount: 1,
				AvgLatencyMs: 100.0,
				TotalCostUSD: 0.01,
			}
			dispatcher.Submit(stat)
		}(i)
	}

	wg.Wait()

	// Stop dispatcher and wait for flush
	dispatcher.Stop(5 * time.Second)

	// Check that processed + dropped = total submitted
	processed := store.upsertCount.Load()
	dropped := dispatcher.DroppedCount()
	total := processed + dropped

	t.Logf("Submitted: %d, Processed: %d, Dropped: %d", numStats, processed, dropped)

	if total < int64(numStats)*90/100 {
		t.Errorf("Too many stats lost: processed=%d, dropped=%d, expected~%d", processed, dropped, numStats)
	}

	// Allow some time for goroutines to clean up
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	// Allow small variance (typically +/- 2 goroutines due to runtime)
	if leaked > 5 {
		t.Errorf("Goroutine leak detected: initial=%d, final=%d, leaked=%d", initialGoroutines, finalGoroutines, leaked)
	}
}

func TestStatsDispatcher_Submit_NonBlocking(t *testing.T) {
	store := &countingStorage{}
	log := testLogger()

	// Create dispatcher with tiny queue
	dispatcher := NewStatsDispatcher(store, log, 2, 1)

	stat := storage.ModelStatDaily{
		Date:         time.Now().UTC(),
		TenantID:     "test",
		Model:        "test",
		SuccessCount: 1,
	}

	// Fill queue
	ok1 := dispatcher.Submit(stat)
	ok2 := dispatcher.Submit(stat)
	ok3 := dispatcher.Submit(stat)

	// Queue should be full now, next submit should fail immediately
	start := time.Now()
	ok4 := dispatcher.Submit(stat)
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("Submit blocked for %v, expected non-blocking", elapsed)
	}

	if ok1 && ok2 && ok3 && ok4 {
		t.Error("Expected at least one submit to fail due to full queue")
	}

	dispatcher.Stop(1 * time.Second)
}

func TestStatsDispatcher_SubmitBlocking_Timeout(t *testing.T) {
	store := &countingStorage{}
	log := testLogger()

	// Create dispatcher with tiny queue
	dispatcher := NewStatsDispatcher(store, log, 1, 1)

	stat := storage.ModelStatDaily{
		Date:         time.Now().UTC(),
		TenantID:     "test",
		Model:        "test",
		SuccessCount: 1,
	}

	// Fill queue
	dispatcher.Submit(stat)

	// This should timeout
	start := time.Now()
	ok := dispatcher.SubmitBlocking(stat, 50*time.Millisecond)
	elapsed := time.Since(start)

	if !ok {
		// Expected to fail
		if elapsed < 40*time.Millisecond || elapsed > 100*time.Millisecond {
			t.Errorf("Timeout duration unexpected: %v", elapsed)
		}
	}

	dispatcher.Stop(1 * time.Second)
}

func TestStatsDispatcher_GracefulShutdown(t *testing.T) {
	store := &countingStorage{}
	log := testLogger()

	dispatcher := NewStatsDispatcher(store, log, 1000, 2)

	// Submit some stats
	for i := 0; i < 100; i++ {
		stat := storage.ModelStatDaily{
			Date:         time.Now().UTC(),
			TenantID:     "test",
			Model:        "model",
			SuccessCount: 1,
		}
		dispatcher.Submit(stat)
	}

	// Stop and verify all items processed
	dispatcher.Stop(5 * time.Second)

	processed := store.upsertCount.Load()
	dropped := dispatcher.DroppedCount()

	if processed+dropped != 100 {
		t.Errorf("Expected 100 total, got processed=%d, dropped=%d", processed, dropped)
	}

	// Should not accept new submissions after stop
	stat := storage.ModelStatDaily{
		Date:         time.Now().UTC(),
		TenantID:     "test",
		Model:        "test",
		SuccessCount: 1,
	}
	ok := dispatcher.Submit(stat)
	if ok {
		t.Error("Expected Submit to fail after Stop")
	}
}
