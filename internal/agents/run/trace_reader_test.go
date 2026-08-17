package agentrun

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	workspacelayout "denova/internal/workspace"
)

func TestExportRunTraceReturnsCompletePersistedJSONL(t *testing.T) {
	workspace := t.TempDir()
	runID := "run-support-export"
	payload := []byte("{\"type\":\"run_created\"}\n{\"type\":\"llm_call\"}\n")
	path := workspacelayout.Path(workspace, "runs", runID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	export, err := ExportRunTrace(TraceLocation{Workspace: workspace}, runID)
	if err != nil {
		t.Fatal(err)
	}
	if export.Filename != runID+".jsonl" {
		t.Fatalf("filename = %q", export.Filename)
	}
	if !bytes.Equal(export.Data, payload) {
		t.Fatalf("export data = %q, want %q", export.Data, payload)
	}
}

func TestExportRunTraceRejectsPathLikeRunID(t *testing.T) {
	if _, err := ExportRunTrace(TraceLocation{Workspace: t.TempDir()}, "../not-a-run"); err == nil {
		t.Fatal("ExportRunTrace should reject a path-like run id")
	}
}

func TestRunTraceLocationCombinesProjectStateAndLegacyTraces(t *testing.T) {
	workspace := t.TempDir()
	stateRoot := t.TempDir()
	location := TraceLocation{Workspace: workspace, StateRoot: stateRoot}
	writeTrace := func(dir, runID, status string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		payload := []byte(`{"type":"run_finished","run_id":"` + runID + `","created_at":"2026-08-02T12:35:38Z","data":{"status":"` + status + `"}}` + "\n")
		if err := os.WriteFile(filepath.Join(dir, runID+".jsonl"), payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	projectRuns := filepath.Join(stateRoot, "runs")
	legacyRuns := workspacelayout.Path(workspace, "runs")
	writeTrace(projectRuns, "run-current", "success")
	writeTrace(projectRuns, "run-shared", "success")
	writeTrace(legacyRuns, "run-legacy", "failed")
	writeTrace(legacyRuns, "run-shared", "failed")

	summaries, err := ListRunTraces(location, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 3 {
		t.Fatalf("trace summaries = %#v, want three deduplicated Runs", summaries)
	}
	statuses := make(map[string]string, len(summaries))
	for _, summary := range summaries {
		statuses[summary.ID] = summary.Status
	}
	if statuses["run-current"] != "success" || statuses["run-legacy"] != "failed" || statuses["run-shared"] != "success" {
		t.Fatalf("trace statuses = %#v", statuses)
	}
	legacy, err := ReadRunTrace(location, "run-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Summary.Status != "failed" {
		t.Fatalf("legacy trace status = %q", legacy.Summary.Status)
	}
	shared, err := ReadRunTrace(location, "run-shared")
	if err != nil {
		t.Fatal(err)
	}
	if shared.Summary.Status != "success" || filepath.Dir(shared.Summary.Path) != projectRuns {
		t.Fatalf("project-state trace should win duplicate Run ID: %#v", shared.Summary)
	}
}
