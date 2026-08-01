package modelio

import (
	"context"
	"strings"
)

type traceSourceKey struct{}

// WithTraceSource attributes a nested provider call without coupling the
// caller to the model-input logging middleware implementation.
func WithTraceSource(ctx context.Context, source string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if source = strings.TrimSpace(source); source == "" {
		return ctx
	}
	return context.WithValue(ctx, traceSourceKey{}, source)
}

// TraceSource returns the explicit provider-call source or the normal Agent
// source when no nested operation supplied one.
func TraceSource(ctx context.Context) string {
	if ctx != nil {
		if source, _ := ctx.Value(traceSourceKey{}).(string); strings.TrimSpace(source) != "" {
			return strings.TrimSpace(source)
		}
	}
	return "agent"
}
