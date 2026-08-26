package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryKeepsStableProjectIdentityAcrossMetadataAndPathChanges(t *testing.T) {
	denovaDir := t.TempDir()
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	registry := NewRegistry(denovaDir)
	record, err := registry.Add(first, TypeGeneral, "General work")
	if err != nil {
		t.Fatal(err)
	}
	originalID := record.ID
	if record.Type != TypeGeneral || record.Status != StatusAvailable {
		t.Fatalf("unexpected new Project: %#v", record)
	}

	record, err = registry.Rename(originalID, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	record, err = registry.Relink(originalID, second)
	if err != nil {
		t.Fatal(err)
	}
	canonicalSecond, err := canonicalDirectory(second, true)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != originalID || record.Name != "Renamed" || record.WorkspacePath != canonicalSecond {
		t.Fatalf("Project identity changed across relink: %#v", record)
	}

	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	record, err = registry.Get(originalID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusMissing {
		t.Fatalf("missing Project should remain addressable, got %#v", record)
	}
	archived, err := registry.Archive(originalID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != StatusArchived {
		t.Fatalf("archive should be a tombstone, got %#v", archived)
	}
}

func TestEnsureHarnessRegistersOneManagedProject(t *testing.T) {
	denovaDir := t.TempDir()
	harnessRoot := filepath.Join(denovaDir, "state")
	if err := os.MkdirAll(harnessRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(denovaDir)
	first, err := registry.EnsureHarness(harnessRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.EnsureHarness(harnessRoot)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != HarnessProjectID || second.ID != first.ID || second.Type != TypeHarness || second.Name != "Harness" {
		t.Fatalf("unexpected Harness Project: first=%#v second=%#v", first, second)
	}
	if _, err := registry.Rename(first.ID, "Renamed"); err == nil {
		t.Fatal("managed Harness Project should not be renamed")
	}
	if _, err := registry.Archive(first.ID); err == nil {
		t.Fatal("managed Harness Project should not be archived")
	}
}

func TestRegistryDoesNotExposeDenovaRootUnlessExplicitlyAdded(t *testing.T) {
	denovaDir := t.TempDir()
	registry := NewRegistry(denovaDir)
	projects, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range projects {
		if project.WorkspacePath == denovaDir {
			t.Fatalf("Denova root was exposed implicitly: %#v", project)
		}
	}

	added, err := registry.Add(denovaDir, TypeGeneral, "Denova data")
	if err != nil {
		t.Fatal(err)
	}
	canonicalDenovaDir, err := canonicalDirectory(denovaDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if added.WorkspacePath != canonicalDenovaDir || added.Type != TypeGeneral {
		t.Fatalf("explicit Denova root should be treated like any folder: %#v", added)
	}
}

func TestRegistryManualOrderUsesStableIDs(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	var records []Record
	for _, name := range []string{"one", "two", "three"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		record, err := registry.Add(path, TypeGeneral, name)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	wanted := []string{records[2].ID, records[0].ID, records[1].ID}
	if err := registry.Reorder(wanted); err != nil {
		t.Fatal(err)
	}
	ordered, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	for index, id := range wanted {
		if ordered[index].ID != id {
			t.Fatalf("manual order mismatch: got %#v want %#v", ordered, wanted)
		}
	}
}

func TestRegistryDiscoveryDoesNotDuplicateCanonicalPathAliases(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := filepath.Join(denovaDir, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".denova"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(denovaDir)
	created, err := registry.EnsureBook(workspace)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].ID != created.ID {
		t.Fatalf("canonical path alias changed or duplicated Project created=%#v projects=%#v", created, projects)
	}
}

func TestRegistryRejectsRelinkOntoArchivedProjectPath(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	firstPath := filepath.Join(t.TempDir(), "first")
	secondPath := filepath.Join(t.TempDir(), "second")
	for _, path := range []string{firstPath, secondPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	first, err := registry.Add(firstPath, TypeGeneral, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Add(secondPath, TypeGeneral, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Archive(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Relink(second.ID, firstPath); err == nil {
		t.Fatal("relink should not steal a directory still owned by an archived Project")
	}
	records, err := registry.List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("relink conflict discarded Project state: %#v", records)
	}
}

func TestRegistryDoesNotSilentlyChangeExistingProjectType(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	workspace := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	book, err := registry.Add(workspace, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Add(workspace, TypeGeneral, "General"); err == nil {
		t.Fatal("adding the same directory with another type should be rejected")
	}
	unchanged, err := registry.Get(book.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Type != TypeBook || unchanged.Name != "Book" {
		t.Fatalf("Project changed after conflicting add: %#v", unchanged)
	}
}

func TestRegistryCanReuseOriginalDirectoryAfterProjectRelink(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	originalPath := filepath.Join(t.TempDir(), "original")
	relinkedPath := filepath.Join(t.TempDir(), "relinked")
	for _, path := range []string{originalPath, relinkedPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	first, err := registry.Add(originalPath, TypeGeneral, "First")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Relink(first.ID, relinkedPath); err != nil {
		t.Fatal(err)
	}
	second, err := registry.Add(originalPath, TypeGeneral, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("reused directory inherited another Project identity: %s", first.ID)
	}
	records, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, record := range records {
		if ids[record.ID] {
			t.Fatalf("duplicate Project identity after path reuse: %#v", records)
		}
		ids[record.ID] = true
	}
	if len(records) != 2 {
		t.Fatalf("Project registry lost path-reuse records: %#v", records)
	}
}
