package httpapi

import (
	"context"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// TestAllAttemptsFailedSummaryRow verifies that when all fallback attempts fail,
// one request_log row is logged per attempt (attempt >= 1) with real model/provider/latency.
// No summary row with attempt=0 is written; request_log is post-execution only.
func TestAllAttemptsFailedSummaryRow(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].Routing.Fallback.MaxAttempts = 2

	store := newFakeStorage()
	reg := providers.NewRegistry()

	// Register failing providers for both models
	reg.Register("openai", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.UpstreamError{StatusCode: 500, Body: "internal error"}
		},
	})
	reg.Register("backup", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.UpstreamError{StatusCode: 500, Body: "internal error"}
		},
	})

	// Use the standard server setup (includes auth middleware)
	handler := setupTestServerWithStorage(cfg, reg, store)

	// Make request that will fail all attempts
	body := `{"model":"model-a","messages":[{"role":"user","content":"test"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	// Verify we got a 5xx error response (upstream errors propagate their status code)
	if w.Code < 500 || w.Code > 599 {
		t.Errorf("expected 5xx, got %d: %s", w.Code, w.Body.String())
	}

	// Verify logged requests: only per-attempt rows (no summary row)
	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.requests) != 2 {
		t.Fatalf("expected 2 log entries (one per attempt), got %d", len(store.requests))
	}

	// Verify attempt 1
	attempt1 := store.requests[0]
	if attempt1.Attempt != 1 {
		t.Errorf("attempt 1: expected Attempt=1, got %d", attempt1.Attempt)
	}
	if attempt1.Model != "model-a" {
		t.Errorf("attempt 1: expected Model='model-a', got '%s'", attempt1.Model)
	}
	if attempt1.Status != "error" {
		t.Errorf("attempt 1: expected Status='error', got '%s'", attempt1.Status)
	}
	if attempt1.ErrorType != "upstream_5xx" {
		t.Errorf("attempt 1: expected ErrorType='upstream_5xx', got '%s'", attempt1.ErrorType)
	}
	if len(attempt1.DecisionSnapshot) == 0 {
		t.Error("attempt 1: DecisionSnapshot must not be NULL")
	}

	// Verify attempt 2
	attempt2 := store.requests[1]
	if attempt2.Attempt != 2 {
		t.Errorf("attempt 2: expected Attempt=2, got %d", attempt2.Attempt)
	}
	if attempt2.Model != "model-b" {
		t.Errorf("attempt 2: expected Model='model-b', got '%s'", attempt2.Model)
	}
	if attempt2.Status != "error" {
		t.Errorf("attempt 2: expected Status='error', got '%s'", attempt2.Status)
	}
	if len(attempt2.DecisionSnapshot) == 0 {
		t.Error("attempt 2: DecisionSnapshot must not be NULL")
	}

	// Both rows must share the same request_id
	if attempt1.RequestID != attempt2.RequestID {
		t.Error("both rows must share the same request_id")
	}
}

// TestDecisionSnapshotPresentOnSuccess verifies that a successful single-attempt request
// logs a row with attempt=1 and a non-null decision_snapshot.
func TestDecisionSnapshotPresentOnSuccess(t *testing.T) {
	cfg := testConfig()

	store := newFakeStorage()
	reg := providers.NewRegistry()

	reg.Register("openai", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID: "resp-1", Object: "chat.completion", Created: 1234567890, Model: req.Model,
				Choices: []providers.ChatChoice{{Index: 0, Message: providers.ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
				Usage:   providers.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
			}, nil
		},
	})
	reg.Register("backup", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.UpstreamError{StatusCode: 500, Body: "internal error"}
		},
	})

	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.requests) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.requests))
	}

	row := store.requests[0]
	if row.Attempt != 1 {
		t.Errorf("expected Attempt=1, got %d", row.Attempt)
	}
	if row.Status != "ok" {
		t.Errorf("expected Status='ok', got '%s'", row.Status)
	}
	if len(row.DecisionSnapshot) == 0 {
		t.Error("success row: DecisionSnapshot must not be NULL")
	}
}

// TestDecisionSnapshotPresentOnFallbackSuccess verifies that both the failed first
// attempt and the successful fallback attempt carry a non-null decision_snapshot.
func TestDecisionSnapshotPresentOnFallbackSuccess(t *testing.T) {
	cfg := testConfig()

	store := newFakeStorage()
	reg := providers.NewRegistry()

	reg.Register("openai", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.UpstreamError{StatusCode: 500, Body: "internal error"}
		},
	})
	reg.Register("backup", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID: "resp-2", Object: "chat.completion", Created: 1234567890, Model: req.Model,
				Choices: []providers.ChatChoice{{Index: 0, Message: providers.ChatMessage{Role: "assistant", Content: "fallback ok"}, FinishReason: "stop"}},
				Usage:   providers.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
			}, nil
		},
	})

	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != 200 {
		t.Fatalf("expected 200 (fallback), got %d: %s", w.Code, w.Body.String())
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.requests) != 2 {
		t.Fatalf("expected 2 log entries (1 error + 1 ok), got %d", len(store.requests))
	}

	// Attempt 1: failed, but must have snapshot
	row1 := store.requests[0]
	if row1.Attempt != 1 {
		t.Errorf("row 1: expected Attempt=1, got %d", row1.Attempt)
	}
	if row1.Status != "error" {
		t.Errorf("row 1: expected Status='error', got '%s'", row1.Status)
	}
	if len(row1.DecisionSnapshot) == 0 {
		t.Error("row 1 (failed attempt): DecisionSnapshot must not be NULL")
	}

	// Attempt 2: success fallback, must have snapshot
	row2 := store.requests[1]
	if row2.Attempt != 2 {
		t.Errorf("row 2: expected Attempt=2, got %d", row2.Attempt)
	}
	if row2.Status != "ok" {
		t.Errorf("row 2: expected Status='ok', got '%s'", row2.Status)
	}
	if len(row2.DecisionSnapshot) == 0 {
		t.Error("row 2 (fallback success): DecisionSnapshot must not be NULL")
	}
}

// TestPrecedenceErrorSnapshotNotNull verifies that when a request fails due to a
// precedence conflict (model not in route group + conflict_policy=error), HTTP 400
// is returned, the provider is not called, and SPEC_150 ensures one request_log
// row is written with status=error.
func TestPrecedenceErrorSnapshotNotNull(t *testing.T) {
	cfg := testConfig()

	// Configure route group "premium" containing only one model
	cfg.Tenants[0].Selection = config.SelectionConfig{
		HeaderModelKey:          "X-Model",
		HeaderRouteKey:          "X-Route-Group",
		AllowModelOverride:      true, // SPEC_151: explicitly enable for this conflict test
		AllowRouteGroupOverride: true,
		RouteGroups: map[string][]string{
			"premium": {"model-a"}, // model-b is NOT in premium
		},
		Precedence: config.PrecedenceConfig{
			Model:          "header",
			ConflictPolicy: "error", // conflict → 400, no upstream call
		},
	}

	store := newFakeStorage()
	reg := providers.NewRegistry()
	// Providers registered but never called — conflict is caught before routing
	reg.Register("openai", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			t.Error("provider must not be called on precedence error")
			return nil, nil
		},
	})
	reg.Register("backup", &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			t.Error("provider must not be called on precedence error")
			return nil, nil
		},
	})

	handler := setupTestServerWithStorage(cfg, reg, store)

	// Send: X-Route-Group=premium, X-Model=model-b (not in premium → conflict)
	body := `{"messages":[{"role":"user","content":"conflict group vs model"}]}`
	w := makeRequest(t, handler, body, map[string]string{
		"X-API-Key":     "key1",
		"X-Route-Group": "premium",
		"X-Model":       "model-b",
	})

	if w.Code != 400 {
		t.Fatalf("expected HTTP 400, got %d: %s", w.Code, w.Body.String())
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	// SPEC_150: routing errors are now logged. One row with status=error expected.
	if len(store.requests) != 1 {
		t.Fatalf("expected 1 request log row (SPEC_150: routing errors logged), got %d", len(store.requests))
	}
	row := store.requests[0]
	if row.Status != "error" {
		t.Errorf("expected Status='error', got %q", row.Status)
	}
	if row.TenantID == "" {
		t.Error("TenantID must not be empty")
	}
}

// newFakeStorage creates a fakeStorage instance for testing.
// The underlying fakeStorage type is defined in handlers_test.go.
func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		requests:    []storage.RequestLog{},
		usages:      []storage.UsageRecord{},
		budgetCheck: storage.BudgetCheck{},
	}
}
