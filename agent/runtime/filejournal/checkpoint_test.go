package filejournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func checkpointTestBinding(sessionID string) runstate.BindingRef {
	return runstate.BindingRef{Kind: "test", Key: sessionID}
}

func openCheckpointTestJournal(t *testing.T, root string, ref runstate.BindingRef, options Options) (*Store, *journal) {
	t.Helper()
	store, err := NewStoreWithOptions(root, options)
	if err != nil {
		t.Fatal(err)
	}
	key, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	journal := opened.(*journal)
	return store, journal
}

func checkpointJournalKey(t *testing.T, ref runstate.BindingRef) string {
	t.Helper()
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func asCheckpointJournal(t *testing.T, opened runstate.Journal) *journal {
	t.Helper()
	result, ok := opened.(*journal)
	if !ok {
		t.Fatalf("journal type = %T, want *journal", opened)
	}
	return result
}

func appendAndReduceCheckpointEvents(t *testing.T, journal *journal, state runstate.JournalCheckpointState, payloads []runstate.EventPayload) {
	t.Helper()
	events, err := journal.Append(context.Background(), state.Cursor(), payloads)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := state.Reduce(event); err != nil {
			t.Fatalf("reduce cursor %d: %v", event.Cursor, err)
		}
	}
}

func TestFileJournalCheckpointRestoresRuntimeReducerState(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("checkpoint-state")
	_, journal := openCheckpointTestJournal(t, root, ref, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := runstate.NewJournalCheckpointState(ref)
	appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "start", CommandKind: "test", OperationID: "operation", Fingerprint: "start-fingerprint"},
	})
	if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if journal.activeGeneration.SnapshotFile == "" {
		t.Fatal("checkpoint generation was not activated")
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	_, reopened := openCheckpointTestJournal(t, root, ref, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	t.Cleanup(func() { _ = reopened.Close() })
	restored := runstate.NewJournalCheckpointState(ref)
	stats, err := reopened.ReplayCheckpoint(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SnapshotGeneration == 0 || stats.RecordsRead != 0 || stats.TailBytesRead != 0 {
		t.Fatalf("cold replay stats = %#v, want snapshot plus empty bounded tail", stats)
	}
	if restored.Cursor() != 1 || len(restored.RetainedEvents()) != 1 {
		t.Fatalf("checkpoint state cursor=%d retained=%d, want 1/1", restored.Cursor(), len(restored.RetainedEvents()))
	}
	record, found, err := reopened.LookupCommand(context.Background(), "start")
	if err != nil || !found || record.Fingerprint != "start-fingerprint" {
		t.Fatalf("checkpoint command = %#v found=%v err=%v", record, found, err)
	}
}

func TestFileJournalCorruptActiveSnapshotReconstructsCanonicalCheckpointFromPrevious(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("checkpoint-fallback")
	_, journal := openCheckpointTestJournal(t, root, ref, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := runstate.NewJournalCheckpointState(ref)
	appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "one", CommandKind: "test", OperationID: "operation-one", Fingerprint: "one"},
	})
	if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "two", CommandKind: "test", OperationID: "operation-two", Fingerprint: "two"},
	})
	if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	activeSnapshot, err := journal.resolveGenerationFile(journal.activeGeneration.SnapshotFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activeSnapshot, []byte(`{"corrupt":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	_, reopened := openCheckpointTestJournal(t, root, ref, Options{})
	t.Cleanup(func() { _ = reopened.Close() })
	restored := runstate.NewJournalCheckpointState(ref)
	stats, err := reopened.ReplayCheckpoint(context.Background(), restored)
	if err != nil {
		t.Fatalf("fallback replay: %v", err)
	}
	if restored.Cursor() != state.Cursor() || stats.SnapshotGeneration == 0 || stats.SnapshotGeneration == journal.activeGeneration.Generation {
		t.Fatalf("fallback state cursor=%d stats=%#v active=%d", restored.Cursor(), stats, journal.activeGeneration.Generation)
	}
	for _, commandID := range []runstate.CommandID{"one", "two"} {
		record, found, err := reopened.LookupCommand(context.Background(), commandID)
		if err != nil || !found || record.Receipt.CommandID != commandID {
			t.Fatalf("command %q after snapshot fallback = %#v found=%v err=%v", commandID, record, found, err)
		}
	}
}

func TestFileJournalCorruptActiveSnapshotReplaysCurrentGenerationTailFromPrevious(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("checkpoint-fallback-with-tail")
	_, journal := openCheckpointTestJournal(t, root, ref, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := runstate.NewJournalCheckpointState(ref)
	for _, id := range []runstate.CommandID{"one", "two"} {
		appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
			runstate.CommandAcceptedEvent{CommandID: id, CommandKind: "test", OperationID: runstate.OperationID(id), Fingerprint: string(id)},
		})
		if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
			t.Fatal(err)
		}
	}
	appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "after-checkpoint", CommandKind: "test", OperationID: "after-checkpoint", Fingerprint: "after-checkpoint"},
	})
	activeSnapshot, err := journal.resolveGenerationFile(journal.activeGeneration.SnapshotFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activeSnapshot, []byte(`{"corrupt":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	_, reopened := openCheckpointTestJournal(t, root, ref, Options{})
	t.Cleanup(func() { _ = reopened.Close() })
	restored := runstate.NewJournalCheckpointState(ref)
	stats, err := reopened.ReplayCheckpoint(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Cursor() != state.Cursor() || stats.SnapshotGeneration == journal.activeGeneration.Generation {
		t.Fatalf("composed fallback cursor=%d stats=%#v, want cursor=%d from previous snapshot", restored.Cursor(), stats, state.Cursor())
	}
	record, found, err := reopened.LookupCommand(context.Background(), "after-checkpoint")
	if err != nil || !found || record.Receipt.Cursor != state.Cursor() {
		t.Fatalf("current generation tail command = %#v found=%v err=%v", record, found, err)
	}
}

func TestFileJournalColdReplayRejectsCorruptCanonicalActiveTail(t *testing.T) {
	for _, corruptActiveSnapshot := range []bool{false, true} {
		name := "active_snapshot_valid"
		if corruptActiveSnapshot {
			name = "active_snapshot_reconstructed"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			ref := checkpointTestBinding("canonical-tail-corruption-" + name)
			store, journal := openCheckpointTestJournal(t, root, ref, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
			state := runstate.NewJournalCheckpointState(ref)
			for _, id := range []runstate.CommandID{"one", "two"} {
				appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
					runstate.CommandAcceptedEvent{CommandID: id, CommandKind: "test", OperationID: runstate.OperationID(id), Fingerprint: string(id)},
				})
				if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
					t.Fatal(err)
				}
			}
			appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
				runstate.CommandAcceptedEvent{CommandID: "confirmed-active-tail", CommandKind: "test", OperationID: "confirmed-active-tail", Fingerprint: "confirmed-active-tail"},
			})
			confirmedCursor := state.Cursor()
			activeTailPath := journal.tailPath
			appendChecksumInvalidCompleteTransaction(t, activeTailPath, confirmedCursor+1, runstate.CommandAcceptedEvent{
				CommandID: "corrupt-active-tail", CommandKind: "test", OperationID: "corrupt-active-tail", Fingerprint: "corrupt-active-tail",
			})
			if corruptActiveSnapshot {
				activeSnapshot, err := journal.resolveGenerationFile(journal.activeGeneration.SnapshotFile)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(activeSnapshot, []byte(`{"corrupt":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			corruptTail, err := os.ReadFile(activeTailPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}

			opened, err := store.OpenJournal(context.Background(), checkpointJournalKey(t, ref))
			if err != nil {
				t.Fatal(err)
			}
			reopened := asCheckpointJournal(t, opened)
			restored := runstate.NewJournalCheckpointState(ref)
			stats, replayErr := reopened.ReplayCheckpoint(context.Background(), restored)
			if replayErr == nil || !strings.Contains(replayErr.Error(), "checksum mismatch") {
				t.Fatalf("cold replay error = %v, want canonical active-tail checksum failure", replayErr)
			}
			if stats.RecordsRead == 0 {
				t.Fatalf("cold replay stats = %#v, want confirmed active-tail prefix to be scanned", stats)
			}
			if restored.Cursor() != 0 {
				t.Fatalf("failed replay published partial cursor %d, want untouched target", restored.Cursor())
			}
			if reopened.initialized || reopened.cursor != 0 {
				t.Fatalf("failed replay made journal writable: initialized=%v cursor=%d", reopened.initialized, reopened.cursor)
			}
			if _, appendErr := reopened.Append(context.Background(), confirmedCursor, []runstate.EventPayload{runstate.CommandAcceptedEvent{
				CommandID: "must-not-append", CommandKind: "test", OperationID: "must-not-append", Fingerprint: "must-not-append",
			}}); appendErr == nil || !strings.Contains(appendErr.Error(), "checksum mismatch") {
				t.Fatalf("append after corrupt cold replay error = %v, want fail-closed checksum error", appendErr)
			}
			assertFileBytesEqual(t, activeTailPath, corruptTail)
			assertNoRecoveryBackup(t, activeTailPath)
			if err := reopened.Close(); err != nil {
				t.Fatal(err)
			}

			engine := runstate.NewScriptedEngine(runstate.EngineScript{})
			recoveryRuntime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
			if err != nil {
				t.Fatal(err)
			}
			if _, openErr := recoveryRuntime.Open(context.Background(), ref); openErr == nil || !strings.Contains(openErr.Error(), "checksum mismatch") {
				t.Fatalf("runtime cold open error = %v, want canonical active-tail checksum failure", openErr)
			}
			if len(engine.Requests()) != 0 {
				t.Fatalf("engine restarted after corrupt cold replay: %#v", engine.Requests())
			}
			assertFileBytesEqual(t, activeTailPath, corruptTail)
			if err := recoveryRuntime.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFileJournalRejectsCorruptNewestManifestAfterActiveTailAppend(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("manifest-fallback")
	store, journal := openCheckpointTestJournal(t, root, ref, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := runstate.NewJournalCheckpointState(ref)
	appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{runstate.CommandAcceptedEvent{CommandID: "one", CommandKind: "test", OperationID: "one", Fingerprint: "one"}})
	if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{runstate.CommandAcceptedEvent{CommandID: "two", CommandKind: "test", OperationID: "two", Fingerprint: "two"}})
	if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "after-newest-activation", CommandKind: "test", OperationID: "after-newest-activation", Fingerprint: "after-newest-activation"},
	})
	activeTailPath := journal.tailPath
	activeTail, err := os.ReadFile(activeTailPath)
	if err != nil {
		t.Fatal(err)
	}
	newestManifest := journal.generationManifestPath(journal.manifestSequence)
	if err := os.WriteFile(newestManifest, []byte(`{"version":1,"checksum":"broken"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	engine := runstate.NewScriptedEngine(runstate.EngineScript{})
	recoveryRuntime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, openErr := recoveryRuntime.Open(context.Background(), ref); openErr == nil ||
		!strings.Contains(openErr.Error(), "newest file journal generation manifest") {
		t.Fatalf("runtime cold open error = %v, want newest manifest corruption", openErr)
	}
	if len(engine.Requests()) != 0 {
		t.Fatalf("engine restarted after newest manifest corruption: %#v", engine.Requests())
	}
	assertFileBytesEqual(t, activeTailPath, activeTail)
	if err := recoveryRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func appendChecksumInvalidCompleteTransaction(t *testing.T, path string, cursor runstate.Cursor, payload runstate.EventPayload) {
	t.Helper()
	diskEvent, err := runstate.EncodeJournalEvent(runstate.Event{Cursor: cursor, Durability: runstate.EventDurable, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(journalTransaction{
		Version: journalVersion, Start: cursor, End: cursor, Events: []runstate.JournalEvent{diskEvent},
		Checksum: strings.Repeat("0", sha256HexLength),
	})
	if err != nil {
		t.Fatal(err)
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := append(append(existing[:len(existing):len(existing)], encoded...), '\n')
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFileBytesEqual(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file %s changed after fail-closed replay\n got: %q\nwant: %q", filepath.Base(path), got, want)
	}
}

func TestFileJournalCheckpointSwitchSurvivesEveryCrashStage(t *testing.T) {
	stages := []journalCheckpointStage{
		checkpointSnapshotDurable,
		checkpointTailDurable,
		checkpointManifestDurable,
		checkpointActivated,
		checkpointGarbageCollected,
	}
	for _, stage := range stages {
		stage := stage
		t.Run(string(stage), func(t *testing.T) {
			root := t.TempDir()
			ref := checkpointTestBinding("crash-" + string(stage))
			_, journal := openCheckpointTestJournal(t, root, ref, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
			state := runstate.NewJournalCheckpointState(ref)
			appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
				runstate.CommandAcceptedEvent{CommandID: "durable", CommandKind: "test", OperationID: "operation", Fingerprint: "durable"},
			})
			crash := errors.New("simulated process crash")
			journal.checkpointHook = func(current journalCheckpointStage) error {
				if current == stage {
					return crash
				}
				return nil
			}
			if err := journal.MaybeCheckpoint(context.Background(), state); !errors.Is(err, crash) {
				t.Fatalf("checkpoint error = %v, want simulated crash", err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}
			_, reopened := openCheckpointTestJournal(t, root, ref, Options{})
			defer reopened.Close()
			restored := runstate.NewJournalCheckpointState(ref)
			if _, err := reopened.ReplayCheckpoint(context.Background(), restored); err != nil {
				t.Fatalf("replay after %s: %v", stage, err)
			}
			if restored.Cursor() != 1 {
				t.Fatalf("cursor after %s = %d, want 1", stage, restored.Cursor())
			}
			record, found, err := reopened.LookupCommand(context.Background(), "durable")
			if err != nil || !found || record.Receipt.Cursor != 1 {
				t.Fatalf("idempotency after %s = %#v found=%v err=%v", stage, record, found, err)
			}
		})
	}
}

func TestFileJournalColdReplayUsesSnapshotAndBoundedTailAfterGenerationGC(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("bounded-cold-replay")
	options := Options{CheckpointTailRecords: 2, CheckpointTailBytes: 1 << 20}
	_, journal := openCheckpointTestJournal(t, root, ref, options)
	state := runstate.NewJournalCheckpointState(ref)
	for index := 0; index < 12; index++ {
		id := runstate.CommandID("command-" + testDecimal(index))
		appendAndReduceCheckpointEvents(t, journal, state, []runstate.EventPayload{
			runstate.CommandAcceptedEvent{CommandID: id, CommandKind: "test", OperationID: runstate.OperationID(id), Fingerprint: string(id)},
		})
		if err := journal.MaybeCheckpoint(context.Background(), state); err != nil {
			t.Fatal(err)
		}
	}
	if journal.activeGeneration.Generation < 2 {
		t.Fatalf("generation = %d, want multiple checkpoints", journal.activeGeneration.Generation)
	}
	if _, err := os.Stat(journal.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy segment still retained after generation GC: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	_, reopened := openCheckpointTestJournal(t, root, ref, options)
	t.Cleanup(func() { _ = reopened.Close() })
	restored := runstate.NewJournalCheckpointState(ref)
	stats, err := reopened.ReplayCheckpoint(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Cursor() != state.Cursor() || stats.SnapshotGeneration == 0 || stats.RecordsRead > options.CheckpointTailRecords {
		t.Fatalf("bounded cold replay state=%d stats=%#v", restored.Cursor(), stats)
	}
	commandFiles, err := filepath.Glob(journal.path + ".command-records/*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(commandFiles) != 12 {
		t.Fatalf("independent command records = %d, want 12", len(commandFiles))
	}
	for _, id := range []runstate.CommandID{"command-0", "command-5", "command-11"} {
		record, found, err := reopened.LookupCommand(context.Background(), id)
		if err != nil || !found || record.Fingerprint != string(id) {
			t.Fatalf("lookup %q across GC = %#v found=%v err=%v", id, record, found, err)
		}
	}
	manifests, _ := filepath.Glob(journal.path + ".generations.*.json")
	snapshots, _ := filepath.Glob(journal.path + ".snapshot.*.json")
	tails, _ := filepath.Glob(journal.path + ".tail.*.jsonl")
	if len(manifests) > 2 || len(snapshots) > 2 || len(tails) > 2 {
		t.Fatalf("generation GC retained manifests=%d snapshots=%d tails=%d", len(manifests), len(snapshots), len(tails))
	}
	for _, path := range append(append(manifests, snapshots...), tails...) {
		if strings.Contains(filepath.Base(path), ".tmp-") {
			t.Fatalf("temporary generation leaked: %s", path)
		}
	}
}

func TestRuntimeReopensFromCheckpointAndRejectsHistoricalDuplicateCommand(t *testing.T) {
	root := t.TempDir()
	store, err := NewStoreWithOptions(root, Options{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	command := runstate.StartTurn{ID: "checkpoint-command", Input: runstate.UserInput{Text: "write"}}
	firstRuntime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "done"}}}),
		store,
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := checkpointTestBinding("runtime-checkpoint-reopen")
	harness, err := firstRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	firstReceipt, err := harness.Submit(context.Background(), command)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	waitCheckpointOperationSettled(t, ctx, observation, firstReceipt.OperationID)
	cancel()
	if err := firstRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	secondRuntime, err := runstate.NewRuntime(runstate.NewScriptedEngine(), store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondRuntime.Close(context.Background()) })
	projected, err := secondRuntime.Project(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if projected.LastOperation == nil || projected.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("checkpoint projection = %#v", projected)
	}
	reopened, err := secondRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.CommandID != firstReceipt.CommandID || replayed.OperationID != firstReceipt.OperationID || replayed.Cursor != firstReceipt.Cursor {
		t.Fatalf("checkpoint duplicate receipt = %#v, want %#v", replayed, firstReceipt)
	}
}

func waitCheckpointOperationSettled(t *testing.T, ctx context.Context, observation runstate.Observation, operationID runstate.OperationID) {
	t.Helper()
	for {
		select {
		case event := <-observation.Events:
			if settled, ok := event.Payload.(runstate.OperationSettledEvent); ok && settled.OperationID == operationID {
				if settled.Status != runstate.OperationSucceeded {
					t.Fatalf("operation status = %q", settled.Status)
				}
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for checkpoint operation settlement: %v", ctx.Err())
		}
	}
}
