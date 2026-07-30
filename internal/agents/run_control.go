package agents

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"
)

// RunControlKind is the closed set of controls accepted by one Agent run.
type RunControlKind string

const (
	RunControlPreempt RunControlKind = "preempt"
	RunControlAbort   RunControlKind = "abort"
)

// RunControl requests a lifecycle transition for the run attached to its channel.
// Closing the channel does not imply either control.
type RunControl struct {
	Kind   RunControlKind
	Reason string
}

// RunOutcomeStatus is the closed set of terminal states returned by a run.
type RunOutcomeStatus string

const (
	RunOutcomeCompleted RunOutcomeStatus = "completed"
	RunOutcomePreempted RunOutcomeStatus = "preempted"
	RunOutcomeAborted   RunOutcomeStatus = "aborted"
	RunOutcomeFailed    RunOutcomeStatus = "failed"
)

// RunOutcome is the transport-independent terminal result of one Agent run.
// Error is reserved for failed runs and context cancellation; intentional
// preempt/abort controls carry their caller-provided explanation in Reason.
type RunOutcome struct {
	Status   RunOutcomeStatus
	Error    error
	Reason   string
	Content  string
	Thinking string
	// MaintenanceOnly means the model call was intentionally deferred after a
	// valid structural checkpoint was staged. The accepted input remains
	// pending and no assistant/domain output is expected from this cycle.
	MaintenanceOnly bool
}

func outcomeFromOutput(status RunOutcomeStatus, err error, reason, content, thinking string) RunOutcome {
	return RunOutcome{
		Status:   status,
		Error:    err,
		Reason:   reason,
		Content:  content,
		Thinking: thinking,
	}
}

// runControlState serializes cancellation requests with terminal
// classification. The AgentCancelFunc contribution bit is the source of truth:
// merely receiving a control does not mean it caused the eventual CancelError.
type runControlState struct {
	mu                  sync.Mutex
	triggered           RunControl
	protocolContributed bool
}

func (s *runControlState) request(control RunControl, cancel agent.AgentCancelFunc) {
	if s == nil || cancel == nil {
		return
	}
	control.Reason = strings.TrimSpace(control.Reason)
	if control.Kind != RunControlPreempt && control.Kind != RunControlAbort {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.triggered.Kind == RunControlAbort || s.triggered.Kind == control.Kind {
		return
	}

	var options []agent.AgentCancelOption
	switch control.Kind {
	case RunControlPreempt:
		options = append(options, agent.WithAgentCancelMode(agent.CancelAfterChatModel|agent.CancelAfterToolCalls))
	case RunControlAbort:
		options = append(options, agent.WithAgentCancelMode(agent.CancelImmediate))
	}
	_, contributed := cancel(options...)
	slog.Info("agent run control cancellation requested", "control", control.Kind, "contributed", contributed)
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

func (s *runControlState) controlForCancel(err error) (RunControl, bool) {
	if s == nil || err == nil {
		return RunControl{}, false
	}
	var cancelErr *agent.CancelError
	if !errors.As(err, &cancelErr) || cancelErr == nil {
		return RunControl{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.triggered.Kind != RunControlPreempt && s.triggered.Kind != RunControlAbort {
		return RunControl{}, false
	}
	return s.triggered, true
}

func (s *runControlState) suppressesStreamCanceledError(event Event) bool {
	if s == nil || event.Type != "error" || eventDataString(event.Data, "message") != agent.ErrStreamCanceled.Error() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.triggered.Kind == RunControlAbort
}

func (s *runControlState) hasTriggeredControl() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.triggered.Kind == RunControlPreempt || s.triggered.Kind == RunControlAbort
}

func startRunControlWatcher(ctx context.Context, controls <-chan RunControl, cancel agent.AgentCancelFunc, state *runControlState) <-chan struct{} {
	done := make(chan struct{})
	if controls == nil || cancel == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("agent run control watcher panic recovered", "error", recovered, "stack", string(debug.Stack()))
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
				state.request(control, cancel)
			}
		}
	}()
	return done
}
