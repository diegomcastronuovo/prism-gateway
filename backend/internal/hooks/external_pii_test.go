package hooks

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

func chatResp(content string) *providers.ChatResponse {
	return &providers.ChatResponse{
		ID:      "test-id",
		Model:   "test-model",
		Created: 1234567890,
		Choices: []providers.ChatChoice{
			{
				Index:        0,
				Message:      providers.ChatMessage{Role: "assistant", Content: content},
				FinishReason: "stop",
			},
		},
	}
}

func TestExternalPII_PreRequest_Allow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		var req WebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if len(req.Body.Messages) != 1 || req.Body.Messages[0].Role != "user" {
			t.Errorf("unexpected messages: %+v", req.Body.Messages)
		}

		// Return allow
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/request"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test message"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Allow {
		t.Errorf("expected Allow decision, got %s", result.Decision)
	}
}

func TestExternalPII_PreRequest_Reject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{
				Reject: &WebhookReject{
					Response: struct {
						Body string `json:"body"`
					}{
						Body: "PII detected by webhook",
					},
				},
			},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_closed",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test@example.com"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Block {
		t.Errorf("expected Block decision, got %s", result.Decision)
	}
	if result.Reason != "PII detected by webhook" {
		t.Errorf("expected reason 'PII detected by webhook', got %q", result.Reason)
	}
}

func TestExternalPII_PreRequest_Modify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{
				Body: &WebhookBody{
					Messages: []providers.ChatMessage{
						{Role: "user", Content: "[REDACTED]"},
					},
				},
				Reason: "email redacted",
			},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test@example.com"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Redact {
		t.Errorf("expected Redact decision, got %s", result.Decision)
	}
	if len(result.Request.Messages) != 1 || result.Request.Messages[0].Content != "[REDACTED]" {
		t.Errorf("expected redacted content, got: %+v", result.Request.Messages)
	}
	if result.Reason != "email redacted" {
		t.Errorf("expected reason 'email redacted', got %q", result.Reason)
	}
}

func TestExternalPII_PostResponse_Allow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure contains choices
		var req WebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			return
		}

		if len(req.Body.Choices) != 1 {
			t.Errorf("expected 1 choice, got %d", len(req.Body.Choices))
		}

		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Response:     config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	resp := chatResp("test response")
	result, err := hook.PostResponse(context.Background(), baseTenant(), chatReq("test"), resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Choices[0].Message.Content != "test response" {
		t.Errorf("response should be unchanged")
	}
}

func TestExternalPII_PostResponse_Modify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{
				Body: &WebhookBody{
					Choices: []providers.ChatChoice{
						{
							Index:        0,
							Message:      providers.ChatMessage{Role: "assistant", Content: "[REDACTED]"},
							FinishReason: "stop",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Response:     config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	resp := chatResp("admin@example.com")
	result, err := hook.PostResponse(context.Background(), baseTenant(), chatReq("test"), resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Choices[0].Message.Content != "[REDACTED]" {
		t.Errorf("expected redacted content, got: %s", result.Response.Choices[0].Message.Content)
	}
	// Verify other fields preserved
	if result.Response.ID != "test-id" || result.Response.Model != "test-model" {
		t.Errorf("response metadata should be preserved")
	}
}

func TestExternalPII_FailOpen_OnTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response exceeding timeout
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    50, // Very short timeout
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Allow {
		t.Errorf("fail_open should allow on timeout, got %s", result.Decision)
	}
}

func TestExternalPII_FailClosed_OnTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    50,
		FailMode:     "fail_closed",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Block {
		t.Errorf("fail_closed should block on timeout, got %s", result.Decision)
	}
	if result.Reason != "PII webhook unavailable" {
		t.Errorf("expected unavailable reason, got %q", result.Reason)
	}
}

func TestExternalPII_InvalidResponse_FailOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return invalid JSON
		w.Write([]byte("{invalid json"))
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Allow {
		t.Errorf("fail_open should allow on invalid response, got %s", result.Decision)
	}
}

func TestExternalPII_InvalidResponse_FailClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return malformed structure: missing "action" key entirely
		json.NewEncoder(w).Encode(map[string]interface{}{
			"missing_action_key": map[string]interface{}{},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_closed",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Block {
		t.Errorf("fail_closed should block on invalid response, got %s", result.Decision)
	}
}

func TestExternalPII_MaxBodyBytes_Request(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_closed",
		MaxBodyBytes: 50, // Very small limit
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	// Create large message that will exceed limit
	largeContent := strings.Repeat("x", 1000)
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq(largeContent))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Block {
		t.Errorf("should block when request exceeds max_body_bytes, got %s", result.Decision)
	}
}

func TestExternalPII_MaxBodyBytes_Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return very large response
		largeResponse := WebhookResponse{
			Action: WebhookAction{
				Body: &WebhookBody{
					Messages: []providers.ChatMessage{
						{Role: "user", Content: strings.Repeat("x", 2000)},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(largeResponse)
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_closed",
		MaxBodyBytes: 100, // Very small limit
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Block {
		t.Errorf("should block when response exceeds max_body_bytes, got %s", result.Decision)
	}
}

func TestExternalPII_RequestDisabled(t *testing.T) {
	// Server should never be called
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: false, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Allow {
		t.Errorf("expected Allow when disabled, got %s", result.Decision)
	}
	if serverCalled {
		t.Error("webhook should not be called when request.enabled=false")
	}
}

func TestExternalPII_ResponseDisabled(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Response:     config.WebhookPhase{Enabled: false, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	resp := chatResp("test")
	result, err := hook.PostResponse(context.Background(), baseTenant(), chatReq("test"), resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != resp {
		t.Error("response should be unchanged when disabled")
	}
	if serverCalled {
		t.Error("webhook should not be called when response.enabled=false")
	}
}

func TestExternalPII_Auth_Bearer(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth: config.WebhookAuth{
			Type:  "bearer",
			Token: "test-token-123",
		},
	}

	hook := NewExternalPII(cfg, slog.Default())
	_, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authHeader != "Bearer test-token-123" {
		t.Errorf("expected 'Bearer test-token-123', got %q", authHeader)
	}
}

func TestExternalPII_Auth_APIKey(t *testing.T) {
	var apiKeyHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKeyHeader = r.Header.Get("X-API-Key")
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		Auth: config.WebhookAuth{
			Type:   "api_key",
			Token:  "secret-key",
			Header: "X-API-Key",
		},
	}

	hook := NewExternalPII(cfg, slog.Default())
	_, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiKeyHeader != "secret-key" {
		t.Errorf("expected 'secret-key', got %q", apiKeyHeader)
	}
}

func TestExternalPII_APIKey_Field(t *testing.T) {
	var apiKeyHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKeyHeader = r.Header.Get("X-API-Key")
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	// Test APIKey field (takes precedence over Auth config)
	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		APIKey:       "sk-api-key-123",
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	_, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiKeyHeader != "sk-api-key-123" {
		t.Errorf("expected 'sk-api-key-123', got %q", apiKeyHeader)
	}
}

func TestExternalPII_APIKey_Precedence(t *testing.T) {
	var apiKeyHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKeyHeader = r.Header.Get("X-API-Key")
		json.NewEncoder(w).Encode(WebhookResponse{
			Action: WebhookAction{Allow: &WebhookAllow{}},
		})
	}))
	defer server.Close()

	// Test that APIKey field takes precedence over Auth config
	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_open",
		MaxBodyBytes: 1048576,
		APIKey:       "sk-api-key-new",
		Auth: config.WebhookAuth{
			Type:   "api_key",
			Token:  "sk-old-token",
			Header: "X-API-Key",
		},
	}

	hook := NewExternalPII(cfg, slog.Default())
	_, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// APIKey should take precedence over Auth.Token
	if apiKeyHeader != "sk-api-key-new" {
		t.Errorf("APIKey should take precedence: expected 'sk-api-key-new', got %q", apiKeyHeader)
	}
}

func TestExternalPII_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_closed",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Block {
		t.Errorf("fail_closed should block on non-200 status, got %s", result.Decision)
	}
}

func TestExternalPII_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json at all"))
	}))
	defer server.Close()

	cfg := config.ExternalPIIHookConfig{
		BaseURL:      server.URL,
		Request:      config.WebhookPhase{Enabled: true, Path: "/"},
		TimeoutMs:    1000,
		FailMode:     "fail_closed",
		MaxBodyBytes: 1048576,
		Auth:         config.WebhookAuth{Type: "none"},
	}

	hook := NewExternalPII(cfg, slog.Default())
	result, err := hook.PreRequest(context.Background(), baseTenant(), chatReq("test"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != Block {
		t.Errorf("fail_closed should block on malformed JSON, got %s", result.Decision)
	}
}

// --- ParsePIIWebhookResponse unit tests ---

func TestParsePIIWebhookResponse_ArkanaAllow(t *testing.T) {
	raw := []byte(`{"action":{}}`)
	action, err := ParsePIIWebhookResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.Kind != NormalizedAllow {
		t.Errorf("expected NormalizedAllow, got %s", action.Kind)
	}
}

func TestParsePIIWebhookResponse_ArkanaReject(t *testing.T) {
	raw := []byte(`{"action":{"status_code":403,"body":"Request rejected due to policy","reason":"PII detected"}}`)
	action, err := ParsePIIWebhookResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.Kind != NormalizedReject {
		t.Errorf("expected NormalizedReject, got %s", action.Kind)
	}
	if action.RejectStatusCode != 403 {
		t.Errorf("expected status 403, got %d", action.RejectStatusCode)
	}
	if action.RejectBody != "Request rejected due to policy" {
		t.Errorf("unexpected reject body: %q", action.RejectBody)
	}
	if action.Reason != "PII detected" {
		t.Errorf("unexpected reason: %q", action.Reason)
	}
}

func TestParsePIIWebhookResponse_ArkanaModify(t *testing.T) {
	raw := []byte(`{"action":{"body":{"messages":[{"role":"user","content":"...masked..."}]},"reason":"Masked by Arkana PII"}}`)
	action, err := ParsePIIWebhookResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.Kind != NormalizedModify {
		t.Errorf("expected NormalizedModify, got %s", action.Kind)
	}
	if len(action.ModifiedMessages) != 1 || action.ModifiedMessages[0].Content != "...masked..." {
		t.Errorf("unexpected modified messages: %+v", action.ModifiedMessages)
	}
	if action.Reason != "Masked by Arkana PII" {
		t.Errorf("unexpected reason: %q", action.Reason)
	}
}

func TestParsePIIWebhookResponse_ArkanaAllowPII(t *testing.T) {
	raw := []byte(`{"action":{"allow_pii":{}}}`)
	action, err := ParsePIIWebhookResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.Kind != NormalizedAllowPII {
		t.Errorf("expected NormalizedAllowPII, got %s", action.Kind)
	}
}

func TestParsePIIWebhookResponse_LegacyAllow(t *testing.T) {
	raw := []byte(`{"action":{"allow":{}}}`)
	action, err := ParsePIIWebhookResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.Kind != NormalizedAllow {
		t.Errorf("expected NormalizedAllow (legacy), got %s", action.Kind)
	}
}
