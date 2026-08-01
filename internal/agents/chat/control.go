package chat

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/run"
)

// runControlState serializes cancellation requests with terminal
// classification. The AgentCancelFunc contribution bit is the source of truth:
// merely receiving a control does not mean it caused the eventual CancelError.
type runControlState struct {
	mu                  sync.Mutex
	triggered           agentrun.Control
	protocolContributed bool
}

func (s *runControlState) request(ctx context.Context, control agentrun.Control, cancel agent.AgentCancelFunc) {
	if s == nil || cancel == nil {
		return
	}
	control.Reason = strings.TrimSpace(control.Reason)
	if control.Kind != agentrun.ControlPreempt && control.Kind != agentrun.ControlAbort {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.triggered.Kind == agentrun.ControlAbort || s.triggered.Kind == control.Kind {
		return
	}

	var options []agent.AgentCancelOption
	switch control.Kind {
	case agentrun.ControlPreempt:
		options = append(options, agent.WithAgentCancelMode(agent.CancelAfterChatModel|agent.CancelAfterToolCalls))
	case agentrun.ControlAbort:
		options = append(options, agent.WithAgentCancelMode(agent.CancelImmediate))
	}
	_, contributed := cancel(options...)
	slog.InfoContext(ctx, "agent run control cancellation requested", "control", control.Kind, "contributed", contributed)
	if !contributed {
		return
	}
	if control.Reason == "" {
		control.Reason = string(control.Kind)
	}
	s.triggered = control
}

func (s *runControlState) wrapProtocolCancel(cancel agent.AgentCancelFunc) agent.AgentCancelFunc {
	if s == nil || cancel == nil {
		return cancel
	}
	return func(options ...agent.AgentCancelOption) (*agent.CancelHandle, bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		handle, contributed := cancel(options...)
		if contributed {
			s.protocolContributed = true
		}
		return handle, contributed
	}
}

func (s *runControlState) protocolTriggered() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.protocolContributed
}

func (s *runControlState) controlForCancel(err error) (agentrun.Control, bool) {
	if s == nil || err == nil {
		return agentrun.Control{}, false
	}
	var cancelErr *agent.CancelError
	if !errors.As(err, &cancelErr) || cancelErr == nil {
		return agentrun.Control{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.triggered.Kind != agentrun.ControlPreempt && s.triggered.Kind != agentrun.ControlAbort {
		return agentrun.Control{}, false
	}
	return s.triggered, true
}

func (s *runControlState) suppressesStreamCanceledError(event agentrun.Event) bool {
	if s == nil || event.Type != "error" || eventDataString(event.Data, "message") != agent.ErrStreamCanceled.Error() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.triggered.Kind == agentrun.ControlAbort
}

func (s *runControlState) hasTriggeredControl() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.triggered.Kind == agentrun.ControlPreempt || s.triggered.Kind == agentrun.ControlAbort
}

func startRunControlWatcher(ctx context.Context, controls <-chan agentrun.Control, cancel agent.AgentCancelFunc, state *runControlState) <-chan struct{} {
	done := make(chan struct{})
	if controls == nil || cancel == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, "agent run control watcher panic recovered", "error", recovered, "stack", string(debug.Stack()))
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case control, ok := <-controls:
				if !ok {
					return
				}
				state.request(ctx, control, cancel)
			}
		}
	}()
	return done
}
