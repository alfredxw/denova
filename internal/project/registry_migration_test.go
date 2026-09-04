package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRegistryLegacyMigrationRebasesManagedProjectFromForeignRoot(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := filepath.Join(denovaDir, ContentDirectoryName, "portable-book")
	if err := os.MkdirAll(filepath.Join(workspace, ".denova"), 0o755); err != nil {
		t.Fatal(err)
	}
	foreignPath := `C:\old-root\Projects\portable-book`
	if runtime.GOOS == "windows" {
		foreignPath = "/Users/writer/old-root/Projects/portable-book"
	}
	writeLegacyBookRegistryForTest(t, denovaDir, legacyBookRegistry{
		Current: foreignPath,
		Books:   []legacyBookRecord{{Name: "Portable", Path: foreignPath}},
	})

	projects, err := NewRegistry(denovaDir).List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("migrated Projects = %#v", projects)
	}
	if projects[0].Location.Kind != LocationManaged || projects[0].Location.Path != "projects/portable-book" {
		t.Fatalf("legacy managed Project location = %#v", projects[0].Location)
	}
	if projects[0].WorkspacePath != resolvedTestPath(t, workspace) || projects[0].Status != StatusAvailable {
		t.Fatalf("legacy managed Project was not rebased: %#v", projects[0])
	}
	all, err := NewRegistry(denovaDir).List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Name != "Portable" {
		t.Fatalf("legacy managed Project was duplicated during rebasing: %#v", all)
	}
}

func TestRegistryLegacyMigrationRebasesRootBookFromForeignDataRoot(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := filepath.Join(denovaDir, "Legacy Root Book")
	if err := os.MkdirAll(filepath.Join(workspace, ".denova"), 0o755); err != nil {
		t.Fatal(err)
	}
	foreignPath := `C:\old-root\.denova\LEGACY ROOT BOOK`
	if runtime.GOOS == "windows" {
		foreignPath = "/Users/writer/.denova/LEGACY ROOT BOOK"
	}
	writeLegacyBookRegistryForTest(t, denovaDir, legacyBookRegistry{
		Current: foreignPath,
		Books:   []legacyBookRecord{{Name: "Legacy title", Path: foreignPath}},
	})

	registry := NewRegistry(denovaDir)
	projects, err := registry.List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("root Book migration created duplicate Projects: %#v", projects)
	}
	project := projects[0]
	if project.Location.Kind != LocationManaged || project.Location.Path != "Legacy Root Book" ||
		project.WorkspacePath != resolvedTestPath(t, workspace) || project.Name != "Legacy title" || project.Status != StatusAvailable {
		t.Fatalf("foreign-root Book was not rebased onto its original identity: %#v", project)
	}
	if registry.CurrentBookPath() != resolvedTestPath(t, workspace) {
		t.Fatalf("current Book was not preserved: %q", registry.CurrentBookPath())
	}
}

func TestRegistryMigrationKeepsOneManagedOwnerForDuplicatedLegacyPaths(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := filepath.Join(denovaDir, ContentDirectoryName, "Portable Book")
	if err := os.MkdirAll(filepath.Join(workspace, ".denova"), 0o755); err != nil {
		t.Fatal(err)
	}
	stalePath := `/old-root/projects/Portable Book`
	if runtime.GOOS == "windows" {
		stalePath = `Z:\old-root\projects\Portable Book`
	}
	const (
		staleID   = "project-stale"
		currentID = "project-current"
	)
	legacy := registryData{
		Version:       projectIDStoreDirectoryRegistryVersion,
		CurrentBookID: currentID,
		Projects: []Record{
			{ID: staleID, Type: TypeBook, Name: "Portable Book", WorkspacePath: stalePath},
			{ID: currentID, Type: TypeBook, Name: "Portable Book", WorkspacePath: workspace},
		},
	}
	raw, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(denovaDir, "projects.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{staleID, currentID} {
		stateRoot := filepath.Join(denovaDir, storeDirectoryName, id)
		if err := os.MkdirAll(stateRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stateRoot, "identity.txt"), []byte(id), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	registry := NewRegistry(denovaDir)
	projects, err := registry.List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("migration changed Project identities: %#v", projects)
	}
	byID := make(map[string]Record, len(projects))
	for _, record := range projects {
		byID[record.ID] = record
	}
	current := byID[currentID]
	if current.Location.Kind != LocationManaged || current.Location.Path != "projects/Portable Book" ||
		current.WorkspacePath != resolvedTestPath(t, workspace) || current.Status != StatusAvailable {
		t.Fatalf("current Project did not retain the managed content location: %#v", current)
	}
	stale := byID[staleID]
	if stale.Location.Kind != LocationExternal || stale.Location.Path != stalePath || stale.Status != StatusMissing {
		t.Fatalf("stale duplicate Project was not preserved as its original external location: %#v", stale)
	}
	if registry.CurrentBookPath() != resolvedTestPath(t, workspace) {
		t.Fatalf("current Book path = %q, want %q", registry.CurrentBookPath(), workspace)
	}
	for _, record := range projects {
		identityPath := filepath.Join(denovaDir, storeDirectoryName, record.StoreDirName, "identity.txt")
		if identity, err := os.ReadFile(identityPath); err != nil || string(identity) != record.ID {
			t.Fatalf("Project %s state identity=%q err=%v", record.ID, identity, err)
		}
	}
}

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
		if record.StoreDirName == "" {
			t.Fatalf("legacy Project missing readable Store directory: %#v", record)
		}
	}
}

func TestRegistryRejectsUnsupportedIntermediateVersion(t *testing.T) {
	denovaDir := t.TempDir()
	raw, err := json.Marshal(registryData{Version: projectIDStoreDirectoryRegistryVersion - 1, Projects: []Record{}})
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

func TestRegistryMigratesProjectIDStoreDirectoriesWithoutChangingIdentity(t *testing.T) {
	denovaDir := t.TempDir()
	firstWorkspace := t.TempDir()
	secondWorkspace := t.TempDir()
	firstID := "project-first"
	secondID := "project-second"
	legacy := registryData{
		Version: projectIDStoreDirectoryRegistryVersion,
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
	legacyState := filepath.Join(denovaDir, storeDirectoryName, firstID, "sessions")
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
	if byID[firstID].StoreDirName != "我的-项目" || byID[secondID].StoreDirName != "我的-项目-2" {
		t.Fatalf("readable Store directory migration = %#v", byID)
	}
	if _, err := os.Stat(filepath.Join(denovaDir, storeDirectoryName, firstID)); !os.IsNotExist(err) {
		t.Fatalf("legacy ID Store directory still exists: %v", err)
	}
	migratedHistory := filepath.Join(denovaDir, storeDirectoryName, "我的-项目", "sessions", "history.jsonl")
	if data, err := os.ReadFile(migratedHistory); err != nil || string(data) != "history\n" {
		t.Fatalf("migrated Project Store data=%q err=%v", data, err)
	}

	var persisted registryData
	readJSONFileForTest(t, filepath.Join(denovaDir, "projects.json"), &persisted)
	if persisted.Version != registryVersion {
		t.Fatalf("migrated registry version = %d, want %d", persisted.Version, registryVersion)
	}
	if len(persisted.Projects) != 2 || persisted.Projects[0].ID != firstID || persisted.Projects[1].ID != secondID {
		t.Fatalf("Project identity changed during Store migration: %#v", persisted.Projects)
	}
	for _, record := range persisted.Projects {
		if record.Location.Kind != LocationExternal || record.Location.Path == "" || record.WorkspacePath != "" {
			t.Fatalf("legacy Project location was not normalized: %#v", record)
		}
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
