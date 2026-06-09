package versions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGoGitVersionCreateDiffAndRestore(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "chapters/ch0001.md", "第一版")
	writeFile(t, dir, "setting/progress.md", "进度一")

	first, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	if first.Version == nil || len(first.Version.ID) != 40 {
		t.Fatalf("expected git commit hash version id, got %#v", first.Version)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "versions", "git", ".git")); err != nil {
		t.Fatalf("expected go-git repository: %v", err)
	}

	writeFile(t, dir, "chapters/ch0001.md", "第二版")
	writeFile(t, dir, "chapters/ch0002.md", "新增章节")
	if err := os.Remove(filepath.Join(dir, "setting", "progress.md")); err != nil {
		t.Fatal(err)
	}

	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	assertChange(t, status.Changes, "chapters/ch0001.md", "modified")
	assertChange(t, status.Changes, "chapters/ch0002.md", "added")
	assertChange(t, status.Changes, "setting/progress.md", "deleted")

	diff, err := service.Diff(first.Version.ID, "chapters/ch0001.md")
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !diff.Text || diff.Original != "第一版" || diff.Modified != "第二版" {
		t.Fatalf("unexpected diff: %#v", diff)
	}

	second, err := service.Create("第二版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create second failed: %v", err)
	}
	if second.Version == nil || second.Version.ID == first.Version.ID {
		t.Fatalf("expected distinct second git commit: first=%#v second=%#v", first.Version, second.Version)
	}

	writeFile(t, dir, "chapters/ch0001.md", "临时改动")
	if _, err := service.Restore(first.Version.ID, settings); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	got := readFile(t, dir, "chapters/ch0001.md")
	if got != "第一版" {
		t.Fatalf("restore ch0001 = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "chapters", "ch0002.md")); !os.IsNotExist(err) {
		t.Fatalf("restore should remove added file, err=%v", err)
	}
	if readFile(t, dir, "setting/progress.md") != "进度一" {
		t.Fatalf("restore should recover deleted progress")
	}

	cleanStatus, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status after restore failed: %v", err)
	}
	if !cleanStatus.Clean || cleanStatus.Latest == nil || cleanStatus.Latest.ID != first.Version.ID {
		t.Fatalf("workspace should be clean at restored version: %#v", cleanStatus)
	}
}

func TestGoGitVersionIgnoresLegacySnapshotIndexEntries(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	legacy := VersionIndex{
		Version:   versionIndexVersion,
		CurrentID: "v20260601000000-deadbeef",
		Items: []VersionEntry{{
			ID:        "v20260601000000-deadbeef",
			Message:   "旧原生快照",
			CreatedAt: "2026-06-01T00:00:00Z",
			Source:    VersionSourceManual,
		}},
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".nova", "versions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nova", "versions", "index.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "chapters/ch0001.md", "第一版")

	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.HasVersions || status.Latest != nil {
		t.Fatalf("legacy snapshot index should not expose versions: %#v", status)
	}
	assertChange(t, status.Changes, "chapters/ch0001.md", "added")

	created, err := service.Create("新版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	history, err := service.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	if len(history) != 1 || created.Version == nil || history[0].ID != created.Version.ID {
		t.Fatalf("history should contain only go-git version: history=%#v created=%#v", history, created.Version)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertChange(t *testing.T, changes []VersionChange, path, status string) {
	t.Helper()
	for _, change := range changes {
		if change.Path == path && change.Status == status {
			return
		}
	}
	t.Fatalf("missing change %s %s in %#v", path, status, changes)
}
