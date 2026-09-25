package agentruntime

import (
	"context"
	"crypto/rand"
	"denova/internal/observability"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

// ExternalRun drains journaled inputs on the product task goroutine. Pausing
// waits for provider/tool settlement; queued inputs cannot wake a paused run.
type ExternalRun struct {
	control       *ExternalController
	factory       ExternalCycleFactory
	emit          func(agentrun.Event)
	receipt       agentrun.CommandReceipt
	cancel        context.CancelCauseFunc // protected by control.mu
	once          sync.Once
	outcome       agentrun.Outcome
	goal          agent.GoalState
	goalEvaluated bool          // false when committed work still needs evaluation after recovery
	guidance      int           // number of instructions supplied to the current provider attempt
	steering      chan struct{} // wakeups only; accepted inputs live in the journal
	replayed      bool
	recoveryKey   string
}

func (run *ExternalRun) Receipt() agentrun.CommandReceipt { return run.receipt }
func (run *ExternalRun) Wait(ctx context.Context) agentrun.Outcome {
	if run.replayed {
		return run.outcome
	}
	run.once.Do(func() {
		defer func() {
			if value := recover(); value != nil {
				err := fmt.Errorf("external product controller panic: %v", value)
				slog.ErrorContext(ctx, "External product controller panicked", "operation_id", run.receipt.OperationID, "error", err)
				run.outcome = agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: err}
			}
			run.control.mu.Lock()
			if run.recoveryKey != "" {
				if err := run.control.update(context.WithoutCancel(ctx), func(state *externalControlState) error {
					saved := state.Receipts[run.recoveryKey]
					saved.Outcome = run.outcome.Status
					state.Receipts[run.recoveryKey] = saved
					return nil
				}); err != nil {
					slog.ErrorContext(ctx, "Could not save external recovery outcome", "error", err)
				}
			}
			if run.cancel != nil {
				run.cancel(context.Canceled)
				run.cancel = nil
			}
			if run.control.active == run {
				run.control.active = nil
			}
			run.control.mu.Unlock()
			switch run.outcome.Status {
			case agentrun.OutcomeCompleted:
				run.send(agentrun.Event{Type: "done", Data: map[string]any{}})
			case agentrun.OutcomeAborted:
				run.send(agentrun.NewAbortedEvent(agentrun.AbortReasonUserRequested))
			case agentrun.OutcomeSuspended:
				if run.outcome.Error != nil && !errors.Is(run.outcome.Error, context.Canceled) {
					slog.ErrorContext(ctx, "External task suspended after failure", "operation_id", run.receipt.OperationID, "error", run.outcome.Error)
					run.send(agentrun.Event{Type: "error", Data: map[string]any{"error_key": "agentRuntime.operationFailed", "details": map[string]any{"detail": observability.ErrorCause(run.outcome.Error)}, "run_id": string(run.receipt.OperationID)}})
				}
				run.send(agentrun.Event{Type: "suspended", Data: map[string]any{"reason": "runtime_paused"}})
			case agentrun.OutcomeFailed:
				run.send(agentrun.Event{Type: "error", Data: map[string]any{"error_key": "agentRuntime.operationFailed", "details": map[string]any{"detail": observability.ErrorCause(run.outcome.Error)}, "run_id": string(run.receipt.OperationID)}})
			}
		}()
		run.outcome = run.drain(ctx)
	})
	return run.outcome
}

func (run *ExternalRun) drain(ctx context.Context) agentrun.Outcome {
	control := run.control
	last := agentrun.Outcome{Status: agentrun.OutcomeCompleted}
	for {
		control.mu.Lock()
		state, err := control.read(context.WithoutCancel(ctx))
		if err != nil {
			control.mu.Unlock()
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
		}
		if state.Phase == agentrun.RunPhaseSuspended {
			control.mu.Unlock()
			return agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: last.Error}
		}
		if state.Phase == agentrun.RunPhaseIdle {
			control.mu.Unlock()
			return agentrun.Outcome{Status: agentrun.OutcomeAborted}
		}
		if ctx.Err() != nil {
			err = control.update(context.WithoutCancel(ctx), func(current *externalControlState) error {
				current.Phase = agentrun.RunPhaseSuspended
				if current.Current != nil {
					current.Current.Resume = true
				}
				return nil
			})
			control.mu.Unlock()
			return agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: errors.Join(ctx.Err(), err)}
		}
		if state.Current == nil {
			err = control.update(ctx, func(current *externalControlState) error {
				if len(current.Queue) == 0 {
					current.Phase = agentrun.RunPhaseIdle
					current.Last = &agentrun.OperationSummary{OperationID: current.OperationID, CommandID: current.CommandID, Status: agentrun.OperationSucceeded, ReceiptCursor: run.receipt.Cursor}
					saved := current.Receipts[string(current.CommandID)]
					saved.Outcome = agentrun.OutcomeCompleted
					current.Receipts[string(current.CommandID)] = saved
				} else {
					input := current.Queue[0]
					current.Current = &input
					current.Queue = current.Queue[1:]
				}
				return nil
			})
			control.mu.Unlock()
			if err != nil {
				return agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: err}
			}
			if len(state.Queue) == 0 {
				return last
			}
			continue
		}
		input := *state.Current
		run.goalEvaluated = false
		run.guidance = len(input.Guidance)
		input.OperationID = state.OperationID
		run.goal = agent.GoalState{}
		if supportsGoal(control.binding.AgentKind) {
			run.goal, _, err = control.store.Goal(ctx)
		}
		if err != nil {
			control.mu.Unlock()
			return agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: err}
		}
		if input.GoalID != "" && (run.goal.ID != input.GoalID || run.goal.Revision != input.GoalRevision || !run.goal.Active()) {
			err = control.update(ctx, func(current *externalControlState) error { current.Current = nil; return nil })
			control.mu.Unlock()
			if err != nil {
				return agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: err}
			}
			continue
		}
		cycleCtx, cancel := context.WithCancelCause(ctx)
		run.cancel = cancel
		run.steering = make(chan struct{}, 1)
		cycleCtx = external.WithSteering(cycleCtx, &external.Steering{
			Changed: run.steering,
			Next: func(ctx context.Context) (external.Guidance, bool, error) {
				control.mu.Lock()
				defer control.mu.Unlock()
				state, err := control.read(ctx)
				if err != nil || state.Phase != agentrun.RunPhaseRunning || state.Current == nil || len(state.Current.Guidance) <= run.guidance {
					return external.Guidance{}, false, err
				}
				return external.Guidance{Request: state.Current.Guidance[run.guidance], Count: run.guidance + 1}, true, nil
			},
			Delivered: func(guidance external.Guidance) {
				control.mu.Lock()
				run.guidance = guidance.Count
				control.mu.Unlock()
			},
			Interrupt: func() { cancel(external.ErrSteered) },
		})
		control.mu.Unlock()
		cycle, err := run.factory(cycleCtx, input, run.cycleEvent, run.afterCommit)
		outcome := agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
		if err == nil {
			outcome = cycle.Wait(cycleCtx)
		}
		consumedGuidance := len(input.Guidance)
		if receipt, ok := cycle.(interface{ ConsumedGuidance() int }); ok {
			consumedGuidance = receipt.ConsumedGuidance()
		}
		control.mu.Lock()
		cause := context.Cause(cycleCtx)
		cancel(context.Canceled)
		run.cancel = nil
		run.steering = nil
		var pendingGoal agent.GoalState
		if outcome.Status == agentrun.OutcomeCompleted && run.goal.Active() && !run.goalEvaluated {
			goal, present, goalErr := control.store.Goal(context.WithoutCancel(ctx))
			if goalErr != nil {
				control.mu.Unlock()
				return agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: goalErr}
			}
			if present && goal.Active() && goal.ID == run.goal.ID && goal.Revision == run.goal.Revision {
				pendingGoal = goal
			}
		}
		err = control.update(context.WithoutCancel(ctx), func(current *externalControlState) error {
			if current.Phase == agentrun.RunPhaseIdle {
				return nil
			}
			if outcome.Status == agentrun.OutcomeCompleted {
				// Guidance admitted after the provider committed becomes a fresh
				// input. It cannot rewrite the completed Game turn or disappear.
				if current.Current != nil && len(current.Current.Guidance) > consumedGuidance {
					for _, request := range current.Current.Guidance[consumedGuidance:] {
						current.Queue = append(current.Queue, ExternalCycleInput{Request: request, Delivery: agentrun.DeliveryFollowUp})
					}
				}
				current.Current = nil
				// A paused evaluation or recovered commit must not replay the
				// accepted user input, or silently abandon an active Goal.
				if pendingGoal.Active() && len(current.Queue) == 0 && (input.Resume || current.Phase == agentrun.RunPhaseSuspended) {
					current.Queue = append(current.Queue, ExternalCycleInput{
						Request:  agentchat.ChatRequest{CommandID: "goal-" + rand.Text(), Message: "Continue the active goal from the confirmed work. Verify what remains before making further changes.", InputVisibility: agentrun.InputModelOnly},
						Delivery: agentrun.DeliveryNextTurn, GoalID: pendingGoal.ID, GoalRevision: pendingGoal.Revision,
					})
				}
				return nil
			}
			if current.Current != nil {
				current.Current.Resume = true
			}
			if !errors.Is(cause, external.ErrSteered) {
				current.Phase = agentrun.RunPhaseSuspended
			}
			return nil
		})
		control.mu.Unlock()
		if err != nil {
			return agentrun.Outcome{Status: agentrun.OutcomeSuspended, Error: err}
		}
		last = outcome
	}
}

func (run *ExternalRun) send(event agentrun.Event) {
	if run.emit != nil {
		run.emit(event)
	}
}
func (run *ExternalRun) cycleEvent(event agentrun.Event) {
	switch event.Type {
	case "done", "aborted", "suspended", "error":
		return
	}
	if source, ok := event.Data.(map[string]any); ok {
		data := make(map[string]any, len(source)+1)
		for key, value := range source {
			data[key] = value
		}
		data["operation_id"] = string(run.receipt.OperationID)
		event.Data = data
	}
	run.send(event)
	if event.Type == "tool_result" || event.Type == "ask_resolved" {
		run.deliverFollowUps()
	}
}

// Follow-up input joins the unfinished task only after a confirmed tool result.
// Next-turn input stays queued; the adapter owns live steering delivery.
func (run *ExternalRun) deliverFollowUps() {
	control := run.control
	control.mu.Lock()
	defer control.mu.Unlock()
	if run.cancel == nil {
		return
	}
	delivered := false
	err := control.update(context.Background(), func(state *externalControlState) error {
		if state.Phase != agentrun.RunPhaseRunning || state.Current == nil {
			return nil
		}
		pending := state.Queue[:0]
		for _, input := range state.Queue {
			if input.Delivery == agentrun.DeliveryFollowUp {
				state.Current.Guidance = append(state.Current.Guidance, input.Request)
				state.Current.Resume, delivered = true, true
			} else {
				pending = append(pending, input)
			}
		}
		state.Queue = pending
		return nil
	})
	if err != nil {
		slog.Error("Could not deliver external follow-up input", "operation_id", run.receipt.OperationID, "error", err)
		run.cancel(err)
	} else if delivered {
		run.notifySteering()
	}
}

// notifySteering is called under control.mu and never blocks tool completion.
func (run *ExternalRun) notifySteering() {
	if run.steering != nil {
		select {
		case run.steering <- struct{}{}:
		default:
		}
	}
}
