package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// SPEC_150: Error logging — routing errors must be persisted to request_log.

// Test 1: block by keyword → status=error, error_type=content_policy_violation, rules_matched in snapshot
func TestSpec150_KeywordBlock_LoggedToRequestLog(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].Routing.Strategy = "smart"
	cfg.Tenants[0].Routing.Smart = config.SmartConfig{
		Stages: []config.SmartStage{{
			Name: "keyword_guard",
			Rules: []config.SmartStageRule{{
				When:   config.SmartRuleCondition{Contains: []string{"forbidden"}},
				Action: config.SmartAction{Block: true, Reason: "keyword blocked"},
			}},
		}},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"messages":[{"role":"user","content":"this has forbidden content"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	reqs := store.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected a request_log row to be inserted for blocked request")
	}

	rl := reqs[0]
	if rl.Status != "error" {
		t.Errorf("Status = %q, want %q", rl.Status, "error")
	}
	if rl.ErrorType != "content_policy_violation" {
		t.Errorf("ErrorType = %q, want %q", rl.ErrorType, "content_policy_violation")
	}
	if rl.Error == "" {
		t.Error("Error field must not be empty")
	}
	if rl.TenantID == "" {
		t.Error("TenantID must not be empty")
	}

	// DecisionSnapshot must contain smart_evaluation with rules_matched
	if rl.DecisionSnapshot == nil {
		t.Fatal("DecisionSnapshot must not be nil for blocked request")
	}
	var snap map[string]interface{}
	if err := json.Unmarshal(rl.DecisionSnapshot, &snap); err != nil {
		t.Fatalf("unmarshal DecisionSnapshot: %v", err)
	}
	smartRaw, ok := snap["smart"]
	if !ok {
		t.Fatal("DecisionSnapshot must have 'smart' field")
	}
	smart := smartRaw.(map[string]interface{})
	if smart["blocked"] != true {
		t.Errorf("smart.blocked = %v, want true", smart["blocked"])
	}
	// SPEC_149 integration: smart_evaluation.rules_matched must be present
	evalRaw, ok := smart["smart_evaluation"]
	if !ok {
		t.Fatal("smart.smart_evaluation must be present when rules matched")
	}
	eval := evalRaw.(map[string]interface{})
	rulesRaw, ok := eval["rules_matched"]
	if !ok {
		t.Fatal("smart_evaluation.rules_matched must be present")
	}
	rules := rulesRaw.([]interface{})
	if len(rules) == 0 {
		t.Error("rules_matched must not be empty for a matched block")
	}
}

// Test 2: prompt_length block → status=error logged (custom reason path)
func TestSpec150_PromptLengthBlock_LoggedToRequestLog(t *testing.T) {
	threshold := 5
	cfg := testConfig()
	cfg.Tenants[0].Routing.Strategy = "smart"
	cfg.Tenants[0].Routing.Smart = config.SmartConfig{
		Stages: []config.SmartStage{{
			Name: "size_guard",
			Rules: []config.SmartStageRule{{
				When:   config.SmartRuleCondition{PromptLength: &config.PromptLengthCondition{GT: &threshold}},
				Action: config.SmartAction{Block: true, Reason: "too long"},
			}},
		}},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"messages":[{"role":"user","content":"this message is definitely longer than 5 chars"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	reqs := store.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected a request_log row for prompt_length block")
	}
	rl := reqs[0]
	if rl.Status != "error" {
		t.Errorf("Status = %q, want %q", rl.Status, "error")
	}
	if rl.Error == "" {
		t.Error("Error must not be empty")
	}
	if rl.DecisionSnapshot == nil {
		t.Error("DecisionSnapshot must not be nil")
	}
}

// Test 3: success path — request_log row present with status=ok (unchanged behavior)
func TestSpec150_Success_StillLogged(t *testing.T) {
	cfg := testConfig()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := `{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	reqs := store.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected a request_log row for successful request")
	}
	// At least one row must have status=ok
	hasOK := false
	for _, rl := range reqs {
		if rl.Status == "ok" {
			hasOK = true
		}
	}
	if !hasOK {
		t.Error("expected at least one request_log row with status=ok")
	}
	// No duplicate: success must not produce an extra error row
	for _, rl := range reqs {
		if rl.Status == "error" {
			t.Errorf("unexpected error row in request_log for successful request: error=%q", rl.Error)
		}
	}
}
