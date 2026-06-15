package router

import (
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func TestPrecedenceResolver(t *testing.T) {
	// Setup test models
	globalModels := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai"},
		{Name: "claude-sonnet-4-6", Provider: "anthropic"},
		{Name: "gemini-2.5-flash", Provider: "google"},
	}

	t.Run("body model only", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6"},
			Selection: config.SelectionConfig{
				Precedence: config.PrecedenceConfig{
					Model: "header", // Default
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		result, err := resolver.Resolve("gpt-4o-mini", "", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ForcedModel != "gpt-4o-mini" {
			t.Errorf("expected forced model 'gpt-4o-mini', got '%s'", result.ForcedModel)
		}
		if result.RequestedSource != "body" {
			t.Errorf("expected source 'body', got '%s'", result.RequestedSource)
		}
		if result.DecisionReason != "explicit_body" {
			t.Errorf("expected reason 'explicit_body', got '%s'", result.DecisionReason)
		}
	})

	t.Run("header model only", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6"},
			Selection: config.SelectionConfig{
				Precedence: config.PrecedenceConfig{
					Model: "header",
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		result, err := resolver.Resolve("", "claude-sonnet-4-6", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ForcedModel != "claude-sonnet-4-6" {
			t.Errorf("expected forced model 'claude-sonnet-4-6', got '%s'", result.ForcedModel)
		}
		if result.RequestedSource != "header" {
			t.Errorf("expected source 'header', got '%s'", result.RequestedSource)
		}
		if result.DecisionReason != "explicit_header" {
			t.Errorf("expected reason 'explicit_header', got '%s'", result.DecisionReason)
		}
	})

	t.Run("body and header conflict - header wins by default", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6"},
			Selection: config.SelectionConfig{
				Precedence: config.PrecedenceConfig{
					Model: "header", // Default
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		result, err := resolver.Resolve("gpt-4o-mini", "claude-sonnet-4-6", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ForcedModel != "claude-sonnet-4-6" {
			t.Errorf("expected forced model 'claude-sonnet-4-6' (header wins), got '%s'", result.ForcedModel)
		}
		if result.RequestedSource != "header" {
			t.Errorf("expected source 'header', got '%s'", result.RequestedSource)
		}
	})

	t.Run("body and header conflict - body wins with precedence.model=body", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6"},
			Selection: config.SelectionConfig{
				Precedence: config.PrecedenceConfig{
					Model: "body", // Legacy mode
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		result, err := resolver.Resolve("gpt-4o-mini", "claude-sonnet-4-6", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ForcedModel != "gpt-4o-mini" {
			t.Errorf("expected forced model 'gpt-4o-mini' (body wins), got '%s'", result.ForcedModel)
		}
		if result.RequestedSource != "body" {
			t.Errorf("expected source 'body', got '%s'", result.RequestedSource)
		}
	})

	t.Run("route group + model inside group", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6", "gemini-2.5-flash"},
			Selection: config.SelectionConfig{
				RouteGroups: map[string][]string{
					"cheap": {"gpt-4o-mini", "gemini-2.5-flash"},
				},
				Precedence: config.PrecedenceConfig{
					Model:          "header",
					ConflictPolicy: "error",
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		result, err := resolver.Resolve("", "gpt-4o-mini", "cheap")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ForcedModel != "gpt-4o-mini" {
			t.Errorf("expected forced model 'gpt-4o-mini', got '%s'", result.ForcedModel)
		}
		if result.DecisionReason != "explicit_header|group:cheap" {
			t.Errorf("expected reason 'explicit_header|group:cheap', got '%s'", result.DecisionReason)
		}
	})

	t.Run("route group + model outside group - error by default", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6", "gemini-2.5-flash"},
			Selection: config.SelectionConfig{
				RouteGroups: map[string][]string{
					"cheap": {"gpt-4o-mini", "gemini-2.5-flash"},
				},
				Precedence: config.PrecedenceConfig{
					Model:          "header",
					ConflictPolicy: "error", // Default
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "claude-sonnet-4-6", "cheap")

		if err == nil {
			t.Fatal("expected error for model outside route group")
		}
		expectedErr := "model 'claude-sonnet-4-6' is not in route group 'cheap'"
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("route group + model conflict - ignore_group policy", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6", "gemini-2.5-flash"},
			Selection: config.SelectionConfig{
				RouteGroups: map[string][]string{
					"cheap": {"gpt-4o-mini", "gemini-2.5-flash"},
				},
				Precedence: config.PrecedenceConfig{
					Model:          "header",
					ConflictPolicy: "ignore_group",
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		result, err := resolver.Resolve("", "claude-sonnet-4-6", "cheap")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ForcedModel != "claude-sonnet-4-6" {
			t.Errorf("expected forced model 'claude-sonnet-4-6', got '%s'", result.ForcedModel)
		}
		if result.DecisionReason != "explicit_header|group_ignored" {
			t.Errorf("expected reason 'explicit_header|group_ignored', got '%s'", result.DecisionReason)
		}
		// Candidates should be original (not filtered by group)
		if len(result.Candidates) != 3 {
			t.Errorf("expected 3 candidates (group ignored), got %d", len(result.Candidates))
		}
	})

	t.Run("route group + model conflict - ignore_model policy", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6", "gemini-2.5-flash"},
			Selection: config.SelectionConfig{
				RouteGroups: map[string][]string{
					"cheap": {"gpt-4o-mini", "gemini-2.5-flash"},
				},
				Precedence: config.PrecedenceConfig{
					Model:          "header",
					ConflictPolicy: "ignore_model",
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		result, err := resolver.Resolve("", "claude-sonnet-4-6", "cheap")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ForcedModel != "" {
			t.Errorf("expected no forced model (ignored), got '%s'", result.ForcedModel)
		}
		if result.DecisionReason != "group:cheap|model_ignored" {
			t.Errorf("expected reason 'group:cheap|model_ignored', got '%s'", result.DecisionReason)
		}
		// Candidates should be filtered to group
		if len(result.Candidates) != 2 {
			t.Errorf("expected 2 candidates (cheap group), got %d", len(result.Candidates))
		}
	})

	t.Run("invalid route group", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini", "claude-sonnet-4-6"},
			Selection: config.SelectionConfig{
				RouteGroups: map[string][]string{
					"cheap": {"gpt-4o-mini"},
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "", "nonexistent")

		if err == nil {
			t.Fatal("expected error for nonexistent route group")
		}
		expectedErr := "route group 'nonexistent' not found"
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("disallowed model", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{"gpt-4o-mini"}, // Only one model allowed
			Selection: config.SelectionConfig{
				Precedence: config.PrecedenceConfig{
					Model: "header",
				},
			},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "claude-sonnet-4-6", "")

		if err == nil {
			t.Fatal("expected error for disallowed model")
		}
		expectedErr := "model 'claude-sonnet-4-6' is not allowed for tenant 'test'"
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("no models allowed", func(t *testing.T) {
		tenant := &config.TenantConfig{
			ID:            "test",
			AllowedModels: []string{}, // Empty allowlist
			Selection:     config.SelectionConfig{},
		}

		resolver := NewPrecedenceResolver(tenant, globalModels)
		_, err := resolver.Resolve("", "", "")

		if err == nil {
			t.Fatal("expected error for empty allowlist")
		}
		expectedErr := "no models allowed for tenant 'test'"
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})
}

// TestPrecedenceResolver_EmbeddingModelsExcluded verifies that models with type="embedding"
// are never included in chat completion candidates, regardless of allowed_models config.
// This prevents the router from calling an embedding provider with a ChatCompletion request.
func TestPrecedenceResolver_EmbeddingModelsExcluded(t *testing.T) {
	globalModels := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai", Type: ""},             // chat (implicit)
		{Name: "gpt-4o", Provider: "openai", Type: "chat"},              // chat (explicit)
		{Name: "text-embedding-3-small", Provider: "openai", Type: "embedding"}, // embedding-only
	}

	tenant := &config.TenantConfig{
		ID:            "t1",
		AllowedModels: []string{"gpt-4o-mini", "gpt-4o", "text-embedding-3-small"},
		Selection:     config.SelectionConfig{},
	}

	resolver := NewPrecedenceResolver(tenant, globalModels)
	result, err := resolver.Resolve("", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range result.Candidates {
		if c.Type == "embedding" {
			t.Errorf("embedding model %q must not appear in chat candidates", c.Name)
		}
	}

	// Chat models must still be present
	if len(result.Candidates) != 2 {
		t.Errorf("expected 2 chat candidates (gpt-4o-mini, gpt-4o), got %d: %v",
			len(result.Candidates), result.Candidates)
	}
}
