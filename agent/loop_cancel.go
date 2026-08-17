package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// loopRunOption configures one Agent or loopRunner execution.
type loopRunOption struct {
	apply func(*agentRunOptions)
}

type agentRunOptions struct {
	cancel *cancelControl
}

func collectLoopRunOptions(opts []loopRunOption) *agentRunOptions {
	options := &agentRunOptions{}
	for _, opt := range opts {
		if opt.apply != nil {
			opt.apply(options)
		}
	}
	return options
}

// cancelMode selects an immediate or safe-point cancellation.
type cancelMode int

const (
	cancelImmediately cancelMode = 0
	cancelAfterModel  cancelMode = 1 << iota
	cancelAfterTools
)

type agentCancelConfig struct {
	Mode    cancelMode
	Timeout *time.Duration
}

// cancelRequestOption configures one cancellation request.
type cancelRequestOption func(*agentCancelConfig)

// withCancelMode selects the requested cancellation point.
func withCancelMode(mode cancelMode) cancelRequestOption {
	return func(config *agentCancelConfig) {
		config.Mode = mode
	}
}

// withCancelTimeout explicitly escalates a safe-point request to
// immediate cancellation after timeout. There is no default timeout.
func withCancelTimeout(timeout time.Duration) cancelRequestOption {
	return func(config *agentCancelConfig) {
		config.Timeout = &timeout
	}
}

// cancelHandle waits for a cancellation request's outcome.
type cancelHandle struct {
	wait func() error
}

// Wait blocks until cancellation is handled or the execution ends.
func (handle *cancelHandle) Wait() error {
	if handle == nil || handle.wait == nil {
		return nil
	}
	return handle.wait()
}

// cancelFunc is safe to call concurrently with Agent execution.
type cancelFunc func(...cancelRequestOption) (*cancelHandle, bool)

// cancelInfo describes the cancellation observed by the loop.
type cancelInfo struct {
	Mode      cancelMode
	Escalated bool
	Timeout   bool
}

// cancelError is emitted through loopEvent.Err when cancellation wins.
type cancelError struct {
	Info *cancelInfo
}

func (err *cancelError) Error() string {
	if err == nil || err.Info == nil {
		return "agent canceled"
	}
	return fmt.Sprintf("agent canceled: mode=%v, escalated=%v", err.Info.Mode, err.Info.Escalated)
}

var (
	// errCancelTimeout is returned by cancelHandle.Wait after explicit escalation.
	errCancelTimeout = errors.New("cancel timed out")
	// errExecutionEnded means the run ended before a request took effect.
	errExecutionEnded = errors.New("execution already ended")
	// errStreamCanceled is sent through a public message/tool stream interrupted immediately.
	errStreamCanceled error = &streamCanceledError{}
)

// streamCanceledError is the concrete stream cancellation sentinel.
type streamCanceledError struct{}

func (*streamCanceledError) Error() string { return "stream canceled" }

func (*streamCanceledError) Is(target error) bool {
	_, ok := target.(*streamCanceledError)
	return ok
}

type cancelControl struct {
	mu sync.Mutex

	requested bool
	immediate bool
	mode      cancelMode
	escalated bool
	timedOut  bool

	cancel context.CancelFunc
	timer  *time.Timer

	terminal      bool
	handled       bool
	result        error
	done          chan struct{}
	requestedDone chan struct{}
	requestedOnce sync.Once
}

// WithCancel creates a single-run cancellation option and controller.
func newLoopCancellation() (loopRunOption, cancelFunc) {
	control := &cancelControl{done: make(chan struct{}), requestedDone: make(chan struct{})}
	option := loopRunOption{apply: func(options *agentRunOptions) {
		options.cancel = control
	}}
	return option, control.request
}

func (control *cancelControl) request(opts ...cancelRequestOption) (*cancelHandle, bool) {
	config := &agentCancelConfig{Mode: cancelImmediately}
	for _, opt := range opts {
		if opt != nil {
			opt(config)
		}
	}
	handle := &cancelHandle{wait: func() error {
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
	if config.Mode == cancelImmediately {
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
	control.requestedOnce.Do(func() { close(control.requestedDone) })
	if shouldCancel && cancel != nil {
		cancel()
	}
	return handle, true
}

func (control *cancelControl) pending(point cancelMode) bool {
	if control == nil {
		return false
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.requested && !control.immediate && !control.terminal && !control.handled && control.mode&point != 0
}

func (control *cancelControl) requestedSignal() <-chan struct{} {
	if control == nil {
		return nil
	}
	return control.requestedDone
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

func (control *cancelControl) safePoint(point cancelMode) *cancelError {
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

func (control *cancelControl) immediateError() *cancelError {
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

func (control *cancelControl) markHandledLocked() *cancelError {
	control.handled = true
	control.terminal = true
	if control.timer != nil {
		control.timer.Stop()
	}
	if control.timedOut {
		control.result = errCancelTimeout
	}
	close(control.done)
	mode := control.mode
	if !control.escalated && control.immediate {
		mode = cancelImmediately
	}
	return &cancelError{Info: &cancelInfo{
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
	control.result = errExecutionEnded
	close(control.done)
}
