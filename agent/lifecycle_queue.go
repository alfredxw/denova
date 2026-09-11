package agent

import (
	"context"
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

// QueuedInput locates a durable supplemental input by Session and command ID.
// The handle owns no separate queue or execution state.
type QueuedInput struct {
	session *Session
	id      string
}

func (queued *QueuedInput) ID() string {
	if queued == nil {
		return ""
	}
	return queued.id
}

func (queued *QueuedInput) Receipt() CommandReceipt {
	if queued == nil || queued.session == nil {
		return CommandReceipt{}
	}
	queued.session.mu.RLock()
	defer queued.session.mu.RUnlock()
	if item := queued.session.inputs[queued.id]; item != nil {
		return item.Receipt
	}
	return CommandReceipt{}
}

func (queued *QueuedInput) Cancel(ctx context.Context, request QueueControlRequest) (CommandReceipt, error) {
	return queued.control(ctx, request, inputCancelled)
}

// Interrupt promotes the input to the next safe steering boundary. It never
// starts an idle Session or resumes a suspended Run.
func (queued *QueuedInput) Interrupt(ctx context.Context, request QueueControlRequest) (CommandReceipt, error) {
	return queued.control(ctx, request, inputPending)
}

func (queued *QueuedInput) control(ctx context.Context, request QueueControlRequest, status inputStatus) (CommandReceipt, error) {
	ctx, err := commandContext(ctx)
	if err != nil {
		return CommandReceipt{}, err
	}
	if queued == nil || queued.session == nil {
		return CommandReceipt{}, ErrRunSettled
	}
	session := queued.session
	if err := session.usable(); err != nil {
		return CommandReceipt{}, err
	}
	id := strings.TrimSpace(request.IdempotencyKey)
	if id == "" {
		id = newPublicID("command")
	}
	if err := ValidateIdempotencyKey(id); err != nil {
		return CommandReceipt{}, err
	}
	reason := strings.TrimSpace(request.Reason)
	if status == inputPending {
		reason = ""
	}
	hash, err := hashCanonical(struct {
		InputID string
		Status  inputStatus
		Reason  string
	}{queued.id, status, reason})
	if err != nil {
		return CommandReceipt{}, err
	}
	session.mu.Lock()
	if session.closed || session.closing {
		session.mu.Unlock()
		return CommandReceipt{}, ErrSessionClosed
	}
	if previous, found := session.controlReceipts[id]; found {
		session.mu.Unlock()
		if previous.Hash != hash {
			return CommandReceipt{}, ErrIdempotencyConflict
		}
		return previous.Receipt, nil
	}
	if session.inputs[id] != nil {
		session.mu.Unlock()
		return CommandReceipt{}, ErrIdempotencyConflict
	}
	item := session.inputs[queued.id]
	if item == nil {
		session.mu.Unlock()
		return CommandReceipt{}, ErrRunSettled
	}
	if item.status == inputConsumed {
		session.mu.Unlock()
		return CommandReceipt{}, ErrInputConsumed
	}
	if item.status == inputCancelled {
		session.mu.Unlock()
		return CommandReceipt{}, ErrInputCancelled
	}
	control := persistedControlReceipt{Receipt: CommandReceipt{CommandID: id, RunID: item.targetRunID, Cursor: session.cursor + 1}, Hash: hash}
	delivery := item.delivery
	if status == inputPending {
		delivery = runstate.DeliverySteer
	}
	update := persistedInputUpdate{CommandID: queued.id, RunID: item.targetRunID, Status: status, Delivery: delivery, Control: &control}
	if err := session.appendRecordLocked(ctx, sessionInputUpdateRecord, update); err != nil {
		session.mu.Unlock()
		return CommandReceipt{}, err
	}
	item.status, item.delivery = status, delivery
	session.controlReceipts[id] = control
	session.cursor = control.Receipt.Cursor
	if status == inputPending {
		session.promoteInputLocked(item)
	}
	active := session.active
	session.mu.Unlock()
	if status == inputPending && active != nil && active.id == item.targetRunID && !active.isSuspended() {
		active.requestPreemption()
	}
	return control.Receipt, nil
}

func (session *Session) promoteInputLocked(item *acceptedInput) {
	for index, candidate := range session.inputOrder {
		if candidate == item {
			copy(session.inputOrder[1:index+1], session.inputOrder[:index])
			session.inputOrder[0] = item
			return
		}
	}
}

func (run *Run) isSuspended() bool {
	if run == nil {
		return false
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	return run.result.Status == ResultSuspended
}
