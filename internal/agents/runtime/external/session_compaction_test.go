package external

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"
	externaljournal "denova/internal/agents/runtime/external/journal"
)

func TestCheckpointCompactionReportsOnlyPersistedProgress(t *testing.T) {
	saveErr := errors.New("checkpoint write failed")
	for _, kind := range []config.RuntimeID{config.RuntimeGrok, config.RuntimeAntigravity} {
		t.Run(string(kind), func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				history     []Message
				reuse       bool
				noStore     bool
				summaryErr  error
				saveErr     error
				wantSummary bool
				wantSaved   bool
			}{
				{name: "empty history"},
				{name: "short history", history: []Message{{Role: "user", Text: "Keep this short conversation.", Cursor: 1}}},
				{name: "saved checkpoint", history: oversizedHistory().History, wantSummary: true, wantSaved: true},
				{name: "reused checkpoint", history: oversizedHistory().History, reuse: true},
				{name: "summary refused", history: oversizedHistory().History, summaryErr: errors.New("provider refused summary"), wantSummary: true},
				{name: "save failed", history: oversizedHistory().History, saveErr: saveErr, wantSummary: true},
				{name: "no checkpoint store", history: oversizedHistory().History, noStore: true, wantSummary: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					_, request, _ := operationFixture(t)
					request.LoadHistory = func(context.Context) ([]Message, error) { return tc.history, nil }
					summaryCalls, saveCalls, releases := 0, 0, 0
					adapter := adapterFunc(func(_ context.Context, input Input, _ Host) (Result, error) {
						if input.Mode != OperationSummarize {
							t.Fatalf("checkpoint maintenance invoked mode %q", input.Mode)
						}
						summaryCalls++
						return Result{Text: "Confirmed earlier conversation."}, tc.summaryErr
					})
					request.Runtime = &Runtime{Selection: config.RuntimeSelection{Kind: kind}, Acquire: func(context.Context) (Adapter, func(), error) {
						return adapter, func() { releases++ }, nil
					}}
					var saved *externaljournal.Checkpoint
					var events []agentrun.Event
					request.Emit = func(event agentrun.Event) {
						if saved == nil {
							t.Fatal("compaction event preceded checkpoint persistence")
						}
						events = append(events, event)
					}
					result, err := CompactSession(t.Context(), request, func(ctx context.Context, preparation HistoryPreparation) (Input, error) {
						if tc.reuse {
							preparation.Checkpoint = &externaljournal.Checkpoint{Version: checkpointVersion, Summary: "Existing summary.",
								SourceStart: 1, SourceEnd: 1, SourceHash: historyHash(tc.history)}
						}
						if !tc.noStore {
							preparation.SaveCheckpoint = func(checkpoint externaljournal.Checkpoint) error {
								saveCalls++
								if tc.saveErr != nil {
									return tc.saveErr
								}
								saved = &checkpoint
								return nil
							}
						}
						return preparation.Prepare(ctx)
					})
					if !errors.Is(err, tc.saveErr) {
						t.Fatalf("error = %v, want %v", err, tc.saveErr)
					}
					if (summaryCalls > 0) != tc.wantSummary || (saved != nil) != tc.wantSaved || releases != 1 {
						t.Fatalf("summary calls=%d save calls=%d saved=%t releases=%d", summaryCalls, saveCalls, saved != nil, releases)
					}
					if !tc.wantSummary && saveCalls != 0 {
						t.Fatalf("unchanged history invoked checkpoint persistence %d times", saveCalls)
					}
					want := agentcompaction.Result{Triggered: tc.wantSaved}
					if !tc.wantSaved && tc.saveErr == nil {
						want.SkippedReason = "no_progress"
					}
					if result != want {
						t.Fatalf("result = %+v, want %+v", result, want)
					}
					if !tc.wantSaved {
						if len(events) != 0 {
							t.Fatalf("unsaved checkpoint emitted events: %+v", events)
						}
						return
					}
					if len(events) != 1 || events[0].Type != "context_compaction" || events[0].DataString("status") != "completed" {
						t.Fatalf("saved checkpoint events: %+v", events)
					}
					data, ok := events[0].Data.(map[string]any)
					if !ok || data["runtime_managed"] != result.RuntimeManaged {
						t.Fatalf("event and result disagree about compaction ownership: event=%+v result=%+v", events[0], result)
					}
				})
			}
		})
	}
}
