package chat

import (
	"context"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/toolresult"
)

// ContextMaintenanceResult distinguishes tool-result cleanup from checkpoint
// compaction while preserving the one-structural-change-per-run invariant.
type ContextMaintenanceResult struct {
	Attempted  bool
	Triggered  bool
	Action     agentcontext.ContextMaintenanceAction
	Cleanup    agentcontext.ContextPressureDecision
	Compaction agentcompaction.Result
}

// ContextPressureConversation is the storage-neutral maintenance boundary.
type ContextPressureConversation interface {
	ContextPressurePolicy(messages []*agent.Message) agentcontext.ContextPressurePolicy
	StageToolResultCleanup(context.Context, []*agent.Message, toolresult.CleanupPlan) error
}

type stagedToolResultCleanupDiscarder interface {
	DiscardStagedToolResultCleanup()
}
