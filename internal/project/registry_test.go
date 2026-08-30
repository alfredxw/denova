package project

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRegistryManagedProjectSurvivesDataDirectoryRelocation(t *testing.T) {
	parent := t.TempDir()
	firstRoot := filepath.Join(parent, "first-root")
	secondRoot := filepath.Join(parent, "second-root")
	workspace := filepath.Join(firstRoot, ContentDirectoryName, "Portable Book")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	firstRegistry := NewRegistry(firstRoot)
	created, err := firstRegistry.Add(workspace, TypeGeneral, "Portable")
	if err != nil {
		t.Fatal(err)
	}
	if created.Location.Kind != LocationManaged || created.Location.Path != "projects/Portable Book" {
		t.Fatalf("managed Project location = %#v", created.Location)
	}
	firstLayout, err := firstRegistry.EnsureState(created)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstLayout.StateRoot, "marker.txt"), []byte("portable\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(firstRoot, secondRoot); err != nil {
		t.Fatal(err)
	}
	secondRegistry := NewRegistry(secondRoot)
	relocated, err := secondRegistry.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantWorkspace := filepath.Join(secondRoot, ContentDirectoryName, "Portable Book")
	if relocated.ID != created.ID || relocated.WorkspacePath != wantWorkspace || relocated.Status != StatusAvailable {
		t.Fatalf("relocated Project = %#v, want workspace %s", relocated, wantWorkspace)
	}
	secondLayout, err := secondRegistry.Layout(relocated)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(secondLayout.StateRoot, "marker.txt")); err != nil || string(data) != "portable\n" {
		t.Fatalf("relocated Project state data=%q err=%v", data, err)
	}

	var persisted registryData
	readJSONFileForTest(t, filepath.Join(secondRoot, "projects.json"), &persisted)
	if len(persisted.Projects) != 1 || persisted.Projects[0].WorkspacePath != "" {
		t.Fatalf("registry persisted a runtime absolute path: %#v", persisted.Projects)
	}
}

func TestRegistryPreservesForeignExternalLocationVerbatim(t *testing.T) {
	denovaDir := t.TempDir()
	foreignPath := `C:\Users\writer\Novel`
	if runtime.GOOS == "windows" {
		foreignPath = "/Users/writer/Novel"
	}
	data := registryData{
		Version: registryVersion,
		Projects: []Record{{
			ID:           "project-external",
			Type:         TypeGeneral,
			Name:         "External",
			StateDirName: "External",
			Location:     ProjectLocation{Kind: LocationExternal, Path: foreignPath},
		}},
	}
	if err := NewRegistry(denovaDir).saveLocked(data); err != nil {
		t.Fatal(err)
	}

	record, err := NewRegistry(denovaDir).Get("project-external")
	if err != nil {
		t.Fatal(err)
	}
	if record.Location.Path != foreignPath || record.WorkspacePath != foreignPath || record.Status != StatusMissing {
		t.Fatalf("foreign external location was reinterpreted: %#v", record)
	}
	var persisted registryData
	readJSONFileForTest(t, filepath.Join(denovaDir, "projects.json"), &persisted)
	if persisted.Projects[0].Location.Path != foreignPath || persisted.Projects[0].WorkspacePath != "" {
		t.Fatalf("foreign external location changed on disk: %#v", persisted.Projects[0])
	}
}

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
	originalStateDir := record.StateDirName
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
	if record.ID != originalID || record.StateDirName != originalStateDir || record.Name != "Renamed" || record.WorkspacePath != canonicalSecond {
		t.Fatalf("Project identity changed across relink: %#v", record)
	}
	layout, err := registry.Layout(record)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(layout.StateRoot) != originalStateDir {
		t.Fatalf("Project state directory changed across rename and relink: %#v", layout)
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

func TestRegistryAllocatesReadableUniqueStateDirectories(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	add := func(name string) Record {
		t.Helper()
		workspace := t.TempDir()
		record, err := registry.Add(workspace, TypeGeneral, name)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}

	first := add("我的 小说")
	second := add("我的-小说")
	if _, err := registry.Archive(first.ID); err != nil {
		t.Fatal(err)
	}
	third := add("我的小说")
	reserved := add("CON")

	if first.StateDirName != "我的-小说" {
		t.Fatalf("first readable state directory = %q", first.StateDirName)
	}
	if second.StateDirName != "我的-小说-2" {
		t.Fatalf("second readable state directory = %q", second.StateDirName)
	}
	if third.StateDirName != "我的小说" {
		t.Fatalf("distinct Project name state directory = %q", third.StateDirName)
	}
	if reserved.StateDirName != "Project-CON" {
		t.Fatalf("Windows reserved state directory = %q", reserved.StateDirName)
	}

	fourth := add("我的 小说")
	if fourth.StateDirName != "我的-小说-3" {
		t.Fatalf("archived state directory was reused: %q", fourth.StateDirName)
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
