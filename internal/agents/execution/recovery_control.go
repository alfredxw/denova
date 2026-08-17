package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

func (r *RecoveryObservation) Resume(
	ctx context.Context,
	action RuntimeRecoveryAction,
	taskID string,
	emit func(agentrun.Event),
) (agentrun.CommandReceipt, error) {
	if r == nil || r.publicBackend == nil || r.publicSession == nil {
		return agentrun.CommandReceipt{}, ErrRuntimeProjectionUnavailable
	}
	status, err := r.CurrentStatus(ctx)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	if !containsRuntimeRecoveryAction(RuntimeRecoveryActions(status), action) {
		return agentrun.CommandReceipt{}, fmt.Errorf(
			"%w: kind=%q action_id=%q operation_id=%q",
			ErrRecoveryActionChanged, action.Kind, action.ActionID, action.OperationID,
		)
	}
	options := r.publicOptions
	options.TaskID = strings.TrimSpace(taskID)
	r.mu.Lock()
	r.publicTerminalDelivered = false
	r.mu.Unlock()
	routedEmit := func(event agentrun.Event) {
		if event.Type == "done" || event.Type == "aborted" || event.Type == "error" {
			r.mu.Lock()
			r.publicTerminalDelivered = true
			r.mu.Unlock()
		}
		if emit != nil {
			emit(event)
		}
	}
	registration := r.publicBackend.bindRecoveryRoute(
		r.publicSession.Key(), string(action.CommandID), options, routedEmit,
	)
	attached, found, err := r.publicSession.AttachRun(ctx, string(action.OperationID))
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	if !found {
		return agentrun.CommandReceipt{}, agent.ErrNoActiveRun
	}
	r.mu.Lock()
	r.publicHandle = r.publicBackend.trackRun(r.publicSession, attached, registration, "")
	r.mu.Unlock()
	return mapPublicReceipt(attached), nil
}

func containsRuntimeRecoveryAction(actions []RuntimeRecoveryAction, selected RuntimeRecoveryAction) bool {
	for _, action := range actions {
		if action == selected {
			return true
		}
	}
	return false
}

// Wait follows the in-process Agent Session rather than the browser connection
// and supplies exactly one terminal display event.
func (r *RecoveryObservation) Wait(ctx context.Context, emit func(agentrun.Event)) agentrun.Outcome {
	if r == nil || r.publicBackend == nil || r.publicSession == nil {
		return agentrun.NewOutcome(
			agentrun.OutcomeFailed, ErrRuntimeProjectionUnavailable,
			ErrRuntimeProjectionUnavailable.Error(), "", "",
		)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	handle := r.publicHandle
	initialCursor := r.publicInitial.Cursor
	initial := r.publicInitial
	r.mu.Unlock()
	if handle != nil {
		outcome := r.publicBackend.wait(ctx, handle)
		r.mu.Lock()
		terminalDelivered := r.publicTerminalDelivered
		r.mu.Unlock()
		if !terminalDelivered {
			terminalDelivered = r.publicBackend.runTerminalProjected(handle)
		}
		if !terminalDelivered {
			emitRecoveryTerminal(emit, outcome)
		}
		return outcome
	}
	if initial.ActiveRunID == "" && len(initial.QueuedRuns) == 0 && len(initial.RecentRuns) > 0 {
		settled := initial.RecentRuns[len(initial.RecentRuns)-1]
		outcome := publicResultOutcome(agent.Result{Status: settled.Status, Reason: settled.Reason}, nil, "", "")
		emitRecoveryTerminal(emit, outcome)
		return outcome
	}
	for {
		select {
		case event, ok := <-r.publicObservation.Events:
			if !ok {
				err := errors.New("Agent recovery observation closed before settlement")
				emitExecutionError(emit, err)
				return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
			}
			if event.Cursor <= initialCursor {
				continue
			}
			settled, ok := event.Payload.(agent.RunSettled)
			if !ok {
				continue
			}
			outcome := publicResultOutcome(agent.Result{Status: settled.Status, Reason: settled.Reason}, nil, "", "")
			emitRecoveryTerminal(emit, outcome)
			return outcome
		case observationErr, ok := <-r.publicObservation.Errors:
			if ok && observationErr != nil {
				emitExecutionError(emit, observationErr)
				return agentrun.NewOutcome(agentrun.OutcomeFailed, observationErr, observationErr.Error(), "", "")
			}
		case <-ctx.Done():
			return agentrun.NewOutcome(agentrun.OutcomeAborted, ctx.Err(), ctx.Err().Error(), "", "")
		}
	}
}

func (backend *publicBackend) runTerminalProjected(handle *publicRunHandle) bool {
	if backend == nil || handle == nil || handle.run == nil {
		return false
	}
	for _, item := range backend.runCycleRegistrations(handle.run.ID(), handle.registration) {
		item.registration.mu.RLock()
		projector := item.registration.projector
		item.registration.mu.RUnlock()
		if projector != nil && projector.TerminalProjected() {
			return true
		}
	}
	return false
}

func emitRecoveryTerminal(emit func(agentrun.Event), outcome agentrun.Outcome) {
	if emit == nil {
		return
	}
	switch outcome.Status {
	case agentrun.OutcomeCompleted:
		emit(agentrun.Event{Type: "done", Data: map[string]string{}})
	case agentrun.OutcomeAborted:
		emit(agentrun.NewAbortedEvent(outcome.Reason))
	default:
		message := outcome.Reason
		if message == "" && outcome.Error != nil {
			message = outcome.Error.Error()
		}
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": message}})
	}
}
