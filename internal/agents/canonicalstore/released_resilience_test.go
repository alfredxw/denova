package canonicalstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	"denova/internal/interactive"
	"denova/internal/project"
	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// The fixture uses persistedTurn and record version 1 from the v0.4.5 tag.
// Both products embed the same released lifecycle facts in their own journal.
func TestReleasedLifecycleReadOnlyOpenAndBackedUpUpgrade(t *testing.T) {
	fixture, err := os.ReadFile("testdata/v0.4.5-lifecycle.json")
	if err != nil {
		t.Fatal(err)
	}
	var records []agentsession.Record
	if err := json.Unmarshal(fixture, &records); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			ctx := t.Context()
			dataDir, workspace := t.TempDir(), t.TempDir()
			registry := project.NewRegistry(dataDir)
			record, err := registry.Add(workspace, project.TypeGeneral, "Released journal")
			if err != nil {
				t.Fatal(err)
			}
			layout, err := registry.EnsureStore(record)
			if err != nil {
				t.Fatal(err)
			}
			binding := agentrun.RuntimeBinding{ProjectID: record.ID, AgentKind: kind}
			var journalPath string
			if kind == agentrun.AgentKindIDE {
				store, err := productsession.NewStore(layout.SessionsDir())
				if err != nil {
					t.Fatal(err)
				}
				sess, err := store.GetOrCreate("released-session")
				if err != nil {
					t.Fatal(err)
				}
				binding.SessionID = sess.ID
				journalPath = filepath.Join(layout.SessionsDir(), sess.ID+".jsonl")
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				store := interactive.NewStore(layout.ContentRoot)
				story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Released story", StoryTellerID: "classic"})
				if err != nil {
					t.Fatal(err)
				}
				binding.StoryID, binding.BranchID = story.ID, "main"
				journalPath = filepath.Join(layout.ContentRoot, "interactive", "story", "story-"+story.ID+".jsonl")
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			key, err := binding.AgentSessionKey()
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
			fixtureRecords := append([]agentsession.Record(nil), records...)
			state := struct {
				ID            string          `json:"id"`
				Revision      uint64          `json:"revision"`
				Summary       string          `json:"summary"`
				ReplacementTo int             `json:"replacement_to"`
				Version       uint16          `json:"version,omitempty"`
				ContextData   *agent.HostData `json:"context_data,omitempty"`
			}{ID: "released-checkpoint", Revision: 1, Summary: "Released summary", ReplacementTo: 2}
			if kind == agentrun.AgentKindInteractiveStory {
				state.ContextData = &agent.HostData{Type: "denova.interactive.compaction", Version: 1, Data: json.RawMessage(`{"source_turn_count":7}`)}
			}
			stateBytes, _ := json.Marshal(state)
			capabilityBytes, _ := json.Marshal(map[string]any{"capability": agent.CompactionCapability, "state": json.RawMessage(stateBytes)})
			fixtureRecords = append(fixtureRecords, agentsession.Record{Kind: "session.capability_set", Version: 1, Data: capabilityBytes})
			if _, err := log.Append(ctx, 0, fixtureRecords...); err != nil {
				t.Fatal(err)
			}
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			source := agent.SourceFunc(func(context.Context, agent.PrepareRequest) (agent.Definition, error) {
				t.Error("opening or queueing a released Session prepared execution")
				return agent.Definition{}, errors.New("unexpected execution")
			})
			owner, err := agent.New(ctx, source, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close(context.Background())
			sess, err := owner.Session(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := sess.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if (snapshot.Compaction == nil) != (kind == agentrun.AgentKindInteractiveStory) {
				t.Fatalf("released compaction projection=%+v kind=%s", snapshot.Compaction, kind)
			}
			statuses := map[string]agent.ResultStatus{}
			for _, run := range snapshot.RecentRuns {
				statuses[run.ID] = run.Status
			}
			if snapshot.ActiveRunID != "" || statuses["released-completed"] != agent.ResultCompleted || statuses["released-incomplete"] != agent.ResultIncomplete {
				t.Fatalf("released state=%+v", snapshot)
			}
			if current, err := os.ReadFile(journalPath); err != nil || !bytes.Equal(current, original) {
				t.Fatalf("read-only open changed released journal: %v", err)
			}
			queued, err := sess.Queue(ctx, agent.Input{Text: "New durable input", IdempotencyKey: "upgrade-once"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := sess.Queue(ctx, agent.Input{Text: "New durable input", IdempotencyKey: "upgrade-once"}); err != nil {
				t.Fatal(err)
			}
			backup, err := os.ReadFile(journalPath + ".pre-resilience-v1.bak")
			if err != nil || !bytes.Equal(backup, original) {
				t.Fatalf("upgrade did not preserve released bytes: %v", err)
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			upgradedLog, err := store.Open(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			var revision agentsession.Revision
			if _, err := upgradedLog.Replay(ctx, func(record agentsession.Record) error { revision = record.Revision; return nil }); err != nil {
				t.Fatal(err)
			}
			beforeCheckpoint, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			state.Version = 2
			state.ContextData = nil
			state.Revision++
			stateBytes, _ = json.Marshal(state)
			capabilityBytes, _ = json.Marshal(map[string]any{"capability": agent.CompactionCapability, "state": json.RawMessage(stateBytes)})
			for range 2 {
				revision, err = upgradedLog.Append(ctx, revision, agentsession.Record{Kind: "session.capability_set", Version: 1, Data: capabilityBytes})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := upgradedLog.Close(); err != nil {
				t.Fatal(err)
			}
			backup, err = os.ReadFile(journalPath + ".pre-incremental-compaction-v2.bak")
			if err != nil || !bytes.Equal(backup, beforeCheckpoint) {
				t.Fatalf("incremental checkpoint backup changed: %v", err)
			}
			indexPath := journalPath[:len(journalPath)-len(".jsonl")] + ".idx.json"
			if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			reopened, err := agent.New(ctx, source, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close(context.Background())
			sess, err = reopened.Session(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := sess.Queue(ctx, agent.Input{Text: "New durable input", IdempotencyKey: "upgrade-once"})
			if err != nil || recovered.Receipt() != queued.Receipt() {
				t.Fatalf("cold receipt=%+v error=%v", recovered, err)
			}
		})
	}
}
