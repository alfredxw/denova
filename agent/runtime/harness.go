package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// Harness is an actor-owned durable command lane. Its only public mutation
// seam is Submit; Observe combines a point-in-time projection with cursor replay.
type Harness struct {
	binding                BindingRef
	engine                 Engine
	journal                Journal
	observationBuffer      int
	inputLimits            InputLimits
	projectionTextMaxBytes int
	retainedEventLimit     int
	retainedMessageLimit   int
	retainedCommandLimit   int
	memoryLimits           BindingMemoryLimits
	requests               chan any
	done                   chan struct{}
	failureMu              sync.RWMutex
	failure                error
	closeMu                sync.Mutex
	closeCall              *harnessCloseCall
	lifecycle              context.Context
	lifecycleCancel        context.CancelFunc
}

type submitRequest struct {
	ctx      context.Context
	command  Command
	response chan submitResponse
}

type submitResponse struct {
	receipt Receipt
	err     error
}

type recoverAcceptedInputRequest struct {
	ctx      context.Context
	action   RecoveryAction
	response chan submitResponse
}

type recoveryInputRequest struct {
	ctx         context.Context
	commandID   CommandID
	operationID OperationID
	response    chan recoveryInputResponse
}

type recoveryInputResponse struct {
	input UserInput
	ok    bool
	err   error
}

type reconcileHostEffectsRequest struct {
	ctx      context.Context
	response chan error
}

type observeRequest struct {
	ctx      context.Context
	after    Cursor
	fromNow  bool
	response chan observeResponse
}

type observeResponse struct {
	observation Observation
	err         error
}

type statusRequest struct{ response chan statusResponse }

type statusResponse struct {
	snapshot StatusSnapshot
	err      error
}

type unsubscribeRequest struct{ id uint64 }

type closeRequest struct {
	response chan error
	// admitted closes after the actor has installed its durable closing fence.
	// Runtime shutdown waits for this boundary before canceling engine lifecycle
	// contexts, so an engine completion cannot start accepted successor work in
	// the gap between lifecycle cancellation and state.closing.
	admitted chan struct{}
}

type harnessCloseCall struct {
	ready chan struct{}
	err   error
}

type engineEventRequest struct {
	operation OperationID
	cycle     int
	event     EngineEvent
	response  chan error
}

type engineDoneRequest struct {
	operation OperationID
	cycle     int
	result    EngineResult
	err       error
}

// journalAppendError distinguishes a failed durability boundary from command
// validation and engine errors. Non-context append failures make the actor's
// in-memory cursor potentially stale (the write may have committed despite the
// returned error), so the only safe continuation is a lease release and replay.
type journalAppendError struct{ err error }

func (e *journalAppendError) Error() string { return "commit agent journal: " + e.err.Error() }
func (e *journalAppendError) Unwrap() error { return e.err }

func newHarness(
	ctx context.Context,
	lifecycle context.Context,
	binding BindingRef,
	engine Engine,
	j Journal,
	observationBuffer int,
	inputLimits InputLimits,
	projectionTextMaxBytes int,
	retainedEventLimit int,
	retainedMessageLimit int,
	retainedCommandLimit int,
	memoryLimits BindingMemoryLimits,
) (*Harness, error) {
	state := newHarnessState(binding)
	state.maxRetainedEvents = retainedEventLimit
	state.maxRetainedMessages = retainedMessageLimit
	state.memoryLimits = memoryLimits.normalized()
	if _, supportsColdLookup := j.(CommandJournalLookup); supportsColdLookup {
		state.maxRetainedCommands = retainedCommandLimit
	}
	if _, err := replayHarnessJournalState(ctx, j, &state); err != nil {
		return nil, fmt.Errorf("replay agent journal: %w", err)
	}
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	lifecycle, lifecycleCancel := context.WithCancel(lifecycle)
	h := &Harness{
		binding: binding.Clone(), engine: engine, journal: j,
		observationBuffer:      observationBuffer,
		inputLimits:            inputLimits.normalized(),
		projectionTextMaxBytes: projectionTextMaxBytes,
		retainedEventLimit:     retainedEventLimit,
		retainedMessageLimit:   retainedMessageLimit,
		retainedCommandLimit:   retainedCommandLimit,
		memoryLimits:           memoryLimits.normalized(),
		requests:               make(chan any, 64),
		done:                   make(chan struct{}),
		lifecycle:              lifecycle,
		lifecycleCancel:        lifecycleCancel,
	}
	// Host effects are fenced behind their exact cycle's output-domain receipt.
	// Unfinished canonical commits must therefore be reconciled before recovery
	// may transfer, acknowledge, or explicitly abandon the durable outbox.
	if err := h.recoverUnfinished(ctx, &state); err != nil {
		lifecycleCancel()
		return nil, err
	}
	go h.run(state)
	go h.closeOnLifecycleEnd()
	return h, nil
}

// Status returns a bounded point-in-time projection without subscribing or
// cloning the durable message timeline.
func (h *Harness) Status(ctx context.Context) (StatusSnapshot, error) {
	if h == nil {
		return StatusSnapshot{}, fmt.Errorf("agent harness is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.terminalError(); err != nil {
		return StatusSnapshot{}, err
	}
	request := statusRequest{response: make(chan statusResponse, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return StatusSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return StatusSnapshot{}, ctx.Err()
	}
	select {
	case response := <-request.response:
		return response.snapshot, response.err
	case <-h.done:
		return StatusSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return StatusSnapshot{}, ctx.Err()
	}
}

// Close aborts an active Engine through its typed control lane, waits for the
// durable terminal event, then releases the actor and all observers.
func (h *Harness) Close(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := h.terminalError(); err != nil {
			if errors.Is(err, ErrHarnessClosed) {
				return nil
			}
			return err
		}
		h.closeMu.Lock()
		pending := h.closeCall
		leader := pending == nil
		if leader {
			pending = &harnessCloseCall{ready: make(chan struct{})}
			h.closeCall = pending
		}
		h.closeMu.Unlock()
		if !leader {
			select {
			case <-pending.ready:
				if (errors.Is(pending.err, context.Canceled) || errors.Is(pending.err, context.DeadlineExceeded)) && ctx.Err() == nil {
					continue
				}
				return pending.err
			case <-h.done:
				return h.closeResult()
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := h.executeClose(ctx)
		h.closeMu.Lock()
		pending.err = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			h.closeCall = nil
		}
		close(pending.ready)
		h.closeMu.Unlock()
		return err
	}
}

func (h *Harness) executeClose(ctx context.Context) error {
	request := closeRequest{response: make(chan error, 1), admitted: make(chan struct{})}
	select {
	case h.requests <- request:
	case <-h.done:
		return h.closeResult()
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.response:
		return err
	case <-ctx.Done():
		// The close request remains actor-owned after admission. This caller may
		// stop waiting without withdrawing the durable abort/close fence; a later
		// caller can join the still-running close through a fresh waiter.
		return ctx.Err()
	case <-h.done:
		return h.closeResult()
	}
}

// admitCloseFence asks the actor to make shutdown ordering durable without
// waiting for an active engine to acknowledge Abort. Runtime.Close uses this
// two-phase boundary to fence every current binding before canceling the shared
// lifecycle that may independently wake engine completion goroutines.
func (h *Harness) admitCloseFence() (<-chan error, error) {
	if h == nil {
		return nil, nil
	}
	request := closeRequest{response: make(chan error, 1), admitted: make(chan struct{})}
	select {
	case h.requests <- request:
	case <-h.done:
		return nil, h.closeResult()
	}
	select {
	case <-request.admitted:
		return request.response, nil
	case <-h.done:
		return nil, h.closeResult()
	}
}

func (h *Harness) closeResult() error {
	err := h.terminalError()
	if errors.Is(err, ErrHarnessClosed) {
		return nil
	}
	return err
}

func (h *Harness) closeOnLifecycleEnd() {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("runtime: binding=%+v lifecycle watcher panic: %v", h.binding, recovered))
		}
	}()
	select {
	case <-h.lifecycle.Done():
		_ = h.Close(context.Background())
	case <-h.done:
	}
}

func (h *Harness) Submit(ctx context.Context, command Command) (Receipt, error) {
	if h == nil || command == nil {
		return Receipt{}, ErrInvalidCommand
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.terminalError(); err != nil {
		return Receipt{}, err
	}
	request := submitRequest{ctx: ctx, command: command, response: make(chan submitResponse, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return Receipt{}, h.terminalError()
	case <-ctx.Done():
		return Receipt{}, ctx.Err()
	}
	select {
	case response := <-request.response:
		return response.receipt, response.err
	case <-h.done:
		return Receipt{}, h.terminalError()
	}
}

// RecoverAcceptedInput resumes one already durable queued input from its safe
// identity. The caller cannot supply replacement input: the actor resolves the
// exact accepted UserInput (including its private restore descriptor) from the
// journal-owned state.
func (h *Harness) RecoverAcceptedInput(ctx context.Context, action RecoveryAction) (Receipt, error) {
	if h == nil {
		return Receipt{}, ErrInvalidCommand
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.terminalError(); err != nil {
		return Receipt{}, err
	}
	request := recoverAcceptedInputRequest{
		ctx: ctx, action: action, response: make(chan submitResponse, 1),
	}
	select {
	case h.requests <- request:
	case <-h.done:
		return Receipt{}, h.terminalError()
	case <-ctx.Done():
		return Receipt{}, ctx.Err()
	}
	select {
	case response := <-request.response:
		return response.receipt, response.err
	case <-h.done:
		return Receipt{}, h.terminalError()
	}
}

// RecoveryInput returns the exact server-owned input named by a current active,
// terminal, or queued recovery identity. It is an internal adapter seam for
// reconstructing display metadata; transport callers never supply or receive
// the private restore descriptor.
func (h *Harness) RecoveryInput(ctx context.Context, commandID CommandID, operationID OperationID) (UserInput, bool, error) {
	if h == nil {
		return UserInput{}, false, ErrInvalidCommand
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request := recoveryInputRequest{
		ctx: ctx, commandID: commandID, operationID: operationID,
		response: make(chan recoveryInputResponse, 1),
	}
	select {
	case h.requests <- request:
	case <-h.done:
		return UserInput{}, false, h.terminalError()
	case <-ctx.Done():
		return UserInput{}, false, ctx.Err()
	}
	select {
	case response := <-request.response:
		return response.input, response.ok, response.err
	case <-h.done:
		return UserInput{}, false, h.terminalError()
	case <-ctx.Done():
		return UserInput{}, false, ctx.Err()
	}
}

func (h *Harness) Observe(ctx context.Context, after Cursor) (Observation, error) {
	return h.observe(ctx, after, false)
}

// ObserveFromNow atomically snapshots current state and subscribes only to
// future events. It avoids replaying a long display timeline when a command
// producer attaches immediately before Submit.
func (h *Harness) ObserveFromNow(ctx context.Context) (Observation, error) {
	return h.observe(ctx, 0, true)
}

func (h *Harness) observe(ctx context.Context, after Cursor, fromNow bool) (Observation, error) {
	if h == nil {
		return Observation{}, fmt.Errorf("agent harness is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.terminalError(); err != nil {
		return Observation{}, err
	}
	request := observeRequest{ctx: ctx, after: after, fromNow: fromNow, response: make(chan observeResponse, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return Observation{}, h.terminalError()
	case <-ctx.Done():
		return Observation{}, ctx.Err()
	}
	select {
	case response := <-request.response:
		return response.observation, response.err
	case <-h.done:
		return Observation{}, h.terminalError()
	case <-ctx.Done():
		return Observation{}, ctx.Err()
	}
}

func (h *Harness) run(state harnessState) {
	defer func() {
		terminal := h.terminalError()
		if recovered := recover(); recovered != nil {
			terminal = fmt.Errorf("%w: actor panic: %v", ErrHarnessFailed, recovered)
			slog.ErrorContext(context.Background(), fmt.Sprintf("runtime: binding=%+v cursor=%d actor failed: %v", h.binding, state.cursor, terminal))
			h.setFailure(terminal)
		}
		if closeErr := safeJournalClose(h.journal); closeErr != nil {
			leaseErr := fmt.Errorf("%w: release journal binding lease: %v", ErrHarnessFailed, closeErr)
			if terminal == nil {
				terminal = leaseErr
			} else {
				terminal = errors.Join(terminal, leaseErr)
			}
			h.setFailure(terminal)
		}
		state.closeSubscribers(terminal)
		for _, waiter := range state.closeWaiters {
			waiter <- terminal
		}
		h.lifecycleCancel()
		close(h.done)
	}()
	for request := range h.requests {
		switch request := request.(type) {
		case submitRequest:
			var receipt Receipt
			var err error
			if state.closing {
				err = ErrHarnessClosed
			} else {
				receipt, err = h.handleSubmit(&state, request.ctx, request.command)
			}
			if terminal, fatal := terminalJournalAppendError(err); fatal {
				h.setFailure(terminal)
				request.response <- submitResponse{err: terminal}
				return
			}
			request.response <- submitResponse{receipt: receipt, err: err}
		case recoverAcceptedInputRequest:
			var receipt Receipt
			var err error
			if state.closing {
				err = ErrHarnessClosed
			} else {
				receipt, err = h.handleRecoverAcceptedInput(&state, request.ctx, request.action)
			}
			if terminal, fatal := terminalJournalAppendError(err); fatal {
				h.setFailure(terminal)
				request.response <- submitResponse{err: terminal}
				return
			}
			request.response <- submitResponse{receipt: receipt, err: err}
		case recoveryInputRequest:
			input, ok, err := h.handleRecoveryInput(&state, request.commandID, request.operationID)
			request.response <- recoveryInputResponse{input: input, ok: ok, err: err}
		case reconcileHostEffectsRequest:
			err := h.reconcilePendingHostEffects(request.ctx, &state)
			if terminal, fatal := terminalJournalAppendError(err); fatal {
				h.setFailure(terminal)
				request.response <- terminal
				return
			}
			request.response <- err
		case observeRequest:
			if state.closing {
				request.response <- observeResponse{err: ErrHarnessClosed}
				continue
			}
			after := request.after
			if request.fromNow {
				after = state.cursor
			}
			observation, err := h.handleObserve(&state, request.ctx, after)
			request.response <- observeResponse{observation: observation, err: err}
		case statusRequest:
			request.response <- statusResponse{snapshot: state.statusSnapshot(h.projectionTextMaxBytes)}
		case unsubscribeRequest:
			state.removeSubscriber(request.id, nil)
		case closeRequest:
			closeNow := func() (closeNow bool) {
				defer close(request.admitted)
				return h.handleClose(&state, request.response)
			}()
			if closeNow {
				return
			}
		case engineEventRequest:
			err := h.handleEngineEvent(&state, request)
			if terminal, fatal := terminalJournalAppendError(err); fatal {
				h.setFailure(terminal)
				request.response <- terminal
				return
			}
			request.response <- err
		case engineDoneRequest:
			h.handleEngineDone(&state, request)
			if state.closing && state.pendingEngineDone != nil && len(state.pendingHostEffects) > 0 {
				h.setFailure(fmt.Errorf("%w: runtime close left %d host effect(s) pending for cold reconciliation", ErrHostEffectRequired, len(state.pendingHostEffects)))
				return
			}
			if state.closing && state.phase == PhaseIdle {
				return
			}
		default:
			panic(fmt.Sprintf("unknown harness request %T", request))
		}
	}
}

func (h *Harness) setFailure(err error) {
	h.failureMu.Lock()
	h.failure = err
	h.failureMu.Unlock()
}

func (h *Harness) terminalError() error {
	h.failureMu.RLock()
	err := h.failure
	h.failureMu.RUnlock()
	if err != nil {
		return err
	}
	select {
	case <-h.done:
		return ErrHarnessClosed
	default:
		return nil
	}
}

func terminalJournalAppendError(err error) (error, bool) {
	var appendErr *journalAppendError
	if !errors.As(err, &appendErr) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, false
	}
	return fmt.Errorf("%w: %v", ErrHarnessFailed, err), true
}

// handleClose returns true when the actor can stop immediately.
func (h *Harness) handleClose(state *harnessState, response chan error) bool {
	state.closeWaiters = append(state.closeWaiters, response)
	if state.closing {
		return state.phase == PhaseIdle
	}
	state.closing = true
	if state.phase == PhaseIdle {
		return true
	}
	// Domain authorization is the actor's commit point. Once it wins ordering,
	// Close waits for its receipt and successful settlement instead of producing
	// an aborted runtime status for canonical state that may already be written.
	if state.outputCommitFinalizing() {
		return false
	}
	if !state.abortRequested {
		if _, err := h.commit(context.Background(), state, []EventPayload{
			AbortRequestedEvent{OperationID: state.activeOperation, Reason: "runtime lifecycle closed"},
		}); err != nil {
			if terminal, fatal := terminalJournalAppendError(err); fatal {
				err = terminal
			}
			h.setFailure(err)
			for _, waiter := range state.closeWaiters {
				waiter <- err
			}
			state.closeWaiters = nil
			return true
		}
	}
	state.sendControl(EngineControl{Kind: EngineControlAbort})
	if state.engineControls == nil && (state.phase == PhaseRunning || state.phase == PhaseCompacting) {
		if state.phase == PhaseRunning {
			if err := h.ensureInputMaterialized(h.lifecycle, state); err != nil {
				h.setFailure(fmt.Errorf("%w: close binding with pending accepted input: %v", ErrHarnessFailed, err))
				return true
			}
		}
		h.failActiveOperation(state, engineDoneRequest{
			operation: state.activeOperation,
			cycle:     state.activeCycle,
			result:    EngineResult{Status: EngineAborted},
		})
		return state.phase == PhaseIdle
	}
	return false
}
