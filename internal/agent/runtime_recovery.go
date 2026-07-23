package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"denova/internal/agentruntime"
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
	CommandID   agentruntime.CommandID
	OperationID agentruntime.OperationID
}

// RuntimeRecoveryDisplayMetadata is the small server-owned subset needed to
// reconnect game display state. The private durable restore descriptor never
// crosses the agent/application boundary.
type RuntimeRecoveryDisplayMetadata struct {
	Message              string
	RegenerateFromTurnID string
}

func RuntimeRecoveryActions(snapshot agentruntime.StatusSnapshot) []RuntimeRecoveryAction {
	actions := make([]RuntimeRecoveryAction, 0, 1+len(snapshot.Queue))
	appendAction := func(action RuntimeRecoveryAction) {
		if len(actions) >= maxRuntimeRecoveryActions || strings.TrimSpace(string(action.CommandID)) == "" || strings.TrimSpace(string(action.OperationID)) == "" {
			return
		}
		actions = append(actions, action)
	}

	if snapshot.RecoveryPaused {
		switch snapshot.Phase {
		case agentruntime.PhaseCompacting:
			if snapshot.ActiveStructural != nil {
				kind := RuntimeRecoveryCompactContext
				if snapshot.ActiveStructural.Kind == agentruntime.StructuralRemoveCompaction {
					kind = RuntimeRecoveryRemoveCompaction
				}
				appendAction(RuntimeRecoveryAction{
					Kind: kind, CommandID: snapshot.ActiveStructural.CommandID,
					OperationID: snapshot.ActiveStructural.OperationID,
				})
			}
		case agentruntime.PhaseRunning:
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
			case agentruntime.DeliverySteer:
				kind = RuntimeRecoverySteer
			case agentruntime.DeliveryFollowUp:
				kind = RuntimeRecoveryFollowUp
			case agentruntime.DeliveryNextTurn:
				kind = RuntimeRecoveryNextTurn
			}
			appendAction(RuntimeRecoveryAction{
				Kind: kind, CommandID: pending.CommandID, OperationID: pending.OperationID,
			})
			return actions
		}
		if snapshot.Phase == agentruntime.PhaseCompacting {
			return actions
		}
	}

	// Queued recovery identities are executable only at an explicit pause
	// boundary, except a durable NextTurn whose parent already reached Idle.
	// Advertising ordinary live queue records would mark healthy work
	// recoverable even though RecoverAcceptedInput must reject it.
	if !snapshot.RecoveryPaused {
		if snapshot.Phase == agentruntime.PhaseIdle {
			for _, item := range snapshot.Queue {
				if item.Delivery == agentruntime.DeliveryNextTurn {
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
		delivery agentruntime.DeliveryKind
		kind     RuntimeRecoveryActionKind
	}{
		{delivery: agentruntime.DeliverySteer, kind: RuntimeRecoverySteer},
		{delivery: agentruntime.DeliveryFollowUp, kind: RuntimeRecoveryFollowUp},
		{delivery: agentruntime.DeliveryNextTurn, kind: RuntimeRecoveryNextTurn},
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

func runtimeRecoveryAbortCommandID(snapshot agentruntime.StatusSnapshot) agentruntime.CommandID {
	material := strings.Join([]string{
		string(snapshot.Binding.Kind), string(snapshot.Binding.Profile), snapshot.Binding.Workspace,
		snapshot.Binding.SessionID, snapshot.Binding.StoryID, snapshot.Binding.BranchID,
		snapshot.Binding.TaskID, string(snapshot.ActiveOperation),
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return agentruntime.CommandID("recovery-abort-" + hex.EncodeToString(sum[:16]))
}

// RuntimeRecoveryStatusProjection opens the exact binding to run canonical
// reconciliation and expose a recovery-paused status. Opening never reruns a
// model/tool effect; accepted work still requires the explicit recovery seam.
func (s *ChatService) RuntimeRecoveryStatusProjection(ctx context.Context, options RunOptions) (agentruntime.StatusSnapshot, error) {
	harness, _, err := s.openRecoveryHarness(ctx, options)
	if err != nil {
		return agentruntime.StatusSnapshot{}, err
	}
	return harness.Status(ctx)
}
