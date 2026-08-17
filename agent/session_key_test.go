package agent

import (
	"context"
	"testing"
)

func TestBindContextSessionKeyPreservesExplicitOption(t *testing.T) {
	ctx := ContextWithSessionKey(context.Background(), "context-session")
	resolved := GetCommonOptions(nil, BindContextSessionKey(ctx, nil, WithSessionKey("explicit-session"))...)
	if resolved.SessionKey != "explicit-session" {
		t.Fatalf("session key = %q", resolved.SessionKey)
	}
}

func TestBindContextSessionKeyUsesContextFallback(t *testing.T) {
	ctx := ContextWithSessionKey(context.Background(), " context-session ")
	resolved := GetCommonOptions(nil, BindContextSessionKey(ctx, nil)...)
	if resolved.SessionKey != "context-session" {
		t.Fatalf("session key = %q", resolved.SessionKey)
	}
}
