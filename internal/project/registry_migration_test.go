package project

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestRegistryV1UpgradeArchivesOnlyStaleLegacyProjectsAndKeepsBackup(t *testing.T) {
	denovaDir := t.TempDir()
	availableLegacy := filepath.Join(t.TempDir(), "available-legacy")
	missingLegacy := filepath.Join(t.TempDir(), "missing-legacy")
	hiddenLegacy := filepath.Join(t.TempDir(), "hidden-legacy")
	userMissing := filepath.Join(t.TempDir(), "user-missing")
	for _, path := range []string{availableLegacy, hiddenLegacy} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeLegacyBookRegistryForTest(t, denovaDir, legacyBookRegistry{
		Books: []legacyBookRecord{
			{Name: "Available legacy", Path: availableLegacy},
			{Name: "Missing legacy", Path: missingLegacy},
		},
		Hidden: []string{hiddenLegacy},
	})

	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	v1 := registryData{
		Version:  1,
		SortMode: SortRecent,
		Projects: []Record{
			projectRecordForMigrationTest(availableLegacy, TypeBook, "Available legacy", now),
			projectRecordForMigrationTest(missingLegacy, TypeBook, "Missing legacy", now),
			projectRecordForMigrationTest(hiddenLegacy, TypeBook, "Hidden legacy", now),
			projectRecordForMigrationTest(userMissing, TypeBook, "User project", now.Add(time.Minute)),
		},
	}
	original := writeRegistryDataForTest(t, denovaDir, v1)

	registry := NewRegistry(denovaDir)
	active, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("v1 cleanup active Projects = %#v", active)
	}
	assertProjectStatusForPath(t, active, availableLegacy, StatusAvailable)
	assertProjectStatusForPath(t, active, userMissing, StatusMissing)

	all, err := registry.List(true)
	if err != nil {
		t.Fatal(err)
	}
	assertProjectStatusForPath(t, all, missingLegacy, StatusArchived)
	assertProjectStatusForPath(t, all, hiddenLegacy, StatusArchived)

	backup, err := os.ReadFile(filepath.Join(denovaDir, "projects.v1.backup.json"))
	if err != nil {
		t.Fatalf("read v1 rollback backup: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatal("v1 rollback backup does not contain the original registry")
	}
	var persisted registryData
	readJSONFileForTest(t, filepath.Join(denovaDir, "projects.json"), &persisted)
	if persisted.Version != registryVersion {
		t.Fatalf("upgraded registry version = %d, want %d", persisted.Version, registryVersion)
	}
}

func projectRecordForMigrationTest(path string, kind Type, name string, createdAt time.Time) Record {
	return Record{
		ID:            stableID(path),
		Type:          kind,
		Name:          name,
		WorkspacePath: path,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
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

func writeRegistryDataForTest(t *testing.T, denovaDir string, data registryData) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(denovaDir, "projects.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
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
