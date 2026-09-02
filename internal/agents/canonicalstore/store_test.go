package canonicalstore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	"denova/internal/interactive"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestStoreEmbedsRootRecordsAndKeepsChildrenInProjectState(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(dataDir)
	record, err := registry.Add(workspace, project.TypeGeneral, "Journal test")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(record)
	if err != nil {
		t.Fatal(err)
	}
	productStore, err := productsession.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	productSession, err := productStore.GetOrCreate("session-one")
	if err != nil {
		t.Fatal(err)
	}
	if err := productSession.Append(agent.UserMessage("canonical input")); err != nil {
		t.Fatal(err)
	}
	if err := productStore.Close(); err != nil {
		t.Fatal(err)
	}
	key, err := (agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindIDE, ProjectID: record.ID, SessionID: productSession.ID,
	}).AgentSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(dataDir, registry)
	if err != nil {
		t.Fatal(err)
	}
	log, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if canonical, ok := log.(agentsession.CanonicalMessageLog); !ok || !canonical.CanonicalMessages() {
		t.Fatal("root Agent Session did not use the product canonical message lane")
	}
	if _, err := log.Append(ctx, 0, agentsession.Record{
		Kind: "session.transcript", Version: 1, Data: json.RawMessage(`{"engine_state":{}}`),
	}); err == nil {
		t.Fatal("embedded root accepted a second transcript authority")
	}
	state := json.RawMessage(`{"capability":"agent.todo","state":{"revision":1}}`)
	if _, err := log.Append(ctx, 0, agentsession.Record{Kind: "session.capability_set", Version: 1, Data: state}); err != nil {
		t.Fatal(err)
	}
	deleted := json.RawMessage(`{"capability":"agent.todo"}`)
	if _, err := log.Append(ctx, 1, agentsession.Record{Kind: "session.capability_delete", Version: 1, Data: deleted}); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(layout.SessionsDir(), productSession.ID+".jsonl")
	journal, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(journal), `"kind":"session.capability_set"`) != 1 ||
		strings.Count(string(journal), `"kind":"session.capability_delete"`) != 1 ||
		strings.Contains(string(journal), `"kind":"session.transcript"`) {
		t.Fatalf("product journal did not contain only the embedded lifecycle record: %s", journal)
	}
	indexPath := strings.TrimSuffix(journalPath, filepath.Ext(journalPath)) + ".idx.json"
	if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	rebuilt, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	var rebuiltRecords []agentsession.Record
	if _, err := rebuilt.Replay(ctx, func(record agentsession.Record) error {
		rebuiltRecords = append(rebuiltRecords, record)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := rebuilt.Close(); err != nil {
		t.Fatal(err)
	}
	if len(rebuiltRecords) != 1 || rebuiltRecords[0].Revision != 2 || rebuiltRecords[0].Kind != "session.capability_delete" {
		t.Fatalf("rebuilt Agent projection = %#v", rebuiltRecords)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "agent-transcripts")); !os.IsNotExist(err) {
		t.Fatalf("obsolete agent-transcripts directory exists: %v", err)
	}

	attributes, err := agent.ChildSessionAttributes(key)
	if err != nil {
		t.Fatal(err)
	}
	attributes["agent"] = "researcher"
	child := agentsession.Key{Namespace: "task.researcher", ID: "child-one", Attributes: attributes}
	childLog, err := store.Open(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	if canonical, ok := childLog.(agentsession.CanonicalMessageLog); ok && canonical.CanonicalMessages() {
		t.Fatal("self-contained child unexpectedly delegated canonical messages")
	}
	if _, err := childLog.Append(ctx, 0, agentsession.Record{
		Kind: "turn.started", Version: 1,
		Data: json.RawMessage(`{"run_id":"child-run","command_id":"child-command","at":"2026-09-02T00:00:00Z"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := childLog.Close(); err != nil {
		t.Fatal(err)
	}
	children, err := filepath.Glob(filepath.Join(layout.SessionsDir(), "children", "*.jsonl"))
	if err != nil || len(children) != 1 {
		t.Fatalf("child journal paths=%v err=%v", children, err)
	}
	keys, err := store.List(ctx, agentsession.Selector{All: true})
	if err != nil || len(keys) != 2 {
		t.Fatalf("canonical Store keys=%#v err=%v", keys, err)
	}
}

func TestStoreEmbedsGameAgentRecordsWithoutChangingStoryProjection(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "game")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(dataDir)
	record, err := registry.Add(workspace, project.TypeGeneral, "Game journal")
	if err != nil {
		t.Fatal(err)
	}
	game := interactive.NewStore(workspace)
	story, err := game.CreateStory(interactive.CreateStoryRequest{Title: "One journal", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := (agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: record.ID,
		StoryID: story.ID, BranchID: "main",
	}).AgentSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(dataDir, registry)
	if err != nil {
		t.Fatal(err)
	}
	log, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(ctx, 0, agentsession.Record{
		Kind: "session.message_checkpoint", Version: 1,
		Data: json.RawMessage(`{"hash":"messages","message_count":0}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := interactive.NewStore(workspace).Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.StoryID != story.ID || snapshot.BranchID != "main" {
		t.Fatalf("Story projection changed after embedded Agent record: %#v", snapshot)
	}
}

func TestStoreMigratesReleasedProductCompactionIntoEmbeddedCapability(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(dataDir)
	record, err := registry.Add(workspace, project.TypeGeneral, "Released compaction")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(record)
	if err != nil {
		t.Fatal(err)
	}
	productStore, err := productsession.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	productSession, err := productStore.GetOrCreate("released-session")
	if err != nil {
		t.Fatal(err)
	}
	if err := productSession.Append(agent.UserMessage("discarded before clear")); err != nil {
		t.Fatal(err)
	}
	if err := productSession.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := productSession.Append(agent.UserMessage("first retained input")); err != nil {
		t.Fatal(err)
	}
	if err := productSession.Append(agent.AssistantMessage("first retained answer", nil)); err != nil {
		t.Fatal(err)
	}
	if err := productSession.Append(agent.UserMessage("tail")); err != nil {
		t.Fatal(err)
	}
	if err := productStore.Close(); err != nil {
		t.Fatal(err)
	}

	journalPath := filepath.Join(layout.SessionsDir(), productSession.ID+".jsonl")
	legacy, err := json.Marshal(map[string]any{
		"type": "context_compaction", "id": "released-checkpoint", "agent_kind": agentrun.AgentKindIDE,
		"epoch": 4, "summary": "Released bounded summary", "source_start_index": 1,
		"source_end_index": 3, "source_message_count": 2, "retained_turns": 1,
		"tokens_before": 2400, "tokens_after": 600, "context_window_tokens": 128000,
		"threshold": 0.8, "created_at": "2026-01-02T03:04:05Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(journalPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(legacy, '\n')); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	beforeMigration, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}

	key, err := (agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindIDE, ProjectID: record.ID, SessionID: productSession.ID,
	}).AgentSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(dataDir, registry)
	if err != nil {
		t.Fatal(err)
	}
	log, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	var migrated agent.CompactionState
	capabilityRecords := 0
	if _, err := log.Replay(ctx, func(record agentsession.Record) error {
		if record.Kind != "session.capability_set" {
			return nil
		}
		var payload struct {
			Capability string          `json:"capability"`
			State      json.RawMessage `json:"state"`
		}
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		if payload.Capability != agent.CompactionCapability {
			return nil
		}
		capabilityRecords++
		return json.Unmarshal(payload.State, &migrated)
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if capabilityRecords != 1 || migrated.ID != "released-checkpoint" || migrated.Revision != 4 ||
		migrated.ReplacementFrom != 0 || migrated.ReplacementTo != 2 || migrated.Summary != "Released bounded summary" {
		t.Fatalf("migrated Compaction = %#v records=%d", migrated, capabilityRecords)
	}

	backups, err := filepath.Glob(filepath.Join(
		dataDir, "backups", "product-session-v0.3.3-compaction", productSession.ID+"-*.jsonl",
	))
	if err != nil || len(backups) != 1 {
		t.Fatalf("released Product Session backups=%#v err=%v", backups, err)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, beforeMigration) {
		t.Fatal("released Product Session backup differs from the pre-migration journal")
	}
	afterMigration, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(afterMigration, []byte(`"type":"context_compaction"`)) ||
		!bytes.Contains(afterMigration, []byte(`"type":"agent_session"`)) {
		t.Fatalf("migration did not keep old evidence and append the converted capability:\n%s", afterMigration)
	}

	reopened, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	afterRetry, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterRetry, afterMigration) {
		t.Fatal("opening a migrated Product Session appended a duplicate conversion")
	}
}
