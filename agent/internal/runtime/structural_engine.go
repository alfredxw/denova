package runtime

import (
	"context"
	"fmt"
	"log/slog"
)

func (h *Harness) startStructuralEngine(state *harnessState, snapshot StructuralOperationSnapshot) {
	snapshot = *cloneStructuralOperationSnapshot(&snapshot)
	engine, ok := h.engine.(StructuralEngine)
	if !ok {
		panic("structural engine capability disappeared after command admission")
	}
	controls := make(chan EngineControl, 8)
	state.engineControls = controls
	operationCtx, cancel := context.WithCancel(h.lifecycle)
	forwarded := make(chan EngineControl, 8)
	go forwardStructuralControls(operationCtx, controls, forwarded, cancel, h.binding, snapshot.OperationID)
	request := StructuralEngineRequest{
		Binding: h.binding.Clone(), Snapshot: *cloneStructuralOperationSnapshot(&snapshot),
		State: cloneRawMessage(state.engineState), Capabilities: cloneCapabilityStates(state.capabilityStates),
		Controls: forwarded,
	}
	go func() {
		returned := false
		defer func() {
			cancel()
			if recovered := recover(); recovered != nil {
				h.postEngineDone(engineDoneRequest{
					operation: snapshot.OperationID, cycle: snapshot.Cycle,
					err: fmt.Errorf("agent structural engine panic: %v", recovered),
				})
				return
			}
			if !returned {
				slog.InfoContext(context.Background(), fmt.Sprintf("runtime: binding=%+v operation=%s structural engine exited without a result", h.binding, snapshot.OperationID))
			}
		}()
		result, err := engine.RunStructural(operationCtx, request, func(event EngineEvent) error {
			response := make(chan error, 1)
			engineRequest := engineEventRequest{
				operation: snapshot.OperationID, cycle: snapshot.Cycle,
				event: event, response: response,
			}
			select {
			case h.requests <- engineRequest:
			case <-h.done:
				return h.terminalError()
			}
			select {
			case err := <-response:
				return err
			case <-h.done:
				return h.terminalError()
			}
		})
		returned = true
		h.postEngineDone(engineDoneRequest{operation: snapshot.OperationID, cycle: snapshot.Cycle, result: result, err: err})
	}()
}

func forwardStructuralControls(
	ctx context.Context,
	input <-chan EngineControl,
	output chan<- EngineControl,
	cancel context.CancelFunc,
	binding BindingRef,
	operationID OperationID,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("runtime: binding=%+v operation=%s structural control bridge panic: %v", binding, operationID, recovered))
			cancel()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case control, ok := <-input:
			if !ok {
				return
			}
			select {
			case output <- control:
			default:
			}
			if control.Kind == EngineControlAbort {
				cancel()
				return
			}
		}
	}
}
