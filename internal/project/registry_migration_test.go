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
}

func TestRegistryRejectsUnsupportedIntermediateVersion(t *testing.T) {
	denovaDir := t.TempDir()
	raw, err := json.Marshal(registryData{Version: registryVersion - 1, Projects: []Record{}})
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
