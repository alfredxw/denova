package external

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	externaljournal "denova/internal/agents/runtime/external/journal"
)

func TestProductAcceptsOnlySettledInterruptedProviderSession(t *testing.T) {
	for _, settled := range []bool{false, true} {
		name := "unconfirmed"
		if settled {
			name = "confirmed"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			adapter := adapterFunc(func(_ context.Context, input Input, _ Host) (Result, error) {
				calls++
				if calls == 1 {
					// Game stops generation once its structured submission is ready.
					cancel()
					return Result{SessionID: "provider-thread", Settled: settled}, context.Canceled
				}
				if (input.SessionID == "provider-thread") != settled || (len(input.History) == 0) != settled {
					t.Fatalf("incorrect continuation after product commit: %+v", input)
				}
				return Result{SessionID: "next-thread"}, nil
			})
			runtime := Runtime{CacheRoot: t.TempDir(), Selection: config.RuntimeSelection{Kind: config.RuntimeCodex},
				Acquire: func(context.Context) (Adapter, func(), error) { return adapter, func() {}, nil }}
			request := SessionRequest{Key: "project/story/main", Boundary: "before", Input: Input{History: []Message{{Role: "user", Text: "canonical history"}}}}
			first, err := runtime.Run(ctx, request, maintenanceHost{})
			if err != context.Canceled {
				t.Fatalf("interruption: %v", err)
			}
			if err := first.Session.Accept(t.Context(), "after"); err != nil {
				t.Fatal(err)
			}
			_ = first.Session.Close()
			request.Boundary = "after"
			second, err := runtime.Run(t.Context(), request, maintenanceHost{})
			if err != nil {
				t.Fatal(err)
			}
			_ = second.Session.Close()
		})
	}
}

func TestFailedHistoryLoadingNeverStartsProvider(t *testing.T) {
	for _, failure := range []error{errors.New("canonical source unavailable"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			calls, loads, saved := 0, 0, 0
			adapter := adapterFunc(func(context.Context, Input, Host) (Result, error) {
				calls++
				return Result{SessionID: "unexpected"}, nil
			})
			runtime := Runtime{CacheRoot: t.TempDir(), Acquire: func(context.Context) (Adapter, func(), error) { return adapter, func() {}, nil }}
			request := SessionRequest{Key: "failed-history", Boundary: "before-input",
				Prepare: func(ctx context.Context, input Input, adapter Adapter) (Input, error) {
					return (HistoryPreparation{Input: input, Adapter: adapter,
						LoadHistory: func(context.Context) ([]Message, error) {
							loads++
							return []Message{{Role: "user", Text: "Partial history must not be used."}}, failure
						}, SaveCheckpoint: func(externaljournal.Checkpoint) error { saved++; return nil },
					}).Prepare(ctx)
				}}
			result, err := runtime.Run(t.Context(), request, maintenanceHost{})
			if result.Session != nil {
				defer result.Session.Close()
			}
			if !errors.Is(err, failure) || calls != 0 || loads != 1 || saved != 0 {
				t.Fatalf("failed reconstruction: provider calls=%d loads=%d checkpoints=%d err=%v", calls, loads, saved, err)
			}
		})
	}
}
