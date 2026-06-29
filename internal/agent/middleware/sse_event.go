package middleware

import "nova/internal/agent"

// SSEEventHandler writes or forwards one outbound SSE event.
type SSEEventHandler func(agent.Event) error

// SSEEventMiddleware chains the next SSE output handler. A middleware forwards an
// event by calling next, mutates output by passing a copied event to next, and
// suppresses output by returning without calling next.
type SSEEventMiddleware interface {
	Next(SSEEventHandler) SSEEventHandler
}

// SSEEventMiddlewareChainOption applies one chain option.
type SSEEventMiddlewareChainOption struct {
	F func(*SSEEventMiddlewareChainOptions)
}

// SSEEventMiddlewareChainOptions holds outbound SSE middleware chain settings.
type SSEEventMiddlewareChainOptions struct {
	HideChapterBodyLiveOutput bool
}

// WithHideChapterBodyLiveOutput controls whether novel chapter bodies are
// hidden from outbound SSE output.
func WithHideChapterBodyLiveOutput(enabled bool) SSEEventMiddlewareChainOption {
	return SSEEventMiddlewareChainOption{F: func(o *SSEEventMiddlewareChainOptions) {
		o.HideChapterBodyLiveOutput = enabled
	}}
}
