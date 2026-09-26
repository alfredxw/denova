package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type contextCheckpointLog struct {
	agentsession.Log
	reject      bool
	loseReceipt bool
	commits     int
}

func (log *contextCheckpointLog) Append(ctx context.Context, revision agentsession.Revision, records ...agentsession.Record) (agentsession.Revision, error) {
	if log.reject {
		return revision, context.Canceled
	}
	log.commits++
	next, err := log.Log.Append(ctx, revision, records...)
	if err == nil && log.loseReceipt {
		return revision, agentsession.ErrCommitUnknown
	}
	return next, err
}

func TestContextCheckpointCommitsCompactionAndPreparedContextTogether(t *testing.T) {
	for _, scenario := range []string{"native", "canonical_metadata", "canonical_product", "native_lost_receipt", "canonical_metadata_lost_receipt", "canonical_product_lost_receipt"} {
		t.Run(scenario, func(t *testing.T) {
			mode := strings.TrimSuffix(scenario, "_lost_receipt")
			lostReceipt := mode != scenario
			ctx := t.Context()
			var store agentsession.Store = agentsession.Memory()
			if mode != "native" {
				store = canonicalMessageTestStore{Store: store}
			}
			owner, err := New(ctx, Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(context.Background()) })
			session, err := owner.Session(ctx, NamedSession("context-transition"))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "canonical_product" {
				envelope, input, err := encodeInput(Text("accepted work"))
				if err != nil {
					t.Fatal(err)
				}
				input.Envelope = envelope
				if err := session.appendRecordLocked(ctx, sessionInputRecord, persistedInput{
					Receipt: CommandReceipt{CommandID: "command", RunID: "run"}, Kind: inputRun, Hash: "accepted-input", Input: input,
				}); err != nil {
					t.Fatal(err)
				}
				if err := session.appendRecordLocked(ctx, turnStartedRecord, persistedTurn{RunID: "run", CommandID: "command"}); err != nil {
					t.Fatal(err)
				}
			}
			before := append(json.RawMessage(nil), session.engineState...)
			state, err := json.Marshal(engineTranscript{Version: engineTranscriptVersion,
				PreparationStage: enginePreparationMaterialized,
				PreparedContext: &preparedContext{Version: 1, Fragments: []ContextFragment{{
					Source: "project", Purpose: "accepted context", Resource: "AGENTS.md", HardLimit: 1024,
					Placement: ContextLeadingMessage, Stability: ContextStablePrefix, Content: "Accepted after compaction",
				}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			update := runstate.EngineTranscriptUpdated{State: state, CapabilityStates: map[string]json.RawMessage{
				compactionCapability: json.RawMessage(`{"id":"accepted-compaction","revision":1}`),
			}}
			log := &contextCheckpointLog{Log: session.log, reject: true}
			session.log = log
			run := &Run{id: "run", session: session, eventsEnd: true, snapshot: runstate.TurnSnapshot{OperationID: "run", CommandID: "command", Cycle: 1}}
			commit := func() error {
				if mode != "canonical_product" {
					return run.commitContextCheckpoint(update)
				}
				return withCanonicalCheckpoint(context.WithValue(ctx, canonicalRunKey{}, run), canonicalUpdate{
					Stage: CommitContext, Snapshot: run.snapshot, State: state, CapabilityStates: update.CapabilityStates,
				}, func(prepare CanonicalCheckpoint) error {
					checkpoint, err := prepare(CommitReceipt{Revision: "1"})
					if err != nil {
						return err
					}
					_, err = log.Append(ctx, checkpoint.ExpectedRevision, checkpoint.Records...)
					return err
				})
			}
			if err := commit(); !errors.Is(err, context.Canceled) {
				t.Fatalf("rejected commit: %v", err)
			}
			if !bytes.Equal(before, session.engineState) || len(session.capabilities) != 0 || len(run.snapshot.State) != 0 {
				t.Fatal("failed transaction published a partial context transition")
			}
			log.reject = false
			log.loseReceipt = lostReceipt
			if err := commit(); lostReceipt {
				if !errors.Is(err, agentsession.ErrCommitUnknown) || session.storageErr == nil {
					t.Fatalf("unconfirmed commit did not block further writes: %v", err)
				}
				if !bytes.Equal(before, session.engineState) || len(session.capabilities) != 0 || len(run.snapshot.State) != 0 {
					t.Fatal("unconfirmed commit published a partial context transition")
				}
				if err := commit(); !errors.Is(err, agentsession.ErrCommitUnknown) {
					t.Fatalf("unconfirmed transaction was retried without reopening: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := run.commitContextCheckpoint(update); err != nil {
					t.Fatal(err)
				}
			}
			if log.commits != 1 {
				t.Fatalf("context and capability used %d transactions", log.commits)
			}
			if mode == "canonical_product" && !lostReceipt {
				// Engine requests retain their initial capability snapshot. A later
				// tool/context commit must not roll the live Run back to that view.
				stale := run.snapshot
				stale.Capabilities = nil
				if err := withCanonicalCheckpoint(context.WithValue(ctx, canonicalRunKey{}, run), canonicalUpdate{
					Stage: CommitContext, Snapshot: stale, State: state,
				}, func(prepare CanonicalCheckpoint) error {
					checkpoint, err := prepare(CommitReceipt{Revision: "2"})
					if err != nil {
						return err
					}
					_, err = log.Append(ctx, checkpoint.ExpectedRevision, checkpoint.Records...)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(run.snapshot.Capabilities[compactionCapability], update.CapabilityStates[compactionCapability]) {
					t.Fatal("ordinary canonical commit lost the live compaction state")
				}
			}
			if err := owner.Close(ctx); err != nil && !(lostReceipt && errors.Is(err, agentsession.ErrCommitUnknown)) {
				t.Fatal(err)
			}
			owner, err = New(ctx, Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			session, err = owner.Session(ctx, NamedSession("context-transition"))
			if err != nil {
				t.Fatal(err)
			}
			restored := session.engineState
			if mode != "native" {
				restored = session.messageCheckpoint.Metadata
			}
			checkpoint, err := decodeEngineTranscript(restored)
			if err != nil || checkpoint.PreparedContext == nil || checkpoint.PreparedContext.Fragments[0].Content != "Accepted after compaction" ||
				!bytes.Equal(session.capabilities[compactionCapability], update.CapabilityStates[compactionCapability]) {
				t.Fatalf("cold reopen mixed compaction and context: state=%s capabilities=%v err=%v", restored, session.capabilities, err)
			}
		})
	}
}
