package agent

// LoopPolicy declares the stable engineering constraints around a single Agent loop.
// It is intentionally separate from prompts so IDE, interactive, lore, and automation
// agents can share the same loop while evolving their own context and tool policies.
type LoopPolicy struct {
	ContextLedger ContextLedgerPolicy
	RunLedger     RunLedgerPolicy
}

// ContextLedgerPolicy controls how context sources are audited.
type ContextLedgerPolicy struct {
	Enabled      bool
	PreviewChars int
}

// RunLedgerPolicy controls per-run JSONL traces written under the workspace.
type RunLedgerPolicy struct {
	Enabled      bool
	Directory    string
	PreviewChars int
}

const (
	defaultContextLedgerPreviewChars = 100
	defaultRunLedgerPreviewChars     = 200
	defaultRunLedgerDirectory        = ".nova/runs"
)

// DefaultLoopPolicy returns Nova's default loop observability policy.
func DefaultLoopPolicy() LoopPolicy {
	return LoopPolicy{
		ContextLedger: ContextLedgerPolicy{
			Enabled:      true,
			PreviewChars: defaultContextLedgerPreviewChars,
		},
		RunLedger: RunLedgerPolicy{
			Enabled:      true,
			Directory:    defaultRunLedgerDirectory,
			PreviewChars: defaultRunLedgerPreviewChars,
		},
	}
}

func (p LoopPolicy) normalized() LoopPolicy {
	defaults := DefaultLoopPolicy()
	if p == (LoopPolicy{}) {
		return defaults
	}
	if p.ContextLedger.PreviewChars <= 0 {
		p.ContextLedger.PreviewChars = defaults.ContextLedger.PreviewChars
	}
	if p.RunLedger.Directory == "" {
		p.RunLedger.Directory = defaults.RunLedger.Directory
	}
	if p.RunLedger.PreviewChars <= 0 {
		p.RunLedger.PreviewChars = defaults.RunLedger.PreviewChars
	}
	return p
}
