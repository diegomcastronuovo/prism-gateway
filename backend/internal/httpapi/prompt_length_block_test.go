package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// configWithPromptLengthStage builds a config with a smart stage that blocks when
// the prompt_length condition matches. customReason may be empty to test fallback behavior.
func configWithPromptLengthStage(stageName, customReason string, condFn func(*config.PromptLengthCondition)) *config.Config {
	cfg := testConfig()
	cfg.Tenants[0].Routing.Strategy = "smart"
	cond := &config.PromptLengthCondition{}
	condFn(cond)
	cfg.Tenants[0].Routing.Smart = config.SmartConfig{
		Stages: []config.SmartStage{
			{
				Name: stageName,
				Rules: []config.SmartStageRule{
					{
						When:   config.SmartRuleCondition{PromptLength: cond},
						Action: config.SmartAction{Block: true, Reason: customReason},
					},
				},
			},
		},
	}
	return cfg
}

// doPromptLengthRequest sends a chat completion with the given content and returns the recorder.
func doPromptLengthRequest(t *testing.T, cfg *config.Config, content string) *httptest.ResponseRecorder {
	t.Helper()
	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"messages":[{"role":"user","content":"` + content + `"}]}`
	return makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})
}

func parseErrorResponse(t *testing.T, body string) (msg, errType string) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	errObj, _ := resp["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatalf("missing error object in response: %s", body)
	}
	return errObj["message"].(string), errObj["type"].(string)
}

// Case 1: prompt_length.lt + block + custom reason → 403 + custom reason + content_policy_violation
func TestPromptLengthBlock_LT_CustomReason(t *testing.T) {
	threshold := 100
	cfg := configWithPromptLengthStage("size_guard", "Pocos caracteres", func(c *config.PromptLengthCondition) {
		c.LT = &threshold
	})

	// "short" is < 100 chars, triggers the lt condition
	w := doPromptLengthRequest(t, cfg, "short")

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	msg, errType := parseErrorResponse(t, w.Body.String())
	if !strings.Contains(msg, "Pocos caracteres") {
		t.Errorf("expected custom reason in message, got: %s", msg)
	}
	if strings.Contains(msg, "prompt length exceeds allowed limit") {
		t.Errorf("got generic message instead of custom reason: %s", msg)
	}
	if errType != "content_policy_violation" {
		t.Errorf("expected type=content_policy_violation, got: %s", errType)
	}
}

// Case 2: prompt_length.gt + block + custom reason → 403 + custom reason + content_policy_violation
func TestPromptLengthBlock_GT_CustomReason(t *testing.T) {
	threshold := 5
	cfg := configWithPromptLengthStage("size_guard", "Demasiados caracteres", func(c *config.PromptLengthCondition) {
		c.GT = &threshold
	})

	// message is > 5 chars, triggers the gt condition
	w := doPromptLengthRequest(t, cfg, "this is a long enough message")

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	msg, errType := parseErrorResponse(t, w.Body.String())
	if !strings.Contains(msg, "Demasiados caracteres") {
		t.Errorf("expected custom reason in message, got: %s", msg)
	}
	if strings.Contains(msg, "prompt length exceeds allowed limit") {
		t.Errorf("got generic message instead of custom reason: %s", msg)
	}
	if errType != "content_policy_violation" {
		t.Errorf("expected type=content_policy_violation, got: %s", errType)
	}
}

// Case 3: prompt_length + block + empty reason → 400 + generic message (fallback preserved)
func TestPromptLengthBlock_NoCustomReason(t *testing.T) {
	threshold := 5
	cfg := configWithPromptLengthStage("size_guard", "" /* no custom reason */, func(c *config.PromptLengthCondition) {
		c.GT = &threshold
	})

	w := doPromptLengthRequest(t, cfg, "this is a long enough message")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	msg, errType := parseErrorResponse(t, w.Body.String())
	if msg != "prompt length exceeds allowed limit" {
		t.Errorf("expected generic prompt-length message, got: %s", msg)
	}
	if errType != "invalid_request_error" {
		t.Errorf("expected type=invalid_request_error, got: %s", errType)
	}
}

// Case 4: contains + block + custom reason → unchanged (regression guard)
func TestContainsBlock_CustomReason(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].Routing.Strategy = "smart"
	cfg.Tenants[0].Routing.Smart = config.SmartConfig{
		Stages: []config.SmartStage{
			{
				Name: "keyword_guard",
				Rules: []config.SmartStageRule{
					{
						When:   config.SmartRuleCondition{Contains: []string{"forbidden"}},
						Action: config.SmartAction{Block: true, Reason: "keyword blocked"},
					},
				},
			},
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider(""))
	store := &fakeStorage{}
	handler := setupTestServerWithStorage(cfg, reg, store)
	body := `{"messages":[{"role":"user","content":"this contains forbidden content"}]}`
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	msg, errType := parseErrorResponse(t, w.Body.String())
	if !strings.Contains(msg, "keyword blocked") {
		t.Errorf("expected custom reason in message, got: %s", msg)
	}
	if errType != "content_policy_violation" {
		t.Errorf("expected type=content_policy_violation, got: %s", errType)
	}
}
