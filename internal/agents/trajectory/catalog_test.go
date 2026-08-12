package trajectory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"
)

func TestReadAdapterComposesWithGenericReadContract(t *testing.T) {
	type genericReadInput struct {
		Path   string `json:"path"`
		Offset int    `json:"offset,omitempty" jsonschema:"minimum=1"`
		Limit  int    `json:"limit,omitempty" jsonschema:"minimum=1"`
	}
	generic, err := agenttools.NewReadAdapter(
		agent.CapabilityIdentity{Kind: "test.read.skill_reference", Version: 1},
		"skill_reference",
		func(_ context.Context, path string) (bool, error) { return strings.HasPrefix(path, "skill://"), nil },
		func(_ context.Context, input genericReadInput) (agenttools.ReadResult, error) {
			return agenttools.ReadResult{Path: input.Path, Content: "reference"}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	trajectory, err := NewReadAdapter(Catalog{Sources: func(context.Context) ([]Source, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agenttools.Read([]agenttools.ReadAdapter{generic, trajectory}); err != nil {
		t.Fatalf("compose generic and trajectory read adapters: %v", err)
	}
}

func TestReadAdapterRejectsOversizedResourceURI(t *testing.T) {
	adapter, err := NewReadAdapter(Catalog{Sources: func(context.Context) ([]Source, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := json.Marshal(map[string]string{"path": Scheme + strings.Repeat("x", maxResourceURIBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Read(context.Background(), string(arguments)); err == nil || !strings.Contains(err.Error(), "exceeds 4096 bytes") {
		t.Fatalf("oversized trajectory resource URI was accepted: %v", err)
	}
}

func TestReadAdapterRejectsLimitAboveCatalogMaximum(t *testing.T) {
	adapter, err := NewReadAdapter(Catalog{Sources: func(context.Context) ([]Source, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Read(context.Background(), `{"path":"trajectory://index","limit":501}`); err == nil || !strings.Contains(err.Error(), "cannot exceed 500") {
		t.Fatalf("oversized trajectory limit was accepted: %v", err)
	}
}

func TestCatalogReadsExistingSessionsWithoutExposingMachinePaths(t *testing.T) {
	dataDir := t.TempDir()
	stateRoot := filepath.Join(dataDir, "project-state")
	store, err := session.NewStore(filepath.Join(stateRoot, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.GetOrCreate("session-one")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	workspace := filepath.Join(dataDir, "private-workspace")
	if err := target.Append(agent.UserMessage("Inspect " + filepath.Join(workspace, "draft.md") + " and " + stateRoot)); err != nil {
		t.Fatal(err)
	}
	if err := target.AppendDisplayEvent(session.DisplayEvent{Role: "thinking", Content: "private reasoning must not become learning evidence"}); err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Sources: func(context.Context) ([]Source, error) {
		return []Source{{ProjectID: "project-one", Name: "Example", Workspace: workspace, StateRoot: stateRoot}}, nil
	}, Limit: 10}
	adapter, err := NewReadAdapter(catalog)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Read(context.Background(), `{"path":"trajectory://index"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "session-one") || strings.Contains(result.Content, workspace) || strings.Contains(result.Content, stateRoot) {
		t.Fatalf("unexpected trajectory index %s", result.Content)
	}
	result, err = adapter.Read(context.Background(), `{"path":"trajectory://projects/project-one/sessions/session-one"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, workspace) || strings.Contains(result.Content, stateRoot) || strings.Contains(result.Content, "private reasoning") || !strings.Contains(result.Content, "session-one") || !strings.Contains(result.Content, "[private-root]") {
		t.Fatalf("unexpected session trajectory %s", result.Content)
	}
}

func TestCatalogRedactsMachinePathsFromRunIndexAndDetail(t *testing.T) {
	dataDir := t.TempDir()
	stateRoot := filepath.Join(dataDir, "private-project-state")
	workspace := filepath.Join(dataDir, "private-workspace")
	runsDir := filepath.Join(stateRoot, "runs")
	if err := os.MkdirAll(runsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	records := []map[string]any{
		{
			"type": "run_created", "run_id": "run-one", "created_at": "2026-01-01T00:00:00Z",
			"data": map[string]any{"workspace": workspace, "path": filepath.Join(stateRoot, "runs", "run-one.jsonl"), "agent_kind": "ide"},
		},
		{
			"type": "run_finished", "run_id": "run-one", "created_at": "2026-01-01T00:00:01Z",
			"data": map[string]any{"status": "success"},
		},
	}
	var encoded []byte
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, append(line, '\n')...)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "run-one.jsonl"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Sources: func(context.Context) ([]Source, error) {
		return []Source{{ProjectID: "project-one", Name: "Example", Workspace: workspace, StateRoot: stateRoot}}, nil
	}, Limit: 10}
	adapter, err := NewReadAdapter(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"trajectory://index", "trajectory://projects/project-one/runs/run-one"} {
		result, err := adapter.Read(context.Background(), `{"path":"`+resource+`"}`)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(result.Content, workspace) || strings.Contains(result.Content, stateRoot) {
			t.Fatalf("trajectory resource %s leaked a machine path: %s", resource, result.Content)
		}
	}
}

func TestOutcomeStoreAndCatalogPreserveExplicitFeedback(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewOutcomeStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Append(Outcome{RunID: "run-one", Signal: OutcomeCorrection, Comment: "Prefer the corrected workflow."})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.CreatedAt.IsZero() {
		t.Fatalf("outcome identity was not assigned: %#v", created)
	}
	adapter, err := NewReadAdapter(Catalog{
		Sources: func(context.Context) ([]Source, error) { return nil, nil }, Outcomes: store, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Read(context.Background(), `{"path":"trajectory://outcomes"}`)
	if err != nil {
		t.Fatal(err)
	}
	var outcomes []Outcome
	if err := json.Unmarshal([]byte(result.Content), &outcomes); err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].RunID != "run-one" || outcomes[0].Signal != OutcomeCorrection {
		t.Fatalf("unexpected outcomes %#v", outcomes)
	}
}

func TestOutcomeStoreRejectsUnboundFeedback(t *testing.T) {
	store, err := NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(Outcome{Signal: OutcomePositive}); err == nil {
		t.Fatal("expected unbound outcome to be rejected")
	}
}

func TestOutcomeStoreOwnsIdentityAndTimestampAndRejectsCorruption(t *testing.T) {
	store, err := NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	forgedTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	created, err := store.Append(Outcome{
		ID: "caller-owned", CreatedAt: forgedTime, RunID: "run-one", Signal: OutcomePositive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "caller-owned" || !strings.HasPrefix(created.ID, "outcome-") || created.CreatedAt.Equal(forgedTime) {
		t.Fatalf("caller controlled durable outcome metadata: %#v", created)
	}
	file, err := os.OpenFile(store.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{broken\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(10); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("corrupt outcome history was silently skipped: %v", err)
	}
}
