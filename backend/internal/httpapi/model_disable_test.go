package httpapi

import (
	"net/http"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// withModelEnabled returns a copy of cfg with model-a's Enabled flag set to v.
func withModelEnabled(cfg *config.Config, v bool) *config.Config {
	for i, m := range cfg.Models {
		if m.Name == "model-a" {
			cfg.Models[i].Enabled = &v
		}
	}
	return cfg
}

func TestChatCompletions_DisabledModel_Returns403(t *testing.T) {
	cfg := withModelEnabled(testConfig(), false)
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatCompletions_ExplicitlyEnabledModel_Passes(t *testing.T) {
	cfg := withModelEnabled(testConfig(), true)
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))

	handler := setupTestServer(cfg, reg)
	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_IsEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil (default)", nil, true},
		{"explicit true", &trueVal, true},
		{"explicit false", &falseVal, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &config.ModelConfig{Enabled: tc.enabled}
			if got := m.IsEnabled(); got != tc.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
