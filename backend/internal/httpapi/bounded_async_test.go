package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"golang.org/x/sync/semaphore"
)

// handlerWithSem creates a minimal Handlers with a semCacheAsync of the given capacity.
func handlerWithSem(cap int64, store storage.Storage) *Handlers {
	return &Handlers{
		semCacheAsync: semaphore.NewWeighted(cap),
		store:         store,
	}
}

// TestAsyncDrop_InsertCacheEntry_FullSemaphore verifies that when the semaphore is
// saturated the InsertSemanticCacheEntry goroutine is NOT spawned and the drop
// counter is incremented with operation="semantic_cache_write".
func TestAsyncDrop_InsertCacheEntry_FullSemaphore(t *testing.T) {
	store := &fakeStorage{}
	h := handlerWithSem(1, store)

	// Saturate the semaphore so TryAcquire fails.
	if !h.semCacheAsync.TryAcquire(1) {
		t.Fatal("expected to acquire semaphore in test setup")
	}

	before := testutil.ToFloat64(gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_write"))

	cacheEntry := storage.SemanticCacheInsert{
		TenantID: "t1",
		Model:    "model-a",
	}

	// Mirror the drop path from orchestrator_run.go Site B.
	if h.semCacheAsync.TryAcquire(1) {
		go func() {
			defer h.semCacheAsync.Release(1)
			_ = h.store.InsertSemanticCacheEntry(context.Background(), cacheEntry)
		}()
	} else {
		gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_write").Inc()
	}

	after := testutil.ToFloat64(gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_write"))
	if after-before != 1 {
		t.Errorf("expected drop counter +1, got +%.0f", after-before)
	}
	// Goroutine must NOT have been spawned — store inserts should be empty.
	store.mu.Lock()
	inserts := len(store.semCacheInserts)
	store.mu.Unlock()
	if inserts != 0 {
		t.Errorf("expected 0 cache inserts (goroutine should not have run), got %d", inserts)
	}

	h.semCacheAsync.Release(1)
}

// TestAsyncDrop_TouchCacheHit_FullSemaphore verifies that when the semaphore is
// saturated the TouchSemanticCacheHit goroutine is NOT spawned and the drop
// counter is incremented with operation="semantic_cache_touch".
func TestAsyncDrop_TouchCacheHit_FullSemaphore(t *testing.T) {
	store := &fakeStorage{}
	h := handlerWithSem(1, store)

	// Saturate the semaphore.
	if !h.semCacheAsync.TryAcquire(1) {
		t.Fatal("expected to acquire semaphore in test setup")
	}

	before := testutil.ToFloat64(gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_touch"))

	entryID := uuid.New()

	// Mirror the drop path from orchestrator_run.go Site A.
	if h.semCacheAsync.TryAcquire(1) {
		go func() {
			defer h.semCacheAsync.Release(1)
			h.store.TouchSemanticCacheHit(context.Background(), entryID, time.Now())
		}()
	} else {
		gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_touch").Inc()
	}

	after := testutil.ToFloat64(gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_touch"))
	if after-before != 1 {
		t.Errorf("expected drop counter +1, got +%.0f", after-before)
	}

	h.semCacheAsync.Release(1)
}

// TestAsyncDrop_InsertCacheEntry_SemaphoreAvailable verifies that when the semaphore
// is NOT saturated the goroutine is spawned and the drop counter is NOT incremented.
func TestAsyncDrop_InsertCacheEntry_SemaphoreAvailable(t *testing.T) {
	store := &fakeStorage{}
	h := handlerWithSem(1, store)

	before := testutil.ToFloat64(gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_write"))

	cacheEntry := storage.SemanticCacheInsert{
		TenantID: "t1",
		Model:    "model-a",
	}

	done := make(chan struct{})

	// Mirror the success path from orchestrator_run.go Site B.
	if h.semCacheAsync.TryAcquire(1) {
		go func() {
			defer h.semCacheAsync.Release(1)
			defer close(done)
			_ = h.store.InsertSemanticCacheEntry(context.Background(), cacheEntry)
		}()
	} else {
		gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_write").Inc()
		close(done)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete within timeout")
	}

	after := testutil.ToFloat64(gatewayotel.AsyncDropTotal.WithLabelValues("semantic_cache_write"))
	if after != before {
		t.Errorf("expected drop counter unchanged (no drop), but it changed by +%.0f", after-before)
	}
}
