package canonicalstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/agents/conversationjournal"
	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	"denova/internal/interactive"
	"denova/internal/project"
	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// Exercise both real product index codecs, historical range reads, cold Agent
// recovery and a full sidecar rebuild. Bodies must remain solely in JSONL.
func TestProductRecoveryDoesNotRetainSettledExecutionBodies(t *testing.T) {
	for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			ctx := t.Context()
			dataDir, workspace := t.TempDir(), t.TempDir()
			registry := project.NewRegistry(dataDir)
			projectRecord, err := registry.Add(workspace, project.TypeGeneral, "History")
			if err != nil {
				t.Fatal(err)
			}
			layout, err := registry.EnsureStore(projectRecord)
			if err != nil {
				t.Fatal(err)
			}
			binding := agentrun.RuntimeBinding{AgentKind: kind, ProjectID: projectRecord.ID}
			var path string
			if kind == agentrun.AgentKindIDE {
				products, err := productsession.NewStore(layout.SessionsDir())
				if err != nil {
					t.Fatal(err)
				}
				sess, err := products.GetOrCreate("long")
				if err != nil {
					t.Fatal(err)
				}
				binding.SessionID = sess.ID
				path = filepath.Join(layout.SessionsDir(), sess.ID+".jsonl")
				if err := products.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				stories := interactive.NewStore(layout.ContentRoot)
				story, err := stories.CreateStory(interactive.CreateStoryRequest{Title: "History", StoryTellerID: "classic"})
				if err != nil {
					t.Fatal(err)
				}
				binding.StoryID, binding.BranchID = story.ID, "main"
				path = filepath.Join(layout.ContentRoot, "interactive", "story", "story-"+story.ID+".jsonl")
				if err := stories.Close(); err != nil {
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
			body := strings.Repeat("historical-body-", 512)
			var revision agentsession.Revision
			makeRecord := func(kind string, value any) agentsession.Record {
				encoded, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				return agentsession.Record{Kind: kind, Version: 1, Data: encoded}
			}
			for batch := range 10 {
				var records []agentsession.Record
				for n := range 100 {
					i := batch*100 + n
					runID, commandID := fmt.Sprint("run-", i), fmt.Sprint("command-", i)
					records = append(records,
						makeRecord("session.input", map[string]any{"receipt": agent.CommandReceipt{RunID: runID, CommandID: commandID, Cursor: agent.Cursor(i + 1)}, "kind": "run", "hash": "retained-original-hash", "input": map[string]any{"text": body}}),
						makeRecord("turn.started", map[string]any{"run_id": runID, "command_id": commandID, "at": "2026-09-20T00:00:00Z"}),
						makeRecord("session.input_update", map[string]any{"command_id": commandID, "run_id": runID, "status": "consumed"}),
						makeRecord("turn.tool", map[string]any{"run_id": runID, "call_id": runID + "-tool", "name": "read", "arguments": map[string]any{}, "started": true, "result": map[string]any{"status": "success", "model_content": body, "display_content": body}}),
						makeRecord("turn.finished", map[string]any{"run_id": runID, "command_id": commandID, "status": "completed", "output": body, "at": "2026-09-20T00:00:01Z"}),
					)
				}
				revision, err = log.Append(ctx, revision, records...)
				if err != nil {
					t.Fatal(err)
				}
			}
			queued := makeRecord("session.input", map[string]any{"receipt": agent.CommandReceipt{CommandID: "pending", Cursor: 1001}, "kind": "queue", "hash": "pending-hash", "input": map[string]any{"text": "Keep this unfinished input"}})
			if _, err := log.Append(ctx, revision, queued); err != nil {
				t.Fatal(err)
			}
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := agent.SourceFunc(func(context.Context, agent.PrepareRequest) (agent.Definition, error) {
				t.Error("history lookup started execution")
				return agent.Definition{}, fmt.Errorf("unexpected execution")
			})
			for _, rebuild := range []bool{false, true} {
				if rebuild {
					if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil {
						t.Fatal(err)
					}
				}
				owner, err := agent.New(ctx, source, agent.WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				sess, err := owner.Session(ctx, key)
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range []string{"command-0", "command-999"} {
					snapshot, found, err := sess.CommandSnapshot(ctx, id)
					if err != nil || !found || snapshot.Output != body || snapshot.Result == nil || snapshot.Result.Status != agent.ResultCompleted {
						t.Fatalf("old result %s: %+v %v %v", id, snapshot, found, err)
					}
				}
				input, found, err := sess.RunInput(ctx, "run-0")
				if err != nil || !found || input.Text != body {
					t.Fatalf("old input: %v %v", found, err)
				}
				if _, found, err := sess.Queued(ctx, "pending"); err != nil || !found {
					t.Fatalf("pending input lost: %v %v", found, err)
				}
				if err := owner.Close(ctx); err != nil {
					t.Fatal(err)
				}
				index, err := os.ReadFile(conversationjournal.SidecarPath(path))
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(index, []byte(body)) {
					t.Fatal("index retained historical execution body")
				}
				if len(index) > len(original)/10 {
					t.Fatalf("index=%d journal=%d", len(index), len(original))
				}
				if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, original) {
					t.Fatalf("historical lookup changed canonical journal: %v", err)
				}
				t.Logf("rebuild=%t index=%d journal=%d", rebuild, len(index), len(original))
			}
		})
	}
}
