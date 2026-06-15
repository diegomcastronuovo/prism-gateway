package hooks

import (
	"context"
	"log/slog"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// Decision represents the outcome of a hook evaluation.
type Decision int

const (
	Allow Decision = iota
	Block
	Redact
	Warn
	AllowPII
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Block:
		return "block"
	case Redact:
		return "redact"
	case Warn:
		return "warn"
	case AllowPII:
		return "allow_pii"
	default:
		return "unknown"
	}
}

// PreResult is the result of a PreRequest hook evaluation.
type PreResult struct {
	Decision   Decision
	Request    providers.ChatRequest // possibly mutated
	Reason     string
	StatusCode int // HTTP status for Block responses (0 = default 400)
}

// PostResult is the result of a PostResponse hook evaluation.
type PostResult struct {
	Response *providers.ChatResponse // possibly mutated
	Reason   string
}

// Hook is the interface every security hook must implement.
type Hook interface {
	Name() string
	PreRequest(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest) (PreResult, error)
	PostResponse(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest, resp *providers.ChatResponse) (PostResult, error)
}

// Registry maps hook names to Hook instances.
type Registry struct {
	hooks map[string]Hook
	log   *slog.Logger
}

func NewRegistry(log *slog.Logger) *Registry {
	return &Registry{
		hooks: make(map[string]Hook),
		log:   log,
	}
}

func (r *Registry) Register(name string, h Hook) {
	r.hooks[name] = h
}

func (r *Registry) Get(name string) (Hook, bool) {
	h, ok := r.hooks[name]
	return h, ok
}

// ForTenant returns the ordered list of hooks active for a tenant.
// Hook activation is implicit: regex_block runs when RegexBlock config is present,
// pii runs when PII config is present and enabled.
func (r *Registry) ForTenant(tenant *config.TenantConfig) []Hook {
	var result []Hook

	// regex_block: activated by presence of tenant-level config
	if tenant.Hooks.RegexBlock != nil {
		if h, ok := r.hooks["regex_block"]; ok {
			result = append(result, h)
		}
	}

	// pii: activated when enabled; creates a per-tenant ExternalPII instance
	if tenant.Hooks.PII != nil && tenant.Hooks.PII.Enabled {
		failMode := "fail_closed"
		if tenant.Hooks.PII.FailOpen {
			failMode = "fail_open"
		}
		extCfg := config.ExternalPIIHookConfig{
			Enabled:      true,
			BaseURL:      tenant.Hooks.PII.URL,
			Request:      config.WebhookPhase{Enabled: true, Path: ""},
			TimeoutMs:    tenant.Hooks.PII.TimeoutMs,
			FailMode:     failMode,
			MaxBodyBytes: 1 << 20, // 1 MB
			Auth:         config.WebhookAuth{Type: "none"},
		}
		result = append(result, NewExternalPII(extCfg, r.log))
	}

	return result
}

// BuildFromConfig creates and registers all hooks based on global + tenant config.
func BuildFromConfig(cfg *config.Config, log *slog.Logger) *Registry {
	reg := NewRegistry(log)

	// regex_block: use global defaults, tenants can override via settings
	globalPatterns := cfg.Hooks.RegexBlock.Patterns
	reg.Register("regex_block", NewRegexBlock(globalPatterns))

	// pii_scan: default mode is "redact"
	reg.Register("pii_scan", NewPIIScan("redact"))

	// prompt_injection: default mode is "block"
	reg.Register("prompt_injection", NewPromptInjection("block"))

	return reg
}
