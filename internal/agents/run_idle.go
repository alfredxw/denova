package agents

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

func agentIdleTimeoutError(scope string, idle time.Duration) error {
	return fmt.Errorf("Agent %s超过 %s 没有收到任何输出，已中断本次运行", scope, idle.Round(time.Second))
}

func waitForRunnerEvent(ctx context.Context, events *agent.AsyncIterator[*agent.AgentEvent], idle time.Duration, cancels ...func()) (*agent.AgentEvent, bool, error) {
	if events == nil {
		return nil, false, nil
	}
	var cancel func()
	if len(cancels) > 0 {
		cancel = cancels[0]
	}
	return waitForAsyncResult(ctx, idle, "主循环", cancel, func() (*agent.AgentEvent, bool, error) {
		event, ok := events.Next()
		return event, ok, nil
	})
}

// messageFrameStream isolates the third-party receive boundary so the waiting
// goroutine can guarantee panic recovery and sequential Close ownership.
type messageFrameStream interface {
	Recv() (*agent.Message, error)
	Close()
}

func recvMessageFrame(ctx context.Context, stream messageFrameStream, idle time.Duration) (*agent.Message, error) {
	if stream == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if idle <= 0 && ctx.Done() == nil {
		return stream.Recv()
	}
	type receiveResult struct {
		frame *agent.Message
		err   error
	}
	const (
		receiveActive uint32 = iota
		receiveAbandoned
		receiveCompleted
	)
	var state atomic.Uint32
	result := make(chan receiveResult, 1)
	go func() {
		received := receiveResult{}
		defer func() {
			if recovered := recover(); recovered != nil {
				received.frame = nil
				received.err = fmt.Errorf("Agent 流式响应等待异步结果 panic: %v\n%s", recovered, string(debug.Stack()))
			}
			result <- received
		}()
		defer func() {
			if !state.CompareAndSwap(receiveActive, receiveCompleted) {
				// A provider-backed Close may race with an in-flight Recv. Once the
				// caller abandons this receive, the receiver goroutine owns cleanup
				// and closes only after Recv returns.
				stream.Close()
			}
		}()
		received.frame, received.err = stream.Recv()
	}()
	var timer *time.Timer
	var timerC <-chan time.Time
	if idle > 0 {
		timer = time.NewTimer(idle)
		timerC = timer.C
		defer timer.Stop()
	}
	abandon := func(cause error) (*agent.Message, error) {
		if !state.CompareAndSwap(receiveActive, receiveAbandoned) {
			// Recv already finished, so closing here is sequential with it.
			stream.Close()
		}
		return nil, cause
	}
	select {
	case received := <-result:
		return received.frame, received.err
	case <-ctx.Done():
		return abandon(ctx.Err())
	case <-timerC:
		return abandon(agentIdleTimeoutError("流式响应", idle))
	}
}

type asyncWaitResult[T any] struct {
	value T
	ok    bool
	err   error
}

// waitForAsyncResult makes a blocking third-party receive responsive to a
// caller context or an optional idle deadline. cancel is advisory because not
// every model provider guarantees that an already blocked receive wakes
// synchronously. The result channel is therefore
// buffered and timeout paths never join the tail goroutine. It exits when the
// producer honors cancellation or eventually closes its stream.
func waitForAsyncResult[T any](ctx context.Context, idle time.Duration, scope string, cancel func(), receive func() (T, bool, error)) (T, bool, error) {
	var zero T
	if receive == nil {
		return zero, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if idle <= 0 && ctx.Done() == nil {
		return receive()
	}
	ch := make(chan asyncWaitResult[T], 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				ch <- asyncWaitResult[T]{err: fmt.Errorf("Agent %s等待异步结果 panic: %v\n%s", scope, recovered, string(debug.Stack()))}
			}
		}()
		value, ok, err := receive()
		ch <- asyncWaitResult[T]{value: value, ok: ok, err: err}
	}()
	var timer *time.Timer
	var timerC <-chan time.Time
	if idle > 0 {
		timer = time.NewTimer(idle)
		timerC = timer.C
		defer timer.Stop()
	}
	cancelReceive := func(resultErr error) (T, bool, error) {
		if cancel != nil {
			cancel()
		}
		return zero, false, resultErr
	}
	select {
	case res := <-ch:
		return res.value, res.ok, res.err
	case <-ctx.Done():
		return cancelReceive(ctx.Err())
	case <-timerC:
		return cancelReceive(agentIdleTimeoutError(scope, idle))
	}
}
