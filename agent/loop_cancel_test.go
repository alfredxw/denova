package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

type cancelAwareGenerateModel struct {
	started chan struct{}
}

func (model *cancelAwareGenerateModel) Generate(ctx context.Context, _ []*Message, _ ...ModelOption) (*Message, error) {
	close(model.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*cancelAwareGenerateModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	return nil, errors.New("unexpected Stream")
}

func TestCancelTimeoutEscalatesOnlyWhenExplicitlyConfigured(t *testing.T) {
	model := &cancelAwareGenerateModel{started: make(chan struct{})}
	agent, err := newModelToolLoop(context.Background(), loopConfig{Name: "timeout", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := newLoopCancellation()
	iterator := newLoopRunner(loopRunnerConfig{Agent: agent}).Query(context.Background(), "go", runOption)
	<-model.started
	handle, contributed := cancel(
		withCancelMode(cancelAfterTools),
		withCancelTimeout(5*time.Millisecond),
	)
	if !contributed {
		t.Fatal("cancel did not contribute")
	}
	event, ok := iterator.Next()
	if !ok {
		t.Fatal("missing cancel event")
	}
	var cancelErr *cancelError
	if !errors.As(event.Err, &cancelErr) {
		t.Fatalf("event error = %T: %v", event.Err, event.Err)
	}
	if !cancelErr.Info.Escalated || !cancelErr.Info.Timeout || cancelErr.Info.Mode != cancelAfterTools {
		t.Fatalf("cancel info = %#v", cancelErr.Info)
	}
	if err := handle.Wait(); !errors.Is(err, errCancelTimeout) {
		t.Fatalf("cancel wait = %v", err)
	}
}

func TestCancelAfterCompletedRunDoesNotContribute(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("done", nil)}}}
	agent, err := newModelToolLoop(context.Background(), loopConfig{Name: "done", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := newLoopCancellation()
	iterator := newLoopRunner(loopRunnerConfig{Agent: agent}).Query(context.Background(), "go", runOption)
	for {
		if _, ok := iterator.Next(); !ok {
			break
		}
	}
	handle, contributed := cancel()
	if contributed {
		t.Fatal("cancel contributed after completion")
	}
	if err := handle.Wait(); !errors.Is(err, errExecutionEnded) {
		t.Fatalf("cancel wait = %v", err)
	}
}
