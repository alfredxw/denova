package external

import (
	"context"
	"testing"

	"denova/config"
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
