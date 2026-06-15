package config

import (
	"encoding/json"
	"testing"
)

func TestModelConfig_UnmarshalJSON_ProviderModelID_PascalCase(t *testing.T) {
	const in = `{
		"Name": "claude-bedrock",
		"Provider": "bedrock",
		"ProviderModelID": "anthropic.claude-3-5-sonnet-20240620-v1:0",
		"Type": "chat",
		"Pricing": { "PromptPer1M": 3, "CompletionPer1M": 15 }
	}`
	var m ModelConfig
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "anthropic.claude-3-5-sonnet-20240620-v1:0"
	if m.ProviderModelID != want {
		t.Fatalf("ProviderModelID=%q, want %q", m.ProviderModelID, want)
	}
	if m.Name != "claude-bedrock" || m.Provider != "bedrock" {
		t.Fatalf("other fields: %#v", m)
	}
}

func TestModelConfig_UnmarshalJSON_ProviderModelID_SnakeCase(t *testing.T) {
	const in = `{
		"name": "m",
		"provider": "bedrock",
		"provider_model_id": "us.anthropic.model",
		"type": "chat",
		"pricing": { "prompt_per_1m": 1, "completion_per_1m": 2 }
	}`
	var m ModelConfig
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.ProviderModelID != "us.anthropic.model" {
		t.Fatalf("ProviderModelID=%q", m.ProviderModelID)
	}
}

func TestModelConfig_UnmarshalJSON_PrefersSnakeWhenBoth(t *testing.T) {
	const in = `{
		"name": "m",
		"provider": "bedrock",
		"provider_model_id": "from-snake",
		"ProviderModelID": "from-pascal",
		"type": "chat",
		"pricing": {}
	}`
	var m ModelConfig
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.ProviderModelID != "from-snake" {
		t.Fatalf("ProviderModelID=%q, want from-snake", m.ProviderModelID)
	}
}

func boolPtr(b bool) *bool { return &b }

// ── Enabled field: marshal / unmarshal / IsEnabled ───────────────────────────

func TestModelConfig_Enabled_MarshalOmitsNil(t *testing.T) {
	m := ModelConfig{Name: "m", Provider: "p"}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	// nil Enabled → omitted entirely, NOT "Enabled":null
	if bytes := string(b); bytes == "" {
		t.Fatal("empty marshal")
	}
	var raw map[string]interface{}
	json.Unmarshal(b, &raw)
	if _, exists := raw["Enabled"]; exists {
		t.Errorf("Enabled key should be absent when nil, got: %s", string(b))
	}
	if _, exists := raw["enabled"]; exists {
		t.Errorf("lowercase 'enabled' key should not appear in marshal output")
	}
}

func TestModelConfig_Enabled_MarshalPascalCase(t *testing.T) {
	m := ModelConfig{Name: "m", Provider: "p", Enabled: boolPtr(true)}
	b, _ := json.Marshal(m)
	var raw map[string]interface{}
	json.Unmarshal(b, &raw)
	if v, ok := raw["Enabled"]; !ok || v != true {
		t.Errorf("want 'Enabled':true, got JSON: %s", string(b))
	}
	if _, bad := raw["enabled"]; bad {
		t.Errorf("lowercase 'enabled' key must not appear; got JSON: %s", string(b))
	}
}

func TestModelConfig_Enabled_MarshalFalse(t *testing.T) {
	m := ModelConfig{Name: "m", Provider: "p", Enabled: boolPtr(false)}
	b, _ := json.Marshal(m)
	var raw map[string]interface{}
	json.Unmarshal(b, &raw)
	if v, ok := raw["Enabled"]; !ok || v != false {
		t.Errorf("want 'Enabled':false, got JSON: %s", string(b))
	}
}

func TestModelConfig_Enabled_UnmarshalLegacyLowercase(t *testing.T) {
	// Legacy stored JSON uses lowercase "enabled" — must be normalised on read.
	const in = `{"Name":"gpt-4o-mini","Provider":"openai","enabled":true}`
	var m ModelConfig
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatal(err)
	}
	if m.Enabled == nil || !*m.Enabled {
		t.Errorf("expected Enabled=true after normalising legacy 'enabled', got %v", m.Enabled)
	}
	if !m.IsEnabled() {
		t.Error("IsEnabled() must return true")
	}
}

func TestModelConfig_Enabled_UnmarshalLegacyFalse(t *testing.T) {
	const in = `{"Name":"m","Provider":"p","enabled":false}`
	var m ModelConfig
	json.Unmarshal([]byte(in), &m)
	if m.Enabled == nil || *m.Enabled {
		t.Errorf("expected Enabled=false, got %v", m.Enabled)
	}
	if m.IsEnabled() {
		t.Error("IsEnabled() must return false")
	}
}

func TestModelConfig_Enabled_UnmarshalCanonicalPascal(t *testing.T) {
	const in = `{"Name":"m","Provider":"p","Enabled":false}`
	var m ModelConfig
	json.Unmarshal([]byte(in), &m)
	if m.Enabled == nil || *m.Enabled {
		t.Errorf("expected Enabled=false, got %v", m.Enabled)
	}
}

func TestModelConfig_Enabled_UnmarshalAbsent(t *testing.T) {
	const in = `{"Name":"m","Provider":"p"}`
	var m ModelConfig
	json.Unmarshal([]byte(in), &m)
	if m.Enabled != nil {
		t.Errorf("expected nil when Enabled absent, got %v", m.Enabled)
	}
	if !m.IsEnabled() {
		t.Error("IsEnabled() must return true when Enabled is nil (default-enabled)")
	}
}
