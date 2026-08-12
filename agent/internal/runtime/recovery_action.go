package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrRecoveryActionChanged means a caller selected safe recovery identity that
// no longer names a pending durable input on this binding.
var ErrRecoveryActionChanged = errors.New("agent runtime recovery action changed")

// RecoveryAction is the complete public authority needed to resume an already
// accepted queued input. It deliberately carries no UserInput, descriptor,
// context reference, or tool payload.
type RecoveryAction struct {
	Kind        DeliveryKind
	CommandID   CommandID
	OperationID OperationID
}

func (h *Harness) handleRecoverAcceptedInput(
	state *harnessState,
	ctx context.Context,
	action RecoveryAction,
) (Receipt, error) {
	if binder, ok := h.engine.(EngineRecoveryContextBinder); ok {
		binder.BindRecoveryContext(ctx)
	}
	if err := validateRecoveryAction(action, h.inputLimits); err != nil {
		return Receipt{}, err
	}
	if len(state.pendingHostEffects) > 0 {
		return Receipt{}, fmt.Errorf("%w: reconcile %d pending effect(s) before resuming accepted input", ErrHostEffectRequired, len(state.pendingHostEffects))
	}
	item, ok := pendingRecoveryInput(state, action)
	if !ok {
		return Receipt{}, fmt.Errorf("%w: kind=%q command_id=%q operation_id=%q", ErrRecoveryActionChanged, action.Kind, action.CommandID, action.OperationID)
	}
	receipt, ok := state.receipts[item.CommandID]
	if !ok {
		if lookup, supported := h.journal.(CommandJournalLookup); supported {
			record, found, err := lookup.LookupCommand(ctx, item.CommandID)
			if err != nil {
				return Receipt{}, fmt.Errorf("lookup durable recovery command %q: %w", item.CommandID, err)
			}
			if found {
				receipt, ok = record.Receipt, true
			}
		}
	}
	if !ok || receipt.CommandID != item.CommandID || receipt.OperationID != item.OperationID {
		return Receipt{}, fmt.Errorf("%w: durable receipt for command %q is unavailable", ErrRecoveryActionChanged, item.CommandID)
	}
	if state.inputRecovery != nil &&
		state.inputRecovery.CommandID == item.CommandID &&
		state.inputRecovery.OperationID == item.OperationID &&
		state.inputRecovery.Delivery == item.Delivery {
		if err := h.resumePendingInputMaterialization(ctx, state, true); err != nil {
			return receipt, err
		}
		receipt.Replayed = true
		return receipt, nil
	}
	if state.inputRecovery != nil {
		return receipt, fmt.Errorf("%w: an earlier accepted input is still awaiting materialization", ErrRecoveryActionChanged)
	}

	var err error
	switch item.Delivery {
	case DeliverySteer, DeliveryFollowUp:
		if !state.recoveryPaused || state.phase != PhaseRunning {
			return Receipt{}, fmt.Errorf("%w: transient input is not recovery-paused", ErrRecoveryActionChanged)
		}
		err = h.resumeRecoveryPausedInput(ctx, state, item.CommandID, item.Delivery)
	case DeliveryNextTurn:
		if state.recoveryPaused && state.phase == PhaseRunning {
			err = h.abandonRecoveredParentForNextTurn(ctx, state, item)
		} else if state.phase == PhaseIdle {
			err = h.resumePendingNextTurn(ctx, state)
		} else {
			return Receipt{}, fmt.Errorf("%w: NextTurn is not at a recoverable boundary", ErrRecoveryActionChanged)
		}
	default:
		return Receipt{}, fmt.Errorf("%w: unsupported recovery delivery %q", ErrInvalidCommand, item.Delivery)
	}
	if err != nil {
		return receipt, err
	}
	receipt.Replayed = true
	return receipt, nil
}

// BindRecoveryContext attaches bounded process-local display routing to this
// binding without mutating durable state or starting work. It is used by the
// attach-only Start recovery action before a later fresh command decides how
// the paused operation proceeds.
func (h *Harness) BindRecoveryContext(ctx context.Context) {
	if h == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if binder, ok := h.engine.(EngineRecoveryContextBinder); ok {
		binder.BindRecoveryContext(ctx)
	}
}

// UnbindRecoveryContext releases the process-local display route owned by ctx.
// It never changes durable state or controls an active operation.
func (h *Harness) UnbindRecoveryContext(ctx context.Context) {
	if h == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if unbinder, ok := h.engine.(EngineRecoveryContextUnbinder); ok {
		unbinder.UnbindRecoveryContext(ctx)
	}
}

func validateRecoveryAction(action RecoveryAction, limits InputLimits) error {
	if action.Kind != DeliverySteer && action.Kind != DeliveryFollowUp && action.Kind != DeliveryNextTurn {
		return fmt.Errorf("%w: unsupported recovery kind %q", ErrInvalidCommand, action.Kind)
	}
	if err := ValidateCommandID(string(action.CommandID), limits); err != nil {
		return err
	}
	if strings.TrimSpace(string(action.OperationID)) == "" || len(action.OperationID) > limits.normalized().MaxOperationIDBytes {
		return fmt.Errorf("%w: recovery operation id is invalid", ErrInvalidCommand)
	}
	return nil
}

func pendingRecoveryInput(state *harnessState, action RecoveryAction) (QueuedInput, bool) {
	if state == nil {
		return QueuedInput{}, false
	}
	if recovery := state.inputRecovery; recovery != nil &&
		recovery.CommandID == action.CommandID && recovery.OperationID == action.OperationID && recovery.Delivery == action.Kind {
		return QueuedInput{
			CommandID: recovery.CommandID, OperationID: recovery.OperationID,
			Delivery: recovery.Delivery, Input: cloneUserInput(state.activeInput),
		}, true
	}
	for _, item := range state.queue {
		if item.Delivery == action.Kind && item.CommandID == action.CommandID && item.OperationID == action.OperationID {
			return cloneQueuedInput(item), true
		}
	}
	return QueuedInput{}, false
}

func (h *Harness) handleRecoveryInput(state *harnessState, commandID CommandID, operationID OperationID) (UserInput, bool, error) {
	if err := ValidateCommandID(string(commandID), h.inputLimits); err != nil {
		return UserInput{}, false, err
	}
	if strings.TrimSpace(string(operationID)) == "" || len(operationID) > h.inputLimits.MaxOperationIDBytes {
		return UserInput{}, false, fmt.Errorf("%w: recovery operation id is invalid", ErrInvalidCommand)
	}
	for _, item := range state.queue {
		if item.CommandID == commandID && item.OperationID == operationID {
			return cloneUserInput(item.Input), true, nil
		}
	}
	if recovery := state.inputRecovery; recovery != nil && recovery.CommandID == commandID && recovery.OperationID == operationID {
		return cloneUserInput(state.activeInput), true, nil
	}
	owned := state.activeCommandID == commandID && state.activeOperation == operationID
	if !owned && state.lastOperation != nil {
		owned = state.lastOperation.CommandID == commandID && state.lastOperation.OperationID == operationID
	}
	if !owned {
		for index := len(state.recentOperations) - 1; index >= 0; index-- {
			summary := state.recentOperations[index]
			if summary.CommandID == commandID && summary.OperationID == operationID {
				owned = true
				break
			}
		}
	}
	if !owned {
		return UserInput{}, false, nil
	}
	for index := len(state.messages) - 1; index >= 0; index-- {
		message := state.messages[index]
		if message.Role == RoleUser && message.Operation == operationID {
			return cloneUserInput(message.Input), true, nil
		}
	}
	return UserInput{}, false, nil
}
