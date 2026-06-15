package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminPIITestConnection_ValidRequest_Returns200WithLatency(t *testing.T) {
	// Mock servers that accept POST and return 200
	requestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer requestSrv.Close()
	responseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer responseSrv.Close()

	h := &Handlers{log: testLogger(), piiTestLimiter: newPIITestRateLimiter()}
	body := map[string]interface{}{
		"request_url":  requestSrv.URL,
		"response_url": responseSrv.URL,
		"timeout_ms":   3000,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/tenant_a/pii/test-connection", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "tenant_a")
	w := httptest.NewRecorder()

	h.AdminPIITestConnection(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp piiTestConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RequestEndpoint.Status != "ok" {
		t.Errorf("request_endpoint.status=%q, want ok", resp.RequestEndpoint.Status)
	}
	if resp.ResponseEndpoint.Status != "ok" {
		t.Errorf("response_endpoint.status=%q, want ok", resp.ResponseEndpoint.Status)
	}
	if resp.RequestEndpoint.StatusCode != 200 || resp.ResponseEndpoint.StatusCode != 200 {
		t.Errorf("status_code: request=%d response=%d", resp.RequestEndpoint.StatusCode, resp.ResponseEndpoint.StatusCode)
	}
	if resp.RequestEndpoint.LatencyMs < 0 || resp.ResponseEndpoint.LatencyMs < 0 {
		t.Error("latency_ms should be non-negative")
	}
}

func TestAdminPIITestConnection_MissingTenantID_Returns400(t *testing.T) {
	h := &Handlers{log: testLogger(), piiTestLimiter: newPIITestRateLimiter()}
	body := []byte(`{"request_url":"https://a.com/r","response_url":"https://a.com/s","timeout_ms":1000}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants//pii/test-connection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "")
	w := httptest.NewRecorder()

	h.AdminPIITestConnection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPIITestConnection_InvalidURL_Returns400(t *testing.T) {
	h := &Handlers{log: testLogger(), piiTestLimiter: newPIITestRateLimiter()}
	body := []byte(`{"request_url":"not-a-url","response_url":"https://b.com/s","timeout_ms":1000}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/t1/pii/test-connection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()

	h.AdminPIITestConnection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminPIITestConnection_TimeoutMsOutOfRange_Returns400(t *testing.T) {
	h := &Handlers{log: testLogger(), piiTestLimiter: newPIITestRateLimiter()}
	for _, timeout := range []int{0, 400, 15000} {
		body, _ := json.Marshal(map[string]interface{}{
			"request_url":  "https://a.com/r",
			"response_url": "https://a.com/s",
			"timeout_ms":   timeout,
		})
		req := httptest.NewRequest(http.MethodPost, "/admin/tenants/t1/pii/test-connection", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("tenant_id", "t1")
		w := httptest.NewRecorder()
		h.AdminPIITestConnection(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("timeout_ms=%d: expected 400, got %d", timeout, w.Code)
		}
	}
}

func TestAdminPIITestConnection_ConnectionRefused_Returns200WithErrorStatus(t *testing.T) {
	// Use a port that is not listening
	h := &Handlers{log: testLogger(), piiTestLimiter: newPIITestRateLimiter()}
	body := []byte(`{"request_url":"http://127.0.0.1:19999/request","response_url":"http://127.0.0.1:19998/response","timeout_ms":500}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/t1/pii/test-connection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "t1")
	w := httptest.NewRecorder()

	h.AdminPIITestConnection(w, req)

	// Spec: we still return 200 with endpoint status "error"
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp piiTestConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RequestEndpoint.Status != "error" {
		t.Errorf("request_endpoint.status=%q, want error", resp.RequestEndpoint.Status)
	}
	if resp.RequestEndpoint.Error == "" {
		t.Error("request_endpoint.error should be set when connection fails")
	}
}

func TestAdminPIITestConnection_RateLimit_Returns429AfterFiveCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()

	limiter := newPIITestRateLimiter()
	h := &Handlers{log: testLogger(), piiTestLimiter: limiter}
	body, _ := json.Marshal(map[string]interface{}{
		"request_url":  srv.URL,
		"response_url": srv.URL,
		"timeout_ms":   5000,
	})

	for i := 0; i < 7; i++ {
		req := httptest.NewRequest(http.MethodPost, "/admin/tenants/rate_tenant/pii/test-connection", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("tenant_id", "rate_tenant")
		w := httptest.NewRecorder()
		h.AdminPIITestConnection(w, req)
		if i < 5 {
			if w.Code != http.StatusOK {
				t.Errorf("call %d: expected 200, got %d", i+1, w.Code)
			}
		} else {
			if w.Code != http.StatusTooManyRequests {
				t.Errorf("call %d: expected 429, got %d", i+1, w.Code)
			}
		}
	}
}

func TestAdminPIITestConnection_WithAPIKey(t *testing.T) {
	var capturedApiKey string
	requestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedApiKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer requestSrv.Close()
	responseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer responseSrv.Close()

	h := &Handlers{log: testLogger(), piiTestLimiter: newPIITestRateLimiter()}
	body := map[string]interface{}{
		"request_url":  requestSrv.URL,
		"response_url": responseSrv.URL,
		"timeout_ms":   3000,
		"api_key":      "sk-test-key-12345",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/tenant_a/pii/test-connection", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "tenant_a")
	w := httptest.NewRecorder()

	h.AdminPIITestConnection(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify API key was sent
	if capturedApiKey != "sk-test-key-12345" {
		t.Errorf("expected API key 'sk-test-key-12345', got %q", capturedApiKey)
	}

	var resp piiTestConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RequestEndpoint.Status != "ok" {
		t.Errorf("request_endpoint.status=%q, want ok", resp.RequestEndpoint.Status)
	}
}
