package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	"fmt"
	"log/slog"
)

// writingRecoveryMutationBatch delays automation triggers until the same
// recovered cycle has an output-domain receipt. Tool verification happens
// before the harness publishes canonical output, so dispatching directly from
// OnMutationsVerified would expose mutations from a cycle that later fails its
// commit barrier.
type writingRecoveryMutationBatch struct {
	participant  agentrun.DomainCommitParticipant
	mutations    []agenttool.Mutation
	verification agenttool.Verification
	dispatch     func(context.Context, []agenttool.Mutation, agenttool.Verification)
}

func (a *App) writingMutationCallback(
	taskID string,
	conversation agentchat.Conversation,
) func(context.Context, []agenttool.Mutation, agenttool.Verification) {
	dispatch := a.automationMutationCallback("ide_agent_post_run")
	participant, supportsCommitReceipt := conversation.(agentrun.DomainCommitParticipant)
	if !supportsCommitReceipt || participant == nil {
		return dispatch
	}
	a.mu.RLock()
	run := a.activeWritingRun
	isRecovery := run != nil && run.task != nil && run.task.ID() == taskID && run.recovery != nil
	a.mu.RUnlock()
	if !isRecovery {
		return dispatch
	}
	return func(_ context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
		run.recoveryMutationMu.Lock()
		run.recoveryMutations = append(run.recoveryMutations, writingRecoveryMutationBatch{
			participant: participant, mutations: cloneRecoveryToolMutations(mutations),
			verification: cloneRecoveryPostRunVerification(verification), dispatch: dispatch,
		})
		run.recoveryMutationMu.Unlock()
	}
}

func (run *writingTaskRun) flushRecoveryMutations(ctx context.Context) {
	if run == nil {
		return
	}
	run.recoveryMutationMu.Lock()
	batches := append([]writingRecoveryMutationBatch(nil), run.recoveryMutations...)
	run.recoveryMutations = nil
	run.recoveryMutationMu.Unlock()
	for _, batch := range batches {
		if batch.participant == nil || batch.dispatch == nil {
			continue
		}
		if _, committed := batch.participant.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput); !committed {
			slog.WarnContext(ctx, fmt.Sprintf("[agent-recovery] skip writing mutation trigger without output receipt task_id=%s mutations=%d", run.task.ID(), len(batch.mutations)))
			continue
		}
		batch.dispatch(context.WithoutCancel(ctx), batch.mutations, batch.verification)
	}
}

func cloneRecoveryToolMutations(values []agenttool.Mutation) []agenttool.Mutation {
	cloned := append([]agenttool.Mutation(nil), values...)
	for index := range cloned {
		cloned[index].LoreItemIDs = append([]string(nil), cloned[index].LoreItemIDs...)
		cloned[index].DeletedLoreItemIDs = append([]string(nil), cloned[index].DeletedLoreItemIDs...)
	}
	return cloned
}

func cloneRecoveryPostRunVerification(value agenttool.Verification) agenttool.Verification {
	value.Checks = append([]agenttool.VerificationCheck(nil), value.Checks...)
	value.Warnings = append([]string(nil), value.Warnings...)
	return value
}
