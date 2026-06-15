package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
)

func TestCreateSemanticAnchor_JWTAdmin_ResolvesTenant(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, _ := multimodalHandlers(store)

	body := `{
		"name": "finance_jwt",
		"text": "banking finance investments markets trading",
		"route_group": "finance"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/semantic/anchors?tenant_id=t1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithJWTAdminContext(req.Context(), "t1", "sub-1", []string{"admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.CreateSemanticAnchor(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp anchorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "finance_jwt" {
		t.Errorf("name: got %q", resp.Name)
	}
}

func TestCreateSemanticAnchor_JWTNonAdmin_Forbidden(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, _ := multimodalHandlers(store)

	body := `{
		"name": "x",
		"text": "hello",
		"route_group": "finance"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/semantic/anchors?tenant_id=t1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithJWTAdminContext(req.Context(), "t1", "sub-1", []string{"user"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.CreateSemanticAnchor(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	var errResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	inner := errResp["error"].(map[string]any)
	if inner["message"] != "insufficient permissions" || inner["type"] != "authorization_error" {
		t.Fatalf("unexpected error body: %v", errResp)
	}
}

func TestCreateSemanticAnchor_APIKey_Unchanged(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)
	rec := doCreateAnchor(h, tenant, `{
		"name": "finance",
		"text": "stock market trends",
		"route_group": "finance"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
