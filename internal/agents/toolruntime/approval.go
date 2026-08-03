package toolruntime

import (
	"context"
	"fmt"

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

// ApprovalChoice is the complete host decision for one prompted tool call.
// AllowWorkspace means the policy-generated command rule was durably stored
// before the host released the blocked call.
type ApprovalChoice string

const (
	ApprovalDenied         ApprovalChoice = "deny"
	ApprovalAllowOnce      ApprovalChoice = "allow_once"
	ApprovalAllowWorkspace ApprovalChoice = "allow_workspace"
)

type ApprovalResult struct {
	Choice ApprovalChoice
}

func (result ApprovalResult) Validate() error {
	switch result.Choice {
	case ApprovalDenied, ApprovalAllowOnce, ApprovalAllowWorkspace:
		return nil
	default:
		return fmt.Errorf("unknown host approval choice %q", result.Choice)
	}
}

// ApprovalHost is supplied by an interactive runtime. Tool governance stays
// independent of the concrete ask/session implementation.
type ApprovalHost interface {
	ApproveTool(context.Context, ApprovalRequest) (ApprovalResult, error)
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
