package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

const publicRunEventBuffer = 1024

// Run is one user-visible durable operation.
type Run struct {
	session   *Session
	id        string
	commandID string
	cursor    runstate.Cursor
	replayed  bool
	events    chan Event
	done      chan struct{}
	observe   context.Context
	stop      context.CancelFunc

	mu           sync.RWMutex
	result       Result
	err          error
	settled      bool
	closeSession bool
}

type runSessionOwnership uint8

const (
	runUsesDurableSession runSessionOwnership = iota
	runOwnsTemporarySession
)

func newPublicRun(
	session *Session,
	receipt runstate.Receipt,
	observation runstate.Observation,
	observeCtx context.Context,
	stop context.CancelFunc,
	ownership runSessionOwnership,
) *Run {
	run := &Run{
		session: session, id: string(receipt.OperationID), commandID: string(receipt.CommandID),
		cursor: receipt.Cursor, replayed: receipt.Replayed,
		events: make(chan Event, publicRunEventBuffer), done: make(chan struct{}), stop: stop,
		observe:      observeCtx,
		closeSession: ownership == runOwnsTemporarySession,
	}
	safeGo(func() { run.consume(observation) }, func(err error) { run.finish(Result{Status: ResultFailed, Reason: err.Error()}, err) })
	return run
}

// Replayed reports whether admission returned an existing durable Run rather
// than accepting a new one.
func (run *Run) Replayed() bool {
	return run != nil && run.replayed
}

func (run *Run) ID() string {
	if run == nil {
		return ""
	}
	return run.id
}

// CommandID returns the caller-owned idempotency key accepted for this Run.
func (run *Run) CommandID() string {
	if run == nil {
		return ""
	}
	return run.commandID
}

// Receipt returns the exact durable admission receipt for this Run.
func (run *Run) Receipt() CommandReceipt {
	if run == nil {
		return CommandReceipt{}
	}
	return CommandReceipt{
		CommandID: run.commandID, RunID: run.id, Cursor: Cursor(run.cursor), Replayed: run.replayed,
	}
}

func (run *Run) Events() <-chan Event {
	if run == nil {
		closed := make(chan Event)
		close(closed)
		return closed
	}
	return run.events
}

func (run *Run) Steer(ctx context.Context, input Input) (CommandReceipt, error) {
	if err := run.usable(); err != nil {
		return CommandReceipt{}, err
	}
	if input.Goal != nil {
		input.Goal = cloneGoalMutation(input.Goal)
		if input.Goal.MutationID == "" {
			input.Goal.MutationID = newPublicID("goal-mutation")
		}
	}
	encoded, runtimeInput, err := encodeInput(input)
	if err != nil {
		return CommandReceipt{}, err
	}
	runtimeInput.RestoreDescriptor = encoded
	commandID := input.IdempotencyKey
	if commandID == "" {
		commandID = newPublicID("command")
	}
	receipt, err := run.session.harness.Submit(ctx, runstate.Steer{
		ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(run.id), Input: runtimeInput,
	})
	if err != nil {
		return CommandReceipt{}, mapRuntimeError(err)
	}
	return mapCommandReceipt(receipt), nil
}

func (run *Run) FollowUp(ctx context.Context, input Input) (*Run, error) {
	if err := run.usable(); err != nil {
		return nil, err
	}
	return run.session.start(ctx, input, run.id, true, runUsesDurableSession)
}

// Queue accepts an input that should run as the next cycle of this same Run
// after the current model response reaches its safe boundary. The returned
// handle controls only that accepted input; it is not a second Run.
func (run *Run) Queue(ctx context.Context, input Input) (*QueuedInput, error) {
	if err := run.usable(); err != nil {
		return nil, err
	}
	if input.Goal != nil {
		input.Goal = cloneGoalMutation(input.Goal)
		if input.Goal.MutationID == "" {
			input.Goal.MutationID = newPublicID("goal-mutation")
		}
	}
	encoded, runtimeInput, err := encodeInput(input)
	if err != nil {
		return nil, err
	}
	runtimeInput.RestoreDescriptor = encoded
	commandID := strings.TrimSpace(input.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	receipt, err := run.session.harness.Submit(ctx, runstate.FollowUp{
		ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(run.id), Input: runtimeInput,
	})
	if err != nil {
		return nil, mapRuntimeError(err)
	}
	return &QueuedInput{run: run, receipt: receipt}, nil
}

// Queued returns a control handle for one still-pending input accepted by this
// Run. The ID is the input's caller-owned IdempotencyKey and is restart-safe.
func (run *Run) Queued(ctx context.Context, id string) (*QueuedInput, bool, error) {
	if err := run.usable(); err != nil {
		return nil, false, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, false, nil
	}
	status, err := run.session.harness.Status(ctx)
	if err != nil {
		return nil, false, mapRuntimeError(err)
	}
	for _, item := range status.Queue {
		if item.Autonomous || string(item.OperationID) != run.id || string(item.CommandID) != id || item.Delivery != runstate.DeliveryFollowUp {
			continue
		}
		return &QueuedInput{run: run, receipt: runstate.Receipt{
			CommandID: item.CommandID, OperationID: item.OperationID, Cursor: item.ReceiptCursor,
		}}, true, nil
	}
	return nil, false, nil
}

func (run *Run) Abort(ctx context.Context, request AbortRequest) (CommandReceipt, error) {
	if run == nil || run.session == nil {
		return CommandReceipt{}, ErrRunSettled
	}
	if err := run.session.usable(); err != nil {
		return CommandReceipt{}, err
	}
	explicitCommandID := strings.TrimSpace(request.IdempotencyKey) != ""
	if !explicitCommandID {
		run.mu.RLock()
		settled := run.settled
		run.mu.RUnlock()
		if settled {
			return CommandReceipt{}, ErrRunSettled
		}
	}
	status, err := run.session.harness.Status(ctx)
	if err != nil {
		return CommandReceipt{}, mapRuntimeError(err)
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	if string(status.ActiveOperation) != run.id {
		// FollowUp returns a future Run whose own operation is present in the
		// NextTurn queue. It remains cancellable through its Run handle.
		for _, item := range status.Queue {
			if item.Delivery != runstate.DeliveryNextTurn || string(item.OperationID) != run.id || string(item.CommandID) != run.commandID {
				continue
			}
			receipt, submitErr := run.session.harness.Submit(ctx, runstate.CancelQueued{
				ID: runstate.CommandID(commandID), OperationID: status.ActiveOperation,
				TargetCommandID: runstate.CommandID(run.commandID), Reason: request.Reason,
			})
			if submitErr != nil {
				return CommandReceipt{}, mapRuntimeError(submitErr)
			}
			return mapCommandReceipt(receipt), nil
		}
		// A transport may retry an already committed Abort after the Run has
		// settled. Submitting the exact caller command lets the durable command
		// index replay its original receipt; a fresh stale command still fails.
		receipt, submitErr := run.session.harness.Submit(ctx, runstate.Abort{
			ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(run.id), Reason: request.Reason,
		})
		if submitErr != nil {
			return CommandReceipt{}, mapRuntimeError(submitErr)
		}
		return mapCommandReceipt(receipt), nil
	}
	receipt, err := run.session.harness.Submit(ctx, runstate.Abort{
		ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(run.id), Reason: request.Reason,
	})
	if err != nil {
		return CommandReceipt{}, mapRuntimeError(err)
	}
	return mapCommandReceipt(receipt), nil
}

// QueuedInput is one accepted same-Run continuation. It can be cancelled or
// promoted to interrupt the current response without copying its durable input.
type QueuedInput struct {
	run     *Run
	receipt runstate.Receipt
}

func (queued *QueuedInput) ID() string {
	if queued == nil {
		return ""
	}
	return string(queued.receipt.CommandID)
}

func (queued *QueuedInput) Receipt() CommandReceipt {
	if queued == nil {
		return CommandReceipt{}
	}
	return mapCommandReceipt(queued.receipt)
}

func (queued *QueuedInput) Cancel(ctx context.Context, request QueueControlRequest) (CommandReceipt, error) {
	if queued == nil || queued.run == nil {
		return CommandReceipt{}, ErrRunSettled
	}
	if err := queued.run.usable(); err != nil {
		return CommandReceipt{}, err
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	receipt, err := queued.run.session.harness.Submit(ctx, runstate.CancelQueued{
		ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(queued.run.id),
		TargetCommandID: queued.receipt.CommandID, Reason: request.Reason,
	})
	if err != nil {
		return CommandReceipt{}, mapRuntimeError(err)
	}
	return mapCommandReceipt(receipt), nil
}

// Interrupt asks the active Run to yield at its safe preemption boundary and
// execute this already accepted continuation next.
func (queued *QueuedInput) Interrupt(ctx context.Context, request QueueControlRequest) (CommandReceipt, error) {
	if queued == nil || queued.run == nil {
		return CommandReceipt{}, ErrRunSettled
	}
	if err := queued.run.usable(); err != nil {
		return CommandReceipt{}, err
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	receipt, err := queued.run.session.harness.Submit(ctx, runstate.SteerQueued{
		ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(queued.run.id),
		TargetCommandID: queued.receipt.CommandID,
	})
	if err != nil {
		return CommandReceipt{}, mapRuntimeError(err)
	}
	return mapCommandReceipt(receipt), nil
}

func mapCommandReceipt(receipt runstate.Receipt) CommandReceipt {
	return CommandReceipt{
		CommandID: string(receipt.CommandID), RunID: string(receipt.OperationID),
		Cursor: Cursor(receipt.Cursor), Replayed: receipt.Replayed,
	}
}

func (run *Run) Respond(ctx context.Context, interactionID string, response InteractionResponse) error {
	if err := run.usable(); err != nil {
		return err
	}
	interactionID = strings.TrimSpace(interactionID)
	if interactionID == "" {
		return ErrInteractionStale
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode Interaction response: %w", err)
	}
	fingerprint, err := hashCanonical(struct {
		ID       string
		Response json.RawMessage
	}{interactionID, encoded})
	if err != nil {
		return err
	}
	_, err = run.session.harness.Submit(ctx, runstate.ResolveInteraction{
		ID: runstate.CommandID("interaction-" + fingerprint), OperationID: runstate.OperationID(run.id),
		InteractionID: interactionID, Response: encoded,
	})
	return mapRuntimeError(err)
}

func (run *Run) Wait(ctx context.Context) (Result, error) {
	if run == nil {
		return Result{}, ErrRunSettled
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-run.done:
		run.mu.RLock()
		result, err := run.result, run.err
		run.mu.RUnlock()
		return result, err
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (run *Run) usable() error {
	if run == nil || run.session == nil {
		return ErrRunSettled
	}
	run.mu.RLock()
	settled := run.settled
	run.mu.RUnlock()
	if settled {
		return ErrRunSettled
	}
	return run.session.usable()
}

func (run *Run) consume(observation runstate.Observation) {
	trackedCalls := make(map[string]trackedToolCall)
	for _, call := range observation.Snapshot.OpenToolCalls {
		if string(call.OperationID) == run.id {
			trackedCalls[call.CallID] = trackedToolCall{runID: run.id, source: publicEventSource(call.Source)}
		}
	}
	events := observation.Events
	errorsChannel := observation.Errors
	lastCursor := runstate.Cursor(run.cursor)
	drops := eventDropState{}
	for {
		if events == nil && errorsChannel == nil {
			if run.isSettled() {
				return
			}
			reconnected, result, err := run.reconnectObservation(lastCursor)
			if err != nil {
				run.finish(Result{Status: ResultFailed, Reason: err.Error()}, err)
				return
			}
			if result != nil {
				publishLatestEvent(run.events, Event{
					Cursor: Cursor(lastCursor), Durability: DurableEvent, RunID: run.id,
					Payload: RunSettled{Status: result.Status, Reason: result.Reason},
				}, &drops)
				run.finishResult(*result)
				return
			}
			observation = reconnected
			events, errorsChannel = observation.Events, observation.Errors
			continue
		}
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if event.Cursor > lastCursor {
				lastCursor = event.Cursor
			}
			mapped, include := mapRunEvent(event, run.id, run.commandID, trackedCalls)
			if !include {
				continue
			}
			publishLatestEvent(run.events, mapped, &drops)
			if settled, ok := mapped.Payload.(RunSettled); ok {
				run.finishResult(Result{Status: settled.Status, Reason: settled.Reason})
				return
			}
		case err, ok := <-errorsChannel:
			if !ok {
				errorsChannel = nil
				continue
			}
			if err != nil {
				events, errorsChannel = nil, nil
			}
		}
	}
}

func (run *Run) reconnectObservation(after runstate.Cursor) (runstate.Observation, *Result, error) {
	ctx := run.observe
	if ctx == nil {
		ctx = context.Background()
	}
	status, err := run.session.harness.Status(ctx)
	if err != nil {
		return runstate.Observation{}, nil, mapRuntimeError(err)
	}
	for _, summary := range status.RecentOperations {
		if string(summary.OperationID) == run.id {
			result := Result{Status: mapResultStatus(summary.Status), Reason: summary.Reason}
			return runstate.Observation{}, &result, nil
		}
	}
	observation, err := run.session.harness.Observe(ctx, after)
	if errors.Is(err, runstate.ErrCursorExpired) {
		observation, err = run.session.harness.ObserveFromNow(ctx)
		if err == nil {
			gap := Event{Cursor: Cursor(observation.Snapshot.Cursor), Durability: EphemeralEvent, RunID: run.id,
				Payload: EventStreamGap{ResumeAfter: Cursor(observation.Snapshot.Cursor)}}
			publishLatestEvent(run.events, gap, &eventDropState{})
		}
	}
	if err != nil {
		return runstate.Observation{}, nil, mapRuntimeError(err)
	}
	return observation, nil, nil
}

func (run *Run) finishResult(result Result) {
	var err error
	if result.Status == ResultFailed || result.Status == ResultIncomplete || result.Status == ResultBlocked {
		err = &RunError{Result: result}
	}
	run.finish(result, err)
}

func (run *Run) finish(result Result, err error) {
	if run == nil {
		return
	}
	run.mu.Lock()
	if run.settled {
		run.mu.Unlock()
		return
	}
	run.settled = true
	run.result = result
	run.err = err
	closeSession := run.closeSession
	run.mu.Unlock()
	if run.stop != nil {
		run.stop()
	}
	// Agent.Run promises an anonymous Session, so settlement is not complete
	// until its durable records have been removed. Close done only after the
	// delete barrier; otherwise Wait can return while the temporary Session is
	// still visible in the Store catalog (and can be observed or backed up by a
	// concurrent owner).
	if closeSession && run.session != nil {
		if deleteErr := run.session.Delete(context.Background()); deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("delete temporary Agent Session: %w", deleteErr))
			run.mu.Lock()
			run.err = err
			run.mu.Unlock()
		}
	}
	if run.session != nil && run.session.agent != nil {
		emitTrace(context.Background(), run.session.agent.trace, TraceEvent{
			Kind: TraceRunSettled, Session: run.session.key, RunID: run.id, Err: err,
		})
	}
	close(run.done)
	close(run.events)
}

func (run *Run) isSettled() bool {
	run.mu.RLock()
	defer run.mu.RUnlock()
	return run.settled
}
