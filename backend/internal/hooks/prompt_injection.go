package hooks

import (
	"context"
	"regexp"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+instructions`),
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?prior\s+instructions`),
	regexp.MustCompile(`(?i)disregard\s+(all\s+)?previous\s+instructions`),
	regexp.MustCompile(`(?i)reveal\s+(your\s+)?(hidden|system|secret)`),
	regexp.MustCompile(`(?i)show\s+(me\s+)?(your\s+)?system\s+prompt`),
	regexp.MustCompile(`(?i)what\s+(is|are)\s+your\s+system\s+(prompt|instructions)`),
	regexp.MustCompile(`(?i)override\s+(your\s+)?(instructions|rules|guidelines)`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(a|an|in)\s+`),
	regexp.MustCompile(`(?i)act\s+as\s+if\s+you\s+have\s+no\s+restrictions`),
	regexp.MustCompile(`(?i)jailbreak`),
	regexp.MustCompile(`(?i)DAN\s+mode`),
}

// PromptInjection detects common prompt injection attempts in user messages.
// Mode: "block" rejects the request, "warn" allows but flags it.
type PromptInjection struct {
	defaultMode string
}

func NewPromptInjection(defaultMode string) *PromptInjection {
	if defaultMode == "" {
		defaultMode = "block"
	}
	return &PromptInjection{defaultMode: defaultMode}
}

func (h *PromptInjection) Name() string { return "prompt_injection" }

func (h *PromptInjection) PreRequest(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest) (PreResult, error) {
	mode := h.modeForTenant(tenant)

	for _, msg := range req.Messages {
		if strings.ToLower(msg.Role) != "user" {
			continue
		}

		for _, re := range injectionPatterns {
			if re.MatchString(msg.Content) {
				matched := re.FindString(msg.Content)
				reason := "potential prompt injection detected: \"" + matched + "\""

				if mode == "block" {
					return PreResult{
						Decision: Block,
						Request:  req,
						Reason:   reason,
					}, nil
				}

				// mode == "warn"
				return PreResult{
					Decision: Warn,
					Request:  req,
					Reason:   reason,
				}, nil
			}
		}
	}

	return PreResult{Decision: Allow, Request: req}, nil
}

func (h *PromptInjection) PostResponse(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest, resp *providers.ChatResponse) (PostResult, error) {
	return PostResult{Response: resp}, nil
}

func (h *PromptInjection) modeForTenant(_ *config.TenantConfig) string {
	return h.defaultMode
}
