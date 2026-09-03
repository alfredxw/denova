package chat

import (
	"context"

	agent "github.com/alfredxw/denova/agent"
)

type defaultMaxTokensMiddleware struct {
	*agent.BaseMiddleware
	maxOutputTokens int
}

// NewDefaultMaxTokensMiddleware projects the provider-profile default into the
// final provider-neutral call. Explicit per-call options keep precedence.
func NewDefaultMaxTokensMiddleware(maxOutputTokens int) agent.Middleware {
	return &defaultMaxTokensMiddleware{
		BaseMiddleware:  &agent.BaseMiddleware{},
		maxOutputTokens: maxOutputTokens,
	}
}

func (middleware *defaultMaxTokensMiddleware) BeforeModelCall(
	ctx context.Context,
	call *agent.ModelCall,
	_ *agent.ModelContext,
) (context.Context, *agent.ModelCall, error) {
	if call == nil || middleware.maxOutputTokens <= 0 {
		return ctx, call, nil
	}
	if agent.GetCommonOptions(&agent.Options{}, call.Options...).MaxTokens != nil {
		return ctx, call, nil
	}
	next := *call
	next.Options = append(append([]agent.ModelOption(nil), call.Options...), agent.WithMaxTokens(middleware.maxOutputTokens))
	return ctx, &next, nil
}
