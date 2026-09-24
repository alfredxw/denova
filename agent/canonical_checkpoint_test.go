package agent

import (
	"context"
	"encoding/json"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestCanonicalInputCheckpointRetriesUncommittedProductTransaction(t *testing.T) {
	ctx := context.Background()
	owner, err := New(ctx, Definition{Model: &lifecycleModel{}}, WithSessionStore(canonicalMessageTestStore{Store: agentsession.Memory()}))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	session, err := owner.Session(ctx, NamedSession("canonical-checkpoint-retry"))
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Text: "queued follow-up", IdempotencyKey: "follow-up"}
	if _, err := session.Queue(ctx, input); err != nil {
		t.Fatal(err)
	}
	encoded, runInput, err := encodeInput(input)
	if err != nil {
		t.Fatal(err)
	}
	runInput.Envelope = encoded
	run := &Run{id: "run-retry", session: session}
	ctx = context.WithValue(ctx, canonicalRunKey{}, run)
	before := session.revision
	err = withCanonicalCheckpoint(ctx, canonicalUpdate{
		Stage: CommitInput, Hash: "input-hash",
		Snapshot: runstate.TurnSnapshot{CommandID: "follow-up", OperationID: "run-retry", Cycle: 2, Input: runInput, Delivery: runstate.DeliveryFollowUp},
	}, func(prepare CanonicalCheckpoint) error {
		first, err := prepare(CommitReceipt{Revision: "1"})
		if err != nil {
			return err
		}
		// Another product writer wins the journal CAS. No part of this first
		// transaction was committed; rebuild against the new product revision.
		second, err := prepare(CommitReceipt{Revision: "2"})
		if err != nil {
			return err
		}
		if first.ExpectedRevision != before || second.ExpectedRevision != before || len(second.Records) != 3 {
			t.Fatalf("retry accumulated records or advanced Agent state: %#v", second)
		}
		var firstCycle persistedCycle
		if err := json.Unmarshal(first.Records[1].Data, &firstCycle); err != nil {
			return err
		}
		if firstCycle.InputCommit.Revision != "1" {
			t.Fatal("retry mutated the abandoned checkpoint")
		}
		_, err = session.log.Append(ctx, second.ExpectedRevision, second.Records...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := decodeEngineTranscript(session.engineState)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 1 || state.Messages[0].Content != input.Text || session.revision != before+3 || run.snapshot.InputCommit.Revision != "2" {
		t.Fatalf("retry did not apply exactly one current checkpoint: messages=%#v revision=%d snapshot=%#v", state.Messages, session.revision, run.snapshot)
	}
}

func TestCanonicalReloadCheckpointsHistoryBeforeNewCapabilities(t *testing.T) {
	ctx := context.Background()
	store := canonicalMessageTestStore{Store: agentsession.Memory()}
	open := func() (*Agent, *Session) {
		owner, err := New(ctx, Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
		if err != nil {
			t.Fatal(err)
		}
		session, err := owner.Session(ctx, NamedSession("reloaded-canonical-history"))
		if err != nil {
			t.Fatal(err)
		}
		return owner, session
	}
	owner, session := open()
	if err := session.LoadCanonicalMessages(ctx, []*Message{UserMessage("previous projection")}); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	err := session.persistTranscriptLocked(ctx)
	session.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	history := []*Message{UserMessage("current canonical history")}
	if err := session.LoadCanonicalMessages(ctx, history); err != nil {
		t.Fatal(err)
	}
	// A subsequent structural operation stores a capability against the imported
	// history without running a model or writing another transcript checkpoint.
	checkpoint := json.RawMessage(`{"id":"current-compaction","revision":1}`)
	session.mu.Lock()
	session.capabilities[compactionCapability] = checkpoint
	err = session.persistCapabilitiesLocked(ctx)
	session.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	owner, session = open()
	defer owner.Close(ctx)
	if err := session.LoadCanonicalMessages(ctx, history); err != nil {
		t.Fatal(err)
	}
	session.mu.RLock()
	preserved := string(session.capabilities[compactionCapability])
	session.mu.RUnlock()
	if preserved != string(checkpoint) {
		t.Fatalf("same canonical history invalidated a newly committed capability: %s", preserved)
	}
}
