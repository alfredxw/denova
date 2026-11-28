package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	agentengine "github.com/alfredxw/denova/agent/engine"
	agentasync "github.com/alfredxw/denova/agent/internal/async"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentinteraction "github.com/alfredxw/denova/agent/lifecycle/interaction"
	agenttrace "github.com/alfredxw/denova/agent/lifecycle/trace"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

func (run *Run) updateEngineTranscript(state json.RawMessage, persist bool) error {
	if state == nil {
		return nil
	}
	run.mu.RLock()
	outputCommit := run.snapshot.OutputCommit
	run.mu.RUnlock()
	run.session.mu.Lock()
	if run.session.canonicalMessages {
		var err error
		state, err = agentengine.AlignCheckpoint(state, run.session.messageCheckpoint, outputCommit)
		if err != nil {
			run.session.mu.Unlock()
			return err
		}
	}
	run.session.engineState = append(json.RawMessage(nil), state...)
	var err error
	if persist {
		err = run.session.persistTranscriptLocked(context.Background())
	}
	state = append(json.RawMessage(nil), run.session.engineState...)
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

func (run *Run) snapshotForCurrentCycle() (agentengine.TurnSnapshot, error) {
	run.mu.RLock()
	snapshot := run.snapshot
	snapshot.State = append(json.RawMessage(nil), snapshot.State...)
	snapshot.Capabilities = agentschema.CloneRawStateMap(snapshot.Capabilities)
	run.mu.RUnlock()
	if snapshot.OperationID == "" {
		return agentengine.TurnSnapshot{}, agentschema.ErrInteractionStale
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

func (run *Run) nextQueuedInput() (agentschema.Input, agentengine.DeliveryKind, bool) {
	run.session.mu.RLock()
	defer run.session.mu.RUnlock()
	for _, item := range run.session.inputOrder {
		if item.targetRunID == run.id && item.status == inputPending && (item.Kind == inputQueue || item.Kind == inputSteer) {
			return cloneInput(item.input), item.delivery, true
		}
	}
	return agentschema.Input{}, "", false
}

func (run *Run) publish(payload agentevent.EventPayload) {
	if run == nil || payload == nil {
		return
	}
	run.eventMu.Lock()
	defer run.eventMu.Unlock()
	event := agentevent.Event{RunID: run.id, Payload: payload}
	run.session.mu.Lock()
	run.session.publishLocked(event)
	event.Cursor = run.session.cursor
	run.session.mu.Unlock()
	run.lastEventCursor = event.Cursor
	if !run.eventsEnd {
		publishLatestEvent(run.events, event, &run.eventDrops)
	}
}

func (run *Run) finish(result agentschema.Result, err error) {
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
	if err == nil && (result.Status == agentschema.ResultFailed || result.Status == agentschema.ResultIncomplete || result.Status == agentschema.ResultBlocked) {
		err = &agentevent.RunError{Result: result}
	}
	output := run.content.String()
	receiptCursor := run.receipt
	run.mu.RUnlock()
	run.cancel()

	var next *Run
	run.session.mu.Lock()
	finishedActive := run.session.active == run
	kind := turnFinishedRecord
	if result.Status != agentschema.ResultCompleted && result.Status != agentschema.ResultAborted {
		kind = turnInterruptedRecord
	}
	finishedAt := time.Now().UTC()
	if appendErr := run.session.appendRecordLocked(context.Background(), kind, persistedTurn{
		RunID: run.id, CommandID: run.commandID, Status: result.Status, Reason: result.Reason, Output: output, At: finishedAt,
	}); appendErr != nil {
		err = errors.Join(err, appendErr)
		run.session.mu.Unlock()
		// A missing or uncertain settlement is still an unfinished journal.
		// Release this failed writer only after executeCycle stopped producers;
		// reopening must replay the facts before any further action.
		err = errors.Join(err, run.session.closeWriter())
		run.mu.Lock()
		run.result, run.err = agentschema.Result{Status: agentschema.ResultSuspended, Reason: "Agent journal requires reopening after a write failure"}, err
		run.mu.Unlock()
		run.endHandle()
		return
	}
	if finishedActive {
		run.session.active = nil
	}
	run.session.removePendingLocked(run)
	run.mu.Lock()
	run.settled, run.result, run.err, run.finishedAt = true, result, err, finishedAt
	run.mu.Unlock()
	run.session.addRecentLocked(agentevent.RunSummary{
		ID: run.id, CommandID: run.commandID, ReceiptCursor: receiptCursor,
		Status: result.Status, Reason: result.Reason, Output: output,
	})
	if finishedActive && err == nil {
		next, err = run.session.advancePendingLocked(run.id)
	}
	run.session.retireRunLocked(run)
	storageErr := run.session.storageErr
	run.session.mu.Unlock()
	if storageErr != nil {
		err = errors.Join(err, run.session.closeWriter())
	}

	run.mu.Lock()
	run.err = err
	run.mu.Unlock()
	run.publish(agentevent.RunSettled{Status: result.Status, Reason: result.Reason})
	agenttrace.EmitTrace(context.Background(), run.session.agent.trace, agenttrace.TraceEvent{
		Kind: agenttrace.TraceRunSettled, Session: run.session.key, RunID: run.id, Err: err,
	})
	run.endHandle()
	if next != nil {
		agentasync.SafeGo(next.execute, func(nextErr error) {
			next.finish(agentschema.Result{Status: agentschema.ResultFailed, Reason: nextErr.Error()}, nextErr)
		})
	}
	if run.ownership == runOwnsTemporarySession {
		_ = run.session.Delete(context.Background())
	}
}

// advancePendingLocked hands off only after a successful settlement and an open
// tree fence. Both settlement and fence release may be the last event to arrive.
// The caller holds session.mu and starts the returned Run exactly once.
func (session *Session) advancePendingLocked(completedID string) (*Run, error) {
	if session.active != nil || len(session.pending) == 0 || session.closed || session.closing || session.treeControl.sealed() {
		return nil, nil
	}
	completed := session.recovery.Runs[completedID]
	if completed.Status != agentschema.ResultCompleted && completed.Status != agentschema.ResultAborted {
		return nil, nil
	}
	next := session.pending[0]
	startedAt := time.Now().UTC()
	if err := session.appendRecordLocked(context.Background(), turnStartedRecord, persistedTurn{
		RunID: next.id, CommandID: next.commandID, At: startedAt,
	}); err != nil {
		return nil, err
	}
	session.pending = session.pending[1:]
	next.markStarted(startedAt)
	session.active = next
	return next, nil
}

func (run *Run) abort(reason string) {
	if run == nil || run.isSettled() {
		return
	}
	run.setAbortReason(reason)
	select {
	case run.controls <- agentengine.Control{Kind: agentengine.ControlAbort}:
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
		run.finish(agentschema.Result{Status: agentschema.ResultAborted, Reason: reason}, nil)
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

func (run *Run) outputSnapshot() agentevent.ActiveOutputSnapshot {
	run.mu.RLock()
	defer run.mu.RUnlock()
	return agentevent.ActiveOutputSnapshot{Content: run.content.String(), Thinking: run.thinking.String()}
}

func (run *Run) pendingInteractionRequests() []agentinteraction.InteractionRequest {
	run.mu.RLock()
	defer run.mu.RUnlock()
	result := make([]agentinteraction.InteractionRequest, 0, len(run.interactions))
	for _, pending := range run.interactions {
		result = append(result, pending.request)
	}
	return result
}

// queuedSnapshotsLocked reads the inbox while Session.mu is held.
func (session *Session) queuedSnapshotsLocked() []agentevent.QueuedRunSnapshot {
	var result []agentevent.QueuedRunSnapshot
	for _, item := range session.inputOrder {
		if item.status != inputPending || (item.Kind != inputQueue && item.Kind != inputSteer) {
			continue
		}
		result = append(result, agentevent.QueuedRunSnapshot{
			ID: item.targetRunID, CommandID: item.Receipt.CommandID, ReceiptCursor: item.Receipt.Cursor, Delivery: publicInputDelivery(item.delivery),
			Text: item.input.Text, InterruptRequested: item.delivery == agentengine.DeliverySteer,
		})
	}
	return result
}

func (run *Run) openToolSnapshots() []agentevent.OpenToolSnapshot {
	run.mu.RLock()
	defer run.mu.RUnlock()
	result := make([]agentevent.OpenToolSnapshot, 0, len(run.openTools))
	for _, tool := range run.openTools {
		result = append(result, tool)
	}
	return result
}

func publicInputDelivery(delivery agentengine.DeliveryKind) agentevent.InputDelivery {
	switch delivery {
	case agentengine.DeliverySteer:
		return agentevent.DeliverySteer
	case agentengine.DeliveryFollowUp:
		return agentevent.DeliveryFollowUp
	case agentengine.DeliveryNextTurn:
		return agentevent.DeliveryNextTurn
	default:
		return ""
	}
}
