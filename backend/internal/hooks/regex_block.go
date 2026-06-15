package hooks

import (
	"context"
	"regexp"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// RegexBlock blocks requests whose message content matches any configured pattern.
type RegexBlock struct {
	defaultPatterns []*regexp.Regexp
}

func NewRegexBlock(patterns []string) *RegexBlock {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if r, err := regexp.Compile(p); err == nil {
			compiled = append(compiled, r)
		}
	}
	return &RegexBlock{defaultPatterns: compiled}
}

func (h *RegexBlock) Name() string { return "regex_block" }

func (h *RegexBlock) PreRequest(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest) (PreResult, error) {
	patterns := h.patternsForTenant(tenant)
	if len(patterns) == 0 {
		return PreResult{Decision: Allow, Request: req}, nil
	}

	for _, msg := range req.Messages {
		for _, re := range patterns {
			if re.MatchString(msg.Content) {
				return PreResult{
					Decision: Block,
					Request:  req,
					Reason:   "message content matched a blocked pattern",
				}, nil
			}
		}
	}

	return PreResult{Decision: Allow, Request: req}, nil
}

func (h *RegexBlock) PostResponse(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest, resp *providers.ChatResponse) (PostResult, error) {
	return PostResult{Response: resp}, nil
}

func (h *RegexBlock) patternsForTenant(tenant *config.TenantConfig) []*regexp.Regexp {
	// Tenant-level override if configured
	if tenant.Hooks.RegexBlock != nil && len(tenant.Hooks.RegexBlock.Patterns) > 0 {
		compiled := make([]*regexp.Regexp, 0, len(tenant.Hooks.RegexBlock.Patterns))
		for _, p := range tenant.Hooks.RegexBlock.Patterns {
			if r, err := regexp.Compile(p); err == nil {
				compiled = append(compiled, r)
			}
		}
		return compiled
	}
	return h.defaultPatterns
}
