package router

import "fmt"

// Router-specific errors with HTTP mapping semantics

// ErrBlockedBySmartStage indicates a smart stage blocked the request.
// Maps to 403 for content-policy blocks; 400 when IsPromptLength is true and no custom reason.
type ErrBlockedBySmartStage struct {
	Reason          string
	Stage           string
	IsPromptLength  bool // true when block was triggered by a prompt_length condition
	PromptLength    int  // character count at the time of blocking
	HasCustomReason bool // true when rule.Action.Reason was explicitly set by the operator
}

func (e *ErrBlockedBySmartStage) Error() string {
	return fmt.Sprintf("request blocked by smart stage '%s': %s", e.Stage, e.Reason)
}

// ErrNoCandidatesAfterSmartStages indicates smart stages filtered all models (maps to 400)
type ErrNoCandidatesAfterSmartStages struct {
	PreferredModels []string
	BannedModels    []string
	ConstraintsUsed bool
}

func (e *ErrNoCandidatesAfterSmartStages) Error() string {
	if len(e.BannedModels) > 0 {
		return fmt.Sprintf("no candidates remain after smart stage bans: %v", e.BannedModels)
	}
	if e.ConstraintsUsed {
		return "no candidates remain after smart stage constraints (cost/latency)"
	}
	if len(e.PreferredModels) > 0 {
		return fmt.Sprintf("no candidates remain after smart stage preferences: %v", e.PreferredModels)
	}
	return "no candidates remain after smart stage filtering"
}

// ErrNoAllowedModels indicates tenant has no allowed models (maps to 400)
type ErrNoAllowedModels struct {
	TenantID string
}

func (e *ErrNoAllowedModels) Error() string {
	return fmt.Sprintf("no models allowed for tenant '%s'", e.TenantID)
}

// ErrAllCandidatesCircuitBroken indicates circuit breaker filtered all models (maps to 503)
type ErrAllCandidatesCircuitBroken struct {
	OpenProviders []string
}

func (e *ErrAllCandidatesCircuitBroken) Error() string {
	return fmt.Sprintf("all candidate providers are circuit broken: %v", e.OpenProviders)
}

// ErrModelNotAllowed indicates requested model is not in tenant allowlist (maps to 403)
type ErrModelNotAllowed struct {
	Model    string
	TenantID string
}

func (e *ErrModelNotAllowed) Error() string {
	return fmt.Sprintf("model '%s' is not allowed for tenant '%s'", e.Model, e.TenantID)
}

// ErrModelNotInRouteGroup indicates requested model not in specified route group (maps to 400)
type ErrModelNotInRouteGroup struct {
	Model      string
	RouteGroup string
}

func (e *ErrModelNotInRouteGroup) Error() string {
	return fmt.Sprintf("model '%s' is not in route group '%s'", e.Model, e.RouteGroup)
}

// ErrRouteGroupNotFound indicates route group doesn't exist (maps to 400)
type ErrRouteGroupNotFound struct {
	RouteGroup string
}

func (e *ErrRouteGroupNotFound) Error() string {
	return fmt.Sprintf("route group '%s' not found", e.RouteGroup)
}

// ErrDefaultRouteGroupEmpty indicates that routing.route_group produced an empty pool
// after intersection with allowed_models (maps to 400 invalid_configuration).
type ErrDefaultRouteGroupEmpty struct {
	RouteGroup string
}

func (e *ErrDefaultRouteGroupEmpty) Error() string {
	return fmt.Sprintf("routing route_group '%s' has no available candidate models", e.RouteGroup)
}

// ErrAllAttemptsFailed is a marker for when all fallback attempts failed (maps to 502)
// The actual error is propagated from the last provider attempt
type ErrAllAttemptsFailed struct {
	Attempts  int
	LastError error
}

func (e *ErrAllAttemptsFailed) Error() string {
	return fmt.Sprintf("all %d attempts failed, last error: %v", e.Attempts, e.LastError)
}

func (e *ErrAllAttemptsFailed) Unwrap() error {
	return e.LastError
}
