package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

func (run *Run) updateEngineTranscript(state json.RawMessage, persist bool) error {
	if state == nil {
		return nil
	}
	run.session.mu.Lock()
	run.session.engineState = append(json.RawMessage(nil), state...)
	var err error
	if persist {
		err = run.session.persistTranscriptLocked(context.Background())
	}
	run.session.mu.Unlock()
	run.mu.Lock()
	run.snapshot.State = append(json.RawMessage(nil), state...)
	run.mu.Unlock()
	return err
}

func (run *Run) persistEngineTranscript() error {
	run.session.mu.Lock()
	err := run.session.persistTranscriptLocked(context.Background())
	run.session.mu.Unlock()
	return err
}

// persistTaskCompletionCheckpoint atomically commits the model-visible
// messages and their delivery receipts. A completion remains pending when the
// Session log cannot commit the whole transaction.
func (run *Run) persistTaskCompletionCheckpoint(state json.RawMessage, ids []string) error {
	if err := run.session.commitTaskCompletionCheckpoint(context.Background(), state, ids); err != nil {
		return err
	}
	run.mu.Lock()
	run.snapshot.State = append(json.RawMessage(nil), state...)
	run.mu.Unlock()
	return nil
}

func (run *Run) snapshotForCurrentCycle() (runstate.TurnSnapshot, error) {
	run.mu.RLock()
	snapshot := run.snapshot
	snapshot.State = append(json.RawMessage(nil), snapshot.State...)
	snapshot.Capabilities = cloneRawStateMap(snapshot.Capabilities)
	run.mu.RUnlock()
	if snapshot.OperationID == "" {
		return runstate.TurnSnapshot{}, ErrInteractionStale
	}
	return snapshot, nil
}

func (run *Run) beginModelResponseLocked(ordinal int) {
	if ordinal == run.modelResponseOrdinal {
		return
	}
	run.modelResponseOrdinal = ordinal
	run.modelContentStart = run.content.Len()
	run.modelThinkingStart = run.thinking.Len()
}

func (run *Run) nextQueuedInput() (Input, runstate.DeliveryKind, bool) {
	run.session.mu.RLock()
	defer run.session.mu.RUnlock()
	for _, item := range run.session.inputOrder {
		if item.targetRunID == run.id && item.status == inputPending && (item.Kind == inputQueue || item.Kind == inputSteer) {
			return cloneInput(item.input), item.delivery, true
		}
	}
	return Input{}, "", false
}

func (run *Run) publish(payload EventPayload) {
	if run == nil || payload == nil {
		return
	}
	run.eventMu.Lock()
	defer run.eventMu.Unlock()
	event := Event{RunID: run.id, Payload: payload}
	run.session.mu.Lock()
	run.session.publishLocked(event)
	event.Cursor = run.session.cursor
	run.session.mu.Unlock()
	if !run.eventsEnd {
		publishLatestEvent(run.events, event, &run.eventDrops)
	}
}

func (run *Run) finish(result Result, err error) {
	if run == nil {
		return
	}
	run.finishMu.Lock()
	defer run.finishMu.Unlock()
	run.mu.RLock()
	if run.settled {
		run.mu.RUnlock()
		return
	}
	if err == nil && (result.Status == ResultFailed || result.Status == ResultIncomplete || result.Status == ResultBlocked) {
		err = &RunError{Result: result}
	}
	output := run.content.String()
	receiptCursor := run.receipt
	run.mu.RUnlock()
	run.cancel()

	var next *Run
	run.session.mu.Lock()
	finishedActive := run.session.active == run
	kind := turnFinishedRecord
	if result.Status != ResultCompleted && result.Status != ResultAborted {
		kind = turnInterruptedRecord
	}
	if appendErr := run.session.appendRecordLocked(context.Background(), kind, persistedTurn{
		RunID: run.id, CommandID: run.commandID, Status: result.Status, Reason: result.Reason, Output: output, At: time.Now().UTC(),
	}); appendErr != nil {
		err = errors.Join(err, appendErr)
		run.session.mu.Unlock()
		// A missing or uncertain settlement is still an unfinished journal.
		// Release this failed writer only after executeCycle stopped producers;
		// reopening must replay the facts before any further action.
		err = errors.Join(err, run.session.closeWriter())
		run.mu.Lock()
		run.result, run.err = Result{Status: ResultSuspended, Reason: "Agent journal requires reopening after a write failure"}, err
		run.mu.Unlock()
		run.endHandle()
		return
	}
	if finishedActive {
		run.session.active = nil
	}
	run.session.removePendingLocked(run)
	run.mu.Lock()
	run.settled, run.result, run.err = true, result, err
	run.mu.Unlock()
	run.session.addRecentLocked(RunSummary{
		ID: run.id, CommandID: run.commandID, ReceiptCursor: receiptCursor,
		Status: result.Status, Reason: result.Reason, Output: output,
	})
	if finishedActive && len(run.session.pending) > 0 && !run.session.closed && !run.session.closing && !run.session.treeControl.sealed() && err == nil {
		candidate := run.session.pending[0]
		startedAt := time.Now().UTC()
		if startErr := run.session.appendRecordLocked(context.Background(), turnStartedRecord, persistedTurn{
			RunID: candidate.id, CommandID: candidate.commandID, At: startedAt,
		}); startErr != nil {
			err = errors.Join(err, startErr)
		} else {
			next = candidate
			run.session.pending = run.session.pending[1:]
			next.markStarted(startedAt)
			run.session.active = next
		}
	}
	storageErr := run.session.storageErr
	run.session.mu.Unlock()
	if storageErr != nil {
		err = errors.Join(err, run.session.closeWriter())
	}

	run.mu.Lock()
	run.err = err
	run.mu.Unlock()
	run.publish(RunSettled{Status: result.Status, Reason: result.Reason})
	emitTrace(context.Background(), run.session.agent.trace, TraceEvent{
		Kind: TraceRunSettled, Session: run.session.key, RunID: run.id, Err: err,
	})
	run.endHandle()
	if next != nil {
		safeGo(next.execute, func(nextErr error) {
			next.finish(Result{Status: ResultFailed, Reason: nextErr.Error()}, nextErr)
		})
	}
	if run.ownership == runOwnsTemporarySession {
		_ = run.session.Delete(context.Background())
	}
}

func (run *Run) abort(reason string) {
	if run == nil || run.isSettled() {
		return
	}
	run.setAbortReason(reason)
	select {
	case run.controls <- runstate.EngineControl{Kind: runstate.EngineControlAbort}:
	default:
		run.cancel()
	}
}

func (run *Run) setAbortReason(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Agent Run aborted"
	}
	run.mu.Lock()
	run.abortReason = reason
	run.mu.Unlock()
}

func (run *Run) abortRequested() bool {
	if run == nil {
		return false
	}
	run.mu.RLock()
	requested := run.abortReason != ""
	run.mu.RUnlock()
	return requested
}

func (run *Run) currentAbortReason() string {
	run.mu.RLock()
	reason := run.abortReason
	run.mu.RUnlock()
	if reason == "" {
		return "Agent Run aborted"
	}
	return reason
}

func (run *Run) abortPending(reason string) bool {
	run.session.mu.Lock()
	for index, candidate := range run.session.pending {
		if candidate != run {
			continue
		}
		run.session.pending = append(run.session.pending[:index], run.session.pending[index+1:]...)
		run.session.mu.Unlock()
		run.finish(Result{Status: ResultAborted, Reason: reason}, nil)
		return true
	}
	run.session.mu.Unlock()
	return false
}

func (run *Run) isSettled() bool {
	if run == nil {
		return true
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	return run.settled
}

func (run *Run) cycleValue() int {
	run.mu.RLock()
	defer run.mu.RUnlock()
	return run.cycle
}

func (run *Run) markStarted(startedAt time.Time) {
	if run == nil || startedAt.IsZero() {
		return
	}
	run.mu.Lock()
	if run.startedAt.IsZero() {
		run.startedAt = startedAt.UTC()
	}
	run.mu.Unlock()
}

func (run *Run) startedAtValue() time.Time {
	if run == nil {
		return time.Time{}
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	return run.startedAt
}

func (run *Run) outputSnapshot() ActiveOutputSnapshot {
	run.mu.RLock()
	defer run.mu.RUnlock()
	return ActiveOutputSnapshot{Content: run.content.String(), Thinking: run.thinking.String()}
}

func (run *Run) pendingInteractionRequests() []InteractionRequest {
	run.mu.RLock()
	defer run.mu.RUnlock()
	result := make([]InteractionRequest, 0, len(run.interactions))
	for _, pending := range run.interactions {
		result = append(result, pending.request)
	}
	return result
}

// queuedSnapshotsLocked reads the inbox while Session.mu is held.
func (session *Session) queuedSnapshotsLocked() []QueuedRunSnapshot {
	var result []QueuedRunSnapshot
	for _, item := range session.inputOrder {
		if item.status != inputPending || (item.Kind != inputQueue && item.Kind != inputSteer) {
			continue
		}
		result = append(result, QueuedRunSnapshot{
			ID: item.targetRunID, CommandID: item.Receipt.CommandID, ReceiptCursor: item.Receipt.Cursor, Delivery: publicInputDelivery(item.delivery),
			Text: item.input.Text, InterruptRequested: item.delivery == runstate.DeliverySteer,
		})
	}
	return result
}

func (run *Run) openToolSnapshots() []OpenToolSnapshot {
	run.mu.RLock()
	defer run.mu.RUnlock()
	result := make([]OpenToolSnapshot, 0, len(run.openTools))
	for _, tool := range run.openTools {
		result = append(result, tool)
	}
	return result
}

func publicInputDelivery(delivery runstate.DeliveryKind) InputDelivery {
	switch delivery {
	case runstate.DeliverySteer:
		return DeliverySteer
	case runstate.DeliveryFollowUp:
		return DeliveryFollowUp
	case runstate.DeliveryNextTurn:
		return DeliveryNextTurn
	default:
		return ""
	}
}
