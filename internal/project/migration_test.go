package project

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	bookversions "denova/internal/book/versions"
	workspacelayout "denova/internal/workspace"
)

func TestEnsureStateCopiesLegacyProjectDataWithoutDeletingSource(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	legacyRoot := workspacelayout.Dir(workspace)
	fixtures := map[string]string{
		filepath.Join(legacyRoot, "sessions", "session.jsonl"): "history\n",
		filepath.Join(legacyRoot, "config.toml"):               "[agent_tools.general]\nshell = false\n",
		filepath.Join(legacyRoot, "changes", "events.jsonl"):   "change\n",
		filepath.Join(legacyRoot, "reviews", "ledger.jsonl"):   "review\n",
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
		filepath.Join(layout.ReviewsDir(), "ledger.jsonl"):   "review\n",
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

func TestEnsureStateRejectsUnsupportedIntermediateReceipt(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	registry := NewRegistry(denovaDir)
	record, err := registry.Add(workspace, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.Layout(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.StateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.StateRoot, "migration.json"), []byte("{\"version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureState(record); err == nil || !strings.Contains(err.Error(), "does not match supported version") {
		t.Fatalf("unsupported receipt error = %v", err)
	}
}

func TestEnsureStateMigratesReleasedVersionRepositoryWithoutDeletingSource(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	chapter := filepath.Join(workspace, "chapters", "ch0001.md")
	if err := os.MkdirAll(filepath.Dir(chapter), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chapter, []byte("released content"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyRepository := filepath.Join(workspace, ".git")
	legacy := bookversions.NewService(workspace, legacyRepository)
	created, err := legacy.Create("released version", bookversions.VersionSourceManual, bookversions.DefaultAutoSettings())
	if err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	registry := NewRegistry(denovaDir)
	record, err := registry.Add(workspace, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	migrated := bookversions.NewService(workspace, layout.VersionRepositoryDir())
	history, err := migrated.History(10)
	if err != nil || len(history) != 1 || history[0].ID != created.Version.ID {
		t.Fatalf("migrated Project history=%#v err=%v", history, err)
	}
	if _, err := os.Stat(legacyRepository); err != nil {
		t.Fatalf("released version source must remain available: %v", err)
	}
	receipt, err := os.ReadFile(filepath.Join(layout.StateRoot, "migration.json"))
	if err != nil || !strings.Contains(string(receipt), `"versions"`) {
		t.Fatalf("version migration receipt=%q err=%v", receipt, err)
	}
}

func TestEnsureStateIsIdempotentForConcurrentProjectResolution(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	legacyChanges := workspacelayout.Path(workspace, "changes")
	if err := os.MkdirAll(legacyChanges, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyChanges, "events.jsonl"), []byte("change\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(denovaDir)
	record, err := registry.Add(workspace, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}

	const callers = 16
	start := make(chan struct{})
	errorsByCaller := make([]error, callers)
	var waitGroup sync.WaitGroup
	waitGroup.Add(callers)
	for index := range callers {
		go func() {
			defer waitGroup.Done()
			<-start
			_, errorsByCaller[index] = registry.EnsureState(record)
		}()
	}
	close(start)
	waitGroup.Wait()
	for index, callErr := range errorsByCaller {
		if callErr != nil {
			t.Fatalf("concurrent state migration %d failed: %v", index, callErr)
		}
	}

	layout, err := registry.Layout(record)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(layout.ChangesDir(), "events.jsonl"))
	if err != nil || string(data) != "change\n" {
		t.Fatalf("migrated state mismatch data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(layout.StateRoot, "migration.json")); err != nil {
		t.Fatalf("migration receipt missing: %v", err)
	}
}

func TestEnsureStateRejectsLegacySymlinks(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	legacySessions := workspacelayout.Path(workspace, "sessions")
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
