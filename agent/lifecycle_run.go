package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

const publicRunEventBuffer = 1024

type runSessionOwnership uint8

const (
	runUsesSession runSessionOwnership = iota
	runOwnsTemporarySession
)

type queuedRunInput struct {
	id        string
	input     Input
	delivery  runstate.DeliveryKind
	cursor    Cursor
	cancelled bool
}

type pendingInteraction struct {
	snapshot runstate.InteractionSnapshot
	request  InteractionRequest
}

// Run is an in-process execution handle. Its event stream and controls can be
// reattached while this process is alive; after restart the persisted Session
// transcript remains, while unfinished work is reported as interrupted.
type Run struct {
	session    *Session
	id         string
	commandID  string
	receipt    Cursor
	input      Input
	events     chan Event
	done       chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	controls   chan runstate.EngineControl
	ownership  runSessionOwnership
	eventMu    sync.Mutex
	eventDrops eventDropState
	eventsEnd  bool

	mu           sync.RWMutex
	result       Result
	err          error
	settled      bool
	startedAt    time.Time
	abortReason  string
	cycle        int
	snapshot     runstate.TurnSnapshot
	delivery     runstate.DeliveryKind
	queue        []*queuedRunInput
	interactions map[string]pendingInteraction
	content      strings.Builder
	thinking     strings.Builder
	toolSources  map[string]EventSource
	openTools    map[string]OpenToolSnapshot
}

func newPublicRun(session *Session, id, commandID string, input Input, delivery runstate.DeliveryKind, ownership runSessionOwnership) *Run {
	ctx, cancel := context.WithCancel(session.agent.ctx)
	return &Run{
		session: session, id: id, commandID: commandID, input: cloneInput(input),
		events: make(chan Event, publicRunEventBuffer), done: make(chan struct{}),
		ctx: ctx, cancel: cancel, controls: make(chan runstate.EngineControl, 32), ownership: ownership,
		interactions: make(map[string]pendingInteraction), toolSources: make(map[string]EventSource),
		openTools: make(map[string]OpenToolSnapshot),
		delivery:  delivery,
	}
}

func (run *Run) ID() string {
	if run == nil {
		return ""
	}
	return run.id
}

func (run *Run) CommandID() string {
	if run == nil {
		return ""
	}
	return run.commandID
}

func (run *Run) Receipt() CommandReceipt {
	if run == nil {
		return CommandReceipt{}
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	return CommandReceipt{CommandID: run.commandID, RunID: run.id, Cursor: run.receipt}
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
	ctx, err := commandContext(ctx)
	if err != nil {
		return CommandReceipt{}, err
	}
	if err := run.usable(); err != nil {
		return CommandReceipt{}, err
	}
	if _, _, err := encodeInput(input); err != nil {
		return CommandReceipt{}, err
	}
	id := strings.TrimSpace(input.IdempotencyKey)
	if id == "" {
		id = newPublicID("command")
	}
	input.IdempotencyKey = id
	run.session.mu.Lock()
	run.mu.Lock()
	cursor := run.session.nextCommandCursorLocked()
	run.queue = append([]*queuedRunInput{{id: id, input: cloneInput(input), delivery: runstate.DeliverySteer, cursor: cursor}}, run.queue...)
	run.mu.Unlock()
	run.session.mu.Unlock()
	select {
	case run.controls <- runstate.EngineControl{Kind: runstate.EngineControlPreempt}:
	case <-run.done:
		return CommandReceipt{}, ErrRunSettled
	}
	return CommandReceipt{CommandID: id, RunID: run.id, Cursor: cursor}, nil
}

func (run *Run) Queue(ctx context.Context, input Input) (*QueuedInput, error) {
	if _, err := commandContext(ctx); err != nil {
		return nil, err
	}
	if err := run.usable(); err != nil {
		return nil, err
	}
	if _, _, err := encodeInput(input); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(input.IdempotencyKey)
	if id == "" {
		id = newPublicID("command")
	}
	input.IdempotencyKey = id
	item := &queuedRunInput{id: id, input: cloneInput(input), delivery: runstate.DeliveryFollowUp}
	run.session.mu.Lock()
	run.mu.Lock()
	item.cursor = run.session.nextCommandCursorLocked()
	run.queue = append(run.queue, item)
	run.mu.Unlock()
	run.session.mu.Unlock()
	return &QueuedInput{run: run, item: item}, nil
}

func (run *Run) Queued(ctx context.Context, id string) (*QueuedInput, bool, error) {
	if _, err := commandContext(ctx); err != nil {
		return nil, false, err
	}
	if err := run.usable(); err != nil {
		return nil, false, err
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	for _, item := range run.queue {
		if item.id == strings.TrimSpace(id) && !item.cancelled {
			return &QueuedInput{run: run, item: item}, true, nil
		}
	}
	return nil, false, nil
}

func (run *Run) FollowUp(ctx context.Context, input Input) (*Run, error) {
	ctx, err := commandContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := run.usable(); err != nil {
		return nil, err
	}
	if _, _, err := encodeInput(input); err != nil {
		return nil, err
	}
	commandID := strings.TrimSpace(input.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	input.IdempotencyKey = commandID
	id, err := run.session.agent.nextRunID(run.session.key)
	if err != nil {
		return nil, err
	}
	next := newPublicRun(run.session, id, commandID, input, runstate.DeliveryNextTurn, runUsesSession)
	startNow := false
	run.session.mu.Lock()
	if run.session.closed {
		run.session.mu.Unlock()
		return nil, ErrSessionClosed
	}
	run.session.runs[id] = next
	if run.session.active == nil && !run.session.maintenance {
		startedAt := time.Now().UTC()
		next.markStarted(startedAt)
		run.session.active = next
		startNow = true
		if err := run.session.appendRecordLocked(ctx, turnStartedRecord, persistedTurn{
			RunID: next.id, CommandID: next.commandID, At: startedAt,
		}); err != nil {
			delete(run.session.runs, id)
			run.session.active = nil
			run.session.mu.Unlock()
			return nil, err
		}
	} else {
		run.session.pending = append(run.session.pending, next)
	}
	next.receipt = run.session.nextCommandCursorLocked()
	run.session.mu.Unlock()
	next.publish(RunAccepted{CommandID: commandID})
	if startNow {
		safeGo(next.execute, func(nextErr error) {
			next.finish(Result{Status: ResultFailed, Reason: nextErr.Error()}, nextErr)
		})
	}
	return next, nil
}

func (run *Run) Abort(ctx context.Context, request AbortRequest) (CommandReceipt, error) {
	if _, err := commandContext(ctx); err != nil {
		return CommandReceipt{}, err
	}
	if run == nil || run.session == nil {
		return CommandReceipt{}, ErrRunSettled
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	if run.isSettled() {
		return CommandReceipt{}, ErrRunSettled
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		reason = "Agent Run aborted"
	}
	if run.abortPending(reason) {
		cursor := run.session.nextCommandCursor()
		return CommandReceipt{CommandID: commandID, RunID: run.id, Cursor: cursor}, nil
	}
	run.setAbortReason(reason)
	select {
	case run.controls <- runstate.EngineControl{Kind: runstate.EngineControlAbort}:
	case <-run.done:
		return CommandReceipt{}, ErrRunSettled
	}
	return CommandReceipt{CommandID: commandID, RunID: run.id, Cursor: run.session.nextCommandCursor()}, nil
}

type QueuedInput struct {
	run  *Run
	item *queuedRunInput
}

func (queued *QueuedInput) ID() string {
	if queued == nil || queued.item == nil {
		return ""
	}
	return queued.item.id
}

func (queued *QueuedInput) Receipt() CommandReceipt {
	if queued == nil || queued.run == nil || queued.item == nil {
		return CommandReceipt{}
	}
	return CommandReceipt{CommandID: queued.item.id, RunID: queued.run.id, Cursor: queued.item.cursor}
}

func (queued *QueuedInput) Cancel(ctx context.Context, request QueueControlRequest) (CommandReceipt, error) {
	if _, err := commandContext(ctx); err != nil {
		return CommandReceipt{}, err
	}
	if queued == nil || queued.run == nil || queued.item == nil {
		return CommandReceipt{}, ErrRunSettled
	}
	if err := queued.run.usable(); err != nil {
		return CommandReceipt{}, err
	}
	queued.run.mu.Lock()
	queued.item.cancelled = true
	queued.run.mu.Unlock()
	id := strings.TrimSpace(request.IdempotencyKey)
	if id == "" {
		id = newPublicID("command")
	}
	return CommandReceipt{CommandID: id, RunID: queued.run.id, Cursor: queued.run.session.nextCommandCursor()}, nil
}

func (queued *QueuedInput) Interrupt(ctx context.Context, request QueueControlRequest) (CommandReceipt, error) {
	if _, err := commandContext(ctx); err != nil {
		return CommandReceipt{}, err
	}
	if queued == nil || queued.run == nil || queued.item == nil {
		return CommandReceipt{}, ErrRunSettled
	}
	if err := queued.run.usable(); err != nil {
		return CommandReceipt{}, err
	}
	queued.run.mu.Lock()
	for index, item := range queued.run.queue {
		if item != queued.item {
			continue
		}
		queued.run.queue = append([]*queuedRunInput{item}, append(queued.run.queue[:index], queued.run.queue[index+1:]...)...)
		item.delivery = runstate.DeliverySteer
		break
	}
	queued.run.mu.Unlock()
	select {
	case queued.run.controls <- runstate.EngineControl{Kind: runstate.EngineControlPreempt}:
	case <-queued.run.done:
		return CommandReceipt{}, ErrRunSettled
	}
	id := strings.TrimSpace(request.IdempotencyKey)
	if id == "" {
		id = newPublicID("command")
	}
	return CommandReceipt{CommandID: id, RunID: queued.run.id, Cursor: queued.run.session.nextCommandCursor()}, nil
}

func (run *Run) Respond(ctx context.Context, interactionID string, response InteractionResponse) error {
	ctx, err := commandContext(ctx)
	if err != nil {
		return err
	}
	if err := run.usable(); err != nil {
		return err
	}
	interactionID = strings.TrimSpace(interactionID)
	if interactionID == "" {
		return ErrInteractionStale
	}
	run.mu.RLock()
	pending, ok := run.interactions[interactionID]
	run.mu.RUnlock()
	if !ok {
		return ErrInteractionStale
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode Interaction response: %w", err)
	}
	resolver, ok := run.session.engine.(runstate.EngineInteractionResolver)
	if !ok {
		return ErrCapabilityUnsupported
	}
	snapshot, err := run.snapshotForCurrentCycle()
	if err != nil {
		return err
	}
	resolution, err := resolver.ResolveInteraction(ctx, runstate.InteractionResolveRequest{
		Snapshot: snapshot, Interaction: pending.snapshot, Response: encoded,
	})
	if err != nil {
		return err
	}
	run.mu.Lock()
	delete(run.interactions, interactionID)
	run.mu.Unlock()
	select {
	case run.controls <- runstate.EngineControl{
		Kind: runstate.EngineControlInteractionResolved, InteractionID: interactionID, Response: resolution,
	}:
		var publicResolution InteractionResolution
		if json.Unmarshal(resolution, &publicResolution) == nil {
			run.publish(InteractionResolved{ID: interactionID, Resolution: publicResolution})
		}
		return nil
	case <-run.done:
		return ErrRunSettled
	}
}

// commandContext makes cancellation an admission decision. Once a control has
// mutated Run state, the method reports the accepted command instead of an
// ambiguous cancellation result for work that may still execute.
func commandContext(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ctx, nil
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
		defer run.mu.RUnlock()
		return run.result, run.err
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (run *Run) usable() error {
	if run == nil || run.session == nil {
		return ErrRunSettled
	}
	if run.isSettled() {
		return ErrRunSettled
	}
	return run.session.usable()
}
