package harness

import (
	"context"
	"denova/internal/agents/run"
	"fmt"
	"runtime/debug"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func prepareHarnessCycle(ctx context.Context, preparer agentrun.CyclePreparer) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("prepare agent harness cycle panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	if err := preparer.PrepareAgentCycle(ctx); err != nil {
		return fmt.Errorf("prepare agent harness cycle: %w", err)
	}
	return nil
}

type harnessPreparationControlResult struct {
	control *runstate.EngineControl
	err     error
}

func prepareHarnessCycleWithControls(
	ctx context.Context,
	controls <-chan runstate.EngineControl,
	preparer agentrun.CyclePreparer,
) (*runstate.EngineControl, error) {
	if controls == nil {
		return nil, prepareHarnessCycle(ctx, preparer)
	}
	prepareCtx, cancel := context.WithCancel(ctx)
	controlDone := watchHarnessPreparationControl(prepareCtx, controls, cancel)
	prepareErr := prepareHarnessCycle(prepareCtx, preparer)
	cancel()
	controlResult := <-controlDone
	if controlResult.err != nil {
		return nil, controlResult.err
	}
	return controlResult.control, prepareErr
}

// watchHarnessPreparationControl consumes at most one coordinator control. A
// control cancels the preparer directly instead of being forwarded to the
// legacy chat.Executor.Run channel, which has no consumer until preparation succeeds.
func watchHarnessPreparationControl(
	ctx context.Context,
	controls <-chan runstate.EngineControl,
	cancel context.CancelFunc,
) <-chan harnessPreparationControlResult {
	done := make(chan harnessPreparationControlResult, 1)
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				if cancel != nil {
					cancel()
				}
				done <- harnessPreparationControlResult{err: fmt.Errorf(
					"agent harness preparation control watcher panic: %v\n%s", recovered, debug.Stack(),
				)}
			}
		}()

		control, ok := receiveHarnessPreparationControl(ctx, controls)
		if !ok {
			done <- harnessPreparationControlResult{}
			return
		}
		if _, err := harnessRunControl(control); err != nil {
			if cancel != nil {
				cancel()
			}
			done <- harnessPreparationControlResult{err: err}
			return
		}
		if cancel != nil {
			cancel()
		}
		done <- harnessPreparationControlResult{control: &control}
	}()
	return done
}

func receiveHarnessPreparationControl(
	ctx context.Context,
	controls <-chan runstate.EngineControl,
) (runstate.EngineControl, bool) {
	select {
	case control, ok := <-controls:
		return control, ok
	default:
	}
	select {
	case control, ok := <-controls:
		return control, ok
	case <-ctx.Done():
		// Prefer a control already admitted concurrently with cancellation. This
		// keeps an accepted Steer/Abort authoritative at the preparation boundary.
		select {
		case control, ok := <-controls:
			return control, ok
		default:
			return runstate.EngineControl{}, false
		}
	}
}

func harnessPreparationControlOutcome(control runstate.EngineControl) (agentrun.Outcome, error) {
	switch control.Kind {
	case runstate.EngineControlPreempt:
		return agentrun.NewOutcome(agentrun.OutcomePreempted, nil, string(control.Kind), "", ""), nil
	case runstate.EngineControlAbort:
		return agentrun.NewOutcome(agentrun.OutcomeAborted, nil, string(control.Kind), "", ""), nil
	default:
		return agentrun.Outcome{}, fmt.Errorf("unsupported agent harness preparation control %q", control.Kind)
	}
}
