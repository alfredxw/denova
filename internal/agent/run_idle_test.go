package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestWaitForRunnerEventTimesOutWhenIteratorIsIdle(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	defer gen.Close()

	_, ok, err := waitForRunnerEvent(context.Background(), iter, 5*time.Millisecond)
	if err == nil {
		t.Fatal("idle iterator should return timeout error")
	}
	if ok {
		t.Fatal("idle iterator should not report an event")
	}
	if !strings.Contains(err.Error(), "没有收到任何输出") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
}

func TestRecvMessageFrameTimesOutAndClosesStream(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](1)
	defer writer.Close()

	_, err := recvMessageFrame(context.Background(), reader, 5*time.Millisecond)
	if err == nil {
		t.Fatal("idle stream should return timeout error")
	}
	if !strings.Contains(err.Error(), "没有收到任何输出") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
}

func TestWaitForAsyncResultRecoversPanic(t *testing.T) {
	_, ok, err := waitForAsyncResult(context.Background(), time.Second, "测试", nil, func() (int, bool, error) {
		panic("boom")
	})
	if err == nil {
		t.Fatal("panic should be returned as an error")
	}
	if ok {
		t.Fatal("panic should not report a successful result")
	}
	if !strings.Contains(err.Error(), "panic") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected panic error: %v", err)
	}
}
