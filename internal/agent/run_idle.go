package agent

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func agentIdleTimeoutError(scope string, idle time.Duration) error {
	return fmt.Errorf("Agent %s超过 %s 没有收到任何输出，已中断本次运行", scope, idle.Round(time.Second))
}

func waitForRunnerEvent(ctx context.Context, events *adk.AsyncIterator[*adk.AgentEvent], idle time.Duration) (*adk.AgentEvent, bool, error) {
	if events == nil {
		return nil, false, nil
	}
	return waitForAsyncResult(ctx, idle, "主循环", nil, func() (*adk.AgentEvent, bool, error) {
		event, ok := events.Next()
		return event, ok, nil
	})
}

func recvMessageFrame(ctx context.Context, stream *schema.StreamReader[*schema.Message], idle time.Duration) (*schema.Message, error) {
	if stream == nil {
		return nil, nil
	}
	frame, _, err := waitForAsyncResult(ctx, idle, "流式响应", stream.Close, func() (*schema.Message, bool, error) {
		frame, err := stream.Recv()
		return frame, true, err
	})
	return frame, err
}

type asyncWaitResult[T any] struct {
	value T
	ok    bool
	err   error
}

func waitForAsyncResult[T any](ctx context.Context, idle time.Duration, scope string, cancel func(), receive func() (T, bool, error)) (T, bool, error) {
	var zero T
	if receive == nil {
		return zero, false, nil
	}
	if idle <= 0 {
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
	timer := time.NewTimer(idle)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res.value, res.ok, res.err
	case <-ctx.Done():
		if cancel != nil {
			cancel()
		}
		return zero, false, ctx.Err()
	case <-timer.C:
		if cancel != nil {
			cancel()
		}
		return zero, false, agentIdleTimeoutError(scope, idle)
	}
}
