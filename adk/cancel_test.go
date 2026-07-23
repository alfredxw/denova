package adk

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
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "timeout", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go", runOption)
	<-model.started
	handle, contributed := cancel(
		WithAgentCancelMode(CancelAfterToolCalls),
		WithAgentCancelTimeout(5*time.Millisecond),
	)
	if !contributed {
		t.Fatal("cancel did not contribute")
	}
	event, ok := iterator.Next()
	if !ok {
		t.Fatal("missing cancel event")
	}
	var cancelErr *CancelError
	if !errors.As(event.Err, &cancelErr) {
		t.Fatalf("event error = %T: %v", event.Err, event.Err)
	}
	if !cancelErr.Info.Escalated || !cancelErr.Info.Timeout || cancelErr.Info.Mode != CancelAfterToolCalls {
		t.Fatalf("cancel info = %#v", cancelErr.Info)
	}
	if err := handle.Wait(); !errors.Is(err, ErrCancelTimeout) {
		t.Fatalf("cancel wait = %v", err)
	}
}

func TestCancelAfterCompletedRunDoesNotContribute(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("done", nil)}}}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "done", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go", runOption)
	for {
		if _, ok := iterator.Next(); !ok {
			break
		}
	}
	handle, contributed := cancel()
	if contributed {
		t.Fatal("cancel contributed after completion")
	}
	if err := handle.Wait(); !errors.Is(err, ErrExecutionEnded) {
		t.Fatalf("cancel wait = %v", err)
	}
}

func TestInterruptErrorClassification(t *testing.T) {
	err := errors.Join(errors.New("outer"), &InterruptError{Reason: "approval required"})
	if !IsInterruptError(err) {
		t.Fatal("wrapped InterruptError was not recognized")
	}
	if IsInterruptError(errors.New("ordinary")) {
		t.Fatal("ordinary error classified as interrupt")
	}
}
