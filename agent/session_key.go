package agent

import (
	"context"
	"strings"
)

type sessionKeyContextKey struct{}

// ContextWithSessionKey binds the stable conversation identity used for
// provider cache routing. An empty key deliberately leaves the context
// unchanged so callers cannot accidentally erase an existing binding.
func ContextWithSessionKey(ctx context.Context, sessionKey string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionKeyContextKey{}, sessionKey)
}

// SessionKeyFromContext returns the caller-owned stable conversation key.
func SessionKeyFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	sessionKey, ok := ctx.Value(sessionKeyContextKey{}).(string)
	sessionKey = strings.TrimSpace(sessionKey)
	return sessionKey, ok && sessionKey != ""
}
