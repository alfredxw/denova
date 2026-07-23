package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"denova/internal/agentruntime"
)

type HarnessDomainCommitStage string

const (
	HarnessDomainCommitInput  HarnessDomainCommitStage = "input"
	HarnessDomainCommitOutput HarnessDomainCommitStage = "output"
)

// HarnessDomainCommitIntent is the bounded declaration sent to the durable
// actor before a conversation may publish its staged canonical output.
type HarnessDomainCommitIntent struct {
	Identity HarnessCycleIdentity
	Stage    HarnessDomainCommitStage
	Hash     string
}

// HarnessDomainCommitReceipt is the canonical proof returned after publish.
// Revision is opaque because writing sessions and game stories use different
// native revision schemes.
type HarnessDomainCommitReceipt struct {
	Identity HarnessCycleIdentity
	Stage    HarnessDomainCommitStage
	Hash     string
	Revision string
}

// HarnessDomainCommitParticipant exposes staged output without letting the
// generic runtime depend on writing or game storage packages.
type HarnessDomainCommitParticipant interface {
	PendingAgentCycleCommit(HarnessDomainCommitStage) (HarnessDomainCommitIntent, bool, error)
	CommitAgentCycleStage(context.Context, HarnessDomainCommitStage, RunOutcome) error
	LastAgentCycleCommitReceipt(HarnessDomainCommitStage) (HarnessDomainCommitReceipt, bool)
}

// HarnessInputDomainCommitBinder lets CommitModelInput publish the accepted
// input only after the durable actor authorizes the input stage. The callback
// is installed by harnessEngine and runs synchronously before model execution.
type HarnessInputDomainCommitBinder interface {
	BindAgentCycleInputCommit(func() error)
}

func validHarnessCycleIdentity(identity HarnessCycleIdentity) bool {
	return strings.TrimSpace(string(identity.CommandID)) != "" && strings.TrimSpace(string(identity.OperationID)) != "" && identity.Cycle > 0
}

func runOutcomeMayCommitDomain(outcome RunOutcome) bool {
	return outcome.Status == RunOutcomeCompleted || outcome.Status == RunOutcomePreempted
}

func coordinateHarnessDomainCommit(
	ctx context.Context,
	emit agentruntime.EngineEventSink,
	participant HarnessDomainCommitParticipant,
	stage HarnessDomainCommitStage,
	outcome RunOutcome,
) error {
	if participant == nil {
		return nil
	}
	if stage == HarnessDomainCommitOutput && !runOutcomeMayCommitDomain(outcome) {
		return commitHarnessCycleStage(ctx, participant, stage, outcome)
	}
	intent, pending, err := participant.PendingAgentCycleCommit(stage)
	if err != nil {
		return fmt.Errorf("prepare %s domain commit intent: %w", stage, err)
	}
	if !pending {
		return commitHarnessCycleStage(ctx, participant, stage, outcome)
	}
	if intent.Stage != stage || !validHarnessCycleIdentity(intent.Identity) || strings.TrimSpace(intent.Hash) == "" {
		return fmt.Errorf("invalid %s domain commit intent", stage)
	}
	identity, err := agentruntimeDomainCommitIdentity(intent)
	if err != nil {
		return err
	}
	if emit == nil {
		return fmt.Errorf("authorize %s domain commit: engine sink is nil", stage)
	}
	if err := emit(agentruntime.EngineDomainCommitIntent{Identity: identity, Hash: intent.Hash}); err != nil {
		if stage == HarnessDomainCommitOutput {
			_ = commitHarnessCycleStage(ctx, participant, stage, RunOutcome{Status: RunOutcomeAborted, Error: err, Reason: err.Error(), Content: outcome.Content, Thinking: outcome.Thinking})
		}
		return fmt.Errorf("authorize %s domain commit: %w", stage, err)
	}
	if err := commitHarnessCycleStage(ctx, participant, stage, outcome); err != nil {
		// Canonical stores dedupe by identity+hash. One exact retry resolves a
		// crash/error after the durable write without risking a second turn.
		if retryErr := commitHarnessCycleStage(ctx, participant, stage, outcome); retryErr != nil {
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
	receiptEvent := agentruntime.EngineDomainCommitReceipt{
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

func agentruntimeDomainCommitIdentity(intent HarnessDomainCommitIntent) (agentruntime.DomainCommitIdentity, error) {
	stage := agentruntime.DomainCommitStage(intent.Stage)
	if stage != agentruntime.DomainCommitInput && stage != agentruntime.DomainCommitOutput {
		return agentruntime.DomainCommitIdentity{}, fmt.Errorf("unsupported domain commit stage %q", intent.Stage)
	}
	return agentruntime.DomainCommitIdentity{
		CommandID: intent.Identity.CommandID, OperationID: intent.Identity.OperationID,
		Cycle: intent.Identity.Cycle, Stage: stage,
	}, nil
}

func commitHarnessCycleStage(ctx context.Context, participant HarnessDomainCommitParticipant, stage HarnessDomainCommitStage, outcome RunOutcome) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("agent harness %s domain commit panic: %v\n%s", stage, recovered, debug.Stack())
		}
	}()
	if err := participant.CommitAgentCycleStage(ctx, stage, outcome); err != nil {
		return fmt.Errorf("commit agent harness %s domain stage: %w", stage, err)
	}
	return nil
}

func reportHarnessOutcome(ctx context.Context, spec HarnessTurnSpec, outcome RunOutcome) {
	if spec.Outcome == nil {
		return
	}
	select {
	case spec.Outcome <- outcome:
	case <-ctx.Done():
	}
}

func commitHarnessCycle(ctx context.Context, commit func(context.Context, RunOutcome) error, outcome RunOutcome) (err error) {
	if commit == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("agent harness cycle commit panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	if err := commit(ctx, outcome); err != nil {
		return fmt.Errorf("commit agent harness cycle: %w", err)
	}
	return nil
}
