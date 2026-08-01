package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type nextTurnPlan struct {
	item       QueuedInput
	snapshotID SnapshotID
	has        bool
	start      bool
}

func (h *Harness) planNextTurn(ctx context.Context, state *harnessState, allowStart bool) nextTurnPlan {
	item, ok := state.firstQueued(DeliveryNextTurn)
	if !ok {
		return nextTurnPlan{}
	}
	plan := nextTurnPlan{item: item, has: true}
	if !allowStart {
		return plan
	}
	plan.start = true
	plan.snapshotID = SnapshotID(newID("snapshot"))
	return plan
}

func (h *Harness) restorePendingInput(ctx context.Context, item QueuedInput) (resultErr error) {
	restorer, ok := h.engine.(EnginePendingInputRestorer)
	if !ok {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("pending input restorer panic: %v", recovered)
		}
	}()
	return restorer.RestorePendingInput(ctx, cloneQueuedInput(item))
}

func (plan nextTurnPlan) appendStart(payloads []EventPayload) []EventPayload {
	if !plan.start {
		return payloads
	}
	return append(payloads,
		QueueConsumedEvent{CommandID: plan.item.CommandID, Delivery: DeliveryNextTurn},
		OperationStartedEvent{OperationID: plan.item.OperationID},
		UserMessageCommittedEvent{Message: newUserMessage(plan.item.OperationID, plan.item.Input)},
		CycleStartedEvent{OperationID: plan.item.OperationID, Cycle: 1, SnapshotID: plan.snapshotID},
		InputMaterializationRecoveryPendingEvent{
			OperationID: plan.item.OperationID, Cycle: 1,
			CommandID: plan.item.CommandID, Delivery: DeliveryNextTurn,
		},
	)
}

func (h *Harness) startPlannedNextTurn(state *harnessState, plan nextTurnPlan) error {
	if !plan.start {
		return nil
	}
	if err := h.resumePendingInputMaterialization(h.lifecycle, state); err != nil {
		if _, fatal := terminalJournalAppendError(err); fatal {
			panic(fmt.Sprintf("operation %s failed to persist NextTurn input materialization: %v", state.activeOperation, err))
		}
		slog.InfoContext(context.Background(), fmt.Sprintf(
			"runtime: binding=%+v command=%s operation=%s accepted NextTurn input remains pending: %v",
			h.binding,
			plan.item.CommandID,
			plan.item.OperationID,
			err,
		))
		return err
	}
	return nil
}

func (plan nextTurnPlan) pendingReason() string {
	if !plan.has || plan.start {
		return ""
	}
	return "accepted NextTurn remains pending for the next binding open"
}

func appendOperationReason(reason, warning string) string {
	reason = strings.TrimSpace(reason)
	warning = strings.TrimSpace(warning)
	if reason == "" {
		return warning
	}
	if warning == "" {
		return reason
	}
	return reason + "; " + warning
}

func transientQueueCancellations(state *harnessState, reason string) []EventPayload {
	payloads := make([]EventPayload, 0, len(state.queue))
	for _, item := range state.queue {
		if item.Delivery == DeliveryNextTurn {
			continue
		}
		payloads = append(payloads, QueueCancelledEvent{CommandID: item.CommandID, Reason: reason})
	}
	return payloads
}

func (h *Harness) resumePendingNextTurn(ctx context.Context, state *harnessState) error {
	if state.phase != PhaseIdle {
		return nil
	}
	plan := h.planNextTurn(ctx, state, !state.closing)
	payloads := transientQueueCancellations(state, "runtime_recovered")
	payloads = plan.appendStart(payloads)
	if len(payloads) == 0 {
		return nil
	}
	if _, err := h.commit(ctx, state, payloads); err != nil {
		return err
	}
	return h.startPlannedNextTurn(state, plan)
}

func (h *Harness) resumeReplayedCommand(state *harnessState, command Command) error {
	if state.recoveryPaused {
		return h.resumeRecoveryPausedCommand(state, command)
	}
	next, ok := command.(NextTurn)
	if ok && state.phase == PhaseIdle {
		for _, item := range state.queue {
			if item.Delivery == DeliveryNextTurn && item.CommandID == next.ID {
				return h.resumePendingNextTurn(h.lifecycle, state)
			}
		}
	}
	if state.phase != PhaseRunning || state.engineControls != nil {
		return nil
	}
	if state.activeCycleCommandID != command.commandID() && !state.abortRequested {
		return nil
	}
	if err := h.ensureInputMaterialized(h.lifecycle, state); err != nil {
		return err
	}
	if state.abortRequested || state.closing {
		h.failActiveOperation(state, engineDoneRequest{
			operation: state.activeOperation,
			cycle:     state.activeCycle,
			result:    EngineResult{Status: EngineAborted},
		})
		return nil
	}
	h.startEngine(state, state.turnSnapshot(state.activeSnapshotID))
	return nil
}
