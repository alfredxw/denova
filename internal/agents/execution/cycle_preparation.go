package execution

import (
	"context"
	"denova/internal/agents/run"
	"fmt"
	"runtime/debug"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func prepareCycle(ctx context.Context, preparer agentrun.CyclePreparer) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("prepare agent execution cycle panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	if err := preparer.PrepareAgentCycle(ctx); err != nil {
		return fmt.Errorf("prepare agent execution cycle: %w", err)
	}
	return nil
}

type cyclePreparationControlResult struct {
	control *runstate.EngineControl
	err     error
}

func prepareCycleWithControls(
	ctx context.Context,
	controls <-chan runstate.EngineControl,
	preparer agentrun.CyclePreparer,
) (*runstate.EngineControl, error) {
	if controls == nil {
		return nil, prepareCycle(ctx, preparer)
	}
	prepareCtx, cancel := context.WithCancel(ctx)
	controlDone := watchCyclePreparationControl(prepareCtx, controls, cancel)
	prepareErr := prepareCycle(prepareCtx, preparer)
	cancel()
	controlResult := <-controlDone
	if controlResult.err != nil {
		return nil, controlResult.err
	}
	return controlResult.control, prepareErr
}

// watchCyclePreparationControl consumes at most one coordinator control. A
// control cancels the preparer directly instead of being forwarded to the
// legacy chat.Executor.Run channel, which has no consumer until preparation succeeds.
func watchCyclePreparationControl(
	ctx context.Context,
	controls <-chan runstate.EngineControl,
	cancel context.CancelFunc,
) <-chan cyclePreparationControlResult {
	done := make(chan cyclePreparationControlResult, 1)
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				if cancel != nil {
					cancel()
				}
				done <- cyclePreparationControlResult{err: fmt.Errorf(
					"agent execution preparation control watcher panic: %v\n%s", recovered, debug.Stack(),
				)}
			}
		}()

		control, ok := receiveCyclePreparationControl(ctx, controls)
		if !ok {
			done <- cyclePreparationControlResult{}
			return
		}
		if _, err := engineRunControl(control); err != nil {
			if cancel != nil {
				cancel()
			}
			done <- cyclePreparationControlResult{err: err}
			return
		}
		if cancel != nil {
			cancel()
		}
		done <- cyclePreparationControlResult{control: &control}
	}()
	return done
}

func receiveCyclePreparationControl(
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

func cyclePreparationControlOutcome(control runstate.EngineControl) (agentrun.Outcome, error) {
	switch control.Kind {
	case runstate.EngineControlPreempt:
		return agentrun.NewOutcome(agentrun.OutcomePreempted, nil, string(control.Kind), "", ""), nil
	case runstate.EngineControlAbort:
		return agentrun.NewOutcome(agentrun.OutcomeAborted, nil, string(control.Kind), "", ""), nil
	default:
		return agentrun.Outcome{}, fmt.Errorf("unsupported agent execution preparation control %q", control.Kind)
	}
}
