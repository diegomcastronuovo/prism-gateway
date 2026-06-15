package providers

import (
	"context"
	"testing"
)

func TestWithMessagesAPIKey_SetsOverride(t *testing.T) {
	ctx := WithMessagesAPIKey(context.Background(), "test-key")
	if got := messagesAPIKeyFromCtx(ctx); got != "test-key" {
		t.Errorf("expected test-key, got %q", got)
	}
}

func TestWithMessagesAPIKey_EmptyKeyNoOp(t *testing.T) {
	ctx := WithMessagesAPIKey(context.Background(), "")
	if got := messagesAPIKeyFromCtx(ctx); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestResolveMessagesAPIKey_CtxOverridesProvider(t *testing.T) {
	a := NewAnthropic("https://api.anthropic.com", "provider-key")
	ctx := WithMessagesAPIKey(context.Background(), "override-key")
	if got := a.resolveMessagesAPIKey(ctx); got != "override-key" {
		t.Errorf("expected override-key, got %q", got)
	}
}

func TestResolveMessagesAPIKey_FallsBackToProviderKey(t *testing.T) {
	a := NewAnthropic("https://api.anthropic.com", "provider-key")
	if got := a.resolveMessagesAPIKey(context.Background()); got != "provider-key" {
		t.Errorf("expected provider-key, got %q", got)
	}
}

func TestResolveMessagesAPIKey_BothEmpty(t *testing.T) {
	a := NewAnthropic("https://api.anthropic.com", "")
	if got := a.resolveMessagesAPIKey(context.Background()); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
