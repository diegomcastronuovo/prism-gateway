package hooks

import (
	"context"
	"regexp"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

var (
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRegex = regexp.MustCompile(`(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`)
)

const redactPlaceholder = "[REDACTED]"

// PIIScan detects emails and phone numbers in message content.
// Mode: "block" rejects the request, "redact" replaces PII with [REDACTED].
type PIIScan struct {
	defaultMode string
}

func NewPIIScan(defaultMode string) *PIIScan {
	if defaultMode == "" {
		defaultMode = "redact"
	}
	return &PIIScan{defaultMode: defaultMode}
}

func (h *PIIScan) Name() string { return "pii_scan" }

func (h *PIIScan) PreRequest(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest) (PreResult, error) {
	mode := h.modeForTenant(tenant)

	for i, msg := range req.Messages {
		hasEmail := emailRegex.MatchString(msg.Content)
		hasPhone := phoneRegex.MatchString(msg.Content)

		// Scan tool call arguments (raw JSON — PII may appear in function arguments).
		if len(msg.ToolCalls) > 0 {
			tc := string(msg.ToolCalls)
			hasEmail = hasEmail || emailRegex.MatchString(tc)
			hasPhone = hasPhone || phoneRegex.MatchString(tc)
		}
		// Scan text content blocks (multimodal messages).
		for _, block := range msg.ContentBlocks {
			if block.Type == "text" {
				hasEmail = hasEmail || emailRegex.MatchString(block.Text)
				hasPhone = hasPhone || phoneRegex.MatchString(block.Text)
			}
		}

		if !hasEmail && !hasPhone {
			continue
		}

		if mode == "block" {
			return PreResult{
				Decision: Block,
				Request:  req,
				Reason:   "PII detected in message content (email or phone number)",
			}, nil
		}

		// mode == "redact"
		content := msg.Content
		content = emailRegex.ReplaceAllString(content, redactPlaceholder)
		content = phoneRegex.ReplaceAllString(content, redactPlaceholder)
		req.Messages[i].Content = content
		for j, block := range msg.ContentBlocks {
			if block.Type == "text" {
				block.Text = emailRegex.ReplaceAllString(block.Text, redactPlaceholder)
				block.Text = phoneRegex.ReplaceAllString(block.Text, redactPlaceholder)
				req.Messages[i].ContentBlocks[j] = block
			}
		}
	}

	// Check if any redaction happened
	for _, msg := range req.Messages {
		if emailRegex.MatchString(msg.Content) || phoneRegex.MatchString(msg.Content) {
			// shouldn't happen after redaction, but just in case
			continue
		}
	}

	// If we made any changes in redact mode, return Redact decision
	if mode == "redact" {
		for _, msg := range req.Messages {
			if containsRedacted(msg.Content) {
				return PreResult{
					Decision: Redact,
					Request:  req,
					Reason:   "PII redacted from message content",
				}, nil
			}
		}
	}

	return PreResult{Decision: Allow, Request: req}, nil
}

func (h *PIIScan) PostResponse(ctx context.Context, tenant *config.TenantConfig, req providers.ChatRequest, resp *providers.ChatResponse) (PostResult, error) {
	mode := h.modeForTenant(tenant)
	if mode != "redact" {
		return PostResult{Response: resp}, nil
	}

	for i, choice := range resp.Choices {
		content := choice.Message.Content
		content = emailRegex.ReplaceAllString(content, redactPlaceholder)
		content = phoneRegex.ReplaceAllString(content, redactPlaceholder)
		resp.Choices[i].Message.Content = content
	}

	return PostResult{Response: resp}, nil
}

func (h *PIIScan) modeForTenant(_ *config.TenantConfig) string {
	return h.defaultMode
}

func containsRedacted(s string) bool {
	return len(s) >= len(redactPlaceholder) && containsSubstring(s, redactPlaceholder)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
