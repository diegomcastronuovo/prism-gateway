package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminTestConnection_Validation(t *testing.T) {
	h := &Handlers{log: testLoggerForAdmin()}

	tests := []struct {
		name       string
		body       testConnectionRequest
		wantStatus int
	}{
		{"missing url", testConnectionRequest{}, http.StatusBadRequest},
		{"empty url", testConnectionRequest{URL: "   "}, http.StatusBadRequest},
		{"invalid scheme", testConnectionRequest{URL: "file:///etc/passwd"}, http.StatusBadRequest},
		{"invalid type", testConnectionRequest{URL: "https://example.com", Type: "other"}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/admin/test-connection", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Admin-Token", "test-token")
			w := httptest.NewRecorder()
			h.AdminTestConnection(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestAdminTestConnection_JWKS_Success(t *testing.T) {
	// Mock JWKS server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[{"kid":"k1","kty":"RSA","alg":"RS256"}]}`))
	}))
	defer server.Close()

	h := &Handlers{log: testLoggerForAdmin()}
	body, _ := json.Marshal(testConnectionRequest{URL: server.URL, Type: "jwks"})
	req := httptest.NewRequest(http.MethodPost, "/admin/test-connection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.AdminTestConnection(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	var resp testConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.Status != 200 {
		t.Errorf("got ok=%v status=%d error=%q, want ok=true status=200", resp.OK, resp.Status, resp.Error)
	}
}

func TestAdminTestConnection_JWKS_EmptyKeys_Fail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer server.Close()

	h := &Handlers{log: testLoggerForAdmin()}
	body, _ := json.Marshal(testConnectionRequest{URL: server.URL, Type: "jwks"})
	req := httptest.NewRequest(http.MethodPost, "/admin/test-connection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.AdminTestConnection(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	var resp testConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.OK {
		t.Errorf("got ok=true, want ok=false for empty keys")
	}
}

func TestAdminTestConnection_Basic_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	h := &Handlers{log: testLoggerForAdmin()}
	body, _ := json.Marshal(testConnectionRequest{URL: server.URL, Type: "basic"})
	req := httptest.NewRequest(http.MethodPost, "/admin/test-connection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.AdminTestConnection(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	var resp testConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.Status != 200 {
		t.Errorf("got ok=%v status=%d, want ok=true status=200", resp.OK, resp.Status)
	}
}
