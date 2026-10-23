package agentruntime

import (
	"context"
	"errors"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
)

func TestExternalRunRetriesRetryableFailure(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "retry", Message: "Continue", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly}}
	attempts := 0
	run, err := control.Start(t.Context(), input, func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		return externalCycleFunc(func(context.Context) agentrun.Outcome {
			attempts++
			if attempts < maxAttempts {
				return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: external.RetryableError(errors.New("stream EOF"))}
			}
			return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome := run.Wait(t.Context())
	if outcome.Status != agentrun.OutcomeCompleted || attempts != maxAttempts {
		t.Fatalf("retry outcome=%+v attempts=%d", outcome, attempts)
	}
}

func TestExternalRunStopsRetryingAfterBound(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "always-fail", Message: "Continue", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly}}
	attempts := 0
	run, err := control.Start(t.Context(), input, func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		return externalCycleFunc(func(context.Context) agentrun.Outcome {
			attempts++
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: external.RetryableError(errors.New("stream EOF"))}
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome := run.Wait(t.Context())
	if outcome.Status != agentrun.OutcomeSuspended || attempts != maxAttempts {
		t.Fatalf("bounded retry outcome=%+v attempts=%d", outcome, attempts)
	}
}

func TestExternalRunDoesNotRetryHardFailure(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "hard", Message: "Continue", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly}}
	attempts := 0
	run, err := control.Start(t.Context(), input, func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		return externalCycleFunc(func(context.Context) agentrun.Outcome {
			attempts++
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: errors.New("blocked by content safety filters")}
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome := run.Wait(t.Context())
	if outcome.Status != agentrun.OutcomeSuspended || attempts != 1 {
		t.Fatalf("hard failure outcome=%+v attempts=%d", outcome, attempts)
	}
}
