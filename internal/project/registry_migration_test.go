package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryLegacyMigrationArchivesMissingAndHiddenBooks(t *testing.T) {
	denovaDir := t.TempDir()
	available := filepath.Join(t.TempDir(), "available-book")
	missing := filepath.Join(t.TempDir(), "missing-book")
	hidden := filepath.Join(denovaDir, "projects", "hidden-book")
	for _, path := range []string{available, hidden} {
		if err := os.MkdirAll(filepath.Join(path, ".denova"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeLegacyBookRegistryForTest(t, denovaDir, legacyBookRegistry{
		Current: missing,
		Books: []legacyBookRecord{
			{Name: "Available", Path: available},
			{Name: "Missing", Path: missing},
		},
		Hidden: []string{hidden},
	})

	registry := NewRegistry(denovaDir)
	active, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("legacy migration exposed stale Projects: %#v", active)
	}
	assertProjectStatusForPath(t, active, available, StatusAvailable)
	all, err := registry.List(true)
	if err != nil {
		t.Fatal(err)
	}
	assertProjectStatusForPath(t, all, missing, StatusArchived)
	assertProjectStatusForPath(t, all, hidden, StatusArchived)
	if current := registry.CurrentBookPath(); current != "" {
		t.Fatalf("missing legacy current Book remained active: %s", current)
	}

	var persisted registryData
	readJSONFileForTest(t, filepath.Join(denovaDir, "projects.json"), &persisted)
	if persisted.Version != registryVersion {
		t.Fatalf("registry version = %d, want %d", persisted.Version, registryVersion)
	}
	for _, record := range persisted.Projects {
		if record.StateDirName == "" {
			t.Fatalf("legacy Project missing readable state directory: %#v", record)
		}
	}
}

func TestRegistryRejectsUnsupportedIntermediateVersion(t *testing.T) {
	denovaDir := t.TempDir()
	raw, err := json.Marshal(registryData{Version: projectIDStateDirectoryRegistryVersion - 1, Projects: []Record{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(denovaDir, "projects.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRegistry(denovaDir).List(true); err == nil {
		t.Fatal("registry accepted an unsupported intermediate schema")
	}
}

func TestRegistryMigratesProjectIDStateDirectoriesWithoutChangingIdentity(t *testing.T) {
	denovaDir := t.TempDir()
	firstWorkspace := t.TempDir()
	secondWorkspace := t.TempDir()
	firstID := "project-first"
	secondID := "project-second"
	legacy := registryData{
		Version: projectIDStateDirectoryRegistryVersion,
		Projects: []Record{
			{ID: firstID, Type: TypeGeneral, Name: "我的 项目", WorkspacePath: firstWorkspace},
			{ID: secondID, Type: TypeGeneral, Name: "我的-项目", WorkspacePath: secondWorkspace},
		},
	}
	raw, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(denovaDir, "projects.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyState := filepath.Join(denovaDir, StateDirectoryName, firstID, "sessions")
	if err := os.MkdirAll(legacyState, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyState, "history.jsonl"), []byte("history\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	projects, err := NewRegistry(denovaDir).List(true)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]Record, len(projects))
	for _, record := range projects {
		byID[record.ID] = record
	}
	if byID[firstID].StateDirName != "我的-项目" || byID[secondID].StateDirName != "我的-项目-2" {
		t.Fatalf("readable state directory migration = %#v", byID)
	}
	if _, err := os.Stat(filepath.Join(denovaDir, StateDirectoryName, firstID)); !os.IsNotExist(err) {
		t.Fatalf("legacy ID state directory still exists: %v", err)
	}
	migratedHistory := filepath.Join(denovaDir, StateDirectoryName, "我的-项目", "sessions", "history.jsonl")
	if data, err := os.ReadFile(migratedHistory); err != nil || string(data) != "history\n" {
		t.Fatalf("migrated Project state data=%q err=%v", data, err)
	}

	var persisted registryData
	readJSONFileForTest(t, filepath.Join(denovaDir, "projects.json"), &persisted)
	if persisted.Version != registryVersion {
		t.Fatalf("migrated registry version = %d, want %d", persisted.Version, registryVersion)
	}
	if len(persisted.Projects) != 2 || persisted.Projects[0].ID != firstID || persisted.Projects[1].ID != secondID {
		t.Fatalf("Project identity changed during state migration: %#v", persisted.Projects)
	}
}

func writeLegacyBookRegistryForTest(t *testing.T, denovaDir string, data legacyBookRegistry) {
	t.Helper()
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(denovaDir, "books.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJSONFileForTest(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func assertProjectStatusForPath(t *testing.T, projects []Record, path string, status Status) {
	t.Helper()
	canonical, err := canonicalDirectory(path, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range projects {
		if samePath(project.WorkspacePath, canonical) {
			if project.Status != status {
				t.Fatalf("Project %s status = %s, want %s", path, project.Status, status)
			}
			return
		}
	}
	t.Fatalf("Project %s not found in %#v", path, projects)
}
