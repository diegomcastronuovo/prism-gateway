package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// enforcementConfig returns a base config for budget enforcement tests.
// mode: "report_only" | "block" | "degrade"
// blockStatus: 0 → use default (402)
func enforcementConfig(mode string, blockStatus int) *config.Config {
	cfg := testConfig()
	// Add route group "cheap" pointing to model-b for degrade tests
	cfg.Tenants[0].Selection.RouteGroups = map[string][]string{
		"cheap": {"model-b"},
	}
	cfg.Tenants[0].Budgets = config.BudgetsConfig{MonthlyUSD: 100.0}
	cfg.Tenants[0].BudgetEnforcement = config.BudgetEnforcementConfig{
		Enabled:           true,
		Mode:              mode,
		BlockStatus:       blockStatus,
		DegradeRouteGroup: "cheap",
		Thresholds: config.BudgetEnforcementThresholds{
			WarnPct: 0.80,
			HardPct: 1.00,
		},
	}
	return cfg
}

func chatBody() string {
	return `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`
}

func chatBodyWithMetadata(meta string) string {
	return fmt.Sprintf(`{"model":"model-a","messages":[{"role":"user","content":"hi"}],"metadata":%s}`, meta)
}

// TestBudgetEnforcement_ReportOnly_NeverBlocks verifies that report_only mode never blocks
// even when spend exceeds budget.
func TestBudgetEnforcement_ReportOnly_NeverBlocks(t *testing.T) {
	cfg := enforcementConfig("report_only", 0)

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{
		monthlySpend: 150.0, // over the 100.0 budget
	}
	handler := setupTestServerWithStorage(cfg, reg, store)

	w := makeRequest(t, handler, chatBody(), map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("report_only should never block; expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestBudgetEnforcement_Block_Returns402 verifies that block mode returns HTTP 402
// when the budget is exceeded.
func TestBudgetEnforcement_Block_Returns402(t *testing.T) {
	cfg := enforcementConfig("block", 0) // default 402

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{
		budgetErr:   storage.ErrBudgetExceeded,
		budgetCheck: storage.BudgetCheck{MonthSpendUSD: 110.0},
	}
	handler := setupTestServerWithStorage(cfg, reg, store)

	w := makeRequest(t, handler, chatBody(), map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Type != "budget_exceeded" {
		t.Errorf("expected error type 'budget_exceeded', got %q", resp.Error.Type)
	}
	if !strings.Contains(resp.Error.Message, "monthly budget exceeded") {
		t.Errorf("expected 'monthly budget exceeded' in message, got: %s", resp.Error.Message)
	}
}

// TestBudgetEnforcement_Block_CustomStatus429 verifies that block_status is respected.
func TestBudgetEnforcement_Block_CustomStatus429(t *testing.T) {
	cfg := enforcementConfig("block", http.StatusTooManyRequests) // 429

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{
		budgetErr:   storage.ErrBudgetExceeded,
		budgetCheck: storage.BudgetCheck{MonthSpendUSD: 110.0},
	}
	handler := setupTestServerWithStorage(cfg, reg, store)

	w := makeRequest(t, handler, chatBody(), map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 (custom block_status), got %d: %s", w.Code, w.Body.String())
	}
}

// TestBudgetEnforcement_Degrade_ForcesRouteGroup verifies that degrade mode reroutes
// the request to the cheaper route group without blocking.
func TestBudgetEnforcement_Degrade_ForcesRouteGroup(t *testing.T) {
	cfg := enforcementConfig("degrade", 0)

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{
		budgetErr:   storage.ErrBudgetExceeded,
		budgetCheck: storage.BudgetCheck{MonthSpendUSD: 110.0},
	}
	handler := setupTestServerWithStorage(cfg, reg, store)

	w := makeRequest(t, handler, chatBody(), map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("degrade should return 200, got %d: %s", w.Code, w.Body.String())
	}

	selectedModel := w.Header().Get("X-Selected-Model")
	if selectedModel != "model-b" {
		t.Errorf("expected model-b (cheap route group), got %q", selectedModel)
	}
}

// TestBudgetEnforcement_TagBudget_Blocks verifies that tag-level budget enforcement blocks
// a request when the tag spend exceeds its limit, even when tenant budget is under limit.
func TestBudgetEnforcement_TagBudget_Blocks(t *testing.T) {
	cfg := enforcementConfig("block", 0)
	cfg.Tenants[0].BudgetEnforcement.TagBudgets = config.TagBudgetsConfig{
		Enabled: true,
		Keys:    []string{"project"},
		MonthlyUSDByTag: map[string]map[string]float64{
			"project": {"marketing": 50.0},
		},
	}

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{
		// Tenant budget is fine (50 out of 100)
		budgetCheck: storage.BudgetCheck{MonthSpendUSD: 50.0},
		budgetErr:   nil,
		// Tag spend exceeds its 50.0 limit
		tagSpend: map[string]float64{
			"project:marketing": 60.0,
		},
	}
	handler := setupTestServerWithStorage(cfg, reg, store)

	body := chatBodyWithMetadata(`{"project":"marketing"}`)
	w := makeRequest(t, handler, body, map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 (tag budget exceeded), got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if !strings.Contains(resp.Error.Message, "marketing") {
		t.Errorf("expected 'marketing' in error message, got: %s", resp.Error.Message)
	}
	if resp.Error.Type != "budget_exceeded" {
		t.Errorf("expected error type 'budget_exceeded', got %q", resp.Error.Type)
	}
}

// TestBudgetEnforcement_FailOpen_DBError verifies that a DB error during budget enforcement
// allows the request through (fail open), even in block mode.
func TestBudgetEnforcement_FailOpen_DBError(t *testing.T) {
	cfg := enforcementConfig("block", 0)

	reg := providers.NewRegistry()
	reg.Register("openai", successProvider("model-a"))
	reg.Register("backup", successProvider("model-b"))

	store := &fakeStorage{
		// Non-ErrBudgetExceeded error → fail open
		budgetErr: fmt.Errorf("connection refused: db down"),
	}
	handler := setupTestServerWithStorage(cfg, reg, store)

	w := makeRequest(t, handler, chatBody(), map[string]string{"X-API-Key": "key1"})

	if w.Code != http.StatusOK {
		t.Fatalf("DB error should fail open with 200, got %d: %s", w.Code, w.Body.String())
	}
}

// newBudgetTestRecorder posts directly to h.ChatCompletions with a prefilled tenant context.
// Only used when we need to bypass the full server middleware chain.
func newBudgetTestRecorder(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	return w
}
