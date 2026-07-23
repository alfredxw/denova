package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/workspacepath"
)

func TestExportRunTraceReturnsCompletePersistedJSONL(t *testing.T) {
	workspace := t.TempDir()
	runID := "run-support-export"
	payload := []byte("{\"type\":\"run_created\"}\n{\"type\":\"llm_call\"}\n")
	path := workspacepath.Path(workspace, "runs", runID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	export, err := ExportRunTrace(workspace, runID)
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
	if _, err := ExportRunTrace(t.TempDir(), "../not-a-run"); err == nil {
		t.Fatal("ExportRunTrace should reject a path-like run id")
	}
}
