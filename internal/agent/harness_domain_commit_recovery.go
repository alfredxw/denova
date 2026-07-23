package agent

import (
	"context"

	runstate "denova/internal/agent/runtime"
)

// HarnessDomainCommitReconciler resolves an exact identity+hash against the
// host's canonical Session or Story store. It is a query-only recovery seam;
// Found must never be inferred from command or operation identity alone.
type HarnessDomainCommitReconciler func(
	context.Context,
	runstate.DomainCommitReconcileRequest,
) (runstate.DomainCommitReconcileResult, error)
