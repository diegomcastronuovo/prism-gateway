package storage

import (
	"encoding/json"
	"testing"
)

// TestToSnakeCase verifies PascalCase/camelCase to snake_case conversion
func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MonthlyUSD", "monthly_usd"},
		{"AllowedModels", "allowed_models"},
		{"RPM", "rpm"},
		{"Budgets", "budgets"},
		{"BudgetThresholds", "budget_thresholds"},
		{"RouteGroups", "route_groups"},
		{"HeaderModelKey", "header_model_key"},
		{"MaxPromptTokens", "max_prompt_tokens"},
		{"WebhookURL", "webhook_url"},
		{"JWKSURL", "jwksurl"}, // Edge case: all caps
		{"id", "id"},           // Already lowercase
		{"", ""},               // Empty string
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toSnakeCase(tt.input)
			if got != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestNormalizeConfigKeys_Simple verifies basic normalization
func TestNormalizeConfigKeys_Simple(t *testing.T) {
	input := map[string]interface{}{
		"MonthlyUSD": 1000.0,
		"Timezone":   "UTC",
	}

	result := normalizeConfigKeys(input)

	expected := map[string]interface{}{
		"monthly_usd": 1000.0,
		"timezone":    "UTC",
	}

	resultMap := result.(map[string]interface{})
	if len(resultMap) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(resultMap))
	}

	for k, v := range expected {
		if resultMap[k] != v {
			t.Errorf("expected %s=%v, got %v", k, v, resultMap[k])
		}
	}
}

// TestNormalizeConfigKeys_Nested verifies nested object normalization
func TestNormalizeConfigKeys_Nested(t *testing.T) {
	input := map[string]interface{}{
		"Budgets": map[string]interface{}{
			"MonthlyUSD": 2000.0,
			"Timezone":   "America/New_York",
		},
		"RateLimit": map[string]interface{}{
			"RPM":   500,
			"Burst": 50,
		},
	}

	result := normalizeConfigKeys(input)

	resultMap := result.(map[string]interface{})
	budgets := resultMap["budgets"].(map[string]interface{})
	if budgets["monthly_usd"] != 2000.0 {
		t.Errorf("expected budgets.monthly_usd=2000.0, got %v", budgets["monthly_usd"])
	}
	if budgets["timezone"] != "America/New_York" {
		t.Errorf("expected budgets.timezone=America/New_York, got %v", budgets["timezone"])
	}

	rateLimit := resultMap["rate_limit"].(map[string]interface{})
	if rateLimit["rpm"] != 500 {
		t.Errorf("expected rate_limit.rpm=500, got %v", rateLimit["rpm"])
	}
}

// TestNormalizeConfigKeys_Array verifies array preservation
func TestNormalizeConfigKeys_Array(t *testing.T) {
	input := map[string]interface{}{
		"AllowedModels": []interface{}{"gpt-4", "gpt-3.5-turbo"},
		"BudgetThresholds": []interface{}{0.7, 0.85, 1.0},
	}

	result := normalizeConfigKeys(input)

	resultMap := result.(map[string]interface{})
	models := resultMap["allowed_models"].([]interface{})
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0] != "gpt-4" {
		t.Errorf("expected first model gpt-4, got %v", models[0])
	}

	thresholds := resultMap["budget_thresholds"].([]interface{})
	if len(thresholds) != 3 {
		t.Fatalf("expected 3 thresholds, got %d", len(thresholds))
	}
}

// TestNormalizeJSONConfig_FullTenantConfig verifies full config normalization
func TestNormalizeJSONConfig_FullTenantConfig(t *testing.T) {
	// Simulate legacy PascalCase config from DB
	legacyConfig := `{
		"ID": "tenant_a",
		"AllowedModels": ["gpt-4o-mini"],
		"Routing": {
			"Strategy": "cost_based",
			"Fallback": {
				"Enabled": true,
				"TimeoutMs": 5000
			}
		},
		"Budgets": {
			"MonthlyUSD": 1000,
			"Timezone": "UTC"
		},
		"RateLimit": {
			"RPM": 100,
			"Burst": 10
		},
		"Compliance": {
			"RetentionDays": 30,
			"LogMode": "metadata_only"
		}
	}`

	normalized, err := NormalizeJSONConfig(json.RawMessage(legacyConfig))
	if err != nil {
		t.Fatalf("NormalizeJSONConfig failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(normalized, &result); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}

	// Verify top-level keys are snake_case
	if _, ok := result["allowed_models"]; !ok {
		t.Error("expected allowed_models key")
	}
	if _, ok := result["AllowedModels"]; ok {
		t.Error("should not have AllowedModels (PascalCase) key")
	}

	// Verify nested keys
	budgets := result["budgets"].(map[string]interface{})
	if _, ok := budgets["monthly_usd"]; !ok {
		t.Error("expected budgets.monthly_usd key")
	}
	if _, ok := budgets["MonthlyUSD"]; ok {
		t.Error("should not have budgets.MonthlyUSD (PascalCase) key")
	}

	routing := result["routing"].(map[string]interface{})
	fallback := routing["fallback"].(map[string]interface{})
	if _, ok := fallback["timeout_ms"]; !ok {
		t.Error("expected routing.fallback.timeout_ms key")
	}
}

// TestNormalizeJSONConfig_PreservesValues verifies values are not modified
func TestNormalizeJSONConfig_PreservesValues(t *testing.T) {
	input := `{
		"Budgets": {
			"MonthlyUSD": 501.25,
			"Timezone": "America/New_York"
		},
		"AllowedModels": ["model-a", "model-b"],
		"RateLimit": {
			"RPM": 999,
			"Burst": 50
		}
	}`

	normalized, err := NormalizeJSONConfig(json.RawMessage(input))
	if err != nil {
		t.Fatalf("NormalizeJSONConfig failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(normalized, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	budgets := result["budgets"].(map[string]interface{})
	if budgets["monthly_usd"] != 501.25 {
		t.Errorf("expected monthly_usd=501.25, got %v", budgets["monthly_usd"])
	}
	if budgets["timezone"] != "America/New_York" {
		t.Errorf("expected timezone=America/New_York, got %v", budgets["timezone"])
	}

	models := result["allowed_models"].([]interface{})
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	rateLimit := result["rate_limit"].(map[string]interface{})
	if rateLimit["rpm"].(float64) != 999 {
		t.Errorf("expected rpm=999, got %v", rateLimit["rpm"])
	}
}

// TestNormalizeJSONConfig_EmptyAndNull verifies edge cases
func TestNormalizeJSONConfig_EmptyAndNull(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty object", "{}"},
		{"null values", `{"Budgets": null}`},
		{"empty arrays", `{"AllowedModels": []}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeJSONConfig(json.RawMessage(tt.input))
			if err != nil {
				t.Errorf("NormalizeJSONConfig failed on %s: %v", tt.name, err)
			}
		})
	}
}

// TestFinalizeGlobalConfigJSON_ProvidersCanonicalKeys verifies providers are stored with
// canonical JSON keys only (no duplicate snake_case alongside PascalCase).
func TestFinalizeGlobalConfigJSON_ProvidersCanonicalKeys(t *testing.T) {
	input := `{
		"providers": {
			"local": {
				"Type": "local",
				"BaseURL": "http://localhost:9001",
				"Enabled": false,
				"APIKeyEnv": "",
				"type": "local",
				"base_url": "http://localhost:9001",
				"enabled": false,
				"api_key_env": ""
			}
		},
		"models": [],
		"circuit_breaker": {},
		"rate_limit": {},
		"smart_routing": {}
	}`

	finalized, err := FinalizeGlobalConfigJSON(json.RawMessage(input))
	if err != nil {
		t.Fatalf("FinalizeGlobalConfigJSON failed: %v", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(finalized, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	prov := root["providers"].(map[string]interface{})
	local := prov["local"].(map[string]interface{})

	for _, bad := range []string{"base_url", "enabled", "api_key_env", "type"} {
		if _, ok := local[bad]; ok {
			t.Errorf("unexpected legacy key %q in normalized provider: %#v", bad, local)
		}
	}
	for _, want := range []string{"Type", "BaseURL", "Enabled", "APIKeyEnv"} {
		if _, ok := local[want]; !ok {
			t.Errorf("missing canonical key %q in %v", want, local)
		}
	}
	if local["BaseURL"] != "http://localhost:9001" {
		t.Errorf("BaseURL=%v", local["BaseURL"])
	}
	if local["Enabled"] != false {
		t.Errorf("Enabled=%v", local["Enabled"])
	}
}

// TestFinalizeGlobalConfigJSON_MergedProviderRemovesLegacyKeys simulates a merge map where
// PascalCase and snake_case keys both exist before finalize (PATCH merge without full finalize).
func TestFinalizeGlobalConfigJSON_MergedProviderRemovesLegacyKeys(t *testing.T) {
	merged := map[string]interface{}{
		"providers": map[string]interface{}{
			"local": map[string]interface{}{
				"Type":      "local",
				"BaseURL":   "http://localhost:9001",
				"APIKeyEnv": "",
				"Enabled":   false,
				"base_url":  "http://localhost:9001",
				"enabled":   false,
			},
		},
		"models": []interface{}{},
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	out, err := FinalizeGlobalConfigJSON(raw)
	if err != nil {
		t.Fatalf("FinalizeGlobalConfigJSON: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	local := root["providers"].(map[string]interface{})["local"].(map[string]interface{})
	for _, bad := range []string{"base_url", "enabled", "api_key_env", "type"} {
		if _, ok := local[bad]; ok {
			t.Errorf("unexpected legacy key %q: %#v", bad, local)
		}
	}
}

// TestFinalizeGlobalConfigJSON_PreservesAwsBedrockSecret verifies aws_secret_access_key is not
// dropped when canonicalizing providers (json:"-" on ProviderConfig previously stripped it).
func TestFinalizeGlobalConfigJSON_PreservesAwsBedrockSecret(t *testing.T) {
	input := `{
		"providers": {
			"bedrock": {
				"type": "aws_bedrock",
				"aws_access_key_id": "AKIATEST",
				"aws_secret_access_key": "persist-this-secret",
				"aws_region": "us-east-1"
			}
		},
		"models": [],
		"circuit_breaker": {},
		"rate_limit": {},
		"smart_routing": {}
	}`
	finalized, err := FinalizeGlobalConfigJSON(json.RawMessage(input))
	if err != nil {
		t.Fatalf("FinalizeGlobalConfigJSON: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(finalized, &root); err != nil {
		t.Fatal(err)
	}
	bedrock := root["providers"].(map[string]interface{})["bedrock"].(map[string]interface{})
	sk, ok := bedrock["aws_secret_access_key"].(string)
	if !ok || sk != "persist-this-secret" {
		t.Fatalf("aws_secret_access_key not preserved in finalized config: %#v", bedrock)
	}
}

func TestFinalizeGlobalConfigJSON_ModelsProviderModelIDSnakeCase(t *testing.T) {
	input := `{
		"providers": {},
		"models": [
			{
				"Name": "claude-bedrock",
				"Provider": "bedrock",
				"ProviderModelID": "anthropic.claude-3-5-sonnet-20240620-v1:0",
				"Type": "chat",
				"Pricing": { "PromptPer1M": 1, "CompletionPer1M": 2 }
			}
		],
		"circuit_breaker": {},
		"rate_limit": {},
		"smart_routing": {}
	}`
	finalized, err := FinalizeGlobalConfigJSON(json.RawMessage(input))
	if err != nil {
		t.Fatalf("FinalizeGlobalConfigJSON: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(finalized, &root); err != nil {
		t.Fatal(err)
	}
	models := root["models"].([]interface{})
	m0 := models[0].(map[string]interface{})
	if _, bad := m0["ProviderModelID"]; bad {
		t.Fatalf("expected ProviderModelID removed after finalize: %#v", m0)
	}
	if got, ok := m0["provider_model_id"].(string); !ok || got != "anthropic.claude-3-5-sonnet-20240620-v1:0" {
		t.Fatalf("provider_model_id=%v %#v", got, m0)
	}
}
