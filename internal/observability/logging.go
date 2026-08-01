// Package observability owns Denova's process-wide structured logging and
// correlation metadata. Domain packages should log with slog's context-aware
// methods so request-scoped fields can be attached without plumbing them into
// every log call manually.
package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

const (
	// RequestIDHeader is returned on every HTTP response so a client-visible
	// failure can be matched to the server logs.
	RequestIDHeader = "X-Request-ID"
	// RequestIDField is the stable log and error-payload field name.
	RequestIDField = "request_id"
)

type requestIDContextKey struct{}

// WithRequestID returns a child context carrying the current HTTP request ID.
// Empty IDs are ignored so callers cannot accidentally erase an existing ID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

// RequestID returns the request ID carried by ctx, or an empty string for
// background work that is not owned by an HTTP request.
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

// ConfigureStructuredLogging installs Denova's process-wide slog logger.
// The context handler adds correlation fields at emission time, which keeps
// logger instances reusable and prevents request data leaking across calls.
func ConfigureStructuredLogging(output io.Writer) {
	base := slog.NewTextHandler(output, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	})
	slog.SetDefault(slog.New(contextHandler{Handler: base}))
}

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if requestID := RequestID(ctx); requestID != "" {
		record.AddAttrs(slog.String(RequestIDField, requestID))
	}
	return h.Handler.Handle(ctx, record)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}
