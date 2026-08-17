package agent

import (
	"context"
	"sync"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

// definitionEngineControls is the single control consumer for an Engine Run.
// It starts before any host preparation and is rebound to the native modelToolLoop once
// preparation succeeds. Keeping one consumer across that handoff closes the
// race where an accepted Steer or Abort used to sit unread while Source,
// Toolset, Context, Goal, or canonical input preparation was blocked.
type definitionEngineControls struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	ended  chan struct{}
	state  *engineControlState

	stopOnce sync.Once
	mu       sync.Mutex
	loop     cancelFunc
	client   *engineInteractionClient
}

func startDefinitionEngineControls(
	ctx context.Context,
	controls <-chan runstate.EngineControl,
) (context.Context, *definitionEngineControls) {
	if ctx == nil {
		ctx = context.Background()
	}
	workCtx, cancel := context.WithCancel(ctx)
	watcher := &definitionEngineControls{
		ctx: workCtx, cancel: cancel, done: make(chan struct{}), ended: make(chan struct{}),
		state: &engineControlState{},
	}
	var endedOnce sync.Once
	finish := func() { endedOnce.Do(func() { close(watcher.ended) }) }
	safeGo(func() {
		defer finish()
		watcher.watch(controls)
	}, func(err error) {
		watcher.state.fail(err)
		watcher.cancel()
		finish()
	})
	return workCtx, watcher
}

func (watcher *definitionEngineControls) watch(controls <-chan runstate.EngineControl) {
	for {
		select {
		case control, ok := <-controls:
			if !ok {
				return
			}
			watcher.handle(control)
		case <-watcher.done:
			return
		case <-watcher.ctx.Done():
			return
		}
	}
}

func (watcher *definitionEngineControls) handle(control runstate.EngineControl) {
	if watcher == nil {
		return
	}
	switch control.Kind {
	case runstate.EngineControlPreempt, runstate.EngineControlAbort:
		watcher.state.set(control.Kind)
		watcher.mu.Lock()
		cancelLoop := watcher.loop
		watcher.mu.Unlock()
		if cancelLoop == nil {
			// Preparation observes this derived context. Cancellation abandons only
			// the current attempt; the Session still owns any queued successor.
			watcher.cancel()
			return
		}
		mode := cancelAfterModel | cancelAfterTools
		if control.Kind == runstate.EngineControlAbort {
			mode = cancelImmediately
		}
		_, _ = cancelLoop(withCancelMode(mode))
	case runstate.EngineControlInteractionResolved:
		watcher.mu.Lock()
		client := watcher.client
		watcher.mu.Unlock()
		if client != nil {
			client.deliver(control.InteractionID, control.Response)
		}
	}
}

// bindLoop atomically transfers future controls to the native cancellation
// seam and delivers any interaction resolution that raced preparation. It
// returns a control already accepted before the handoff so the caller can
// settle without entering a modelToolLoop whose preparation context was cancelled.
func (watcher *definitionEngineControls) bindLoop(
	cancel cancelFunc,
	client *engineInteractionClient,
) runstate.EngineControlKind {
	if watcher == nil {
		return ""
	}
	watcher.mu.Lock()
	watcher.loop = cancel
	watcher.client = client
	watcher.mu.Unlock()
	return watcher.state.kind()
}

func (watcher *definitionEngineControls) stop() {
	if watcher == nil {
		return
	}
	watcher.stopOnce.Do(func() {
		close(watcher.done)
	})
	<-watcher.ended
}

func (watcher *definitionEngineControls) close() {
	if watcher == nil {
		return
	}
	watcher.stop()
	watcher.cancel()
}

func (watcher *definitionEngineControls) controlledPreparationResult(err error) (runstate.EngineResult, error, bool) {
	if watcher == nil {
		return runstate.EngineResult{}, err, false
	}
	if watcherErr := watcher.state.err(); watcherErr != nil {
		return runstate.EngineResult{}, watcherErr, true
	}
	switch watcher.state.kind() {
	case runstate.EngineControlPreempt:
		return runstate.EngineResult{Status: runstate.EnginePreempted}, nil, true
	case runstate.EngineControlAbort:
		return runstate.EngineResult{Status: runstate.EngineAborted}, nil, true
	default:
		// Parent lifecycle cancellation remains an ordinary cancellation. Only a
		// control accepted by this Run is translated into a terminal Engine status.
		return runstate.EngineResult{}, err, false
	}
}
