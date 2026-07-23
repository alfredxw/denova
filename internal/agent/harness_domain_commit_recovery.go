package agent

import (
	"context"

	"denova/internal/agentruntime"
)

// HarnessDomainCommitReconciler resolves an exact identity+hash against the
// host's canonical Session or Story store. It is a query-only recovery seam;
// Found must never be inferred from command or operation identity alone.
type HarnessDomainCommitReconciler func(
	context.Context,
	agentruntime.DomainCommitReconcileRequest,
) (agentruntime.DomainCommitReconcileResult, error)
