package agentruntime

import (
	agentcompaction "denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

// ProjectSessionCompaction converts the Agent-owned checkpoint to the existing
// context-analysis shape without writing a competing Session record.
func ProjectSessionCompaction(
	state *agentrun.AgentCompactionState,
	agentKind string,
) *session.ContextCompaction {
	if state == nil {
		return nil
	}
	result := agentcompaction.Result{
		Triggered: true, Phase: "agent", Summary: state.Summary,
		Epoch: int(state.Revision), TokensAfter: state.TokenEstimate,
		SourceMessageCount: max(0, state.ReplacementTo-state.ReplacementFrom),
	}
	return &session.ContextCompaction{
		ID: state.ID, CompactionCheckpoint: agentcompaction.NewCheckpoint(agentKind, result),
		SourceStartIndex: state.ReplacementFrom, SourceEndIndex: state.ReplacementTo,
		SourceMessageCount: result.SourceMessageCount,
	}
}
