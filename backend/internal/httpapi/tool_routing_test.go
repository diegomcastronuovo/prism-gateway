package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// toolRouteRegistry builds a registry with both chat and embedding providers.
func toolRouteRegistry() *providers.Registry {
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	reg.Register("backup", successProvider(""))
	reg.RegisterEmbedding("openai", fakeEmbeddingProvider{vec: fixedVector()})
	return reg
}

// Test 1: Tool route match short-circuits model routing.
func TestToolRoute_MatchAboveThreshold(t *testing.T) {
	cfg := routeTestConfig()
	match := &storage.SemanticRouteMatch{
		Name:       "invoice_parser",
		Action:     "parse_invoice",
		Similarity: 0.92,
		Threshold:  0.80,
	}
	store := &fakeStorage{semanticRouteMatch: match}
	reg := toolRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"parse this invoice"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Tool-Route"); got != "invoice_parser" {
		t.Errorf("X-Tool-Route: want invoice_parser, got %q", got)
	}
	if got := w.Header().Get("X-Tool-Action"); got != "parse_invoice" {
		t.Errorf("X-Tool-Action: want parse_invoice, got %q", got)
	}
	if got := w.Header().Get("X-Tool-Route-Similarity"); got == "" {
		t.Error("X-Tool-Route-Similarity header missing")
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Model != "tool-route" {
		t.Errorf("model: want tool-route, got %q", resp.Model)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	// Dynamic routing header must NOT be present (tool routing short-circuited first).
	if got := w.Header().Get("X-Dynamic-Route"); got != "" {
		t.Errorf("X-Dynamic-Route should be absent when tool routing fires, got %q", got)
	}
}

// Test 2: No-match continues to model routing.
func TestToolRoute_NoMatch_ContinuesNormal(t *testing.T) {
	cfg := routeTestConfig()
	store := &fakeStorage{} // semanticRouteMatch = nil → no routes
	reg := toolRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (normal routing), got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Tool-Route"); got != "" {
		t.Errorf("X-Tool-Route should be absent when no match, got %q", got)
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Model == "tool-route" {
		t.Error("model should not be tool-route when no route matched")
	}
}

// Test 3: Below-threshold continues to model routing.
func TestToolRoute_BelowThreshold_ContinuesNormal(t *testing.T) {
	cfg := routeTestConfig()
	match := &storage.SemanticRouteMatch{
		Name:       "weather",
		Action:     "get_weather",
		Similarity: 0.50,
		Threshold:  0.80,
	}
	store := &fakeStorage{semanticRouteMatch: match}
	reg := toolRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (normal routing), got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Tool-Route"); got != "" {
		t.Errorf("X-Tool-Route should be absent on below-threshold, got %q", got)
	}
}

// Test 4: DB/embedding failure fails open (normal routing continues).
func TestToolRoute_DBError_FailOpen(t *testing.T) {
	cfg := routeTestConfig()
	store := &fakeStorage{semanticRouteErr: errors.New("db down")}
	reg := toolRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open), got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Tool-Route"); got != "" {
		t.Errorf("X-Tool-Route should be absent on DB error (fail-open), got %q", got)
	}
}

// Test 5: Response format uses model="tool-route" with structured JSON content.
func TestToolRoute_ResponseFormat(t *testing.T) {
	cfg := routeTestConfig()
	match := &storage.SemanticRouteMatch{
		Name:       "crm_lookup",
		Action:     "lookup_customer",
		Similarity: 0.91,
		Threshold:  0.75,
	}
	store := &fakeStorage{semanticRouteMatch: match}
	reg := toolRouteRegistry()

	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"model":"model-a","messages":[{"role":"user","content":"find customer details"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Model != "tool-route" {
		t.Errorf("model: want tool-route, got %q", resp.Model)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("object: want chat.completion, got %q", resp.Object)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: want stop, got %q", resp.Choices[0].FinishReason)
	}

	// Content must be structured JSON: {"tool":"...","route":"...","similarity":...}
	content := resp.Choices[0].Message.TextContent()
	var toolResp map[string]interface{}
	if err := json.Unmarshal([]byte(content), &toolResp); err != nil {
		t.Fatalf("content should be JSON, got: %q, err: %v", content, err)
	}
	if toolResp["tool"] != "lookup_customer" {
		t.Errorf("tool: want lookup_customer, got %v", toolResp["tool"])
	}
	if toolResp["route"] != "crm_lookup" {
		t.Errorf("route: want crm_lookup, got %v", toolResp["route"])
	}
	if _, ok := toolResp["similarity"]; !ok {
		t.Error("similarity field missing from tool route response")
	}
}

// Test 6: Existing dynamic route CRUD endpoints continue working.
func TestToolRoute_DynamicRouteCRUD_StillWorks(t *testing.T) {
	store := &fakeStorage{}
	h := routeTestHandlers(store)

	body := `{"name":"weather","action":"get_weather","utterances":["what is the weather?"]}`
	rec := doCreateRoute(h, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateSemanticRoute: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp semanticRouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "weather" {
		t.Errorf("name: want weather, got %q", resp.Name)
	}
	if resp.Action != "get_weather" {
		t.Errorf("action: want get_weather, got %q", resp.Action)
	}
}
