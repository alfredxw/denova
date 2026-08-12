package sessionfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	"github.com/alfredxw/denova/agent/session"
	"github.com/alfredxw/denova/agent/session/sessiontest"
)

func appendRuntimePayload(
	t *testing.T,
	log *logFile,
	state runstate.JournalCheckpointState,
	payload runstate.EventPayload,
) {
	t.Helper()
	event := runstate.Event{Cursor: state.Cursor() + 1, Durability: runstate.EventDurable, Payload: payload}
	encoded, err := runstate.MarshalJournalEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(context.Background(), session.Revision(state.Cursor()), session.Record{
		Kind: runtimeRecordKind, Version: runtimeRecordV1, Data: encoded,
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.Reduce(event); err != nil {
		t.Fatal(err)
	}
}

func TestStoreContract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(t testing.TB) session.Store {
		store, err := New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}

func TestPendingDeletionMarkerHidesAndPurgesGenericSessionBeforeReopen(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{Namespace: "test", ID: "pending-delete"}
	opened, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.Append(context.Background(), 0, session.Record{
		Kind: "test.record", Version: 1, Data: json.RawMessage(`{"old":true}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	base, _, err := store.baseForKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureDeletionMarker(base+".deleting.json", key); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".manifest.json.tmp-orphan", []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(context.Background(), session.Selector{Namespace: key.Namespace, ID: key.ID})
	if err != nil || len(listed) != 0 {
		t.Fatalf("pending deletion catalog=%#v error=%v", listed, err)
	}

	fresh, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	stats, err := fresh.Replay(context.Background(), func(session.Record) error { return nil })
	if err != nil || stats.RecordsRead != 0 {
		t.Fatalf("fresh replay stats=%#v error=%v", stats, err)
	}
	if _, err := os.Stat(base + ".deleting.json"); !os.IsNotExist(err) {
		t.Fatalf("deletion marker remained after recovery: %v", err)
	}
	if _, err := os.Stat(base + ".manifest.json.tmp-orphan"); !os.IsNotExist(err) {
		t.Fatalf("manifest temporary remained after recovery: %v", err)
	}
}

func TestStoreReopensCommittedRecords(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{Namespace: "test", ID: "reopen", Attributes: map[string]string{"branch": "main"}}
	first, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Append(context.Background(), 0, session.Record{
		Kind: "test.record", Version: 1, Data: json.RawMessage(`{"value":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var records []session.Record
	stats, err := second.Replay(context.Background(), func(record session.Record) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordsRead != 1 || len(records) != 1 || records[0].Revision != 1 || string(records[0].Data) != `{"value":1}` {
		t.Fatalf("stats=%#v records=%#v", stats, records)
	}
}

func TestRuntimeCheckpointColdReplayReadsOnlyBoundedCanonicalTail(t *testing.T) {
	store, err := NewWithOptions(t.TempDir(), Options{CheckpointTailRecords: 2, CheckpointTailBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{Namespace: "test", ID: "runtime-checkpoint"}
	ref := runstate.BindingRef{Kind: key.Namespace, Key: key.ID}
	opened, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	log := opened.(*logFile)
	state := runstate.NewJournalCheckpointState(ref)
	appendRuntimePayload(t, log, state, runstate.CommandAcceptedEvent{
		CommandID: "one", CommandKind: "test", OperationID: "operation-one", Fingerprint: "one",
	})
	appendRuntimePayload(t, log, state, runstate.CommandAcceptedEvent{
		CommandID: "two", CommandKind: "test", OperationID: "operation-two", Fingerprint: "two",
	})
	if err := log.MaybeRuntimeCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	appendRuntimePayload(t, log, state, runstate.CommandAcceptedEvent{
		CommandID: "three", CommandKind: "test", OperationID: "operation-three", Fingerprint: "three",
	})
	indexPath := log.runtimeCommandIndexPath()
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	// Checkpoint recovery must not need an O(history) coverage-index load. The
	// checkpoint's command snapshot plus bounded canonical tail rebuild it.
	if err := os.Remove(indexPath); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cold := reopened.(*logFile)
	restored := runstate.NewJournalCheckpointState(ref)
	stats, err := cold.ReplayRuntimeCheckpoint(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Cursor() != 3 || stats.SnapshotGeneration == 0 || stats.TailBytesRead == 0 ||
		stats.RecordsRead != 1 || stats.EventsRead != 1 || cold.fullReplayCount != 0 || cold.runtimeIndexLoadCount != 0 {
		t.Fatalf("restored cursor=%d stats=%#v full_replays=%d index_loads=%d", restored.Cursor(), stats, cold.fullReplayCount, cold.runtimeIndexLoadCount)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("checkpoint recovery did not rebuild command index: %v", err)
	}
	if record, found, err := cold.LookupRuntimeCommand(context.Background(), "new-after-recovery"); err != nil || found {
		t.Fatalf("post-checkpoint absent lookup record=%#v found=%t error=%v", record, found, err)
	}
	if cold.fullReplayCount != 0 || cold.runtimeIndexLoadCount != 0 {
		t.Fatalf("post-checkpoint absent lookup scanned history: full_replays=%d index_loads=%d", cold.fullReplayCount, cold.runtimeIndexLoadCount)
	}
}

func TestRuntimeCheckpointMissingOrCorruptSidecarRebuildsFromCanonicalLog(t *testing.T) {
	for _, mutation := range []struct {
		name string
		run  func(string) error
	}{
		{name: "missing", run: func(path string) error { return os.Remove(path) }},
		{name: "corrupt", run: func(path string) error { return os.WriteFile(path, []byte(`{"corrupt":true}`), 0o600) }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			store, err := NewWithOptions(t.TempDir(), Options{CheckpointTailRecords: 1})
			if err != nil {
				t.Fatal(err)
			}
			key := session.Key{Namespace: "test", ID: "checkpoint-" + mutation.name}
			ref := runstate.BindingRef{Kind: key.Namespace, Key: key.ID}
			opened, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			log := opened.(*logFile)
			state := runstate.NewJournalCheckpointState(ref)
			appendRuntimePayload(t, log, state, runstate.CommandAcceptedEvent{
				CommandID: "durable", CommandKind: "test", OperationID: "operation", Fingerprint: "stable",
			})
			if err := log.MaybeRuntimeCheckpoint(context.Background(), state); err != nil {
				t.Fatal(err)
			}
			checkpointPath := log.runtimeCheckpointPath()
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			if err := mutation.run(checkpointPath); err != nil {
				t.Fatal(err)
			}

			fallback, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			fallbackLog := fallback.(*logFile)
			rebuilt := runstate.NewJournalCheckpointState(ref)
			if _, err := fallbackLog.ReplayRuntimeCheckpoint(context.Background(), rebuilt); err != nil {
				t.Fatal(err)
			}
			if rebuilt.Cursor() != 1 {
				t.Fatalf("rebuilt cursor=%d", rebuilt.Cursor())
			}
			if err := fallbackLog.Close(); err != nil {
				t.Fatal(err)
			}
			indexPath := fallbackLog.runtimeCommandIndexPath()
			if err := os.Remove(indexPath); err != nil {
				t.Fatal(err)
			}

			bounded, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			defer bounded.Close()
			boundedLog := bounded.(*logFile)
			restored := runstate.NewJournalCheckpointState(ref)
			stats, err := boundedLog.ReplayRuntimeCheckpoint(context.Background(), restored)
			if err != nil {
				t.Fatal(err)
			}
			if restored.Cursor() != 1 || stats.TailBytesRead != 0 || boundedLog.fullReplayCount != 0 || boundedLog.runtimeIndexLoadCount != 0 {
				t.Fatalf("restored cursor=%d stats=%#v full_replays=%d index_loads=%d", restored.Cursor(), stats, boundedLog.fullReplayCount, boundedLog.runtimeIndexLoadCount)
			}
			if _, err := os.Stat(indexPath); err != nil {
				t.Fatalf("empty-tail checkpoint recovery did not rebuild command index: %v", err)
			}
		})
	}
}

func TestRuntimeCommandSidecarMissingOrCorruptRebuildsOnce(t *testing.T) {
	for _, mutation := range []struct {
		name string
		run  func(string) error
	}{
		{name: "missing", run: func(path string) error { return os.Remove(path) }},
		{name: "corrupt", run: func(path string) error { return os.WriteFile(path, []byte(`{"corrupt":true}`), 0o600) }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			key := session.Key{Namespace: "test", ID: "command-" + mutation.name}
			ref := runstate.BindingRef{Kind: key.Namespace, Key: key.ID}
			opened, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			log := opened.(*logFile)
			state := runstate.NewJournalCheckpointState(ref)
			appendRuntimePayload(t, log, state, runstate.CommandAcceptedEvent{
				CommandID: "historical", CommandKind: "test", OperationID: "operation", Fingerprint: "stable",
			})
			path := log.runtimeCommandPath("historical")
			if err := mutation.run(path); err != nil {
				t.Fatal(err)
			}
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			cold, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			coldLog := cold.(*logFile)
			before := coldLog.fullReplayCount
			record, found, err := coldLog.LookupRuntimeCommand(context.Background(), "historical")
			if err != nil || !found || record.Fingerprint != "stable" || coldLog.fullReplayCount != before+1 {
				t.Fatalf("first lookup record=%#v found=%t error=%v full_replays=%d", record, found, err, coldLog.fullReplayCount)
			}
			firstRebuilds := coldLog.fullReplayCount
			record, found, err = coldLog.LookupRuntimeCommand(context.Background(), "historical")
			if err != nil || !found || record.Fingerprint != "stable" || coldLog.fullReplayCount != firstRebuilds {
				t.Fatalf("bounded lookup record=%#v found=%t error=%v full_replays=%d", record, found, err, coldLog.fullReplayCount)
			}
			if err := coldLog.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeCommandIndexBoundsAbsentLookupAndRebuildsOnce(t *testing.T) {
	for _, mutation := range []struct {
		name string
		run  func(string) error
	}{
		{name: "valid", run: func(string) error { return nil }},
		{name: "missing", run: func(path string) error { return os.Remove(path) }},
		{name: "corrupt", run: func(path string) error { return os.WriteFile(path, []byte(`{"corrupt":true}`), 0o600) }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			key := session.Key{Namespace: "test", ID: "command-index-" + mutation.name}
			ref := runstate.BindingRef{Kind: key.Namespace, Key: key.ID}
			opened, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			log := opened.(*logFile)
			state := runstate.NewJournalCheckpointState(ref)
			appendRuntimePayload(t, log, state, runstate.CommandAcceptedEvent{
				CommandID: "historical", CommandKind: "test", OperationID: "operation", Fingerprint: "stable",
			})
			indexPath := log.runtimeCommandIndexPath()
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			if err := mutation.run(indexPath); err != nil {
				t.Fatal(err)
			}

			cold, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			defer cold.Close()
			coldLog := cold.(*logFile)
			before := coldLog.fullReplayCount
			if record, found, err := coldLog.LookupRuntimeCommand(context.Background(), "never-accepted"); err != nil || found {
				t.Fatalf("absent lookup record=%#v found=%t error=%v", record, found, err)
			}
			wantReplays := before
			if mutation.name != "valid" {
				wantReplays++
			}
			if coldLog.fullReplayCount != wantReplays {
				t.Fatalf("first absent lookup full_replays=%d, want %d", coldLog.fullReplayCount, wantReplays)
			}
			indexLoads := coldLog.runtimeIndexLoadCount
			if record, found, err := coldLog.LookupRuntimeCommand(context.Background(), "another-new-command"); err != nil || found {
				t.Fatalf("second absent lookup record=%#v found=%t error=%v", record, found, err)
			}
			if coldLog.fullReplayCount != wantReplays {
				t.Fatalf("rebuilt index did not bound the next absent lookup: full_replays=%d want=%d", coldLog.fullReplayCount, wantReplays)
			}
			if coldLog.runtimeIndexLoadCount != indexLoads {
				t.Fatalf("bounded absent lookup reloaded the complete index: loads=%d want=%d", coldLog.runtimeIndexLoadCount, indexLoads)
			}
		})
	}
}

func TestDeleteRemovesRuntimeAccelerationAndRecreateStartsEmpty(t *testing.T) {
	store, err := NewWithOptions(t.TempDir(), Options{CheckpointTailRecords: 1})
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{Namespace: "test", ID: "delete-runtime-sidecars"}
	ref := runstate.BindingRef{Kind: key.Namespace, Key: key.ID}
	opened, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	log := opened.(*logFile)
	state := runstate.NewJournalCheckpointState(ref)
	appendRuntimePayload(t, log, state, runstate.CommandAcceptedEvent{
		CommandID: "deleted", CommandKind: "test", OperationID: "operation", Fingerprint: "stable",
	})
	if err := log.MaybeRuntimeCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	checkpointPath := log.runtimeCheckpointPath()
	indexPath := log.runtimeCommandIndexPath()
	receiptPath := log.runtimeCommandPath("deleted")
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{checkpointPath, indexPath, receiptPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected runtime acceleration %s: %v", path, err)
		}
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{checkpointPath, indexPath, receiptPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted runtime acceleration remained %s: %v", path, err)
		}
	}
	recreated, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer recreated.Close()
	stats, err := recreated.Replay(context.Background(), func(session.Record) error { return nil })
	if err != nil || stats.RecordsRead != 0 {
		t.Fatalf("recreated Session replay stats=%#v error=%v", stats, err)
	}
}
