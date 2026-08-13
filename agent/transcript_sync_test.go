package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestSessionSyncTranscriptAtomicallyRebuildsCanonicalHistoryAcrossColdReopen(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("new answer", nil)}}
	store := &persistentMemoryStore{Store: agentsession.Memory()}
	owner, err := New(context.Background(), Definition{
		Model: model, ModelIdentity: CapabilityIdentity{Kind: "model.transcript-sync-test", Version: 1},
		Goal: admissionGoalManager{},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	key := NamedSession("canonical-story-branch")
	source := CapabilityIdentity{Kind: "test.story.main.transcript", Version: 1, ConfigHash: "story-main"}
	session, err := owner.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	goal, err := session.UpdateGoal(context.Background(), GoalMutation{
		Kind: GoalSet, Objective: "keep the long-running objective", MutationID: "sync-goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	setClearTestCapability(t, session, TodoCapability, TodoState{
		Revision: 1, Items: []TodoItem{{ID: "stale", Text: "old plan", Status: TodoPending}},
	})
	setClearTestCapability(t, session, cleanupCapability, CleanupState{
		ID: "stale-cleanup", Revision: 1, SourceRevision: "old", SourceHash: strings.Repeat("a", 64),
		SourceStart: 0, SourceEnd: 1,
		Replacements: []CleanupReplacement{{MessageIndex: 0, ToolCallID: "old", Placeholder: "[old]"}},
		Renderer:     "test", CreatedAt: now, UpdatedAt: now,
	})
	setClearTestCapability(t, session, compactionCapability, CompactionState{
		ID: "stale-compaction", Revision: 1, SourceRevision: "old", SourceHash: strings.Repeat("b", 64),
		Summary: "stale", ReplacementFrom: 0, ReplacementTo: 1, CreatedAt: now,
	})
	setClearTestCapability(t, session, compactionHealthCapability, compactionHealthState{
		Fingerprint: "stale", ConsecutiveFailures: 2, FailureCode: "stale",
	})
	if err := session.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}

	index := 0
	imported := []*Message{
		UserMessage("existing player action"),
		AssistantMessage("", []ToolCall{{Index: &index, ID: "read-world", Type: "function", Function: FunctionCall{Name: "read", Arguments: `{"path":"world.md"}`}}}),
		ToolMessage(TextToolResult("complete canonical lore evidence"), "read-world", WithToolName("read")),
		AssistantMessage("existing narrative", nil),
	}
	hash, err := TranscriptHash(imported)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 7, SourceHash: hash, Messages: imported,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || !result.Rebuilt || result.State.Revision != 1 || result.State.SourceRevision != 7 || result.State.SourceHash != hash {
		t.Fatalf("sync result=%#v", result)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TranscriptSync == nil || snapshot.TranscriptSync.Source != source || snapshot.TranscriptSync.SourceHash != hash ||
		snapshot.Todo != nil || snapshot.Cleanup != nil || snapshot.Compaction != nil {
		t.Fatalf("post-sync snapshot=%#v", snapshot)
	}
	if snapshot.ClearRevision != 0 {
		t.Fatalf("transcript generation retained stale Clear revision %d", snapshot.ClearRevision)
	}
	if snapshot.Goal == nil || snapshot.Goal.ID != goal.ID {
		t.Fatalf("Goal after transcript sync=%#v, want %#v", snapshot.Goal, goal)
	}
	for _, capability := range []string{TodoCapability, cleanupCapability, compactionCapability, compactionHealthCapability} {
		state, stateErr := session.harness.CapabilityState(context.Background(), capability)
		if stateErr != nil || state.Exists {
			t.Fatalf("reset capability %q state=%#v err=%v", capability, state, stateErr)
		}
	}
	clearCapabilityState, err := session.harness.CapabilityState(context.Background(), clearCapability)
	if err != nil || clearCapabilityState.Exists {
		t.Fatalf("transcript sync retained Clear capability=%#v err=%v", clearCapabilityState, err)
	}

	// A synchronized generation starts its own maintenance revisions. Revision
	// one must be visible even when the replaced generation had been cleared.
	setClearTestCapability(t, session, cleanupCapability, CleanupState{
		ID: "new-cleanup", Revision: 1, SourceRevision: "sync:7", SourceHash: hash,
		SourceStart: 2, SourceEnd: len(imported),
		Replacements: []CleanupReplacement{{MessageIndex: 2, ToolCallID: "read-world", Placeholder: "[recoverable evidence stored]"}},
		Renderer:     "test", CreatedAt: now, UpdatedAt: now,
	})
	setClearTestCapability(t, session, compactionCapability, CompactionState{
		ID: "new-compaction", Revision: 1, SourceRevision: "sync:7", SourceHash: hash,
		Summary: "new bounded checkpoint", ReplacementFrom: 0, ReplacementTo: 1, CreatedAt: now,
	})
	maintenanceSnapshot, err := session.Snapshot(context.Background())
	if err != nil || maintenanceSnapshot.Cleanup == nil || maintenanceSnapshot.Compaction == nil {
		t.Fatalf("new synchronized-generation maintenance is hidden: snapshot=%#v err=%v", maintenanceSnapshot, err)
	}
	// A normally settled product revision whose canonical projection already
	// equals Agent raw history advances provenance without discarding useful
	// maintenance state.
	advanced, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 8, SourceHash: hash, Messages: imported,
	})
	if err != nil || !advanced.Applied || advanced.Rebuilt || advanced.State.Revision != result.State.Revision+1 {
		t.Fatalf("forward provenance advance=%#v err=%v", advanced, err)
	}
	preserved, err := session.Snapshot(context.Background())
	if err != nil || preserved.Cleanup == nil || preserved.Compaction == nil {
		t.Fatalf("provenance advance discarded maintenance: snapshot=%#v err=%v", preserved, err)
	}

	// A clear/edit generation differs from canonical history and therefore does
	// a real rebuild, resetting old maintenance and the Clear boundary together.
	if err := session.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 9, SourceHash: hash, Messages: imported,
	})
	if err != nil || !rebuilt.Applied || !rebuilt.Rebuilt || rebuilt.State.Revision != result.State.Revision+2 {
		t.Fatalf("post-clear rebuild=%#v err=%v", rebuilt, err)
	}

	// Exact product retries are idempotent and do not create a new generation.
	replay, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 9, SourceHash: hash, Messages: imported,
	})
	if err != nil || replay.Applied || replay.Rebuilt || replay.State.Revision != result.State.Revision+2 {
		t.Fatalf("exact sync replay=%#v err=%v", replay, err)
	}
	changed := cloneMessages(imported)
	changed[0].Content = "different action at the same source revision"
	if _, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 9, Messages: changed,
	}); !errors.Is(err, ErrTranscriptSyncConflict) {
		t.Fatalf("same-revision changed content error=%v, want conflict", err)
	}
	if _, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source:         CapabilityIdentity{Kind: "test.other-story.transcript", Version: 1},
		SourceRevision: 10, Messages: imported,
	}); !errors.Is(err, ErrTranscriptSyncConflict) {
		t.Fatalf("changed source identity error=%v, want conflict", err)
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	session, err = owner.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := session.Snapshot(context.Background())
	if err != nil || reopened.TranscriptSync == nil || reopened.TranscriptSync.SourceHash != hash {
		t.Fatalf("cold snapshot=%#v err=%v", reopened, err)
	}
	run, err := session.Run(context.Background(), Input{Text: "continue the branch", IdempotencyKey: "after-sync"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("post-sync run=%#v err=%v", result, waitErr)
	}
	calls := model.calls()
	if len(calls) != 1 || len(calls[0]) != len(imported)+1 {
		t.Fatalf("model calls=%#v", calls)
	}
	for index := range imported {
		if calls[0][index].Role != imported[index].Role || calls[0][index].Content != imported[index].Content ||
			calls[0][index].ToolCallID != imported[index].ToolCallID {
			t.Fatalf("imported message %d changed: got=%#v want=%#v", index, calls[0][index], imported[index])
		}
	}
	if calls[0][len(imported)].Role != User || calls[0][len(imported)].Content != "continue the branch" {
		t.Fatalf("current model input=%#v", calls[0][len(imported)])
	}
}

func TestSessionSyncTranscriptRebuildsObsoleteInternalCheckpointOnExactCanonicalRetry(t *testing.T) {
	owner, err := New(context.Background(), Definition{
		Model:         &lifecycleModel{},
		ModelIdentity: CapabilityIdentity{Kind: "model.transcript-version-rebuild-test", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("transcript-version-rebuild"))
	if err != nil {
		t.Fatal(err)
	}
	source := CapabilityIdentity{Kind: "test.transcript-version-rebuild", Version: 1}
	canonical := []*Message{UserMessage("canonical product history")}
	initial, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 3, Messages: canonical,
	})
	if err != nil {
		t.Fatal(err)
	}
	setClearTestCapability(t, session, TodoCapability, TodoState{
		Revision: 1, Items: []TodoItem{{ID: "obsolete", Text: "discard with obsolete generation", Status: TodoPending}},
	})

	checkpoint, err := session.harness.EngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	obsolete, err := json.Marshal(engineTranscript{
		Version:  engineTranscriptVersion - 1,
		Messages: []*Message{UserMessage("obsolete private transcript")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.harness.ReplaceEngineCheckpoint(context.Background(), runstate.EngineCheckpointUpdate{
		ExpectedState: checkpoint.StateDescriptor,
		State:         obsolete,
	}); err != nil {
		t.Fatal(err)
	}

	rebuilt, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 3, Messages: canonical,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rebuilt.Applied || !rebuilt.Rebuilt || rebuilt.State.Revision != initial.State.Revision+1 {
		t.Fatalf("obsolete checkpoint rebuild=%#v", rebuilt)
	}
	current, err := session.harness.EngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := decodeEngineTranscript(current.State)
	if err != nil {
		t.Fatal(err)
	}
	if transcript.Version != engineTranscriptVersion || len(transcript.Messages) != 1 ||
		transcript.Messages[0].Content != canonical[0].Content {
		t.Fatalf("rebuilt transcript=%#v", transcript)
	}
	if _, exists := current.Capabilities[TodoCapability]; exists {
		t.Fatalf("obsolete transcript rebuild retained %q", TodoCapability)
	}
}

func TestSessionSyncTranscriptPreservesAgentOwnedResponseMetadataAndMaintenance(t *testing.T) {
	response := AssistantMessage("canonical answer", nil)
	response.ReasoningContent = "provider-owned reasoning evidence"
	response.ResponseMeta = &ResponseMeta{
		FinishReason: "stop",
		Usage:        &TokenUsage{PromptTokens: 21, CompletionTokens: 5, TotalTokens: 26},
	}
	owner, err := New(context.Background(), Definition{
		Model:         &lifecycleModel{responses: []*Message{response}},
		ModelIdentity: CapabilityIdentity{Kind: "model.transcript-source-equivalence-test", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("canonical-source-equivalence"))
	if err != nil {
		t.Fatal(err)
	}
	source := CapabilityIdentity{Kind: "test.canonical-source-equivalence", Version: 1}
	if _, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 1, Messages: []*Message{UserMessage("existing product history")},
	}); err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{
		Text: "new product input", IdempotencyKey: "canonical-source-equivalence-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("seed run=%#v err=%v", result, waitErr)
	}

	now := time.Date(2026, time.August, 12, 18, 0, 0, 0, time.UTC)
	setClearTestCapability(t, session, cleanupCapability, CleanupState{
		ID: "active-cleanup", Revision: 1, SourceRevision: "run:1", SourceHash: strings.Repeat("a", 64),
		SourceStart: 0, SourceEnd: 1,
		Replacements: []CleanupReplacement{{MessageIndex: 0, ToolCallID: "source-equivalence", Placeholder: "[cleaned]"}},
		Renderer:     "test", CreatedAt: now, UpdatedAt: now,
	})
	setClearTestCapability(t, session, compactionCapability, CompactionState{
		ID: "active-compaction", Revision: 1, SourceRevision: "run:1", SourceHash: strings.Repeat("b", 64),
		Summary: "active checkpoint", ReplacementFrom: 0, ReplacementTo: 1, CreatedAt: now,
	})
	canonical := []*Message{
		UserMessage("existing product history"),
		UserMessage("new product input"),
		AssistantMessage("canonical answer", nil),
	}
	advanced, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 2, Messages: canonical,
	})
	if err != nil || !advanced.Applied || advanced.Rebuilt {
		t.Fatalf("source-equivalent provenance advance=%#v err=%v", advanced, err)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil || snapshot.Cleanup == nil || snapshot.Compaction == nil {
		t.Fatalf("source-equivalent advance discarded maintenance: snapshot=%#v err=%v", snapshot, err)
	}
	checkpoint, err := session.harness.EngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := decodeEngineTranscript(checkpoint.State)
	if err != nil || len(transcript.Messages) != len(canonical) {
		t.Fatalf("raw transcript=%#v err=%v", transcript.Messages, err)
	}
	assistant := transcript.Messages[len(transcript.Messages)-1]
	if assistant.ReasoningContent != "provider-owned reasoning evidence" ||
		assistant.AgentMeta == nil || assistant.AgentMeta.ModelResponseOrdinal != 1 ||
		assistant.ResponseMeta == nil || assistant.ResponseMeta.Usage == nil || assistant.ResponseMeta.Usage.TotalTokens != 26 {
		t.Fatalf("Agent-owned response metadata was overwritten: %#v", assistant)
	}
}

func TestSessionSyncTranscriptRejectsBusyAndIncompleteToolBatchWithoutPartialMutation(t *testing.T) {
	model := newGatedLifecycleModel()
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("sync-busy"))
	if err != nil {
		t.Fatal(err)
	}
	source := CapabilityIdentity{Kind: "test.sync-busy.transcript", Version: 1}
	if _, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		SourceRevision: 1, Messages: []*Message{UserMessage("missing source identity")},
	}); err == nil || !strings.Contains(err.Error(), "source capability identity is incomplete") {
		t.Fatalf("missing source identity error=%v", err)
	}
	if _, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 1,
		Messages: []*Message{AssistantMessage("", []ToolCall{{ID: "half", Type: "function", Function: FunctionCall{Name: "read"}}})},
	}); err == nil || !strings.Contains(err.Error(), "incomplete tool-result batch") {
		t.Fatalf("incomplete tool batch error=%v", err)
	}
	initial, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 1, Messages: []*Message{UserMessage("stable imported base")},
	})
	if err != nil || !initial.Applied {
		t.Fatalf("initial sync=%#v err=%v", initial, err)
	}
	run, err := session.Run(context.Background(), Input{Text: "block", IdempotencyKey: "sync-busy-run"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	if _, err := session.SyncTranscript(context.Background(), TranscriptSyncRequest{
		Source: source, SourceRevision: 2, Messages: []*Message{UserMessage("must not partially replace")},
	}); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("busy SyncTranscript error=%v, want ErrSessionBusy", err)
	}
	checkpoint, err := session.harness.EngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state, err := decodeEngineTranscript(checkpoint.State)
	if err != nil || len(state.Messages) != 1 || state.Messages[0].Content != "stable imported base" {
		t.Fatalf("busy sync mutated transcript=%#v err=%v", state.Messages, err)
	}
	syncState, present, err := transcriptSyncStateFrom(checkpoint.Capabilities)
	if err != nil || !present || syncState.SourceRevision != 1 {
		t.Fatalf("busy sync state=%#v present=%t err=%v", syncState, present, err)
	}
	if _, err := run.Abort(context.Background(), AbortRequest{Reason: "finish sync busy test"}); err != nil {
		t.Fatal(err)
	}
	model.responses <- AssistantMessage("ignored", nil)
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultAborted {
		t.Fatalf("aborted run=%#v err=%v", result, waitErr)
	}
}
