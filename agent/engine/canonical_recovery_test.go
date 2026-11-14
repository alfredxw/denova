package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent/context/history"
	"github.com/alfredxw/denova/agent/schema"
	"github.com/alfredxw/denova/agent/session/canonical"
)

type recoveryHistory struct {
	head     canonical.CanonicalHistoryHead
	messages []*schema.Message
}

func TestContinuationKeepsContextStateOrderAcrossCanonicalRecovery(t *testing.T) {
	feedback := schema.UserMessage("Continue the accepted work")
	feedback.Extra = map[string]any{schema.ExtraKeyAgentContinuation: true}
	stateMessage := history.NewContextStateMessage(schema.ContextFragment{
		Source: "workspace", Purpose: "current workspace", Resource: "workspace", StateID: "workspace", Revision: "1",
		Stability: schema.ContextSessionState, Placement: schema.ContextStateMessage, Content: "restored context", HardLimit: 1024,
	}, strings.Repeat("a", 64), "initialize", "")
	source := recoveryHistory{head: canonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}, messages: []*schema.Message{
		schema.UserMessage("original input"), schema.AssistantMessage("accepted prose", nil), feedback, stateMessage,
	}}
	contextState, err := history.RebuildContextStateSnapshot(source.messages)
	if err != nil {
		t.Fatal(err)
	}
	state := engineTranscript{Version: 1, HistoryHead: source.head, Messages: source.messages, ContextState: contextState}
	checkpoint, err := canonicalMessageCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	if got := canonical.CanonicalContextStateOrder(source.messages); !reflect.DeepEqual(got, source.messages) {
		t.Error("continuation feedback was reordered as a newly admitted user input")
	}
	raw, matched, err := RestoreCanonicalCheckpoint(t.Context(), checkpoint, source.head, source)
	if err != nil || !matched {
		t.Fatalf("streaming recovery changed continuation order: matched=%t error=%v", matched, err)
	}
	restored, err := decodeEngineTranscript(raw)
	if err != nil || !reflect.DeepEqual(restored.Messages, source.messages) {
		t.Fatalf("streaming recovery changed accepted messages: %v", err)
	}
	imported, err := PrepareCanonicalImport(t.Context(), CanonicalImportRequest{Messages: source.messages, Head: source.head, Checkpoint: checkpoint, Active: true, Canonical: true})
	if err != nil {
		t.Fatalf("canonical import changed continuation order: %v", err)
	}
	importedState, err := decodeEngineTranscript(imported.State)
	if err != nil || !reflect.DeepEqual(importedState.Messages, source.messages) {
		t.Fatalf("canonical import changed accepted messages: %v", err)
	}
}

func (s recoveryHistory) CanonicalHistoryHead(context.Context) (canonical.CanonicalHistoryHead, error) {
	return s.head, nil
}
func (s recoveryHistory) CanonicalMessages(context.Context) ([]*schema.Message, error) {
	panic("recovery must use the streaming visitor")
}
func (s recoveryHistory) VisitCanonicalMessages(_ context.Context, visit func(*schema.Message) error) error {
	for _, message := range s.messages {
		if err := visit(message.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func TestRestoreCanonicalCheckpointIgnoresProviderTelemetry(t *testing.T) {
	head := canonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}
	continuation := map[string]any{"payload": []any{map[string]any{"encrypted_content": "token"}}}
	// In-memory transcript carries provider telemetry on the assistant message.
	withTelemetry := []*schema.Message{
		schema.UserMessage("current user"),
		func() *schema.Message {
			m := schema.AssistantMessage("narrative", nil)
			m.Extra = map[string]any{
				"provider":              "openai",
				"protocol":              "openai-responses",
				"response_id":           "resp-1",
				"model":                 "gemini-3.8-flash-high",
				"created":               int64(1700000000),
				"response_status":       "completed",
				"provider-continuation": continuation,
			}
			return m
		}(),
	}
	// The reconstructed canonical window strips telemetry from the narrative,
	// exactly as ModelTurnMessages does, keeping only the continuation.
	reconstructed := []*schema.Message{
		schema.UserMessage("current user"),
		func() *schema.Message {
			m := schema.AssistantMessage("narrative", nil)
			m.Extra = map[string]any{"provider-continuation": continuation}
			return m
		}(),
	}
	state := engineTranscript{Version: 2, HistoryHead: head, Messages: withTelemetry}
	checkpoint, err := canonicalMessageCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	source := recoveryHistory{head: head, messages: reconstructed}
	raw, matched, err := RestoreCanonicalCheckpoint(t.Context(), checkpoint, head, source)
	if err != nil || !matched {
		t.Fatalf("restore rejected provider telemetry divergence: matched=%t err=%v", matched, err)
	}
	restored, err := decodeEngineTranscript(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Messages, reconstructed) {
		t.Fatalf("restored messages differ: %#v", restored.Messages)
	}
}

func TestRestoreCanonicalCheckpointAcceptsReleasedInlineMetadata(t *testing.T) {
	source := recoveryHistory{head: canonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}, messages: []*schema.Message{
		schema.UserMessage("archived user"), schema.AssistantMessage("archived answer", nil),
		schema.UserMessage("current user"), schema.AssistantMessage("current answer", nil),
	}}
	contextState := schema.UserMessage("current workspace context")
	contextState.Extra = map[string]any{"agent.context_state": "v1"}
	source.messages = append(source.messages[:3], contextState, source.messages[3])
	state := engineTranscript{Version: 2, HistoryHead: source.head, Archive: &history.HistoryArchive{From: 0, To: 2}, Messages: canonical.CanonicalContextStateOrder(source.messages)[2:]}
	checkpoint, err := canonicalMessageCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, released := range []bool{false, true} {
		t.Run(map[bool]string{false: "message-free", true: "released-inline"}[released], func(t *testing.T) {
			if released {
				checkpoint.Metadata, err = json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
			}
			raw, matched, err := RestoreCanonicalCheckpoint(t.Context(), checkpoint, source.head, source)
			if err != nil || !matched {
				t.Fatalf("restore: matched=%t err=%v", matched, err)
			}
			restored, err := decodeEngineTranscript(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored, state) {
				t.Fatalf("restored transcript differs: %#v", restored)
			}
			fresh, err := CanonicalMessageCheckpoint(raw)
			if err != nil {
				t.Fatal(err)
			}
			var metadata engineTranscript
			if err := json.Unmarshal(fresh.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			if len(metadata.Messages) != 0 {
				t.Fatal("new checkpoint repeated released message bodies")
			}
		})
	}
}

// A persisted fingerprint that no longer matches the canonical bodies only
// invalidates the cached window; the journal is still complete and readable.
func TestRestoreCanonicalCheckpointDeclinesChangedContent(t *testing.T) {
	head := canonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}
	state := engineTranscript{Version: 2, HistoryHead: head, Messages: []*schema.Message{
		schema.UserMessage("original user"), schema.AssistantMessage("original answer", nil),
	}}
	checkpoint, err := canonicalMessageCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	source := recoveryHistory{head: head, messages: []*schema.Message{
		schema.UserMessage("original user"), schema.AssistantMessage("repaired answer", nil),
	}}
	raw, matched, err := RestoreCanonicalCheckpoint(t.Context(), checkpoint, head, source)
	if err != nil || matched || raw != nil {
		t.Fatalf("changed content: raw=%s matched=%t err=%v", raw, matched, err)
	}
}

// A window implementation that cannot honour the checkpoint coordinates must
// not fail the conversation either.
func TestRestoreCanonicalCheckpointDeclinesRefusedWindow(t *testing.T) {
	head := canonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}
	state := engineTranscript{Version: 2, HistoryHead: head, Messages: []*schema.Message{
		schema.UserMessage("original user"), schema.AssistantMessage("original answer", nil),
	}}
	checkpoint, err := canonicalMessageCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	source := refusedWindowHistory{recoveryHistory{head: head, messages: state.Messages}}
	raw, matched, err := RestoreCanonicalCheckpoint(t.Context(), checkpoint, head, source)
	if err != nil || matched || raw != nil {
		t.Fatalf("refused window: raw=%s matched=%t err=%v", raw, matched, err)
	}
}

type refusedWindowHistory struct{ recoveryHistory }

func (refusedWindowHistory) ReadCanonicalWindow(context.Context, int, *history.HistoryArchive) ([]*schema.Message, error) {
	return nil, fmt.Errorf("%w: aligned Game message count changed (1 != 2)", canonical.ErrInvalidCanonicalMessages)
}

func TestLegacyUnversionedCheckpointWithTelemetryRestoresAndMigrates(t *testing.T) {
	head := canonical.CanonicalHistoryHead{Identity: "lane", Revision: "4"}
	retainedAnswer := schema.AssistantMessage("recent prose", nil)
	retainedAnswer.Extra = map[string]any{
		"provider":              "openai",
		"protocol":              "openai-responses",
		"response_id":           "resp-legacy-1",
		"model":                 "gpt-5",
		"created":               int64(1700000123),
		"response_status":       "completed",
		"provider-continuation": map[string]any{"payload": []any{map[string]any{"encrypted_content": "tok"}}},
	}
	fullMessages := []*schema.Message{
		schema.UserMessage("archived chapter prompt"),
		schema.AssistantMessage("archived chapter body", nil),
		schema.UserMessage("recent prompt"),
		retainedAnswer,
	}
	archive := &history.HistoryArchive{From: 0, To: 2}
	windowMessages := fullMessages[2:]
	legacyHash, err := schema.HashCanonical(windowMessages)
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := schema.HashCanonicalMessages(windowMessages)
	if err != nil {
		t.Fatal(err)
	}
	if legacyHash == newHash {
		t.Fatal("test fixture requires legacy hash with telemetry to differ from telemetry-stripped hash")
	}
	metadataState := engineTranscript{Version: 2, HistoryHead: head, Archive: archive}
	metadataBytes, err := json.Marshal(metadataState)
	if err != nil {
		t.Fatal(err)
	}
	legacyCheckpoint := PersistedMessageCheckpoint{
		Version:      messageCheckpointLegacyHashVersion,
		Archive:      archive,
		Hash:         legacyHash,
		MessageCount: len(fullMessages),
		Metadata:     metadataBytes,
	}

	source := recoveryHistory{head: head, messages: fullMessages}
	raw, matched, err := RestoreCanonicalCheckpoint(t.Context(), legacyCheckpoint, head, source)
	if err != nil || !matched {
		t.Fatalf("RestoreCanonicalCheckpoint rejected legacy telemetry checkpoint: matched=%t err=%v", matched, err)
	}
	restored, err := decodeEngineTranscript(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Archive, archive) || len(restored.Messages) != 2 || restored.Messages[1].Content != "recent prose" {
		t.Fatalf("unexpected restored legacy transcript: %#v", restored)
	}

	// Simulate an append-only advance or fallback import where PrepareCanonicalImport
	// validates the legacy checkpoint and preserves CompactionCapability while migrating
	// the checkpoint to the current version.
	compactionRaw, err := json.Marshal(history.CompactionRecord{
		Version:         2,
		ID:              "compact-legacy",
		Revision:        1,
		Summary:         "Legacy compacted summary",
		ReplacementFrom: 0,
		ReplacementTo:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	appended := append(schema.CloneMessages(fullMessages), schema.UserMessage("continue writing"))
	imported, err := PrepareCanonicalImport(t.Context(), CanonicalImportRequest{
		Messages:     appended,
		Head:         canonical.CanonicalHistoryHead{Identity: head.Identity, Revision: "5"},
		Checkpoint:   legacyCheckpoint,
		Capabilities: map[string]json.RawMessage{history.CompactionCapability: compactionRaw},
		Canonical:    true,
	})
	if err != nil {
		t.Fatalf("PrepareCanonicalImport failed on legacy checkpoint: %v", err)
	}
	if len(imported.Invalidated) != 0 {
		t.Fatalf("PrepareCanonicalImport invalidated legacy capabilities: %v", imported.Invalidated)
	}
	if imported.Checkpoint.Version != messageCheckpointCanonicalHashVersion {
		t.Fatalf("migrated checkpoint version = %d, want %d", imported.Checkpoint.Version, messageCheckpointCanonicalHashVersion)
	}
	importedState, err := decodeEngineTranscript(imported.State)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(importedState.Archive, archive) || len(importedState.Messages) != 3 {
		t.Fatalf("migrated state lost archive or messages: %#v", importedState)
	}

	// Future unknown versions must be declined safely without failing.
	futureCheckpoint := legacyCheckpoint
	futureCheckpoint.Version = 99
	rawFuture, matchedFuture, err := RestoreCanonicalCheckpoint(t.Context(), futureCheckpoint, head, source)
	if err != nil || matchedFuture || rawFuture != nil {
		t.Fatalf("future checkpoint version was not declined cleanly: raw=%s matched=%t err=%v", rawFuture, matchedFuture, err)
	}
}
