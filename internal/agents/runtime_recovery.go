package agents

import (
	"context"
	"crypto/sha256"
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
	CommandID   CommandID
	OperationID OperationID
}

// RuntimeRecoveryDisplayMetadata is the small server-owned subset needed to
// reconnect game display state. The private durable restore descriptor never
// crosses the agent/application boundary.
type RuntimeRecoveryDisplayMetadata struct {
	Message              string
	RegenerateFromTurnID string
}

func RuntimeRecoveryActions(snapshot RuntimeStatus) []RuntimeRecoveryAction {
	actions := make([]RuntimeRecoveryAction, 0, 1+len(snapshot.Queue))
	appendAction := func(action RuntimeRecoveryAction) {
		if len(actions) >= maxRuntimeRecoveryActions || strings.TrimSpace(string(action.CommandID)) == "" || strings.TrimSpace(string(action.OperationID)) == "" {
			return
		}
		actions = append(actions, action)
	}

	if snapshot.RecoveryPaused {
		switch snapshot.Phase {
		case RunPhaseCompacting:
			if snapshot.ActiveStructural != nil {
				kind := RuntimeRecoveryCompactContext
				if snapshot.ActiveStructural.Kind == StructuralRemoveCompaction {
					kind = RuntimeRecoveryRemoveCompaction
				}
				appendAction(RuntimeRecoveryAction{
					Kind: kind, CommandID: snapshot.ActiveStructural.CommandID,
					OperationID: snapshot.ActiveStructural.OperationID,
				})
			}
		case RunPhaseRunning:
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
			case DeliverySteer:
				kind = RuntimeRecoverySteer
			case DeliveryFollowUp:
				kind = RuntimeRecoveryFollowUp
			case DeliveryNextTurn:
				kind = RuntimeRecoveryNextTurn
			}
			appendAction(RuntimeRecoveryAction{
				Kind: kind, CommandID: pending.CommandID, OperationID: pending.OperationID,
			})
			return actions
		}
		if snapshot.Phase == RunPhaseCompacting {
			return actions
		}
	}

	// Queued recovery identities are executable only at an explicit pause
	// boundary, except a durable NextTurn whose parent already reached Idle.
	// Advertising ordinary live queue records would mark healthy work
	// recoverable even though RecoverAcceptedInput must reject it.
	if !snapshot.RecoveryPaused {
		if snapshot.Phase == RunPhaseIdle {
			for _, item := range snapshot.Queue {
				if item.Delivery == DeliveryNextTurn {
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
		delivery DeliveryKind
		kind     RuntimeRecoveryActionKind
	}{
		{delivery: DeliverySteer, kind: RuntimeRecoverySteer},
		{delivery: DeliveryFollowUp, kind: RuntimeRecoveryFollowUp},
		{delivery: DeliveryNextTurn, kind: RuntimeRecoveryNextTurn},
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

func runtimeRecoveryAbortCommandID(snapshot RuntimeStatus) CommandID {
	binding, _ := json.Marshal(snapshot.Binding)
	material := string(binding) + "\x00" + string(snapshot.ActiveOperation)
	sum := sha256.Sum256([]byte(material))
	return CommandID("recovery-abort-" + hex.EncodeToString(sum[:16]))
}

// RuntimeRecoveryStatusProjection opens the exact binding to run canonical
// reconciliation and expose a recovery-paused status. Opening never reruns a
// model/tool effect; accepted work still requires the explicit recovery seam.
func (s *ChatService) RuntimeRecoveryStatusProjection(ctx context.Context, options RunOptions) (RuntimeStatus, error) {
	harness, _, err := s.openRecoveryHarness(ctx, options)
	if err != nil {
		return RuntimeStatus{}, err
	}
	status, err := harness.Status(ctx)
	if err != nil {
		return RuntimeStatus{}, err
	}
	return runtimeStatusFromSnapshot(status)
}
