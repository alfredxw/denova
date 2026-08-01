package harness

import (
	"context"
	"denova/internal/agents/run"
	"fmt"
	"runtime/debug"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// bridgeHarnessControls owns the only goroutine introduced by this adapter.
// Its terminal error channel is always closed after one value, so Run can wait
// for complete cleanup without a timeout or a leaked blocked send.
func bridgeHarnessControls(
	ctx context.Context,
	source <-chan runstate.EngineControl,
	cancel context.CancelFunc,
) (<-chan agentrun.Control, <-chan error) {
	if source == nil {
		done := make(chan error, 1)
		done <- nil
		close(done)
		return nil, done
	}

	// Match the durable coordinator control lane capacity so controls admitted
	// immediately after preparation cannot block before chat.Executor.Run attaches its
	// watcher on the next statement.
	destination := make(chan agentrun.Control, 8)
	done := make(chan error, 1)
	go func() {
		defer close(destination)
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("agent harness control bridge panic: %v\n%s", recovered, debug.Stack())
				if cancel != nil {
					cancel()
				}
				done <- err
			}
		}()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case control, ok := <-source:
				if !ok {
					done <- nil
					return
				}
				mapped, err := harnessRunControl(control)
				if err != nil {
					if cancel != nil {
						cancel()
					}
					done <- err
					return
				}
				select {
				case destination <- mapped:
				case <-ctx.Done():
					done <- nil
					return
				}
			}
		}
	}()
	return destination, done
}

func harnessRunControl(control runstate.EngineControl) (agentrun.Control, error) {
	switch control.Kind {
	case runstate.EngineControlPreempt:
		return agentrun.Control{Kind: agentrun.ControlPreempt, Reason: string(control.Kind)}, nil
	case runstate.EngineControlAbort:
		return agentrun.Control{Kind: agentrun.ControlAbort, Reason: string(control.Kind)}, nil
	default:
		return agentrun.Control{}, fmt.Errorf("unsupported agent harness engine control %q", control.Kind)
	}
}
