package project

import (
	"os"
	"path/filepath"
	"testing"

	"denova/internal/workspacepath"
)

func TestEnsureStateCopiesLegacyProjectDataWithoutDeletingSource(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	legacyRoot := workspacepath.Dir(workspace)
	fixtures := map[string]string{
		filepath.Join(legacyRoot, "sessions", "session.jsonl"): "history\n",
		filepath.Join(legacyRoot, "config.toml"):               "[agent_tools.general]\nshell = false\n",
		filepath.Join(legacyRoot, "changes", "events.jsonl"):   "change\n",
		filepath.Join(legacyRoot, "runs", "run.json"):          "{}\n",
		filepath.Join(legacyRoot, "artifacts", "artifact.txt"): "artifact\n",
		filepath.Join(legacyRoot, "automations", "tasks.json"): "{\"tasks\":[]}\n",
	}
	for path, content := range fixtures {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	registry := NewRegistry(denovaDir)
	record, err := registry.Add(workspace, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	destinations := map[string]string{
		layout.SessionsDir() + string(filepath.Separator) + "session.jsonl": "history\n",
		layout.ConfigPath(): "[agent_tools.general]\nshell = false\n",
		filepath.Join(layout.ChangesDir(), "events.jsonl"):   "change\n",
		filepath.Join(layout.RunsDir(), "run.json"):          "{}\n",
		filepath.Join(layout.ArtifactsDir(), "artifact.txt"): "artifact\n",
		filepath.Join(layout.AutomationsDir(), "tasks.json"): "{\"tasks\":[]}\n",
	}
	for path, want := range destinations {
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != want {
			t.Fatalf("migrated state mismatch path=%s data=%q err=%v", path, data, readErr)
		}
	}
	for path, want := range fixtures {
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != want {
			t.Fatalf("legacy rollback source changed path=%s data=%q err=%v", path, data, readErr)
		}
	}

	if _, err := registry.EnsureState(record); err != nil {
		t.Fatalf("migration should be idempotent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.StateRoot, "migration.json")); err != nil {
		t.Fatalf("migration receipt missing: %v", err)
	}
}

func TestEnsureStateRejectsLegacySymlinks(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	legacySessions := workspacepath.Path(workspace, "sessions")
	if err := os.MkdirAll(filepath.Dir(legacySessions), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), legacySessions); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(denovaDir)
	record, err := registry.Add(workspace, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureState(record); err == nil {
		t.Fatal("expected legacy state symlink migration to fail")
	}
}
