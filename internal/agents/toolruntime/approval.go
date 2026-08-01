package toolruntime

import (
	"context"

	"denova/config"
	"denova/internal/agents/toolapproval"
)

// ApprovalRequest is the bounded execution identity and policy decision shown
// by a host before one tool call may proceed.
type ApprovalRequest struct {
	Mode           config.AgentApprovalMode
	ToolName       string
	ProviderCallID string
	ExecutionID    string
	Arguments      string
	Decision       toolapproval.Decision
}

// ApprovalHost is supplied by an interactive runtime. Tool governance stays
// independent of the concrete ask/session implementation.
type ApprovalHost interface {
	ApproveTool(context.Context, ApprovalRequest) (bool, error)
}

type approvalHostContextKey struct{}

func ContextWithApprovalHost(ctx context.Context, host ApprovalHost) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if host == nil {
		return ctx
	}
	return context.WithValue(ctx, approvalHostContextKey{}, host)
}

func approvalHostFromContext(ctx context.Context) (ApprovalHost, bool) {
	if ctx == nil {
		return nil, false
	}
	host, ok := ctx.Value(approvalHostContextKey{}).(ApprovalHost)
	return host, ok && host != nil
}
