package agents

import (
	"context"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

func TestWaitForRunnerEventTimesOutWhenIteratorIsIdle(t *testing.T) {
	iter, gen := agent.NewAsyncIteratorPair[*agent.AgentEvent]()
	_, ok, err := waitForRunnerEvent(context.Background(), iter, 5*time.Millisecond, gen.Close)
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
	reader, writer := agent.Pipe[*agent.Message](1)
	defer writer.Close()

	_, err := recvMessageFrame(context.Background(), reader, 5*time.Millisecond)
	if err == nil {
		t.Fatal("idle stream should return timeout error")
	}
	if !strings.Contains(err.Error(), "没有收到任何输出") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
}

func TestRecvMessageFrameRecoversProducerPanic(t *testing.T) {
	_, err := recvMessageFrame(context.Background(), panickingMessageFrameStream{}, time.Second)
	if err == nil {
		t.Fatal("producer panic should be returned as an error")
	}
	if !strings.Contains(err.Error(), "panic") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected panic error: %v", err)
	}
}

type panickingMessageFrameStream struct{}

func (panickingMessageFrameStream) Recv() (*agent.Message, error) { panic("boom") }
func (panickingMessageFrameStream) Close()                        {}

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

func TestBorrowedAsyncReceiveReturnsBeforeBlockedProducerAndExitsAfterProducerClose(t *testing.T) {
	producerClosed := make(chan struct{})
	receiverDone := make(chan struct{})
	cancelCalled := make(chan struct{})
	_, _, err := waitForAsyncResult(context.Background(), 5*time.Millisecond, "借用流", func() {
		close(cancelCalled)
	}, func() (int, bool, error) {
		defer close(receiverDone)
		<-producerClosed
		return 1, true, nil
	})
	if err == nil {
		t.Fatal("borrowed receive should time out")
	}
	select {
	case <-cancelCalled:
	default:
		t.Fatal("borrowed receive cancellation seam was not invoked")
	}
	select {
	case <-receiverDone:
		t.Fatal("borrowed receive unexpectedly claimed cancellation could unblock Recv")
	default:
	}
	close(producerClosed)
	select {
	case <-receiverDone:
	case <-time.After(time.Second):
		t.Fatal("borrowed receive goroutine did not exit after producer close")
	}
}

func TestWaitForRunnerEventReturnsWhenCancellationDoesNotCloseProducer(t *testing.T) {
	iterator, generator := agent.NewAsyncIteratorPair[*agent.AgentEvent]()
	cancelCalled := make(chan struct{})
	_, ok, err := waitForRunnerEvent(context.Background(), iterator, 5*time.Millisecond, func() {
		close(cancelCalled)
	})
	if err == nil || ok {
		t.Fatalf("idle iterator result = ok:%t err:%v, want timeout", ok, err)
	}
	select {
	case <-cancelCalled:
	default:
		t.Fatal("iterator cancellation was not requested")
	}
	// Agent exposes only the iterator to the consumer; cancellation is not the
	// same operation as closing this producer. Closing it after the timeout
	// must remain safe and lets the tail receive finish naturally.
	generator.Close()
}
