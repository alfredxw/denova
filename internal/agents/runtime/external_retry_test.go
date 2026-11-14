package agentruntime

import (
	"context"
	"errors"
	"sync/atomic"
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

func TestExternalRunRetryUsesRecoveryEntry(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "retry-resume", Message: "Continue", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly}}
	var resumes []bool
	attempts := 0
	run, err := control.Start(t.Context(), input, func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		resumes = append(resumes, input.Resume)
		return externalCycleFunc(func(context.Context) agentrun.Outcome {
			attempts++
			if attempts < 2 {
				return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: external.RetryableError(errors.New("stream EOF"))}
			}
			return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome := run.Wait(t.Context())
	if outcome.Status != agentrun.OutcomeCompleted || attempts != 2 {
		t.Fatalf("recovery-entry retry outcome=%+v attempts=%d", outcome, attempts)
	}
	if len(resumes) != 2 || resumes[0] || !resumes[1] {
		t.Fatalf("retry did not re-enter the recovery entry resume flags=%v", resumes)
	}
}

func TestExternalRunStopsRetryingAfterCancel(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "cancel-retry", Message: "Continue", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly}}
	ctx, cancel := context.WithCancel(t.Context())
	attempts := 0
	run, err := control.Start(ctx, input, func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		return externalCycleFunc(func(context.Context) agentrun.Outcome {
			attempts++
			cancel()
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: external.RetryableError(errors.New("stream EOF"))}
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome := run.Wait(ctx)
	if attempts != 1 {
		t.Fatalf("cancel did not stop the retry loop attempts=%d", attempts)
	}
	if outcome.Status != agentrun.OutcomeSuspended {
		t.Fatalf("cancelled outcome=%+v", outcome)
	}
}

// TestExternalRunRetryRestartsProvider drives the real provider lease through a
// mock adapter: the first attempt fails with a retryable error and the second
// automatically re-enters the recovery entry, so the provider is actually
// invoked again instead of replaying the persisted failure receipt.
func TestExternalRunRetryRestartsProvider(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	var providerCalls atomic.Int32
	adapter := controlAdapter(func(ctx context.Context, input external.Input, _ external.Host) (external.Result, error) {
		if providerCalls.Add(1) == 1 {
			return external.Result{}, external.RetryableError(errors.New("stream EOF"))
		}
		return external.Result{Text: "Confirmed retry", SessionID: "session-2", Settled: true}, nil
	})
	runtime := &external.Runtime{CacheRoot: t.TempDir(), Acquire: func(context.Context) (external.Adapter, func(), error) {
		return adapter, func() {}, nil
	}}
	var resumes []bool
	input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "retry-provider", Message: "Continue", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly}}
	run, err := control.Start(t.Context(), input, func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), after func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		resumes = append(resumes, input.Resume)
		return externalCycleFunc(func(ctx context.Context) agentrun.Outcome {
			response, err := runtime.Run(ctx, external.SessionRequest{Key: "retry-provider", Input: external.Input{Text: input.Request.Message}}, &external.MaintenanceObserver{})
			if err != nil {
				return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
			}
			defer response.Session.Close()
			if err := after(ctx, response.Session); err != nil {
				return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
			}
			return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome := run.Wait(t.Context())
	if outcome.Status != agentrun.OutcomeCompleted || providerCalls.Load() != 2 {
		t.Fatalf("retry outcome=%+v providerCalls=%d", outcome, providerCalls.Load())
	}
	if len(resumes) != 2 || resumes[0] || !resumes[1] {
		t.Fatalf("retry did not re-enter the recovery entry resume flags=%v", resumes)
	}
}
