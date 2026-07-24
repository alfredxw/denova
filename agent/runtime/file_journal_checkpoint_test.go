package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func checkpointTestBinding(sessionID string) BindingRef {
	return testBinding(sessionID)
}

func openCheckpointTestJournal(t *testing.T, root string, ref BindingRef, options FileJournalOptions) (*FileJournalStore, *fileJournal) {
	t.Helper()
	store, err := NewFileJournalStoreWithOptions(root, options)
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
	journal := opened.(*fileJournal)
	return store, journal
}

func appendAndReduceCheckpointEvents(t *testing.T, journal *fileJournal, state *harnessState, payloads []EventPayload) {
	t.Helper()
	events, err := journal.Append(context.Background(), state.cursor, payloads)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := state.reduce(event); err != nil {
			t.Fatalf("reduce cursor %d: %v", event.Cursor, err)
		}
	}
}

func TestFileJournalCheckpointRestoresAllPendingObligations(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("checkpoint-obligations")
	_, journal := openCheckpointTestJournal(t, root, ref, FileJournalOptions{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := newHarnessState(ref)
	state.maxRetainedEvents = 128
	state.maxRetainedMessages = 128
	state.maxRetainedCommands = 128
	effect, err := NewToolHostEffect(ref, "operation", 1, "finished-call", 0, HostEffectToolMutationCommitted, json.RawMessage(`{"version":1,"mutation":"pending"}`))
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "start", OperationID: "operation", Cycle: 1, Stage: DomainCommitOutput}
	appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: "operation", Fingerprint: "start-fingerprint"},
		OperationStartedEvent{OperationID: "operation"},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "write", Input: UserInput{Text: "write", RestoreDescriptor: json.RawMessage(`{"restore":1}`)}, Operation: "operation"}},
		CycleStartedEvent{OperationID: "operation", Cycle: 1, SnapshotID: "snapshot"},
		CommandAcceptedEvent{CommandID: "follow", CommandKind: "follow_up", OperationID: "operation", Fingerprint: "follow-fingerprint"},
		QueueEnqueuedEvent{Item: QueuedInput{CommandID: "follow", OperationID: "operation", Delivery: DeliveryFollowUp, Input: UserInput{Text: "continue", RestoreDescriptor: json.RawMessage(`{"restore":2}`)}}},
		ToolCallStartedEvent{Call: ToolCallState{CallID: "open-call", Name: "open", OperationID: "operation", Cycle: 1}},
		ToolCallStartedEvent{Call: ToolCallState{CallID: "finished-call", Name: "mutate", OperationID: "operation", Cycle: 1}},
		ToolCallFinishedEvent{CallID: "finished-call", Name: "mutate", HostEffects: []HostEffect{effect}},
		DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "domain-hash"},
		InputMaterializationRecoveryPendingEvent{OperationID: "operation", Cycle: 1, CommandID: "start", Delivery: DeliveryFollowUp},
	})
	state.activeContent.WriteString("partial content")
	state.activeThinking.WriteString("partial thinking")
	if err := journal.MaybeCheckpoint(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if journal.activeGeneration.SnapshotFile == "" {
		t.Fatal("checkpoint generation was not activated")
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	_, reopened := openCheckpointTestJournal(t, root, ref, FileJournalOptions{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	t.Cleanup(func() { _ = reopened.Close() })
	restored := newHarnessState(ref)
	restored.maxRetainedEvents, restored.maxRetainedMessages, restored.maxRetainedCommands = 128, 128, 128
	stats, err := reopened.ReplayHarnessState(context.Background(), &restored)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SnapshotGeneration == 0 || stats.RecordsRead != 0 || stats.TailBytesRead != 0 {
		t.Fatalf("cold replay stats = %#v, want snapshot plus empty bounded tail", stats)
	}
	if restored.inputRecovery == nil || len(restored.queue) != 1 || len(restored.openToolCalls) != 1 ||
		len(restored.pendingHostEffects) != 1 || restored.pendingDomainCommit() == nil {
		t.Fatalf("checkpoint lost obligations: input=%#v queue=%#v tools=%#v effects=%#v domains=%#v",
			restored.inputRecovery, restored.queue, restored.openToolCalls, restored.pendingHostEffects, restored.domainCommits)
	}
	if string(restored.activeInput.RestoreDescriptor) != `{"restore":1}` || string(restored.queue[0].Input.RestoreDescriptor) != `{"restore":2}` {
		t.Fatalf("private restore descriptors were not restored: active=%s queued=%s", restored.activeInput.RestoreDescriptor, restored.queue[0].Input.RestoreDescriptor)
	}
	if got := string(restored.pendingHostEffects[effect.ID].Payload); got != string(effect.Payload) {
		t.Fatalf("pending host effect payload = %s, want %s", got, effect.Payload)
	}
	if restored.activeContent.String() != "partial content" || restored.activeThinking.String() != "partial thinking" || !restored.activeOutputRehydrated {
		t.Fatalf("active stream recovery = content %q thinking %q rehydrate=%v", restored.activeContent.String(), restored.activeThinking.String(), restored.activeOutputRehydrated)
	}
}

func TestFileJournalCheckpointCorruptionFallsBackToPreviousGeneration(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("checkpoint-fallback")
	_, journal := openCheckpointTestJournal(t, root, ref, FileJournalOptions{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := newHarnessState(ref)
	appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{
		CommandAcceptedEvent{CommandID: "one", CommandKind: "test", OperationID: "operation-one", Fingerprint: "one"},
	})
	if err := journal.MaybeCheckpoint(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{
		CommandAcceptedEvent{CommandID: "two", CommandKind: "test", OperationID: "operation-two", Fingerprint: "two"},
	})
	if err := journal.MaybeCheckpoint(context.Background(), &state); err != nil {
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

	_, reopened := openCheckpointTestJournal(t, root, ref, FileJournalOptions{})
	t.Cleanup(func() { _ = reopened.Close() })
	restored := newHarnessState(ref)
	stats, err := reopened.ReplayHarnessState(context.Background(), &restored)
	if err != nil {
		t.Fatalf("fallback replay: %v", err)
	}
	if restored.cursor != state.cursor || stats.SnapshotGeneration == 0 || stats.SnapshotGeneration == journal.activeGeneration.Generation {
		t.Fatalf("fallback state cursor=%d stats=%#v active=%d", restored.cursor, stats, journal.activeGeneration.Generation)
	}
	for _, commandID := range []CommandID{"one", "two"} {
		record, found, err := reopened.LookupCommand(context.Background(), commandID)
		if err != nil || !found || record.Receipt.CommandID != commandID {
			t.Fatalf("command %q after snapshot fallback = %#v found=%v err=%v", commandID, record, found, err)
		}
	}
}

func TestFileJournalCorruptActiveSnapshotReplaysCurrentGenerationTailFromPrevious(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("checkpoint-fallback-with-tail")
	_, journal := openCheckpointTestJournal(t, root, ref, FileJournalOptions{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := newHarnessState(ref)
	for _, id := range []CommandID{"one", "two"} {
		appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{
			CommandAcceptedEvent{CommandID: id, CommandKind: "test", OperationID: OperationID(id), Fingerprint: string(id)},
		})
		if err := journal.MaybeCheckpoint(context.Background(), &state); err != nil {
			t.Fatal(err)
		}
	}
	appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{
		CommandAcceptedEvent{CommandID: "after-checkpoint", CommandKind: "test", OperationID: "after-checkpoint", Fingerprint: "after-checkpoint"},
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

	_, reopened := openCheckpointTestJournal(t, root, ref, FileJournalOptions{})
	t.Cleanup(func() { _ = reopened.Close() })
	restored := newHarnessState(ref)
	stats, err := reopened.ReplayHarnessState(context.Background(), &restored)
	if err != nil {
		t.Fatal(err)
	}
	if restored.cursor != state.cursor || stats.SnapshotGeneration == journal.activeGeneration.Generation {
		t.Fatalf("composed fallback cursor=%d stats=%#v, want cursor=%d from previous snapshot", restored.cursor, stats, state.cursor)
	}
	record, found, err := reopened.LookupCommand(context.Background(), "after-checkpoint")
	if err != nil || !found || record.Receipt.Cursor != state.cursor {
		t.Fatalf("current generation tail command = %#v found=%v err=%v", record, found, err)
	}
}

func TestFileJournalCorruptNewestManifestFallsBackToPriorManifest(t *testing.T) {
	root := t.TempDir()
	ref := checkpointTestBinding("manifest-fallback")
	_, journal := openCheckpointTestJournal(t, root, ref, FileJournalOptions{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	state := newHarnessState(ref)
	appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{CommandAcceptedEvent{CommandID: "one", CommandKind: "test", OperationID: "one", Fingerprint: "one"}})
	if err := journal.MaybeCheckpoint(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{CommandAcceptedEvent{CommandID: "two", CommandKind: "test", OperationID: "two", Fingerprint: "two"}})
	if err := journal.MaybeCheckpoint(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	newestManifest := journal.generationManifestPath(journal.manifestSequence)
	if err := os.WriteFile(newestManifest, []byte(`{"version":1,"checksum":"broken"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	_, reopened := openCheckpointTestJournal(t, root, ref, FileJournalOptions{})
	t.Cleanup(func() { _ = reopened.Close() })
	restored := newHarnessState(ref)
	if _, err := reopened.ReplayHarnessState(context.Background(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.cursor != state.cursor {
		t.Fatalf("prior manifest replay cursor = %d, want %d", restored.cursor, state.cursor)
	}
}

func TestFileJournalCheckpointSwitchSurvivesEveryCrashStage(t *testing.T) {
	stages := []fileJournalCheckpointStage{
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
			_, journal := openCheckpointTestJournal(t, root, ref, FileJournalOptions{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
			state := newHarnessState(ref)
			appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{
				CommandAcceptedEvent{CommandID: "durable", CommandKind: "test", OperationID: "operation", Fingerprint: "durable"},
			})
			crash := errors.New("simulated process crash")
			journal.checkpointHook = func(current fileJournalCheckpointStage) error {
				if current == stage {
					return crash
				}
				return nil
			}
			if err := journal.MaybeCheckpoint(context.Background(), &state); !errors.Is(err, crash) {
				t.Fatalf("checkpoint error = %v, want simulated crash", err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}
			_, reopened := openCheckpointTestJournal(t, root, ref, FileJournalOptions{})
			defer reopened.Close()
			restored := newHarnessState(ref)
			if _, err := reopened.ReplayHarnessState(context.Background(), &restored); err != nil {
				t.Fatalf("replay after %s: %v", stage, err)
			}
			if restored.cursor != 1 {
				t.Fatalf("cursor after %s = %d, want 1", stage, restored.cursor)
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
	options := FileJournalOptions{CheckpointTailRecords: 2, CheckpointTailBytes: 1 << 20}
	_, journal := openCheckpointTestJournal(t, root, ref, options)
	state := newHarnessState(ref)
	for index := 0; index < 12; index++ {
		id := CommandID("command-" + testDecimal(index))
		appendAndReduceCheckpointEvents(t, journal, &state, []EventPayload{
			CommandAcceptedEvent{CommandID: id, CommandKind: "test", OperationID: OperationID(id), Fingerprint: string(id)},
		})
		if err := journal.MaybeCheckpoint(context.Background(), &state); err != nil {
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
	restored := newHarnessState(ref)
	stats, err := reopened.ReplayHarnessState(context.Background(), &restored)
	if err != nil {
		t.Fatal(err)
	}
	if restored.cursor != state.cursor || stats.SnapshotGeneration == 0 || stats.RecordsRead > options.CheckpointTailRecords {
		t.Fatalf("bounded cold replay state=%d stats=%#v", restored.cursor, stats)
	}
	commandFiles, err := filepath.Glob(journal.path + ".command-records/*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(commandFiles) != 12 {
		t.Fatalf("independent command records = %d, want 12", len(commandFiles))
	}
	for _, id := range []CommandID{"command-0", "command-5", "command-11"} {
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
	store, err := NewFileJournalStoreWithOptions(root, FileJournalOptions{CheckpointTailRecords: 1, CheckpointTailBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	command := StartTurn{ID: "checkpoint-command", Input: UserInput{Text: "write"}}
	firstRuntime, err := NewRuntime(
		NewScriptedEngine(EngineScript{Events: []EngineEvent{EngineAssistantFinal{Content: "done"}}}),
		store,
		RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "runtime-checkpoint-reopen")
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

	secondRuntime, err := NewRuntime(NewScriptedEngine(), store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondRuntime.Close(context.Background()) })
	projected, err := secondRuntime.Project(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if projected.LastOperation == nil || projected.LastOperation.Status != OperationSucceeded {
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

func waitCheckpointOperationSettled(t *testing.T, ctx context.Context, observation Observation, operationID OperationID) {
	t.Helper()
	for {
		select {
		case event := <-observation.Events:
			if settled, ok := event.Payload.(OperationSettledEvent); ok && settled.OperationID == operationID {
				if settled.Status != OperationSucceeded {
					t.Fatalf("operation status = %q", settled.Status)
				}
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for checkpoint operation settlement: %v", ctx.Err())
		}
	}
}
