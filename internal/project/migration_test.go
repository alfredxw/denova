package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	bookversions "denova/internal/book/versions"
	workspacelayout "denova/internal/workspace"
)

func TestEnsureStoreCopiesLegacyProjectDataWithoutDeletingSource(t *testing.T) {
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
	layout, err := registry.EnsureStore(record)
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

	if _, err := registry.EnsureStore(record); err != nil {
		t.Fatalf("migration should be idempotent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.StoreRoot, "migration.json")); err != nil {
		t.Fatalf("migration receipt missing: %v", err)
	}
}

func TestResolveMissingProjectDefersStateMigrationUntilContentReturns(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "book")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(denovaDir)
	record, err := registry.Add(workspace, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(workspace); err != nil {
		t.Fatal(err)
	}

	missing, layout, err := registry.Resolve(record.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Status != StatusMissing {
		t.Fatalf("resolved Project status = %s, want %s", missing.Status, StatusMissing)
	}
	if _, err := os.Stat(layout.StoreRoot); !os.IsNotExist(err) {
		t.Fatalf("missing finalized Project Store migration: %v", err)
	}

	legacySession := workspacelayout.Path(workspace, "sessions", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(legacySession), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacySession, []byte("history\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, restoredLayout, err := registry.Resolve(record.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	migratedSession := filepath.Join(restoredLayout.SessionsDir(), "session.jsonl")
	if data, err := os.ReadFile(migratedSession); err != nil || string(data) != "history\n" {
		t.Fatalf("deferred Project Store migration data=%q err=%v", data, err)
	}
}

func TestResolveMalformedExternalProjectForArchiveDoesNotOpenWorkspace(t *testing.T) {
	denovaDir := t.TempDir()
	const projectID = "project-broken-path"
	registry := NewRegistry(denovaDir)
	if err := registry.saveLocked(registryData{
		Version: registryVersion,
		Projects: []Record{{
			ID:           projectID,
			Type:         TypeBook,
			Name:         "Book",
			StoreDirName: "Book",
			Location: ProjectLocation{
				Kind: LocationExternal,
				Path: `D:\mnt\d\Code\denova\D:\mnt\d\Code\denova\.denova\projects\Book`,
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	record, layout, err := registry.Resolve(projectID, false)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusMissing {
		t.Fatalf("malformed external Project status = %s, want %s", record.Status, StatusMissing)
	}
	if _, err := os.Stat(layout.StoreRoot); !os.IsNotExist(err) {
		t.Fatalf("malformed external Project opened migration state: %v", err)
	}
	archived, err := registry.Archive(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != StatusArchived {
		t.Fatalf("malformed external Project archive status = %s", archived.Status)
	}
}

func TestEnsureStoreRejectsUnsupportedIntermediateReceipt(t *testing.T) {
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
	if err := os.MkdirAll(layout.StoreRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.StoreRoot, "migration.json"), []byte("{\"version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureStore(record); err == nil || !strings.Contains(err.Error(), "does not match supported version") {
		t.Fatalf("unsupported receipt error = %v", err)
	}
}

func TestEnsureStoreMigratesReleasedReceiptWithoutRecopyingData(t *testing.T) {
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
	if err := os.MkdirAll(layout.SessionsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	currentSession := filepath.Join(layout.SessionsDir(), "current.jsonl")
	if err := os.WriteFile(currentSession, []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	completedAt := time.Date(2026, time.August, 1, 2, 3, 4, 0, time.UTC)
	released := struct {
		Version     int       `json:"version"`
		Source      string    `json:"source"`
		CompletedAt time.Time `json:"completed_at"`
		Copied      []string  `json:"copied"`
	}{
		Version: 3, Source: workspace,
		CompletedAt: completedAt, Copied: []string{"sessions"},
	}
	raw, err := json.MarshalIndent(released, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(layout.StoreRoot, "migration.json")
	if err := os.WriteFile(receiptPath, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := registry.EnsureStore(record); err != nil {
		t.Fatal(err)
	}
	if session, err := os.ReadFile(currentSession); err != nil || string(session) != "current\n" {
		t.Fatalf("completed Store migration was repeated: data=%q err=%v", session, err)
	}
	migratedRaw, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(migratedRaw), `"source"`) || strings.Contains(string(migratedRaw), workspace) {
		t.Fatalf("migrated receipt retained its runtime source: %s", migratedRaw)
	}
	var migrated migrationReceipt
	if err := json.Unmarshal(migratedRaw, &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.Version != storeMigrationVersion || !migrated.CompletedAt.Equal(completedAt) ||
		len(migrated.Copied) != 1 || migrated.Copied[0] != "sessions" {
		t.Fatalf("migrated receipt = %#v", migrated)
	}
}

func TestRegistryMigrationUpgradesReleasedReceiptsForUnavailableProjects(t *testing.T) {
	denovaDir := t.TempDir()
	registry := NewRegistry(denovaDir)
	workspaces := []string{t.TempDir(), t.TempDir()}
	records := make([]Record, 0, len(workspaces))
	for _, workspace := range workspaces {
		record, err := registry.Add(workspace, TypeGeneral, "Project")
		if err != nil {
			t.Fatal(err)
		}
		layout, err := registry.Layout(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(layout.StoreRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		released := struct {
			Version int    `json:"version"`
			Source  string `json:"source"`
		}{Version: 3, Source: workspace}
		raw, err := json.Marshal(released)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(layout.StoreRoot, "migration.json"), append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := os.Remove(workspaces[1]); err != nil {
		t.Fatal(err)
	}

	projects, err := NewRegistry(denovaDir).List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != len(records) {
		t.Fatalf("migrated Projects = %#v", projects)
	}
	statusByID := make(map[string]Status, len(projects))
	for _, project := range projects {
		statusByID[project.ID] = project.Status
	}
	if statusByID[records[0].ID] != StatusAvailable || statusByID[records[1].ID] != StatusMissing {
		t.Fatalf("migrated Project statuses = %#v", statusByID)
	}
	for _, record := range records {
		receiptPath := filepath.Join(denovaDir, storeDirectoryName, record.StoreDirName, "migration.json")
		raw, err := os.ReadFile(receiptPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"source"`) {
			t.Fatalf("Project %s receipt was not upgraded: %s", record.ID, raw)
		}
		var receipt migrationReceipt
		if err := json.Unmarshal(raw, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Version != storeMigrationVersion {
			t.Fatalf("Project %s receipt version = %d", record.ID, receipt.Version)
		}
	}
}

func TestEnsureStoreMigratesReleasedVersionRepositoryWithoutDeletingSource(t *testing.T) {
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
	layout, err := registry.EnsureStore(record)
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
	receipt, err := os.ReadFile(filepath.Join(layout.StoreRoot, "migration.json"))
	if err != nil || !strings.Contains(string(receipt), `"versions"`) {
		t.Fatalf("version migration receipt=%q err=%v", receipt, err)
	}
}

func TestEnsureStoreIsIdempotentForConcurrentProjectResolution(t *testing.T) {
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
			_, errorsByCaller[index] = registry.EnsureStore(record)
		}()
	}
	close(start)
	waitGroup.Wait()
	for index, callErr := range errorsByCaller {
		if callErr != nil {
			t.Fatalf("concurrent Store migration %d failed: %v", index, callErr)
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
	if _, err := os.Stat(filepath.Join(layout.StoreRoot, "migration.json")); err != nil {
		t.Fatalf("migration receipt missing: %v", err)
	}
}

func TestEnsureStoreRejectsLegacySymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires optional Windows symlink privileges")
	}
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
	if _, err := registry.EnsureStore(record); err == nil {
		t.Fatal("expected legacy state symlink migration to fail")
	}
}
