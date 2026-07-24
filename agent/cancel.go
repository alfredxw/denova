package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// AgentRunOption configures one Agent or Runner execution.
type AgentRunOption struct {
	apply func(*agentRunOptions)
}

type agentRunOptions struct {
	cancel *cancelControl
}

func collectAgentRunOptions(opts []AgentRunOption) *agentRunOptions {
	options := &agentRunOptions{}
	for _, opt := range opts {
		if opt.apply != nil {
			opt.apply(options)
		}
	}
	return options
}

// CancelMode selects an immediate or safe-point cancellation.
type CancelMode int

const (
	CancelImmediate      CancelMode = 0
	CancelAfterChatModel CancelMode = 1 << iota
	CancelAfterToolCalls
)

type agentCancelConfig struct {
	Mode    CancelMode
	Timeout *time.Duration
}

// AgentCancelOption configures one cancellation request.
type AgentCancelOption func(*agentCancelConfig)

// WithAgentCancelMode selects the requested cancellation point.
func WithAgentCancelMode(mode CancelMode) AgentCancelOption {
	return func(config *agentCancelConfig) {
		config.Mode = mode
	}
}

// WithAgentCancelTimeout explicitly escalates a safe-point request to
// immediate cancellation after timeout. There is no default timeout.
func WithAgentCancelTimeout(timeout time.Duration) AgentCancelOption {
	return func(config *agentCancelConfig) {
		config.Timeout = &timeout
	}
}

// CancelHandle waits for a cancellation request's outcome.
type CancelHandle struct {
	wait func() error
}

// Wait blocks until cancellation is handled or the execution ends.
func (handle *CancelHandle) Wait() error {
	if handle == nil || handle.wait == nil {
		return nil
	}
	return handle.wait()
}

// AgentCancelFunc is safe to call concurrently with Agent execution.
type AgentCancelFunc func(...AgentCancelOption) (*CancelHandle, bool)

// AgentCancelInfo describes the cancellation observed by the loop.
type AgentCancelInfo struct {
	Mode      CancelMode
	Escalated bool
	Timeout   bool
}

// CancelError is emitted through AgentEvent.Err when cancellation wins.
type CancelError struct {
	Info *AgentCancelInfo
}

func (err *CancelError) Error() string {
	if err == nil || err.Info == nil {
		return "agent canceled"
	}
	return fmt.Sprintf("agent canceled: mode=%v, escalated=%v", err.Info.Mode, err.Info.Escalated)
}

var (
	// ErrCancelTimeout is returned by CancelHandle.Wait after explicit escalation.
	ErrCancelTimeout = errors.New("cancel timed out")
	// ErrExecutionEnded means the run ended before a request took effect.
	ErrExecutionEnded = errors.New("execution already ended")
	// ErrStreamCanceled is sent through a public message/tool stream interrupted immediately.
	ErrStreamCanceled error = &StreamCanceledError{}
)

// StreamCanceledError is the concrete stream cancellation sentinel.
type StreamCanceledError struct{}

func (*StreamCanceledError) Error() string { return "stream canceled" }

func (*StreamCanceledError) Is(target error) bool {
	_, ok := target.(*StreamCanceledError)
	return ok
}

type cancelControl struct {
	mu sync.Mutex

	requested bool
	immediate bool
	mode      CancelMode
	escalated bool
	timedOut  bool

	cancel context.CancelFunc
	timer  *time.Timer

	terminal bool
	handled  bool
	result   error
	done     chan struct{}
}

// WithCancel creates a single-run cancellation option and controller.
func WithCancel() (AgentRunOption, AgentCancelFunc) {
	control := &cancelControl{done: make(chan struct{})}
	option := AgentRunOption{apply: func(options *agentRunOptions) {
		options.cancel = control
	}}
	return option, control.request
}

func (control *cancelControl) request(opts ...AgentCancelOption) (*CancelHandle, bool) {
	config := &agentCancelConfig{Mode: CancelImmediate}
	for _, opt := range opts {
		if opt != nil {
			opt(config)
		}
	}
	handle := &CancelHandle{wait: func() error {
		<-control.done
		control.mu.Lock()
		defer control.mu.Unlock()
		return control.result
	}}

	control.mu.Lock()
	if control.terminal || control.handled {
		control.mu.Unlock()
		return handle, false
	}
	control.requested = true
	if config.Mode == CancelImmediate {
		control.immediate = true
		if control.timer != nil {
			control.timer.Stop()
			control.timer = nil
		}
	} else {
		control.mode |= config.Mode
	}
	cancel := control.cancel
	shouldCancel := control.immediate
	if config.Timeout != nil && !control.immediate {
		if control.timer != nil {
			control.timer.Stop()
		}
		control.timer = time.AfterFunc(*config.Timeout, func() {
			defer func() { _ = recover() }()
			control.escalateTimeout()
		})
	}
	control.mu.Unlock()
	if shouldCancel && cancel != nil {
		cancel()
	}
	return handle, true
}

func (control *cancelControl) bind(cancel context.CancelFunc) {
	if control == nil {
		return
	}
	control.mu.Lock()
	control.cancel = cancel
	shouldCancel := control.requested && control.immediate && !control.terminal
	control.mu.Unlock()
	if shouldCancel {
		cancel()
	}
}

func (control *cancelControl) escalateTimeout() {
	control.mu.Lock()
	if control.terminal || control.handled || !control.requested {
		control.mu.Unlock()
		return
	}
	control.immediate = true
	control.escalated = true
	control.timedOut = true
	cancel := control.cancel
	control.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (control *cancelControl) safePoint(point CancelMode) *CancelError {
	if control == nil {
		return nil
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if !control.requested || control.terminal || control.handled {
		return nil
	}
	if !control.immediate && control.mode&point == 0 {
		return nil
	}
	return control.markHandledLocked()
}

func (control *cancelControl) immediateError() *CancelError {
	if control == nil {
		return nil
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if !control.requested || !control.immediate || control.terminal || control.handled {
		return nil
	}
	return control.markHandledLocked()
}

func (control *cancelControl) isImmediateRequested() bool {
	if control == nil {
		return false
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.requested && control.immediate && !control.terminal
}

func (control *cancelControl) markHandledLocked() *CancelError {
	control.handled = true
	control.terminal = true
	if control.timer != nil {
		control.timer.Stop()
	}
	if control.timedOut {
		control.result = ErrCancelTimeout
	}
	close(control.done)
	mode := control.mode
	if !control.escalated && control.immediate {
		mode = CancelImmediate
	}
	return &CancelError{Info: &AgentCancelInfo{
		Mode:      mode,
		Escalated: control.escalated,
		Timeout:   control.timedOut,
	}}
}

func (control *cancelControl) finish() {
	if control == nil {
		return
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.terminal {
		return
	}
	control.terminal = true
	if control.timer != nil {
		control.timer.Stop()
	}
	control.result = ErrExecutionEnded
	close(control.done)
}
