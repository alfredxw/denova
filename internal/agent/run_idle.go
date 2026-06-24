package agent

import (
	"context"
	"fmt"
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
	if idle <= 0 {
		event, ok := events.Next()
		return event, ok, nil
	}
	type result struct {
		event *adk.AgentEvent
		ok    bool
	}
	ch := make(chan result, 1)
	go func() {
		event, ok := events.Next()
		ch <- result{event: event, ok: ok}
	}()
	timer := time.NewTimer(idle)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res.event, res.ok, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-timer.C:
		return nil, false, agentIdleTimeoutError("主循环", idle)
	}
}

func recvMessageFrame(ctx context.Context, stream *schema.StreamReader[*schema.Message], idle time.Duration) (*schema.Message, error) {
	if stream == nil {
		return nil, nil
	}
	if idle <= 0 {
		return stream.Recv()
	}
	type result struct {
		frame *schema.Message
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		frame, err := stream.Recv()
		ch <- result{frame: frame, err: err}
	}()
	timer := time.NewTimer(idle)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res.frame, res.err
	case <-ctx.Done():
		stream.Close()
		return nil, ctx.Err()
	case <-timer.C:
		stream.Close()
		return nil, agentIdleTimeoutError("流式响应", idle)
	}
}
