package runtime

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

// EngineScript is a deterministic engine boundary used by integration tests.
// Events are emitted first, then the script optionally waits for a release or
// control signal. LoseProcess simulates a worker disappearing without a result.
type EngineScript struct {
	Events         []EngineEvent
	Continue       <-chan struct{}
	WaitForControl EngineControlKind
	Result         EngineResult
	Err            error
	LoseProcess    bool
}

type ScriptedEngine struct {
	mu       sync.Mutex
	scripts  []EngineScript
	requests []EngineRequest
}

func NewScriptedEngine(scripts ...EngineScript) *ScriptedEngine {
	return &ScriptedEngine{scripts: append([]EngineScript(nil), scripts...)}
}

func (e *ScriptedEngine) NewEngine(context.Context, BindingRef) (Engine, error) {
	if e == nil {
		return nil, fmt.Errorf("scripted engine is nil")
	}
	return e, nil
}

func (e *ScriptedEngine) Run(ctx context.Context, request EngineRequest, emit EngineEventSink) (EngineResult, error) {
	e.mu.Lock()
	if len(e.scripts) == 0 {
		e.mu.Unlock()
		return EngineResult{}, fmt.Errorf("scripted engine has no remaining script")
	}
	script := e.scripts[0]
	e.scripts = e.scripts[1:]
	e.requests = append(e.requests, cloneEngineRequest(request))
	e.mu.Unlock()

	for _, event := range script.Events {
		select {
		case <-ctx.Done():
			return EngineResult{Status: EngineAborted}, ctx.Err()
		default:
		}
		if err := emit(event); err != nil {
			return EngineResult{}, err
		}
	}
	if script.LoseProcess {
		runtime.Goexit()
	}
	if script.Continue != nil {
		select {
		case <-script.Continue:
		case <-ctx.Done():
			return EngineResult{Status: EngineAborted}, ctx.Err()
		}
	}
	if script.WaitForControl != "" {
		for {
			select {
			case control := <-request.Controls:
				if control.Kind != script.WaitForControl {
					continue
				}
				switch control.Kind {
				case EngineControlPreempt:
					return EngineResult{Status: EnginePreempted}, script.Err
				case EngineControlAbort:
					return EngineResult{Status: EngineAborted}, script.Err
				default:
					return EngineResult{}, fmt.Errorf("unsupported scripted control %q", control.Kind)
				}
			case <-ctx.Done():
				return EngineResult{Status: EngineAborted}, ctx.Err()
			}
		}
	}
	if script.Result.Status == "" {
		script.Result.Status = EngineCompleted
	}
	return script.Result, script.Err
}

func (e *ScriptedEngine) Requests() []EngineRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	requests := make([]EngineRequest, len(e.requests))
	for index, request := range e.requests {
		requests[index] = cloneEngineRequest(request)
	}
	return requests
}

func cloneEngineRequest(request EngineRequest) EngineRequest {
	request.Snapshot.Input = cloneUserInput(request.Snapshot.Input)
	request.Controls = nil
	return request
}
