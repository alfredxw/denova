package agent

import (
	"context"
	"fmt"
	"runtime/debug"

	runstate "denova/internal/agent/runtime"
)

// bridgeHarnessControls owns the only goroutine introduced by this adapter.
// Its terminal error channel is always closed after one value, so Run can wait
// for complete cleanup without a timeout or a leaked blocked send.
func bridgeHarnessControls(
	ctx context.Context,
	source <-chan runstate.EngineControl,
	cancel context.CancelFunc,
) (<-chan RunControl, <-chan error) {
	if source == nil {
		done := make(chan error, 1)
		done <- nil
		close(done)
		return nil, done
	}

	// Match the durable coordinator control lane capacity so controls admitted
	// immediately after preparation cannot block before turnExecutor.Run attaches its
	// watcher on the next statement.
	destination := make(chan RunControl, 8)
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

func harnessRunControl(control runstate.EngineControl) (RunControl, error) {
	switch control.Kind {
	case runstate.EngineControlPreempt:
		return RunControl{Kind: RunControlPreempt, Reason: string(control.Kind)}, nil
	case runstate.EngineControlAbort:
		return RunControl{Kind: RunControlAbort, Reason: string(control.Kind)}, nil
	default:
		return RunControl{}, fmt.Errorf("unsupported agent harness engine control %q", control.Kind)
	}
}
