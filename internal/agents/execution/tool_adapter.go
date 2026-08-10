package execution

import (
	"context"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// displayEngineEvent maps display deltas only. Tool execution events
// deliberately come from toolLifecycleObserver instead of legacy UI
// events, because recovery requires the start record before the real effect.
func displayEngineEvent(event agentrun.Event) (runstate.EngineEvent, bool) {
	switch event.Type {
	case "chunk":
		return runstate.EngineAssistantDelta{Delta: event.DataString("content")}, true
	case "thinking":
		return runstate.EngineThinkingDelta{Delta: event.DataString("content")}, true
	default:
		return nil, false
	}
}

type toolLifecycleObserver struct {
	sink        *engineEventSink
	binding     runstate.BindingRef
	operationID runstate.OperationID
	cycle       int
	options     agentrun.Options
}

func (o toolLifecycleObserver) BeforeTool(_ context.Context, decision agenttool.Decision, arguments string) error {
	if o.sink == nil {
		return fmt.Errorf("agent execution tool lifecycle sink is nil")
	}
	return o.sink.send(runstate.EngineToolStarted{
		CallID:    decision.ExecutionID,
		Name:      decision.ToolName,
		Arguments: json.RawMessage(append([]byte(nil), arguments...)),
	})
}

func (o toolLifecycleObserver) AfterTool(_ context.Context, record agenttool.ExecutionRecord) error {
	if o.sink == nil {
		return fmt.Errorf("agent execution tool lifecycle sink is nil")
	}
	effect, hasEffect, err := agenttoolruntime.NewCommittedToolMutationHostEffect(o.binding, o.operationID, o.cycle, record, o.options)
	if err != nil {
		return err
	}
	finished := runstate.EngineToolFinished{
		CallID:      record.ExecutionID,
		Name:        record.ToolName,
		Result:      record.Result,
		IsError:     !strings.EqualFold(strings.TrimSpace(record.Status), "success"),
		RetrySafety: toolRetrySafety(record),
	}
	if hasEffect {
		finished.HostEffects = []runstate.HostEffect{effect}
	}
	if err := o.sink.send(finished); err != nil {
		return err
	}
	// Runtime owns the durable effect from this point. It may reconcile and ack
	// only after the same operation/cycle has an exact output-domain receipt;
	// a failed output commit durably abandons the effect without host delivery.
	return nil
}

func toolRetrySafety(record agenttool.ExecutionRecord) runstate.RetrySafety {
	switch record.Descriptor.Recovery {
	case agenttool.ToolRecoveryReadOnly, agenttool.ToolRecoveryIdempotent:
		return runstate.RetrySafe
	case agenttool.ToolRecoveryReconcilable:
		// Reconciliation can determine the effect, but blind retry is not safe.
		return runstate.RetryUnknown
	case agenttool.ToolRecoveryNonIdempotent:
		return runstate.RetryUnsafe
	default:
		return runstate.RetryUnknown
	}
}

type engineEventSink struct {
	mu     sync.Mutex
	emit   runstate.EngineEventSink
	cancel context.CancelFunc
	err    error
}

// send serializes legacy deltas and potentially parallel tool callbacks. The
// first sink failure cancels the Agent run and remains the terminal adapter
// error; later cleanup events must not obscure it.
func (s *engineEventSink) send(event runstate.EngineEvent) error {
	if s == nil {
		return fmt.Errorf("agent execution engine sink is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if s.emit == nil {
		s.err = fmt.Errorf("agent execution engine event sink is nil")
	} else if err := s.emit(event); err != nil {
		s.err = fmt.Errorf("emit agent execution engine event %T: %w", event, err)
	}
	if s.err != nil && s.cancel != nil {
		s.cancel()
	}
	return s.err
}

func (s *engineEventSink) failure() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

var _ agenttoolruntime.ToolLifecycleObserver = toolLifecycleObserver{}
