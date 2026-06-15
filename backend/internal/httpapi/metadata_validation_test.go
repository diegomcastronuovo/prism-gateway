package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// buildMetadataBody builds a JSON request body for /v1/chat/completions with optional metadata.
func buildMetadataBody(t *testing.T, metadata interface{}) string {
	t.Helper()
	type body struct {
		Model    string        `json:"model"`
		Messages []ChatMessage `json:"messages"`
		Metadata interface{}   `json:"metadata,omitempty"`
	}
	b, err := json.Marshal(body{
		Model:    "model-a",
		Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Metadata: metadata,
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(b)
}

func TestMetadata_Valid(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)
	body := buildMetadataBody(t, map[string]interface{}{
		"project":     "demo",
		"cost_center": "eng-001",
		"env":         "prod",
	})
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid metadata, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetadata_Omitted(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)
	// No metadata field — backward compatible
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 without metadata, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetadata_NonStringValue(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))

	handler := setupTestServer(cfg, reg)
	// value 123 is not a string
	body := `{"model":"model-a","messages":[{"role":"user","content":"hi"}],"metadata":{"key":123}}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-string value, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetadata_TooManyKeys(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))

	handler := setupTestServer(cfg, reg)

	// Build metadata with 21 keys
	meta := make(map[string]interface{}, 21)
	for i := 0; i < 21; i++ {
		meta[strings.Repeat("k", i+1)+"x"] = "v"
	}
	body := buildMetadataBody(t, meta)
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >20 keys, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetadata_KeyTooLong(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))

	handler := setupTestServer(cfg, reg)
	meta := map[string]interface{}{strings.Repeat("a", 65): "val"}
	body := buildMetadataBody(t, meta)
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for key >64 chars, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetadata_ValueTooLong(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))

	handler := setupTestServer(cfg, reg)
	meta := map[string]interface{}{"key": strings.Repeat("v", 257)}
	body := buildMetadataBody(t, meta)
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for value >256 chars, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetadata_TooBig(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))

	handler := setupTestServer(cfg, reg)
	// Build metadata where aggregate JSON > 4096 bytes (20 keys × 256 char values > 4KB)
	meta := make(map[string]interface{}, 20)
	for i := 0; i < 20; i++ {
		key := strings.Repeat("k", 10) + string(rune('a'+i))
		meta[key] = strings.Repeat("v", 256)
	}
	body := buildMetadataBody(t, meta)
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for metadata >4KB, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetadata_StoredInRequestLog(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := buildMetadataBody(t, map[string]interface{}{
		"project": "demo",
	})
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Find the success log row (status="ok")
	var found bool
	for _, req := range store.Requests() {
		if req.Status == "ok" {
			if req.Metadata == nil {
				t.Errorf("expected Metadata to be non-nil in request log, got nil")
			}
			// Verify the JSON contains the project key
			var m map[string]string
			if err := json.Unmarshal(req.Metadata, &m); err != nil {
				t.Errorf("failed to unmarshal stored metadata: %v", err)
			}
			if m["project"] != "demo" {
				t.Errorf("expected project=demo, got %s", m["project"])
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("no success request log found in fakeStorage")
	}
}
