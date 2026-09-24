package external

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

func TestNativeGuidanceIsCanonicalBeforeSubsequentWork(t *testing.T) {
	service, request, _ := operationFixture(t)
	guidance := Guidance{Request: agentchat.ChatRequest{CommandID: "live-guidance", Message: "Keep the ending"}, Count: 1}
	request.PrepareGuidance = func(_ context.Context, input agentchat.ChatRequest) (StartRequest, error) {
		return StartRequest{Input: Input{Text: input.Message}, Message: agent.Message{Role: agent.User, Content: input.Message}, Metadata: session.MessageMetadata{MessageID: input.CommandID + "-input"}}, nil
	}
	request.Adapter = adapterFunc(func(ctx context.Context, _ Input, host Host) (Result, error) {
		pending := true
		controls := &Steering{Next: func(context.Context) (Guidance, bool, error) { return guidance, pending, nil }, Delivered: func(Guidance) { pending = false }}
		check := func(count int) error {
			return request.Session.ReadExternal(ctx, func(state session.ExternalState) error {
				for _, operation := range state.Projection.Operations {
					if operation.GuidanceCount != count {
						return fmt.Errorf("guidance prefix=%d, want %d", operation.GuidanceCount, count)
					}
				}
				return nil
			})
		}
		if err := controls.Deliver(ctx, host, func(context.Context, Input) error { return ErrSteerUnavailable }); err != nil || !pending {
			return Result{}, fmt.Errorf("ended native turn consumed input: %v", err)
		}
		if err := controls.Deliver(ctx, host, func(_ context.Context, input Input) error {
			if input.Text != guidance.Request.Message {
				return errors.New("prepared guidance changed")
			}
			return check(0)
		}); err != nil {
			return Result{}, err
		}
		if err := check(1); err != nil {
			return Result{}, err
		}
		return Result{Text: "Kept the ending", Settled: true}, nil
	})
	operation, err := service.Start(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome: %+v", outcome)
	}
	if operation.ConsumedGuidance() != 1 {
		t.Fatal("accepted guidance prefix was lost")
	}
	history, err := ReadHistory(t.Context(), request.Session)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range history.Messages {
		found = found || message.Text == guidance.Request.Message
	}
	if !found {
		t.Fatal("native input is missing from canonical history")
	}
	// Force reload from canonical facts, independent of the live operation.
	if err := service.Recover(t.Context(), request.ProjectID, request.Session); err != nil {
		t.Fatal(err)
	}
	if err := request.Session.ReadExternal(t.Context(), func(state session.ExternalState) error {
		if state.Projection.Operations[operation.id].GuidanceCount != 1 {
			return errors.New("recovery lost native guidance receipt")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
