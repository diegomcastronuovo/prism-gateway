package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// mlCapturingStore wraps NopStorage and captures the last LogRequest and SaveUsage calls.
type mlCapturingStore struct {
	storage.NopStorage
	lastRow         *storage.RequestLog
	lastUsageRecord *storage.UsageRecord
}

func (s *mlCapturingStore) LogRequest(_ context.Context, rl storage.RequestLog) error {
	s.lastRow = &rl
	return nil
}

func (s *mlCapturingStore) SaveUsage(_ context.Context, u storage.UsageRecord) error {
	s.lastUsageRecord = &u
	return nil
}

// mlTestHandlers builds a minimal Handlers for ML tests with the given models configured.
func mlTestHandlers(models []config.ModelConfig) *Handlers {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"local": {Type: "local"},
		},
		Models: models,
	}
	return &Handlers{
		cfg:              cfg,
		log:              testLogger(),
		store:            storage.NopStorage{},
		tenantCache:      config.NewTenantConfigCache(0),
		globalCfgCache:   config.NewGlobalConfigCache(0),
		errorClassifier:  router.NewErrorClassifier(),
	}
}

// mlTestHandlersWithStore is like mlTestHandlers but uses a provided store for capture.
func mlTestHandlersWithStore(models []config.ModelConfig, store storage.Storage) *Handlers {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{"local": {Type: "local"}},
		Models:    models,
	}
	return &Handlers{
		cfg:              cfg,
		log:              testLogger(),
		store:            store,
		tenantCache:      config.NewTenantConfigCache(0),
		globalCfgCache:   config.NewGlobalConfigCache(0),
		errorClassifier:  router.NewErrorClassifier(),
	}
}

// mlModelConfig builds an ML ModelConfig pointing at the given upstream URL.
func mlModelConfig(name, endpoint string) config.ModelConfig {
	return config.ModelConfig{
		Name:     name,
		Provider: "local",
		Type:     "ml",
		Execution: &config.MLExecutionConfig{
			Endpoint: endpoint,
			Protocol: "http",
		},
		Observable: &config.MLObservableConfig{
			Fields: []config.MLObservableField{
				{Path: "input.features", Type: "json", Role: "input"},
				{Path: "output.score", Type: "number", Role: "output"},
			},
		},
	}
}

// tenantCtx returns a context with a minimal tenant attached (no AllowedModels).
// Use tenantCtxAllowing for tests that reach the execution path.
func tenantCtx() func(*http.Request) *http.Request {
	tenant := &config.TenantConfig{ID: "test-tenant"}
	return func(r *http.Request) *http.Request {
		return r.WithContext(auth.WithTenant(r.Context(), tenant))
	}
}

// tenantCtxAllowing returns a context with a tenant that has modelName in AllowedModels.
func tenantCtxAllowing(modelName string) func(*http.Request) *http.Request {
	tenant := &config.TenantConfig{ID: "test-tenant", AllowedModels: []string{modelName}}
	return func(r *http.Request) *http.Request {
		return r.WithContext(auth.WithTenant(r.Context(), tenant))
	}
}

// --- Tests ---

// TestML_MissingModelNameHeader verifies 400 when X-Model-Name is absent.
func TestML_MissingModelNameHeader(t *testing.T) {
	h := mlTestHandlers(nil)
	withTenant := tenantCtx()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Model-Type", "ml")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, []byte(`{}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Message == "" {
		t.Error("expected error message")
	}
}

// TestML_ModelNotFound verifies 404 when the model does not exist in config.
func TestML_ModelNotFound(t *testing.T) {
	h := mlTestHandlers(nil) // no models
	withTenant := tenantCtx()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Model-Type", "ml")
	req.Header.Set("X-Model-Name", "nonexistent")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, []byte(`{}`))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestML_WrongType verifies 400 when model type is not "ml".
func TestML_WrongType(t *testing.T) {
	h := mlTestHandlers([]config.ModelConfig{
		{Name: "gpt-4o", Provider: "openai", Type: "chat"},
	})
	withTenant := tenantCtx()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Model-Type", "ml")
	req.Header.Set("X-Model-Name", "gpt-4o")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, []byte(`{}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestML_NoExecutionEndpoint verifies 400 when an ml model has no execution endpoint configured.
func TestML_NoExecutionEndpoint(t *testing.T) {
	h := mlTestHandlers([]config.ModelConfig{
		{Name: "bad-ml", Provider: "local", Type: "ml", Execution: nil},
	})
	withTenant := tenantCtx()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Model-Type", "ml")
	req.Header.Set("X-Model-Name", "bad-ml")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, []byte(`{}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestML_SuccessfulUpstreamCall verifies the body is forwarded and the response returned as-is.
func TestML_SuccessfulUpstreamCall(t *testing.T) {
	// Start a fake ML upstream.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"output":{"score":0.92}}`))
	}))
	defer upstream.Close()

	h := mlTestHandlers([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)})
	withTenant := tenantCtxAllowing("fraud-v1")

	body := []byte(`{"input":{"features":{"amount":1000,"country":"AR"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Type", "ml")
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	output, ok := resp["output"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected output object, got: %v", resp)
	}
	if score, ok := output["score"].(float64); !ok || score != 0.92 {
		t.Errorf("expected score=0.92, got %v", output["score"])
	}
	// X-Model-Name header must be set in response.
	if w.Header().Get("X-Model-Name") != "fraud-v1" {
		t.Errorf("expected X-Model-Name=fraud-v1, got %q", w.Header().Get("X-Model-Name"))
	}
}

// TestML_UpstreamPropagatesNon200 verifies non-200 upstream statuses are passed through.
func TestML_UpstreamPropagatesNon200(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":"invalid features"}`))
	}))
	defer upstream.Close()

	h := mlTestHandlers([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)})
	withTenant := tenantCtxAllowing("fraud-v1")

	body := []byte(`{"input":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Type", "ml")
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

// TestML_ChatCompletionsIntercept verifies that X-Model-Type=ml bypasses the LLM pipeline
// via the ChatCompletions handler entry point.
func TestML_ChatCompletionsIntercept(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"output":{"score":0.77}}`))
	}))
	defer upstream.Close()

	cfg := testConfig()
	cfg.Models = append(cfg.Models, mlModelConfig("fraud-v1", upstream.URL))
	// Allow tenant t1 to use the ML model.
	cfg.Tenants[0].AllowedModels = append(cfg.Tenants[0].AllowedModels, "fraud-v1")

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	reg.Register("backup", successProvider(""))
	handler := setupTestServer(cfg, reg)

	body := `{"input":{"features":{"amount":500}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "key1")
	req.Header.Set("X-Model-Type", "ml")
	req.Header.Set("X-Model-Name", "fraud-v1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["output"] == nil {
		t.Error("expected output field in ML response")
	}
}

// ── request_log persistence tests ────────────────────────────────────────────

// TestML_PersistsRequestLog_OnSuccess verifies that a successful ML request
// inserts a request_log row with status="ok" and populated observable metadata.
func TestML_PersistsRequestLog_OnSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"output":{"score":0.92}}`))
	}))
	defer upstream.Close()

	store := &mlCapturingStore{}
	h := mlTestHandlersWithStore([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)}, store)
	withTenant := tenantCtxAllowing("fraud-v1")

	body := []byte(`{"input":{"features":{"amount":1000}}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.lastRow == nil {
		t.Fatal("expected request_log row to be persisted, got nil")
	}
	row := store.lastRow
	if row.Status != "ok" {
		t.Errorf("status: want ok, got %q", row.Status)
	}
	if row.Model != "fraud-v1" {
		t.Errorf("model: want fraud-v1, got %q", row.Model)
	}
	if row.Provider != "local" {
		t.Errorf("provider: want local, got %q", row.Provider)
	}
	if row.Strategy != "ml" {
		t.Errorf("strategy: want ml, got %q", row.Strategy)
	}
	if row.TenantID != "test-tenant" {
		t.Errorf("tenant_id: want test-tenant, got %q", row.TenantID)
	}
	if row.Attempt != 1 {
		t.Errorf("attempt: want 1, got %d", row.Attempt)
	}
	if row.LatencyMs < 0 {
		t.Errorf("latency_ms must be non-negative, got %d", row.LatencyMs)
	}
	if row.Error != "" {
		t.Errorf("error must be empty on success, got %q", row.Error)
	}
	if row.Metadata == nil {
		t.Error("metadata must not be nil when observable fields are configured")
	} else {
		var meta map[string]interface{}
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			t.Fatalf("metadata is not valid JSON: %v", err)
		}
		obs, ok := meta["observable"].(map[string]interface{})
		if !ok {
			t.Fatalf("metadata.observable missing or not object: %v", meta)
		}
		if obs["input.features"] == nil {
			t.Error("expected metadata.observable to contain input.features")
		}
		if obs["output.score"] == nil {
			t.Error("expected metadata.observable to contain output.score")
		}
	}
}

// TestML_PersistsRequestLog_OnFailure verifies that a failed ML upstream call
// still inserts a request_log row with status="error".
func TestML_PersistsRequestLog_OnFailure(t *testing.T) {
	// Point to a port that is not listening so the upstream call fails.
	store := &mlCapturingStore{}
	model := config.ModelConfig{
		Name:     "fraud-v1",
		Provider: "local",
		Type:     "ml",
		Execution: &config.MLExecutionConfig{
			Endpoint: "http://127.0.0.1:19999", // unreachable
		},
		Observable: &config.MLObservableConfig{
			Fields: []config.MLObservableField{
				{Path: "input.features", Type: "json", Role: "input"},
			},
		},
	}
	h := mlTestHandlersWithStore([]config.ModelConfig{model}, store)
	withTenant := tenantCtxAllowing("fraud-v1")

	body := []byte(`{"input":{"features":{"amount":500}}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
	if store.lastRow == nil {
		t.Fatal("expected request_log row to be persisted on failure, got nil")
	}
	row := store.lastRow
	if row.Status != "error" {
		t.Errorf("status: want error, got %q", row.Status)
	}
	if row.Error == "" {
		t.Error("error field must be populated on failure")
	}
	if row.Model != "fraud-v1" {
		t.Errorf("model: want fraud-v1, got %q", row.Model)
	}
	if row.Strategy != "ml" {
		t.Errorf("strategy: want ml, got %q", row.Strategy)
	}
}

// TestBuildMLObservableMetadata_ExtractsConfiguredFields verifies that only configured
// fields are included in the metadata and the structure matches the spec.
func TestBuildMLObservableMetadata_ExtractsConfiguredFields(t *testing.T) {
	m := &config.ModelConfig{
		Name: "test",
		Observable: &config.MLObservableConfig{
			Fields: []config.MLObservableField{
				{Path: "input.features", Type: "json", Role: "input"},
				{Path: "output.score", Type: "number", Role: "output"},
			},
		},
	}
	reqBody := []byte(`{"input":{"features":{"amount":1000},"extra":"ignored"}}`)
	respBody := []byte(`{"output":{"score":0.92,"raw":"ignored"}}`)

	meta := buildMLObservableMetadata(m, reqBody, respBody)
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(meta, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	obs, ok := parsed["observable"].(map[string]interface{})
	if !ok {
		t.Fatalf("observable key missing: %v", parsed)
	}
	if obs["input.features"] == nil {
		t.Error("input.features should be present")
	}
	if obs["output.score"] == nil {
		t.Error("output.score should be present")
	}
	// The "extra" and "raw" fields must NOT be present.
	if obs["input.extra"] != nil || obs["output.raw"] != nil {
		t.Error("non-configured fields must not appear in metadata")
	}
}

// TestBuildMLObservableMetadata_NilObservable verifies nil is returned when
// no observable fields are configured.
func TestBuildMLObservableMetadata_NilObservable(t *testing.T) {
	m := &config.ModelConfig{Name: "test", Observable: nil}
	result := buildMLObservableMetadata(m, []byte(`{"x":1}`), []byte(`{"y":2}`))
	if result != nil {
		t.Errorf("expected nil for model with no observable config, got %s", result)
	}
}

// TestBuildMLObservableMetadata_MissingPathOmitted verifies that a path absent from
// the payload is silently omitted (not returned as null).
func TestBuildMLObservableMetadata_MissingPathOmitted(t *testing.T) {
	m := &config.ModelConfig{
		Name: "test",
		Observable: &config.MLObservableConfig{
			Fields: []config.MLObservableField{
				{Path: "output.missing", Type: "number", Role: "output"},
			},
		},
	}
	respBody := []byte(`{"output":{"score":0.5}}`) // "missing" is not in payload
	meta := buildMLObservableMetadata(m, nil, respBody)
	if meta != nil {
		// If all paths are missing, result should be nil (empty observable map → nil).
		t.Errorf("expected nil when all paths are missing, got %s", meta)
	}
}

// TestExtractJSONPath covers the path navigation helper.
func TestExtractJSONPath(t *testing.T) {
	data := map[string]interface{}{
		"input": map[string]interface{}{
			"features": map[string]interface{}{"amount": float64(1000)},
		},
		"output": map[string]interface{}{"score": 0.92},
		"flat":   "hello",
	}

	tests := []struct {
		path string
		want interface{}
	}{
		{"input.features", map[string]interface{}{"amount": float64(1000)}},
		{"output.score", 0.92},
		{"flat", "hello"},
		{"missing", nil},
		{"input.missing", nil},
		{"output.score.deep", nil}, // non-map leaf
	}

	for _, tc := range tests {
		got := extractJSONPath(data, tc.path)
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(tc.want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("path=%q: got %s, want %s", tc.path, gotJSON, wantJSON)
		}
	}
}

// ── tenant authorization tests ────────────────────────────────────────────────

// TestML_AuthZ_AllowedModel verifies that an ML request succeeds when the model
// is explicitly listed in tenant.allowed_models.
func TestML_AuthZ_AllowedModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"output":{"score":0.9}}`))
	}))
	defer upstream.Close()

	h := mlTestHandlers([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)})
	withTenant := tenantCtxAllowing("fraud-v1")

	body := []byte(`{"input":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestML_AuthZ_NilAllowedModels verifies that a tenant with allowed_models=nil
// cannot execute any ML model.
func TestML_AuthZ_NilAllowedModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called when authorization fails")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := mlTestHandlers([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)})
	// tenantCtx() produces a tenant with nil AllowedModels
	withTenant := tenantCtx()

	body := []byte(`{"input":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "authorization_error" {
		t.Errorf("expected type=authorization_error, got %q", resp.Error.Type)
	}
}

// TestML_AuthZ_ModelNotInAllowedList verifies that a tenant whose allowed_models
// does not include the requested model is rejected.
func TestML_AuthZ_ModelNotInAllowedList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called when authorization fails")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := mlTestHandlers([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)})
	// Tenant is allowed a different model, not fraud-v1
	withTenant := tenantCtxAllowing("other-model")

	body := []byte(`{"input":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "authorization_error" {
		t.Errorf("expected type=authorization_error, got %q", resp.Error.Type)
	}
}

// TestML_AuthZ_NoUpstreamCallOnReject verifies that the upstream is not called
// when tenant authorization fails.
func TestML_AuthZ_NoUpstreamCallOnReject(t *testing.T) {
	var upstreamCalled bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"output":{}}`))
	}))
	defer upstream.Close()

	h := mlTestHandlers([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)})
	withTenant := tenantCtx() // nil AllowedModels → should be rejected before upstream

	body := []byte(`{"input":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if upstreamCalled {
		t.Error("upstream must not be called when tenant authorization fails")
	}
}

// TestML_SavesUsageRecord_OnSuccess verifies that a successful ML request writes a
// usage record so the request is included in FinOps aggregation.
func TestML_SavesUsageRecord_OnSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"output":{"score":0.91}}`))
	}))
	defer upstream.Close()

	store := &mlCapturingStore{}
	h := mlTestHandlersWithStore([]config.ModelConfig{mlModelConfig("fraud-v1", upstream.URL)}, store)
	withTenant := tenantCtxAllowing("fraud-v1")

	body := []byte(`{"input":{"features":{"amount":500}}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.lastUsageRecord == nil {
		t.Fatal("expected SaveUsage to be called for ML request, got nil")
	}
	u := store.lastUsageRecord
	if u.Model != "fraud-v1" {
		t.Errorf("usage model: want fraud-v1, got %q", u.Model)
	}
	if u.Provider != "local" {
		t.Errorf("usage provider: want local, got %q", u.Provider)
	}
	if u.TenantID != "test-tenant" {
		t.Errorf("usage tenant_id: want test-tenant, got %q", u.TenantID)
	}
	// ML models have zero token cost — infra allocation computed separately.
	if u.PromptTokens != 0 || u.CompletionTokens != 0 || u.TotalTokens != 0 {
		t.Errorf("expected zero tokens for ML usage, got prompt=%d completion=%d total=%d",
			u.PromptTokens, u.CompletionTokens, u.TotalTokens)
	}
	if u.CostUSD != 0 {
		t.Errorf("expected zero cost_usd for ML usage, got %v", u.CostUSD)
	}
}

// TestML_NoUsageRecord_OnFailure verifies that a failed ML request does NOT write
// a usage record (consistent with LLM behavior).
func TestML_NoUsageRecord_OnFailure(t *testing.T) {
	store := &mlCapturingStore{}
	model := config.ModelConfig{
		Name:      "fraud-v1",
		Provider:  "local",
		Type:      "ml",
		Execution: &config.MLExecutionConfig{Endpoint: "http://localhost:1"}, // unreachable
	}
	h := mlTestHandlersWithStore([]config.ModelConfig{model}, store)
	withTenant := tenantCtxAllowing("fraud-v1")

	body := []byte(`{"input":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-Model-Name", "fraud-v1")
	req = withTenant(req)
	w := httptest.NewRecorder()

	h.handleMLRequest(w, req, body)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
	if store.lastUsageRecord != nil {
		t.Error("SaveUsage must NOT be called on ML failure")
	}
}
