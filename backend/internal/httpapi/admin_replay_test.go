package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

// replayTestConfig returns a config with two models and cost routing strategy.
func replayTestConfig() *config.Config {
	return &config.Config{
		Auth: config.AuthConfig{Mode: "api_key"},
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai", BaseURL: "http://fake", APIKeyEnv: ""},
			"backup": {Type: "openai", BaseURL: "http://fake2", APIKeyEnv: ""},
		},
		Models: []config.ModelConfig{
			{Name: "model-a", Provider: "openai", Pricing: config.Pricing{PromptPer1M: 1.0, CompletionPer1M: 2.0}},
			{Name: "model-b", Provider: "backup", Pricing: config.Pricing{PromptPer1M: 0.5, CompletionPer1M: 1.0}},
		},
		Tenants: []config.TenantConfig{
			{
				ID:            "t1",
				APIKeys:       []string{"key1"},
				AllowedModels: []string{"model-a", "model-b"},
				Routing: config.RoutingConfig{
					Strategy: "cost",
				},
			},
		},
	}
}

// replaySnapshot builds a routing snapshot JSON for a given model and candidate set.
func replaySnapshot(model string, candidates []string) json.RawMessage {
	snap := router.RoutingSnapshot{
		RoutingStrategy: "cost",
		CandidateModels: candidates,
		SelectedModel:   model,
		Provider:        "openai",
		Timestamp:       time.Now().UTC(),
	}
	b, _ := json.Marshal(snap)
	return json.RawMessage(b)
}

// replayRow builds a storage.ReplayRow for use in tests.
func replayRow(model string, promptTok, compTok int, costUSD float64, snap json.RawMessage) storage.ReplayRow {
	return storage.ReplayRow{
		RequestID:        uuid.New().String(),
		Timestamp:        time.Now().UTC(),
		TenantID:         "t1",
		Model:            model,
		Strategy:         "cost",
		PromptTokens:     promptTok,
		CompletionTokens: compTok,
		CostUSD:          costUSD,
		RoutingSnapshot:  snap,
	}
}

// replayHandlers constructs a Handlers struct for replay tests.
func replayHandlers(cfg *config.Config, store storage.Storage) *Handlers {
	return &Handlers{
		cfg:            cfg,
		store:          store,
		router:         router.New(),
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

// doTrafficReplay posts a TrafficReplayRequest to the handler and returns the recorder.
func doTrafficReplay(h *Handlers, body TrafficReplayRequest) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/traffic/replay", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.AdminTrafficReplay(w, req)
	return w
}

// TestTrafficReplay_NoProviderCalls verifies that traffic replay never calls any provider.
func TestTrafficReplay_NoProviderCalls(t *testing.T) {
	var callCount atomic.Int32
	countingProvider := &fakeProvider{
		handler: func(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
			callCount.Add(1)
			return nil, nil
		},
	}
	// Register counting providers (they must not be called during replay).
	reg := providers.NewRegistry()
	reg.Register("openai", countingProvider)
	reg.Register("backup", countingProvider)

	cfg := replayTestConfig()
	snap := replaySnapshot("model-a", []string{"model-a", "model-b"})
	store := &fakeStorage{
		replayRows: []storage.ReplayRow{
			replayRow("model-a", 100, 50, 0.001, snap),
			replayRow("model-a", 200, 100, 0.002, snap),
		},
	}
	h := replayHandlers(cfg, store)

	w := doTrafficReplay(h, TrafficReplayRequest{
		TenantID: "t1",
		Dataset:  "last_24h",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if n := callCount.Load(); n != 0 {
		t.Errorf("expected 0 provider calls during replay, got %d", n)
	}
}

// TestTrafficReplay_DetectsModelChange verifies that cost routing (cheaper model-b)
// is detected as a model change when replaying rows that originally used model-a.
func TestTrafficReplay_DetectsModelChange(t *testing.T) {
	cfg := replayTestConfig()
	// Tenant strategy is "cost" → model-b (cheaper) will be selected.
	snap := replaySnapshot("model-a", []string{"model-a", "model-b"})
	store := &fakeStorage{
		replayRows: []storage.ReplayRow{
			replayRow("model-a", 100, 50, 0.001, snap),
			replayRow("model-a", 200, 100, 0.002, snap),
		},
	}
	h := replayHandlers(cfg, store)

	w := doTrafficReplay(h, TrafficReplayRequest{
		TenantID: "t1",
		Dataset:  "last_24h",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TrafficReplayResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ChangedModels != 2 {
		t.Errorf("expected changed_models=2, got %d", resp.ChangedModels)
	}
	if resp.ModelChanges["model-a -> model-b"] != 2 {
		t.Errorf("expected model_changes['model-a -> model-b']=2, got %v", resp.ModelChanges)
	}
}

// TestTrafficReplay_CostDelta verifies that the cost delta is calculated correctly.
// model-a: 1.0/2.0 per 1M; model-b: 0.5/1.0 per 1M (half price).
// Row: 10000 prompt, 0 completion, original cost $0.01.
// With cost strategy, model-b is selected.
// New cost = 10000 * 0.5 / 1_000_000 = $0.005 → delta ≈ -0.005.
func TestTrafficReplay_CostDelta(t *testing.T) {
	cfg := replayTestConfig()
	snap := replaySnapshot("model-a", []string{"model-a", "model-b"})
	store := &fakeStorage{
		replayRows: []storage.ReplayRow{
			replayRow("model-a", 10_000, 0, 0.01, snap),
		},
	}
	h := replayHandlers(cfg, store)

	w := doTrafficReplay(h, TrafficReplayRequest{
		TenantID: "t1",
		Dataset:  "last_24h",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TrafficReplayResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// New cost = 10000 * 0.5 / 1e6 = $0.005; delta = 0.005 - 0.01 = -0.005
	const wantDelta = -0.005
	const tolerance = 1e-9
	if diff := resp.CostDeltaUSD - wantDelta; diff < -tolerance || diff > tolerance {
		t.Errorf("expected cost_delta_usd≈%.6f, got %.6f", wantDelta, resp.CostDeltaUSD)
	}
}

// TestTrafficReplay_LimitRespected verifies that the limit field caps the number of rows processed.
func TestTrafficReplay_LimitRespected(t *testing.T) {
	cfg := replayTestConfig()
	snap := replaySnapshot("model-a", []string{"model-a", "model-b"})
	rows := make([]storage.ReplayRow, 5)
	for i := range rows {
		rows[i] = replayRow("model-a", 100, 50, 0.001, snap)
	}
	store := &fakeStorage{replayRows: rows}
	h := replayHandlers(cfg, store)

	w := doTrafficReplay(h, TrafficReplayRequest{
		TenantID: "t1",
		Dataset:  "last_24h",
		Limit:    3,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TrafficReplayResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RequestsReplayed != 3 {
		t.Errorf("expected requests_replayed=3, got %d", resp.RequestsReplayed)
	}
}

// TestTrafficReplay_DatasetWindow verifies that an invalid dataset returns HTTP 400.
func TestTrafficReplay_DatasetWindow(t *testing.T) {
	cfg := replayTestConfig()
	h := replayHandlers(cfg, &fakeStorage{})

	w := doTrafficReplay(h, TrafficReplayRequest{
		TenantID: "t1",
		Dataset:  "bad_dataset",
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid dataset, got %d: %s", w.Code, w.Body.String())
	}
}
