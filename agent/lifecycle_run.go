package agent

import (
	"context"
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

type pendingInteraction struct {
	snapshot runstate.InteractionSnapshot
	request  InteractionRequest
}

// Run is an in-process execution handle. Its event stream and controls can be
// reattached while this process is alive; after restart the persisted Session
// transcript remains, while unfinished work is reported as interrupted.
type Run struct {
	session       *Session
	id            string
	commandID     string
	receipt       Cursor
	input         Input
	events        chan Event
	done          chan struct{}
	executionDone chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	controls      chan runstate.EngineControl
	ownership     runSessionOwnership
	eventMu       sync.Mutex
	eventDrops    eventDropState
	eventsEnd     bool
	endOnce       sync.Once
	finishMu      sync.Mutex

	mu                   sync.RWMutex
	result               Result
	err                  error
	settled              bool
	resuming             bool
	treeResumeID         string
	startedAt            time.Time
	abortReason          string
	suspendReason        string
	cycle                int
	snapshot             runstate.TurnSnapshot
	delivery             runstate.DeliveryKind
	interactions         map[string]pendingInteraction
	responses            map[string]persistedInteractionResponse
	tools                map[string]persistedTool
	content              strings.Builder
	thinking             strings.Builder
	modelResponseOrdinal int
	modelContentStart    int
	modelThinkingStart   int
	toolSources          map[string]EventSource
	openTools            map[string]OpenToolSnapshot
}

func newPublicRun(session *Session, id, commandID string, input Input, delivery runstate.DeliveryKind, ownership runSessionOwnership) *Run {
	ctx, cancel := context.WithCancel(session.agent.ctx)
	return &Run{
		session: session, id: id, commandID: commandID, input: cloneInput(input),
		events: make(chan Event, publicRunEventBuffer), done: make(chan struct{}),
		executionDone: make(chan struct{}),
		ctx:           ctx, cancel: cancel, controls: make(chan runstate.EngineControl, 32), ownership: ownership,
		interactions: make(map[string]pendingInteraction), toolSources: make(map[string]EventSource),
		responses: make(map[string]persistedInteractionResponse), tools: make(map[string]persistedTool),
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
	if err := run.usable(); err != nil {
		return CommandReceipt{}, err
	}
	receipt, _, err := run.session.receiveInput(ctx, input, inputSteer, run.id, runUsesSession)
	return receipt, err
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
	if err := run.session.usable(); err != nil {
		return CommandReceipt{}, err
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		reason = "Agent Run aborted"
	}
	run.session.mu.Lock()
	if run.session.closing {
		run.session.mu.Unlock()
		return CommandReceipt{}, ErrSessionClosed
	}
	if run.session.runs[run.id] != run {
		run.session.mu.Unlock()
		return CommandReceipt{}, ErrRunSettled
	}
	if run.isSettled() {
		if _, found := run.session.controlReceipts[commandID]; !found {
			run.session.mu.Unlock()
			return CommandReceipt{}, ErrRunSettled
		}
		receipt, err := run.session.acceptControlLocked(ctx, "abort", run.id, commandID, reason)
		run.session.mu.Unlock()
		return receipt, err
	}
	receipt, err := run.session.acceptControlLocked(ctx, "abort", run.id, commandID, reason)
	run.session.mu.Unlock()
	if err != nil {
		return CommandReceipt{}, err
	}
	if run.isSuspended() {
		run.finish(Result{Status: ResultAborted, Reason: reason}, nil)
		return receipt, nil
	}
	if run.abortPending(reason) {
		return receipt, nil
	}
	run.setAbortReason(reason)
	select {
	case run.controls <- runstate.EngineControl{Kind: runstate.EngineControlAbort}:
	case <-run.done:
		return CommandReceipt{}, ErrRunSettled
	}
	return receipt, nil
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
	run.session.mu.RLock()
	current := run.session.runs[run.id] == run
	run.session.mu.RUnlock()
	if !current {
		return ErrRunSettled
	}
	return run.session.usable()
}
