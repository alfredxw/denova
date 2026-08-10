package execution

import (
	"context"
	"denova/internal/agents/run"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func coordinateDomainCommit(
	ctx context.Context,
	emit runstate.EngineEventSink,
	participant agentrun.DomainCommitParticipant,
	stage agentrun.DomainCommitStage,
	outcome agentrun.Outcome,
) error {
	if participant == nil {
		return nil
	}
	if stage == agentrun.DomainCommitOutput && !agentrun.OutcomeMayCommitDomain(outcome) {
		return commitCycleStage(ctx, participant, stage, outcome)
	}
	intent, pending, err := participant.PendingAgentCycleCommit(stage)
	if err != nil {
		return fmt.Errorf("prepare %s domain commit intent: %w", stage, err)
	}
	if !pending {
		return commitCycleStage(ctx, participant, stage, outcome)
	}
	if intent.Stage != stage || !agentrun.ValidCycleIdentity(intent.Identity) || strings.TrimSpace(intent.Hash) == "" {
		return fmt.Errorf("invalid %s domain commit intent", stage)
	}
	identity, err := runtimeDomainCommitIdentity(intent)
	if err != nil {
		return err
	}
	if emit == nil {
		return fmt.Errorf("authorize %s domain commit: engine sink is nil", stage)
	}
	if err := emit(runstate.EngineDomainCommitIntent{Identity: identity, Hash: intent.Hash}); err != nil {
		if stage == agentrun.DomainCommitOutput {
			_ = commitCycleStage(ctx, participant, stage, agentrun.Outcome{Status: agentrun.OutcomeAborted, Error: err, Reason: err.Error(), Content: outcome.Content, Thinking: outcome.Thinking})
		}
		return fmt.Errorf("authorize %s domain commit: %w", stage, err)
	}
	if err := commitCycleStage(ctx, participant, stage, outcome); err != nil {
		// Canonical stores dedupe by identity+hash. One exact retry resolves a
		// crash/error after the durable write without risking a second turn.
		if retryErr := commitCycleStage(ctx, participant, stage, outcome); retryErr != nil {
			return errors.Join(err, retryErr)
		}
	}
	receipt, ok := participant.LastAgentCycleCommitReceipt(stage)
	if !ok {
		return fmt.Errorf("%s domain commit completed without a canonical receipt", stage)
	}
	if receipt.Stage != stage || receipt.Identity != intent.Identity || receipt.Hash != intent.Hash || strings.TrimSpace(receipt.Revision) == "" {
		return fmt.Errorf("%s domain commit returned a mismatched canonical receipt", stage)
	}
	receiptEvent := runstate.EngineDomainCommitReceipt{
		Identity: identity, Hash: receipt.Hash, Revision: receipt.Revision,
	}
	if err := emit(receiptEvent); err != nil {
		// Actor journal appends can report an ambiguous post-write failure. The
		// receipt event is idempotent, so one exact retry reconciles that window.
		if retryErr := emit(receiptEvent); retryErr != nil {
			return fmt.Errorf("record %s domain commit receipt: %w", stage, errors.Join(err, retryErr))
		}
	}
	return nil
}

func runtimeDomainCommitIdentity(intent agentrun.DomainCommitIntent) (runstate.DomainCommitIdentity, error) {
	stage := runstate.DomainCommitStage(intent.Stage)
	if stage != runstate.DomainCommitInput && stage != runstate.DomainCommitOutput {
		return runstate.DomainCommitIdentity{}, fmt.Errorf("unsupported domain commit stage %q", intent.Stage)
	}
	return runstate.DomainCommitIdentity{
		CommandID: runstate.CommandID(intent.Identity.CommandID), OperationID: runstate.OperationID(intent.Identity.OperationID),
		Cycle: intent.Identity.Cycle, Stage: stage,
	}, nil
}

func commitCycleStage(ctx context.Context, participant agentrun.DomainCommitParticipant, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("agent execution %s domain commit panic: %v\n%s", stage, recovered, debug.Stack())
		}
	}()
	if err := participant.CommitAgentCycleStage(ctx, stage, outcome); err != nil {
		return fmt.Errorf("commit agent execution %s domain stage: %w", stage, err)
	}
	return nil
}

func reportOperationOutcome(ctx context.Context, spec cycleSpec, outcome agentrun.Outcome) {
	if spec.Outcome == nil {
		return
	}
	select {
	case spec.Outcome <- outcome:
	case <-ctx.Done():
	}
}

func commitCycle(ctx context.Context, commit func(context.Context, agentrun.Outcome) error, outcome agentrun.Outcome) (err error) {
	if commit == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("agent execution cycle commit panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	if err := commit(ctx, outcome); err != nil {
		return fmt.Errorf("commit agent execution cycle: %w", err)
	}
	return nil
}
