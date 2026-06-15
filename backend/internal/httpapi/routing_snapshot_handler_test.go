package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// snapshotTestConfig returns a minimal config for routing snapshot tests.
func snapshotTestConfig() *config.Config {
	cfg := testConfig()
	cfg.Tenants[0].Routing.Strategy = "round_robin"
	return cfg
}

func snapshotTestRegistry() *providers.Registry {
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))
	return reg
}

// makeChatRequest performs a POST /v1/chat/completions and returns the response recorder.
func makeChatRequest(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	return makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
}

// TestRoutingSnapshot_SavedInRequestLog verifies that a successful chat completion
// results in a RequestLog row with a non-nil RoutingSnapshot.
func TestRoutingSnapshot_SavedInRequestLog(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()
	handler := setupTestServerWithStorage(cfg, snapshotTestRegistry(), store)

	w := makeChatRequest(t, handler)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Give async log goroutine time to complete
	time.Sleep(50 * time.Millisecond)

	requests := store.Requests()
	if len(requests) == 0 {
		t.Fatal("expected at least one RequestLog row")
	}

	// Find the success row
	var found bool
	for _, rl := range requests {
		if rl.Status == "ok" {
			if rl.RoutingSnapshot == nil {
				t.Error("RoutingSnapshot is nil on success row")
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no success RequestLog row found")
	}
}

// TestRoutingSnapshot_IncludesModelAndProvider verifies that the RoutingSnapshot
// in the request log contains non-empty selected_model and provider fields.
func TestRoutingSnapshot_IncludesModelAndProvider(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()
	handler := setupTestServerWithStorage(cfg, snapshotTestRegistry(), store)

	w := makeChatRequest(t, handler)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	time.Sleep(50 * time.Millisecond)

	requests := store.Requests()
	var snapshotJSON json.RawMessage
	for _, rl := range requests {
		if rl.Status == "ok" && rl.RoutingSnapshot != nil {
			snapshotJSON = rl.RoutingSnapshot
			break
		}
	}
	if snapshotJSON == nil {
		t.Fatal("no success row with routing_snapshot found")
	}

	var snap router.RoutingSnapshot
	if err := json.Unmarshal(snapshotJSON, &snap); err != nil {
		t.Fatalf("failed to unmarshal routing snapshot: %v", err)
	}
	if snap.SelectedModel == "" {
		t.Error("routing snapshot: selected_model is empty")
	}
	if snap.Provider == "" {
		t.Error("routing snapshot: provider is empty")
	}
	if snap.RoutingStrategy == "" {
		t.Error("routing snapshot: routing_strategy is empty")
	}
	if len(snap.CandidateModels) == 0 {
		t.Error("routing snapshot: candidate_models is empty")
	}
}

// TestRoutingSnapshot_AdminGetEndpoint verifies GET /admin/requests/{id}/routing
// returns 200 with the routing_snapshot field when a snapshot exists.
func TestRoutingSnapshot_AdminGetEndpoint(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()
	handler := setupTestServerWithStorage(cfg, snapshotTestRegistry(), store)

	// Make a successful request to populate the store
	w := makeChatRequest(t, handler)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	time.Sleep(50 * time.Millisecond)

	// Find the requestID from the logged row
	requests := store.Requests()
	var requestID string
	for _, rl := range requests {
		if rl.Status == "ok" && rl.RoutingSnapshot != nil {
			requestID = rl.RequestID
			break
		}
	}
	if requestID == "" {
		t.Fatal("no success row with routing_snapshot found")
	}

	// Build the Handlers directly and call GetRoutingSnapshot
	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/admin/requests/%s/routing", requestID),
		nil)
	req.SetPathValue("request_id", requestID)

	rw := httptest.NewRecorder()
	h.GetRoutingSnapshot(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rw.Code, rw.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(rw.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := body["routing_snapshot"]; !ok {
		t.Error("response body missing 'routing_snapshot' key")
	}
	if _, ok := body["request_id"]; !ok {
		t.Error("response body missing 'request_id' key")
	}
	if _, ok := body["tenant_id"]; !ok {
		t.Error("response body missing 'tenant_id' key")
	}
}

// TestRoutingSnapshot_ReplayUsesStoredModel verifies POST /admin/replay/{id}?mode=deterministic
// returns 200 with the selected_model matching the stored snapshot value.
func TestRoutingSnapshot_ReplayUsesStoredModel(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()
	handler := setupTestServerWithStorage(cfg, snapshotTestRegistry(), store)

	// Make a successful request to populate the store
	w := makeChatRequest(t, handler)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	time.Sleep(50 * time.Millisecond)

	// Find the requestID and stored model from the logged row
	requests := store.Requests()
	var requestID string
	var storedModel string
	for _, rl := range requests {
		if rl.Status == "ok" && rl.RoutingSnapshot != nil {
			requestID = rl.RequestID
			var snap router.RoutingSnapshot
			if err := json.Unmarshal(rl.RoutingSnapshot, &snap); err == nil {
				storedModel = snap.SelectedModel
			}
			break
		}
	}
	if requestID == "" {
		t.Fatal("no success row with routing_snapshot found")
	}
	if storedModel == "" {
		t.Fatal("stored model is empty in snapshot")
	}

	// Build the Handlers directly and call ReplayRequest
	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/admin/replay/%s?mode=deterministic", requestID),
		strings.NewReader(""))
	req.SetPathValue("request_id", requestID)

	rw := httptest.NewRecorder()
	h.ReplayRequest(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rw.Code, rw.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(rw.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	var returnedModel string
	if err := json.Unmarshal(body["selected_model"], &returnedModel); err != nil {
		t.Fatalf("failed to parse selected_model: %v", err)
	}
	if returnedModel != storedModel {
		t.Errorf("replay selected_model = %q, want %q", returnedModel, storedModel)
	}
	if _, ok := body["routing_snapshot"]; !ok {
		t.Error("response body missing 'routing_snapshot' key")
	}
}

// TestRoutingSnapshot_OpenAIStyleID verifies that a non-UUID request ID
// (e.g. an OpenAI-style "chatcmpl-..." string) works end-to-end through
// the Go layer after migrating request_id from UUID to TEXT.
func TestRoutingSnapshot_OpenAIStyleID(t *testing.T) {
	const openAIStyleID = "chatcmpl-mock-1772923005"

	store := &fakeStorage{}
	cfg := snapshotTestConfig()

	// Seed fakeStorage with a request row using an OpenAI-style request ID.
	snapshotJSON, _ := (&router.RoutingSnapshot{
		SelectedModel:   "model-a",
		Provider:        "openai",
		RoutingStrategy: "round_robin",
		CandidateModels: []string{"model-a"},
	}).ToJSON()
	store.mu.Lock()
	store.requests = append(store.requests, storage.RequestLog{
		RequestID:       openAIStyleID,
		TenantID:        cfg.Tenants[0].ID,
		Status:          "ok",
		RoutingSnapshot: snapshotJSON,
	})
	store.mu.Unlock()

	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/admin/requests/%s/routing", openAIStyleID),
		nil)
	req.SetPathValue("request_id", openAIStyleID)

	rw := httptest.NewRecorder()
	h.GetRoutingSnapshot(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rw.Code, rw.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(rw.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := body["routing_snapshot"]; !ok {
		t.Error("response body missing 'routing_snapshot' key")
	}
}

// TestRoutingSnapshot_ResponseIDMatchesStoredRequestID verifies the end-to-end
// correlation: the "id" field in the /v1/chat/completions response must equal
// the request_log.request_id used by GET /admin/requests/{id}/routing.
// This is the key regression test for the UUID-vs-provider-ID mismatch bug.
func TestRoutingSnapshot_ResponseIDMatchesStoredRequestID(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()
	handler := setupTestServerWithStorage(cfg, snapshotTestRegistry(), store)

	// Make a successful request and capture the response body.
	w := makeChatRequest(t, handler)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Extract the "id" field from the API response body.
	var apiBody map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&apiBody); err != nil {
		t.Fatalf("decode api response: %v", err)
	}
	var responseID string
	if err := json.Unmarshal(apiBody["id"], &responseID); err != nil {
		t.Fatalf("unmarshal id: %v", err)
	}
	if responseID == "" {
		t.Fatal("api response has empty id field")
	}

	// Give the async log goroutine time to complete.
	time.Sleep(50 * time.Millisecond)

	// The stored request_log.request_id for the success row must equal responseID.
	requests := store.Requests()
	var storedID string
	for _, rl := range requests {
		if rl.Status == "ok" && rl.RoutingSnapshot != nil {
			storedID = rl.RequestID
			break
		}
	}
	if storedID == "" {
		t.Fatal("no success row found in store")
	}
	if storedID != responseID {
		t.Errorf("stored request_id %q != api response id %q — routing snapshot lookup would fail", storedID, responseID)
	}

	// Also verify that the admin endpoint returns 200 when queried by responseID.
	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/admin/requests/%s/routing", responseID), nil)
	req.SetPathValue("request_id", responseID)
	rw := httptest.NewRecorder()
	h.GetRoutingSnapshot(rw, req)
	if rw.Code != http.StatusOK {
		t.Errorf("GetRoutingSnapshot returned %d using response id %q, want 200", rw.Code, responseID)
	}
}

// TestReplayRequest_ReturnsDiagnosticsWhenPresent verifies replay response includes
// decision_reason, decision_snapshot, route_group, routing_strategy, fallback_attempts.
func TestReplayRequest_ReturnsDiagnosticsWhenPresent(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()
	decisionReason := "explicit_body"
	snapshotJSON, _ := (&router.RoutingSnapshot{
		SelectedModel:    "model-a",
		Provider:         "openai",
		RoutingStrategy:  "smart",
		RouteGroup:       "default",
		FallbackAttempts: 0,
		CandidateModels:  []string{"model-a", "model-b"},
	}).ToJSON()
	decisionSnapshotJSON := []byte(`{"selected_provider":"openai","selected_model":"model-a","fallback_used":false,"reason":"explicit_body"}`)
	store.mu.Lock()
	store.requests = append(store.requests, storage.RequestLog{
		RequestID:       "chatcmpl-replay-diag",
		TenantID:        cfg.Tenants[0].ID,
		Status:          "ok",
		Strategy:        "smart",
		RoutingSnapshot: snapshotJSON,
		DecisionReason:  decisionReason,
		DecisionSnapshot: decisionSnapshotJSON,
	})
	store.mu.Unlock()

	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/replay/chatcmpl-replay-diag?mode=deterministic", nil)
	req.SetPathValue("request_id", "chatcmpl-replay-diag")
	rw := httptest.NewRecorder()
	h.ReplayRequest(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rw.Code, rw.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rw.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Existing fields
	if body["provider"] != "openai" || body["selected_model"] != "model-a" || body["tenant_id"] != cfg.Tenants[0].ID {
		t.Errorf("provider/selected_model/tenant_id: %v", body)
	}
	if body["routing_snapshot"] == nil {
		t.Error("routing_snapshot should be present")
	}
	// New diagnostics
	if body["decision_reason"] != "explicit_body" {
		t.Errorf("decision_reason=%v, want explicit_body", body["decision_reason"])
	}
	if body["decision_snapshot"] == nil {
		t.Error("decision_snapshot should be present when stored")
	}
	if body["route_group"] != "default" {
		t.Errorf("route_group=%v, want default", body["route_group"])
	}
	if body["routing_strategy"] != "smart" {
		t.Errorf("routing_strategy=%v, want smart", body["routing_strategy"])
	}
	if body["fallback_attempts"] != float64(0) {
		t.Errorf("fallback_attempts=%v, want 0", body["fallback_attempts"])
	}
}

// TestReplayRequest_ReturnsExplicitNullsWhenAbsent verifies replay response includes
// decision_reason, decision_snapshot, route_group, routing_strategy, fallback_attempts as null when absent.
func TestReplayRequest_ReturnsExplicitNullsWhenAbsent(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()
	// Minimal snapshot: no route_group, no decision_reason/decision_snapshot in row
	snapshotJSON, _ := (&router.RoutingSnapshot{
		SelectedModel:    "model-a",
		Provider:         "openai",
		RoutingStrategy:  "round_robin",
		RouteGroup:       "",
		FallbackAttempts: 0,
		CandidateModels:  []string{"model-a"},
	}).ToJSON()
	store.mu.Lock()
	store.requests = append(store.requests, storage.RequestLog{
		RequestID:       "chatcmpl-nulls",
		TenantID:        cfg.Tenants[0].ID,
		Status:          "ok",
		RoutingSnapshot: snapshotJSON,
		// DecisionReason and DecisionSnapshot left zero
	})
	store.mu.Unlock()

	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/replay/chatcmpl-nulls?mode=deterministic", nil)
	req.SetPathValue("request_id", "chatcmpl-nulls")
	rw := httptest.NewRecorder()
	h.ReplayRequest(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rw.Code, rw.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rw.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Keys must be present; null when absent
	for _, key := range []string{"decision_reason", "decision_snapshot", "route_group", "routing_strategy", "fallback_attempts"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response must include key %q (can be null)", key)
		}
	}
	if body["route_group"] != nil {
		t.Errorf("route_group should be null when empty, got %v", body["route_group"])
	}
	if body["decision_reason"] != nil {
		t.Errorf("decision_reason should be null when absent, got %v", body["decision_reason"])
	}
	// Existing fields still present
	if body["provider"] != "openai" || body["selected_model"] != "model-a" {
		t.Errorf("existing fields: provider=%v selected_model=%v", body["provider"], body["selected_model"])
	}
}

// TestReplayRequest_SmartRankingDetailsReturned verifies that when decision_snapshot.smart
// contains ranking_details, the replay response exposes them verbatim.
func TestReplayRequest_SmartRankingDetailsReturned(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()

	snapshotJSON, _ := (&router.RoutingSnapshot{
		SelectedModel:    "model-a",
		Provider:         "openai",
		RoutingStrategy:  "smart",
		RouteGroup:       "default",
		CandidateModels:  []string{"model-a", "model-b"},
	}).ToJSON()

	// Build a DecisionSnapshot with SmartDecision containing ranking_details.
	ds := router.DecisionSnapshot{
		Plan: []string{"model-a", "model-b"},
		Routing: router.RoutingDecision{Strategy: "smart"},
		Smart: &router.SmartDecision{
			EstimatedCostsUSD:    map[string]float64{"model-a": 0.0002, "model-b": 0.0003},
			BudgetPressure:       0.01,
			CostOptimizerApplied: true,
			EffectiveWeights:     map[string]float64{"cost": 0.7, "latency": 0.1, "errors": 0.2},
			EffectiveCostWeight:  0.7,
			RankingDetails: []router.SmartCandidateExplain{
				{
					Model:           "model-a",
					Raw:             router.SmartCandidateRaw{CostUSD: 0.0002, LatencyMs: 2000, ErrorRate: 0},
					Normalized:      router.SmartCandidateNormalized{Cost: 0.0, Latency: 0.0, Errors: 0.0},
					ScoreComponents: router.SmartCandidateScoreComponents{Cost: 0.7, Latency: 0.1, Errors: 0.2},
					FinalScore:      1.0,
					MetricSources:   map[string]string{"cost": "estimated", "latency": "default_no_history", "errors": "default_no_history"},
					UsedDefaults:    true,
				},
				{
					Model:           "model-b",
					Raw:             router.SmartCandidateRaw{CostUSD: 0.0003, LatencyMs: 2000, ErrorRate: 0},
					Normalized:      router.SmartCandidateNormalized{Cost: 1.0, Latency: 0.0, Errors: 0.0},
					ScoreComponents: router.SmartCandidateScoreComponents{Cost: 0.0, Latency: 0.1, Errors: 0.2},
					FinalScore:      0.3,
					MetricSources:   map[string]string{"cost": "estimated", "latency": "default_no_history", "errors": "default_no_history"},
					UsedDefaults:    true,
				},
			},
		},
		Precedence: router.PrecedenceDecision{PoolSize: 2},
	}
	dsJSON, _ := ds.ToJSON()

	store.mu.Lock()
	store.requests = append(store.requests, storage.RequestLog{
		RequestID:        "chatcmpl-ranking-details",
		TenantID:         cfg.Tenants[0].ID,
		Status:           "ok",
		Strategy:         "smart",
		RoutingSnapshot:  snapshotJSON,
		DecisionSnapshot: dsJSON,
	})
	store.mu.Unlock()

	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/replay/chatcmpl-ranking-details?mode=deterministic", nil)
	req.SetPathValue("request_id", "chatcmpl-ranking-details")
	rw := httptest.NewRecorder()
	h.ReplayRequest(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rw.Code, rw.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rw.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// decision_snapshot must be present
	ds2, ok := body["decision_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("decision_snapshot missing or wrong type: %T", body["decision_snapshot"])
	}

	smart, ok := ds2["smart"].(map[string]interface{})
	if !ok {
		t.Fatalf("decision_snapshot.smart missing or wrong type")
	}

	// effective_weights must be present
	ew, ok := smart["effective_weights"].(map[string]interface{})
	if !ok {
		t.Fatalf("effective_weights missing or wrong type")
	}
	if ew["cost"] != 0.7 {
		t.Errorf("effective_weights.cost = %v, want 0.7", ew["cost"])
	}

	// ranking_details must be present and ordered
	rd, ok := smart["ranking_details"].([]interface{})
	if !ok {
		t.Fatalf("ranking_details missing or wrong type: %T", smart["ranking_details"])
	}
	if len(rd) != 2 {
		t.Fatalf("ranking_details length = %d, want 2", len(rd))
	}

	first := rd[0].(map[string]interface{})
	if first["model"] != "model-a" {
		t.Errorf("ranking_details[0].model = %v, want model-a", first["model"])
	}
	if first["final_score"] != 1.0 {
		t.Errorf("ranking_details[0].final_score = %v, want 1.0", first["final_score"])
	}
	if first["used_defaults"] != true {
		t.Errorf("ranking_details[0].used_defaults = %v, want true", first["used_defaults"])
	}

	raw, ok := first["raw"].(map[string]interface{})
	if !ok {
		t.Fatalf("ranking_details[0].raw missing")
	}
	if raw["latency_ms"] != 2000.0 {
		t.Errorf("raw.latency_ms = %v, want 2000", raw["latency_ms"])
	}

	sources, ok := first["metric_sources"].(map[string]interface{})
	if !ok {
		t.Fatalf("ranking_details[0].metric_sources missing")
	}
	if sources["latency"] != "default_no_history" {
		t.Errorf("metric_sources.latency = %v, want default_no_history", sources["latency"])
	}
}

// TestReplayRequest_NonSmartHasNoRankingDetails verifies that non-smart routing replays
// do not fabricate ranking_details (the field is absent or null).
func TestReplayRequest_NonSmartHasNoRankingDetails(t *testing.T) {
	store := &fakeStorage{}
	cfg := snapshotTestConfig()

	snapshotJSON, _ := (&router.RoutingSnapshot{
		SelectedModel:    "model-a",
		Provider:         "openai",
		RoutingStrategy:  "round_robin",
		CandidateModels:  []string{"model-a"},
	}).ToJSON()

	// decision_snapshot without smart section (non-smart routing)
	dsJSON := []byte(`{"routing":{"strategy":"round_robin"},"plan":["model-a"],"precedence":{"pool_size":1,"requested_source":"none","requested_model":"","route_group":""}}`)

	store.mu.Lock()
	store.requests = append(store.requests, storage.RequestLog{
		RequestID:        "chatcmpl-non-smart",
		TenantID:         cfg.Tenants[0].ID,
		Status:           "ok",
		Strategy:         "round_robin",
		RoutingSnapshot:  snapshotJSON,
		DecisionSnapshot: dsJSON,
	})
	store.mu.Unlock()

	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/replay/chatcmpl-non-smart?mode=deterministic", nil)
	req.SetPathValue("request_id", "chatcmpl-non-smart")
	rw := httptest.NewRecorder()
	h.ReplayRequest(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rw.Code, rw.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rw.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	ds2, ok := body["decision_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("decision_snapshot missing or wrong type")
	}

	// smart section must be absent (nil) for non-smart routing
	if smart, exists := ds2["smart"]; exists && smart != nil {
		if smartMap, ok := smart.(map[string]interface{}); ok {
			if rd, exists := smartMap["ranking_details"]; exists && rd != nil {
				t.Errorf("non-smart replay must not have ranking_details, got %v", rd)
			}
		}
	}
	// existing fields intact
	if body["selected_model"] != "model-a" {
		t.Errorf("selected_model = %v, want model-a", body["selected_model"])
	}
}
