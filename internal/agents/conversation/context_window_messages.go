package conversation

import (
	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

func newContextRewindSummaryMessage(operation session.ContextOperation) *agent.Message {
	receipts := make([]agentcontext.RewindMutationReceipt, 0, len(operation.MutationReceipts))
	for _, receipt := range operation.MutationReceipts {
		receipts = append(receipts, agentcontext.RewindMutationReceipt{
			Tool: receipt.Tool, CallID: receipt.CallID, Scope: receipt.Scope, Summary: receipt.Summary,
		})
	}
	return agentcontext.NewRewindSummaryMessage(agentcontext.RewindSummaryInput{
		CheckpointID: operation.CheckpointID, Purpose: operation.Purpose,
		Report: operation.Report, MutationReceipts: receipts,
	})
}

func applyContextWindowProjection(history []*agent.Message, effectiveStart int, projection session.ContextWindowProjection) []*agent.Message {
	var prefix []*agent.Message
	if boundary := projection.Checkpoint.ResolvedBoundary; boundary != nil {
		prefix = boundary.CanonicalPrefix
	}
	return agentcontext.ApplyWindowProjection(
		history, effectiveStart, prefix, projection.RewindAfterIndex,
		newContextRewindSummaryMessage(projection.Rewind),
	)
}
