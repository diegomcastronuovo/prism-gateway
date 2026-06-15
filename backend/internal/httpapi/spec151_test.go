package httpapi

import (
	"net/http"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// SPEC_151: Headers Enable — per-tenant allow_model_override / allow_route_group_override flags.
// Both default to false. When false, the corresponding header is treated as absent.

// Test 1: both flags false → both override headers ignored, routing proceeds normally, no conflict error.
func TestSpec151_BothFalse_HeadersIgnored(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].Selection = config.SelectionConfig{
		AllowModelOverride:      false,
		AllowRouteGroupOverride: false,
		RouteGroups: map[string][]string{
			"cheap": {"model-b"},
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{
		"X-API-Key":     "key1",
		"X-Model":       "model-b",
		"X-Route-Group": "cheap",
	})

	// Must succeed — headers ignored, normal routing applies, no conflict
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (both headers ignored), got %d: %s", w.Code, w.Body.String())
	}
}

// Test 2: model override true, route-group override false → model header honored, route-group ignored, no conflict.
func TestSpec151_ModelEnabled_RouteGroupDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].Selection = config.SelectionConfig{
		AllowModelOverride:      true,
		AllowRouteGroupOverride: false,
		RouteGroups: map[string][]string{
			"cheap": {"model-b"},
		},
		Precedence: config.PrecedenceConfig{
			Model:          "header",
			ConflictPolicy: "error",
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)

	// model-a is NOT in "cheap" group, but route-group header should be ignored → no conflict
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{
		"X-API-Key":     "key1",
		"X-Model":       "model-a",
		"X-Route-Group": "cheap",
	})

	// model header honored → routes to model-a; no conflict because route-group header is disabled
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (route-group disabled, no conflict), got %d: %s", w.Code, w.Body.String())
	}
	selected := w.Header().Get("X-Selected-Model")
	if selected != "model-a" {
		t.Errorf("expected X-Selected-Model=model-a (from model header), got %q", selected)
	}
}

// Test 3: model override false, route-group override true → route-group header honored, model header ignored.
func TestSpec151_ModelDisabled_RouteGroupEnabled(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].Selection = config.SelectionConfig{
		AllowModelOverride:      false,
		AllowRouteGroupOverride: true,
		RouteGroups: map[string][]string{
			"cheap": {"model-b"},
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)

	// X-Model should be ignored; X-Route-Group should restrict candidates to model-b
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{
		"X-API-Key":     "key1",
		"X-Model":       "model-a",
		"X-Route-Group": "cheap",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	selected := w.Header().Get("X-Selected-Model")
	if selected != "model-b" {
		t.Errorf("expected model-b (cheap group applied), got %q", selected)
	}
}

// Test 4: both flags true → current conflict behavior preserved (conflict_policy=error returns 400).
func TestSpec151_BothEnabled_ConflictPolicyApplies(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].Selection = config.SelectionConfig{
		AllowModelOverride:      true,
		AllowRouteGroupOverride: true,
		HeaderModelKey:          "X-Model",
		HeaderRouteKey:          "X-Route-Group",
		RouteGroups: map[string][]string{
			"premium": {"model-a"},
		},
		Precedence: config.PrecedenceConfig{
			Model:          "header",
			ConflictPolicy: "error",
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)

	// model-b is NOT in premium → conflict with conflict_policy=error → 400
	body := `{"messages":[{"role":"user","content":"conflict test"}]}`
	w := makeRequest(t, handler, body, map[string]string{
		"X-API-Key":     "key1",
		"X-Model":       "model-b",
		"X-Route-Group": "premium",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (conflict), got %d: %s", w.Code, w.Body.String())
	}
}

// Test 5: existing tenant config missing the new fields → effective false behavior (headers ignored).
func TestSpec151_MissingFields_DefaultFalse(t *testing.T) {
	cfg := testConfig()
	// Selection deliberately omits AllowModelOverride and AllowRouteGroupOverride (zero values = false)
	cfg.Tenants[0].Selection = config.SelectionConfig{
		RouteGroups: map[string][]string{
			"cheap": {"model-b"},
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{
		"X-API-Key":     "key1",
		"X-Model":       "model-b",
		"X-Route-Group": "cheap",
	})

	// Both headers must be ignored (zero-value = false); routing proceeds without conflict
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (zero-value flags = false, headers ignored), got %d: %s", w.Code, w.Body.String())
	}
}

// Test 6: new tenant (no explicit selection config) → both false, headers ignored.
func TestSpec151_NewTenant_BothDefaultFalse(t *testing.T) {
	cfg := testConfig()
	// No Selection config set at all (zero value)
	cfg.Tenants[0].Selection = config.SelectionConfig{}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	handler := setupTestServer(cfg, reg)

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{
		"X-API-Key":     "key1",
		"X-Model":       "model-b",
		"X-Route-Group": "cheap",
	})

	// Headers must be treated as absent — normal routing without error
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (new tenant, both flags false), got %d: %s", w.Code, w.Body.String())
	}
}
