package middleware

import "nova/internal/agent"

// SSEEventMiddlewareChain applies outbound SSE middleware in registration order.
type SSEEventMiddlewareChain struct {
	middlewares []SSEEventMiddleware
}

// NewSSEEventMiddlewareChain creates the outbound SSE middleware chain for one
// client stream. Optional presentation filters are enabled through options.
func NewSSEEventMiddlewareChain(options ...SSEEventMiddlewareChainOption) *SSEEventMiddlewareChain {
	opts := applySSEEventMiddlewareChainOptions(options...)
	middlewares := make([]SSEEventMiddleware, 0, 1)
	if opts.HideChapterBodyLiveOutput {
		middlewares = append(middlewares, newWriteFileChapterBodySSEMiddleware())
	}
	return newSSEEventMiddlewareChainWithMiddlewares(middlewares...)
}

func newSSEEventMiddlewareChainWithMiddlewares(middlewares ...SSEEventMiddleware) *SSEEventMiddlewareChain {
	return &SSEEventMiddlewareChain{middlewares: append([]SSEEventMiddleware(nil), middlewares...)}
}

func applySSEEventMiddlewareChainOptions(options ...SSEEventMiddlewareChainOption) SSEEventMiddlewareChainOptions {
	var out SSEEventMiddlewareChainOptions
	for _, option := range options {
		if option.F != nil {
			option.F(&out)
		}
	}
	return out
}

// Next returns a handler that runs middleware before the final SSE writer.
func (c *SSEEventMiddlewareChain) Next(final SSEEventHandler) SSEEventHandler {
	if final == nil {
		final = func(agent.Event) error { return nil }
	}
	if c == nil {
		return final
	}
	wrapped := final
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		middleware := c.middlewares[i]
		if middleware == nil {
			continue
		}
		wrapped = middleware.Next(wrapped)
	}
	return wrapped
}
