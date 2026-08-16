package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	projectdomain "denova/internal/project"
)

func TestGlobalAgentRunTracesMergeArchivedProjectsAndIsolateFailures(t *testing.T) {
	dataDir := t.TempDir()
	registry := projectdomain.NewRegistry(dataDir)
	first, firstLayout := addGlobalTraceProject(t, registry, filepath.Join(t.TempDir(), "first"), "First Project")
	second, secondLayout := addGlobalTraceProject(t, registry, filepath.Join(t.TempDir(), "second"), "Archived Project")
	broken, brokenLayout := addGlobalTraceProject(t, registry, filepath.Join(t.TempDir(), "broken"), "Broken Project")
	writeGlobalTrace(t, firstLayout.RunsDir(), "run-older", "2026-08-15T08:00:00Z", "ide", "success")
	writeGlobalTrace(t, secondLayout.RunsDir(), "run-newer", "2026-08-15T09:00:00Z", "interactive_story", "failed")
	if _, err := registry.Archive(second.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(second.WorkspacePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brokenLayout.RunsDir(), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{DenovaDir: dataDir, Labs: config.ResolvedLabs{DeveloperMode: true}}
	application := &App{cfg: cfg, projectRegistry: registry, workspace: first.WorkspacePath}
	cfg.ProjectID = first.ID

	catalog, err := application.GlobalAgentRunTraces(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Runs) != 2 || catalog.Runs[0].ID != "run-newer" || catalog.Runs[1].ID != "run-older" {
		t.Fatalf("globally sorted Runs = %#v", catalog.Runs)
	}
	if catalog.Runs[0].ProjectID != second.ID || catalog.Runs[0].ProjectName != "Archived Project" {
		t.Fatalf("archived Project metadata = %#v", catalog.Runs[0])
	}
	wantURI := fmt.Sprintf("trajectory://projects/%s/runs/run-newer", second.ID)
	if catalog.Runs[0].TrajectoryURI != wantURI || catalog.Runs[0].Path != "" {
		t.Fatalf("global Run identity = %#v, want URI %q and no local path", catalog.Runs[0], wantURI)
	}
	if len(catalog.Issues) != 1 || catalog.Issues[0].ProjectID != broken.ID {
		t.Fatalf("partial issues = %#v", catalog.Issues)
	}

	// Foreground Book state is irrelevant to this user-level catalog.
	application.workspace = filepath.Join(t.TempDir(), "unrelated-foreground")
	cfg.ProjectID = broken.ID
	limited, err := application.GlobalAgentRunTraces(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.Runs) != 1 || limited.Runs[0].TrajectoryURI != wantURI {
		t.Fatalf("limited global Runs changed with foreground Book: %#v", limited.Runs)
	}

	detail, err := application.ProjectAgentRunTrace(second.ID, "run-newer")
	if err != nil {
		t.Fatalf("read archived Project Run detail: %v", err)
	}
	if detail.Summary.ID != "run-newer" {
		t.Fatalf("archived Run detail = %#v", detail.Summary)
	}
	exported, err := application.ExportProjectAgentRunTrace(second.ID, "run-newer")
	if err != nil || exported.Filename != "run-newer.jsonl" {
		t.Fatalf("archived Run export = %#v, %v", exported, err)
	}
}

func TestGlobalAgentRunTracesRequiresDeveloperMode(t *testing.T) {
	application := &App{
		cfg:             &config.Config{DenovaDir: t.TempDir()},
		projectRegistry: projectdomain.NewRegistry(t.TempDir()),
	}
	_, err := application.GlobalAgentRunTraces(context.Background(), 100)
	if !errors.Is(err, ErrDeveloperModeDisabled) {
		t.Fatalf("GlobalAgentRunTraces error = %v, want ErrDeveloperModeDisabled", err)
	}
}

func addGlobalTraceProject(t *testing.T, registry *projectdomain.Registry, workspace, name string) (projectdomain.Record, projectdomain.Layout) {
	t.Helper()
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	record, err := registry.Add(workspace, projectdomain.TypeBook, name)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	return record, layout
}

func writeGlobalTrace(t *testing.T, runsDir, runID, createdAt, agentKind, status string) {
	t.Helper()
	if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(
		"{\"type\":\"run_created\",\"run_id\":%q,\"created_at\":%q,\"data\":{\"agent_kind\":%q}}\n"+
			"{\"type\":\"agent_run\",\"run_id\":%q,\"created_at\":%q,\"data\":{\"status\":%q,\"duration_ms\":1200}}\n"+
			"{\"type\":\"run_finished\",\"run_id\":%q,\"created_at\":%q,\"data\":{\"status\":%q}}\n",
		runID, createdAt, agentKind,
		runID, createdAt, status,
		runID, createdAt, status,
	)
	if err := os.WriteFile(filepath.Join(runsDir, runID+".jsonl"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
}
