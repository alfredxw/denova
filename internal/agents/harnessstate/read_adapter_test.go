package harnessstate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agentstate "github.com/alfredxw/denova/agent/state"
)

func TestHarnessReadAdapterReadsCurrentManifestAndFiles(t *testing.T) {
	manager := openTestManager(t)
	ctx := context.Background()
	adapter, err := NewReadAdapter(manager)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := manager.Store().Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Store().Update(ctx, agentstate.ChangeSet{
		BaseRevision: empty.Revision,
		Changes:      []agentstate.Change{{Path: "prompts/general.md", Content: []byte("First prompt")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := adapter.Read(ctx, `{"path":"harness://state/current"}`)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Path != "harness://state/current" {
		t.Fatalf("manifest path = %q", manifest.Path)
	}
	var document harnessStateManifest
	if err := json.Unmarshal([]byte(manifest.Content), &document); err != nil {
		t.Fatal(err)
	}
	if document.Revision != first.Snapshot.Revision || len(document.Files) != 1 {
		t.Fatalf("manifest = %#v", document)
	}

	_, err = manager.Store().Update(ctx, agentstate.ChangeSet{
		BaseRevision: first.Snapshot.Revision,
		Changes:      []agentstate.Change{{Path: "prompts/general.md", Content: []byte("Second prompt")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := adapter.Read(ctx, `{"path":"harness://state/prompts/general.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if file.Content != "Second prompt" || file.Path != "harness://state/prompts/general.md" {
		t.Fatalf("current file = %#v", file)
	}
}

func TestHarnessReadAdapterRejectsParentTraversal(t *testing.T) {
	manager := openTestManager(t)
	adapter, err := NewReadAdapter(manager)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Read(context.Background(), `{"path":"harness://state/../secret"}`); err == nil {
		t.Fatal("Harness State read accepted parent traversal")
	}
}

func TestHarnessReadAdapterRejectsLimitAboveMaximum(t *testing.T) {
	manager := openTestManager(t)
	ctx := context.Background()
	current, err := manager.Store().Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store().Update(ctx, agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes:      []agentstate.Change{{Path: "prompts/general.md", Content: []byte("Prompt")}},
	}); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewReadAdapter(manager)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Read(ctx, `{"path":"harness://state/prompts/general.md","limit":10001}`)
	if err == nil || !strings.Contains(err.Error(), "cannot exceed 10000") {
		t.Fatalf("oversized Harness State read limit was accepted: %v", err)
	}
}
