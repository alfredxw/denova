package toolruntime

import (
	"context"
	"errors"
	"strings"

	producttools "denova/internal/agents/tools"
)

type hostToolIdentityKey struct{}

// HostToolIdentity binds one non-Native tool execution to the same product
// mutation services. IDs are allocated by the host after durable acceptance;
// none is taken from tool arguments or used as a filesystem path.
type HostToolIdentity struct {
	OperationID    string
	ExecutionID    string
	ProviderCallID string
	SessionID      string
	ReviewThreadID string
}

func ContextWithHostToolIdentity(ctx context.Context, identity HostToolIdentity) (context.Context, error) {
	if ctx == nil || strings.TrimSpace(identity.OperationID) == "" || strings.TrimSpace(identity.ExecutionID) == "" || strings.TrimSpace(identity.SessionID) == "" {
		return nil, errors.New("host tool requires an accepted operation, execution and session")
	}
	ctx = producttools.ContextWithWorkspaceChangeScope(ctx, producttools.WorkspaceChangeScope{
		RunID: identity.OperationID, SessionID: identity.SessionID, ReviewThreadID: identity.ReviewThreadID,
	})
	return context.WithValue(ctx, hostToolIdentityKey{}, identity), nil
}
