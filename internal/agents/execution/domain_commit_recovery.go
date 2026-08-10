package execution

import (
	"context"
	"denova/internal/agents/run"
)

// DomainCommitReconciler resolves an exact identity+hash against the
// host's canonical Session or Story store. It is a query-only recovery seam;
// Found must never be inferred from command or operation identity alone.
type DomainCommitReconciler func(
	context.Context,
	agentrun.DomainCommitReconcileRequest,
) (agentrun.DomainCommitReconcileResult, error)
