package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

// bootstrapFakeStore is a minimal fake storage for bootstrap tests
type bootstrapFakeStore struct {
	storage.NopStorage
	apiKeyCount   int
	countErr      error
	createErr     error
	createResult  storage.APIKeyCreateResult
}

func (f *bootstrapFakeStore) CountAPIKeys(ctx context.Context) (int, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.apiKeyCount, nil
}

func (f *bootstrapFakeStore) CreateAPIKey(ctx context.Context, tenantID, name string, scopes []string, expiresAt *time.Time, actorSub string, actorRoles []string) (storage.APIKeyCreateResult, error) {
	if f.createErr != nil {
		return storage.APIKeyCreateResult{}, f.createErr
	}
	// Return the pre-configured result
	return f.createResult, nil
}

func TestBootstrapAPIKeyHandler_FirstKeySuccess(t *testing.T) {
	// Setup
	store := &bootstrapFakeStore{
		apiKeyCount: 0, // Table is empty
		createResult: storage.APIKeyCreateResult{
			APIKeyMeta: storage.APIKeyMeta{
				ID:       uuid.New(),
				TenantID: "system:bootstrap",
				Name:     "Bootstrap Admin Key",
				Prefix:   "rk_live_abc123",
				Scopes:   []string{"admin_write"},
			},
			Key: "rk_live_abc123xyz_plaintext_secret_key_123456789",
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	// Set environment variable
	os.Setenv("BOOTSTRAP_KEY", "test-bootstrap-key-secret")
	defer os.Unsetenv("BOOTSTRAP_KEY")

	// Create request
	body := BootstrapAPIKeyRequest{
		Name:   "Admin Key",
		Scopes: []string{"admin_write"},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/admin/bootstrap/api-keys",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Bootstrap-Key", "test-bootstrap-key-secret")
	req.Header.Set("Content-Type", "application/json")

	// Record response
	w := httptest.NewRecorder()
	handler(w, req)

	// Verify response
	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var resp BootstrapAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Key != store.createResult.Key {
		t.Errorf("expected key %s, got %s", store.createResult.Key, resp.Key)
	}

	if resp.Prefix != "rk_live_abc123" {
		t.Errorf("expected prefix rk_live_abc123, got %s", resp.Prefix)
	}

	if len(resp.Scopes) != 1 || resp.Scopes[0] != "admin_write" {
		t.Errorf("expected scopes [admin_write], got %v", resp.Scopes)
	}
}

func TestBootstrapAPIKeyHandler_AlreadyExists(t *testing.T) {
	// Setup: api_keys table has 1 key already
	store := &bootstrapFakeStore{
		apiKeyCount: 1,
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	os.Setenv("BOOTSTRAP_KEY", "test-bootstrap-key-secret")
	defer os.Unsetenv("BOOTSTRAP_KEY")

	// Create request
	body := BootstrapAPIKeyRequest{
		Name:   "Admin Key",
		Scopes: []string{"admin_write"},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/admin/bootstrap/api-keys",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Bootstrap-Key", "test-bootstrap-key-secret")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	// Should return 403 Forbidden
	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}

	var errResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["error"] == nil {
		t.Error("expected error field in response")
	}
}

func TestBootstrapAPIKeyHandler_InvalidKey(t *testing.T) {
	store := &bootstrapFakeStore{
		apiKeyCount: 0,
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	os.Setenv("BOOTSTRAP_KEY", "correct-key")
	defer os.Unsetenv("BOOTSTRAP_KEY")

	body := BootstrapAPIKeyRequest{
		Name:   "Admin Key",
		Scopes: []string{"admin_write"},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/admin/bootstrap/api-keys",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Bootstrap-Key", "wrong-key") // Wrong key
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	// Should return 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestBootstrapAPIKeyHandler_NoBootstrapKeyEnv(t *testing.T) {
	store := &bootstrapFakeStore{
		apiKeyCount: 0,
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	// Ensure BOOTSTRAP_KEY is not set
	os.Unsetenv("BOOTSTRAP_KEY")

	body := BootstrapAPIKeyRequest{
		Name:   "Admin Key",
		Scopes: []string{"admin_write"},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/admin/bootstrap/api-keys",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Bootstrap-Key", "some-key")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	// Should return 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestBootstrapAPIKeyHandler_InvalidScope(t *testing.T) {
	store := &bootstrapFakeStore{
		apiKeyCount: 0,
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	os.Setenv("BOOTSTRAP_KEY", "test-key")
	defer os.Unsetenv("BOOTSTRAP_KEY")

	body := BootstrapAPIKeyRequest{
		Name:   "Admin Key",
		Scopes: []string{"invalid_scope"}, // Invalid scope
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/admin/bootstrap/api-keys",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Bootstrap-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	// Should return 400 Bad Request
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestBootstrapAPIKeyHandler_CountError(t *testing.T) {
	// Setup: storage returns error when counting
	store := &bootstrapFakeStore{
		countErr: fmt.Errorf("simulated db error"),
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	os.Setenv("BOOTSTRAP_KEY", "test-key")
	defer os.Unsetenv("BOOTSTRAP_KEY")

	body := BootstrapAPIKeyRequest{
		Name:   "Admin Key",
		Scopes: []string{"admin_write"},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/admin/bootstrap/api-keys",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Bootstrap-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	// Should return 500 Internal Server Error
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestBootstrapAPIKeyHandler_MethodNotAllowed(t *testing.T) {
	store := &bootstrapFakeStore{
		apiKeyCount: 0,
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	os.Setenv("BOOTSTRAP_KEY", "test-key")
	defer os.Unsetenv("BOOTSTRAP_KEY")

	// Try GET instead of POST
	req := httptest.NewRequest("GET", "/admin/bootstrap/api-keys", nil)
	req.Header.Set("X-Bootstrap-Key", "test-key")

	w := httptest.NewRecorder()
	handler(w, req)

	// Should return 405 Method Not Allowed
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestBootstrapAPIKeyHandler_DefaultScopes(t *testing.T) {
	// Test that default scopes are set if not provided
	store := &bootstrapFakeStore{
		apiKeyCount: 0,
		createResult: storage.APIKeyCreateResult{
			APIKeyMeta: storage.APIKeyMeta{
				ID:       uuid.New(),
				TenantID: "system:bootstrap",
				Name:     "Bootstrap Admin Key",
				Prefix:   "rk_live_def",
				Scopes:   []string{"admin_write"}, // Default
			},
			Key: "rk_live_def_key",
		},
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := BootstrapAPIKeyHandler(store, log)

	os.Setenv("BOOTSTRAP_KEY", "test-key")
	defer os.Unsetenv("BOOTSTRAP_KEY")

	// Request without scopes
	body := BootstrapAPIKeyRequest{
		Name: "Admin Key",
		// No scopes
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/admin/bootstrap/api-keys",
		bytes.NewReader(bodyJSON))
	req.Header.Set("X-Bootstrap-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler(w, req)

	// Should succeed with default scopes
	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}
