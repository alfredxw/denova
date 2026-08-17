package agentrun

import agentcontext "denova/internal/agents/context"

const (
	defaultRunLedgerPreviewChars = 200
	defaultRunLedgerDirectory    = ".denova/runs"
)

// LoopPolicy declares the stable observability constraints around one Agent
// loop. Prompt composition and product-specific context remain outside it.
type LoopPolicy struct {
	ContextLedger agentcontext.AuditPolicy
	RunLedger     LedgerPolicy
}

// LedgerPolicy controls per-run JSONL traces written under the workspace.
type LedgerPolicy struct {
	Enabled      bool
	Directory    string
	PreviewChars int
}

// DefaultLoopPolicy returns Denova's default loop observability policy.
func DefaultLoopPolicy() LoopPolicy {
	return LoopPolicy{
		ContextLedger: agentcontext.AuditPolicy{Enabled: true, PreviewChars: agentcontext.DefaultAuditPreviewChars},
		RunLedger: LedgerPolicy{
			Enabled: true, Directory: defaultRunLedgerDirectory, PreviewChars: defaultRunLedgerPreviewChars,
		},
	}
}

// Normalize fills omitted policy values without overriding explicit feature
// switches.
func (policy LoopPolicy) Normalize() LoopPolicy {
	defaults := DefaultLoopPolicy()
	if policy == (LoopPolicy{}) {
		return defaults
	}
	if policy.ContextLedger.PreviewChars <= 0 {
		policy.ContextLedger.PreviewChars = defaults.ContextLedger.PreviewChars
	}
	if policy.RunLedger.Directory == "" {
		policy.RunLedger.Directory = defaults.RunLedger.Directory
	}
	if policy.RunLedger.PreviewChars <= 0 {
		policy.RunLedger.PreviewChars = defaults.RunLedger.PreviewChars
	}
	return policy
}
