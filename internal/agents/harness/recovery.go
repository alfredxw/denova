package harness

import (
	"context"
	"crypto/sha256"
	"denova/internal/agents/run"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const maxRuntimeRecoveryActions = 16

var (
	// ErrRecoveryRequired tells a new root Start caller that durable accepted
	// work must be rehydrated or explicitly controlled first.
	ErrRecoveryRequired = errors.New("agent runtime recovery is required")
	// ErrRecoveryActionChanged means the selected identity is no longer in the
	// current server-derived recovery plan.
	ErrRecoveryActionChanged = errors.New("agent runtime recovery action changed")
)

type RuntimeRecoveryActionKind string

const (
	RuntimeRecoveryAttach           RuntimeRecoveryActionKind = "start_turn"
	RuntimeRecoveryAbort            RuntimeRecoveryActionKind = "abort"
	RuntimeRecoverySteer            RuntimeRecoveryActionKind = "steer"
	RuntimeRecoveryFollowUp         RuntimeRecoveryActionKind = "follow_up"
	RuntimeRecoveryNextTurn         RuntimeRecoveryActionKind = "next_turn"
	RuntimeRecoveryCompactContext   RuntimeRecoveryActionKind = "compact_context"
	RuntimeRecoveryRemoveCompaction RuntimeRecoveryActionKind = "remove_compaction"
)

// RuntimeRecoveryAction is intentionally safe for public projection. It names
// accepted work without carrying its private durable descriptor or user input.
type RuntimeRecoveryAction struct {
	Kind        RuntimeRecoveryActionKind
	CommandID   agentrun.CommandID
	OperationID agentrun.OperationID
}

// RuntimeRecoveryDisplayMetadata is the small server-owned subset needed to
// reconnect game display state. The private durable restore descriptor never
// crosses the agent/application boundary.
type RuntimeRecoveryDisplayMetadata struct {
	Message              string
	RegenerateFromTurnID string
}

func RuntimeRecoveryActions(snapshot agentrun.RuntimeStatus) []RuntimeRecoveryAction {
	actions := make([]RuntimeRecoveryAction, 0, 1+len(snapshot.Queue))
	appendAction := func(action RuntimeRecoveryAction) {
		if len(actions) >= maxRuntimeRecoveryActions || strings.TrimSpace(string(action.CommandID)) == "" || strings.TrimSpace(string(action.OperationID)) == "" {
			return
		}
		actions = append(actions, action)
	}

	if snapshot.RecoveryPaused {
		switch snapshot.Phase {
		case agentrun.RunPhaseCompacting:
			if snapshot.ActiveStructural != nil {
				kind := RuntimeRecoveryCompactContext
				if snapshot.ActiveStructural.Kind == agentrun.StructuralRemoveCompaction {
					kind = RuntimeRecoveryRemoveCompaction
				}
				appendAction(RuntimeRecoveryAction{
					Kind: kind, CommandID: snapshot.ActiveStructural.CommandID,
					OperationID: snapshot.ActiveStructural.OperationID,
				})
			}
		case agentrun.RunPhaseRunning:
			appendAction(RuntimeRecoveryAction{
				Kind: RuntimeRecoveryAttach, CommandID: snapshot.ActiveCommandID,
				OperationID: snapshot.ActiveOperation,
			})
		}
		appendAction(RuntimeRecoveryAction{
			Kind: RuntimeRecoveryAbort, CommandID: runtimeRecoveryAbortCommandID(snapshot),
			OperationID: snapshot.ActiveOperation,
		})
		if pending := snapshot.InputRecovery; pending != nil {
			var kind RuntimeRecoveryActionKind
			switch pending.Delivery {
			case agentrun.DeliverySteer:
				kind = RuntimeRecoverySteer
			case agentrun.DeliveryFollowUp:
				kind = RuntimeRecoveryFollowUp
			case agentrun.DeliveryNextTurn:
				kind = RuntimeRecoveryNextTurn
			}
			appendAction(RuntimeRecoveryAction{
				Kind: kind, CommandID: pending.CommandID, OperationID: pending.OperationID,
			})
			return actions
		}
		if snapshot.Phase == agentrun.RunPhaseCompacting {
			return actions
		}
	}

	// Queued recovery identities are executable only at an explicit pause
	// boundary, except a durable NextTurn whose parent already reached Idle.
	// Advertising ordinary live queue records would mark healthy work
	// recoverable even though RecoverAcceptedInput must reject it.
	if !snapshot.RecoveryPaused {
		if snapshot.Phase == agentrun.RunPhaseIdle {
			for _, item := range snapshot.Queue {
				if item.Delivery == agentrun.DeliveryNextTurn {
					appendAction(RuntimeRecoveryAction{
						Kind: RuntimeRecoveryNextTurn, CommandID: item.CommandID, OperationID: item.OperationID,
					})
					break
				}
			}
		}
		return actions
	}
	for _, candidate := range []struct {
		delivery agentrun.DeliveryKind
		kind     RuntimeRecoveryActionKind
	}{
		{delivery: agentrun.DeliverySteer, kind: RuntimeRecoverySteer},
		{delivery: agentrun.DeliveryFollowUp, kind: RuntimeRecoveryFollowUp},
		{delivery: agentrun.DeliveryNextTurn, kind: RuntimeRecoveryNextTurn},
	} {
		for _, item := range snapshot.Queue {
			if item.Delivery != candidate.delivery {
				continue
			}
			appendAction(RuntimeRecoveryAction{Kind: candidate.kind, CommandID: item.CommandID, OperationID: item.OperationID})
			return actions
		}
	}
	return actions
}

func runtimeRecoveryAbortCommandID(snapshot agentrun.RuntimeStatus) agentrun.CommandID {
	binding, _ := json.Marshal(snapshot.Binding)
	material := string(binding) + "\x00" + string(snapshot.ActiveOperation)
	sum := sha256.Sum256([]byte(material))
	return agentrun.CommandID("recovery-abort-" + hex.EncodeToString(sum[:16]))
}

// RuntimeRecoveryStatusProjection opens the exact binding to run canonical
// reconciliation and expose a recovery-paused status. Opening never reruns a
// model/tool effect; accepted work still requires the explicit recovery seam.
func (s *Service) RuntimeRecoveryStatusProjection(ctx context.Context, options agentrun.Options) (agentrun.RuntimeStatus, error) {
	harness, _, err := s.openRecoveryHarness(ctx, options)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	status, err := harness.Status(ctx)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	return agentrun.RuntimeStatusFromSnapshot(status)
}
