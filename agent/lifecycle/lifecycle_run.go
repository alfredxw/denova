package lifecycle

import (
	"context"
	"strings"
	"sync"
	"time"

	agentengine "github.com/alfredxw/denova/agent/engine"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentinteraction "github.com/alfredxw/denova/agent/lifecycle/interaction"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

const publicRunEventBuffer = 1024

type runSessionOwnership uint8

const (
	runUsesSession runSessionOwnership = iota
	runOwnsTemporarySession
)

type pendingInteraction struct {
	snapshot agentengine.InteractionSnapshot
	request  agentinteraction.InteractionRequest
}

// Run is an in-process execution handle. Its event stream and controls can be
// reattached while this process is alive; after restart the persisted Session
// transcript remains, while unfinished work is reported as interrupted.
type Run struct {
	session       *Session
	id            string
	commandID     string
	receipt       agentevent.Cursor
	input         agentschema.Input
	events        chan agentevent.Event
	done          chan struct{}
	executionDone chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	controls      chan agentengine.Control
	ownership     runSessionOwnership
	eventMu         sync.Mutex
	eventDrops      eventDropState
	lastEventCursor agentevent.Cursor
	eventsEnd       bool
	endOnce       sync.Once
	finishMu      sync.Mutex

	mu                   sync.RWMutex
	result               agentschema.Result
	err                  error
	settled              bool
	resuming             bool
	treeResumeID         string
	startedAt            time.Time
	finishedAt           time.Time
	abortReason          string
	suspendReason        string
	cycle                int
	snapshot             agentengine.TurnSnapshot
	delivery             agentengine.DeliveryKind
	interactions         map[string]pendingInteraction
	responses            map[string]persistedInteractionResponse
	tools                map[string]agentengine.PersistedTool
	content              strings.Builder
	thinking             strings.Builder
	modelResponseOrdinal int
	modelContentStart    int
	modelThinkingStart   int
	toolSources          map[string]agentevent.EventSource
	openTools            map[string]agentevent.OpenToolSnapshot
}

func newPublicRun(session *Session, id, commandID string, input agentschema.Input, delivery agentengine.DeliveryKind, ownership runSessionOwnership) *Run {
	ctx, cancel := context.WithCancel(session.agent.ctx)
	return &Run{
		session: session, id: id, commandID: commandID, input: cloneInput(input),
		events: make(chan agentevent.Event, publicRunEventBuffer), done: make(chan struct{}),
		executionDone: make(chan struct{}),
		ctx:           ctx, cancel: cancel, controls: make(chan agentengine.Control, 32), ownership: ownership,
		interactions: make(map[string]pendingInteraction), toolSources: make(map[string]agentevent.EventSource),
		responses: make(map[string]persistedInteractionResponse), tools: make(map[string]agentengine.PersistedTool),
		openTools: make(map[string]agentevent.OpenToolSnapshot),
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

func (run *Run) Receipt() agentevent.CommandReceipt {
	if run == nil {
		return agentevent.CommandReceipt{}
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	return agentevent.CommandReceipt{CommandID: run.commandID, RunID: run.id, Cursor: run.receipt}
}

func (run *Run) Events() <-chan agentevent.Event {
	if run == nil {
		closed := make(chan agentevent.Event)
		close(closed)
		return closed
	}
	return run.events
}

// EventCursor returns the latest live event cursor published for this Run.
// Hosts can use it to drain in-process observers up to a synchronous boundary.
func (run *Run) EventCursor() agentevent.Cursor {
	if run == nil {
		return 0
	}
	run.eventMu.Lock()
	defer run.eventMu.Unlock()
	return run.lastEventCursor
}

func (run *Run) Steer(ctx context.Context, input agentschema.Input) (agentevent.CommandReceipt, error) {
	if run == nil || run.session == nil {
		return agentevent.CommandReceipt{}, agentschema.ErrRunSettled
	}
	receipt, _, err := run.session.receiveInput(ctx, input, inputSteer, run.id, runUsesSession)
	return receipt, err
}

func (run *Run) Abort(ctx context.Context, request agentevent.AbortRequest) (agentevent.CommandReceipt, error) {
	if _, err := commandContext(ctx); err != nil {
		return agentevent.CommandReceipt{}, err
	}
	if run == nil || run.session == nil {
		return agentevent.CommandReceipt{}, agentschema.ErrRunSettled
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	if err := run.session.usable(); err != nil {
		return agentevent.CommandReceipt{}, err
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		reason = "Agent Run aborted"
	}
	run.session.mu.Lock()
	if run.session.closing {
		run.session.mu.Unlock()
		return agentevent.CommandReceipt{}, agentschema.ErrSessionClosed
	}
	// Accepted controls belong to the Session journal, even after the live
	// Run is retired. Revalidate the request before acknowledging its receipt.
	if _, found := run.session.controlReceipts[commandID]; found {
		receipt, err := run.session.acceptControlLocked(ctx, "abort", run.id, commandID, reason)
		run.session.mu.Unlock()
		return receipt, err
	}
	if run.session.runs[run.id] != run || run.isSettled() {
		run.session.mu.Unlock()
		return agentevent.CommandReceipt{}, agentschema.ErrRunSettled
	}
	receipt, err := run.session.acceptControlLocked(ctx, "abort", run.id, commandID, reason)
	run.session.mu.Unlock()
	if err != nil {
		return agentevent.CommandReceipt{}, err
	}
	if run.isSuspended() {
		run.finish(agentschema.Result{Status: agentschema.ResultAborted, Reason: reason}, nil)
		return receipt, nil
	}
	if run.abortPending(reason) {
		return receipt, nil
	}
	run.setAbortReason(reason)
	select {
	case run.controls <- agentengine.Control{Kind: agentengine.ControlAbort}:
	case <-run.done:
		return receipt, agentschema.ErrRunSettled
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

func (run *Run) Wait(ctx context.Context) (agentschema.Result, error) {
	if run == nil {
		return agentschema.Result{}, agentschema.ErrRunSettled
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
		return agentschema.Result{}, ctx.Err()
	}
}

func (run *Run) usable() error {
	if run == nil || run.session == nil {
		return agentschema.ErrRunSettled
	}
	if run.isSettled() {
		return agentschema.ErrRunSettled
	}
	run.session.mu.RLock()
	current := run.session.runs[run.id] == run
	run.session.mu.RUnlock()
	if !current {
		return agentschema.ErrRunSettled
	}
	return run.session.usable()
}
