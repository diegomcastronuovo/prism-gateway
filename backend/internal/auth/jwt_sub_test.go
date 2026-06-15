package auth

import (
	"context"
	"testing"
)

func TestJWTSubFromContext(t *testing.T) {
	t.Parallel()
	if JWTSubFromContext(context.Background()) != nil {
		t.Fatal("expected nil without sub in context")
	}
	ctx := WithSub(context.Background(), "user-abc")
	p := JWTSubFromContext(ctx)
	if p == nil || *p != "user-abc" {
		t.Fatalf("expected user-abc, got %v", p)
	}
	ctx2 := WithSub(context.Background(), "  ")
	if JWTSubFromContext(ctx2) != nil {
		t.Fatal("whitespace-only sub should be nil")
	}
}
