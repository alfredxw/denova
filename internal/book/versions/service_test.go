package versions

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
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
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected workspace .git repository: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "versions")); !os.IsNotExist(err) {
		t.Fatalf("should not create .nova/versions metadata directory, err=%v", err)
	}
	writeFile(t, dir, ".nova/sessions/internal.txt", "内部数据")
	writeFile(t, dir, ".nova/lore/items.json", "[]")
	writeFile(t, dir, ".gitignore", ".nova\n")
	files, err := service.commitFiles(first.Version.ID)
	if err != nil {
		t.Fatalf("commitFiles failed: %v", err)
	}
	if _, ok := files[".nova/sessions/internal.txt"]; ok {
		t.Fatalf("first commit should not include .nova file created later: %v", sortedVersionFilePaths(files))
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
	secondFiles, err := service.commitFiles(second.Version.ID)
	if err != nil {
		t.Fatalf("commitFiles second failed: %v", err)
	}
	if _, ok := secondFiles[".nova/sessions/internal.txt"]; !ok {
		t.Fatalf("second commit should include .nova creative state: %v", sortedVersionFilePaths(secondFiles))
	}
	if _, ok := secondFiles[".nova/lore/items.json"]; !ok {
		t.Fatalf("second commit should include .nova lore state: %v", sortedVersionFilePaths(secondFiles))
	}
	if _, ok := secondFiles[".gitignore"]; !ok {
		t.Fatalf("second commit should include workspace .gitignore: %v", sortedVersionFilePaths(secondFiles))
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
	if _, err := os.Stat(filepath.Join(dir, ".nova", "sessions", "internal.txt")); !os.IsNotExist(err) {
		t.Fatalf("restore should remove .nova content absent from target version, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "lore", "items.json")); !os.IsNotExist(err) {
		t.Fatalf("restore should remove .nova lore content absent from target version, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "versions")); !os.IsNotExist(err) {
		t.Fatalf("restore should not create .nova/versions metadata directory, err=%v", err)
	}

	cleanStatus, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status after restore failed: %v", err)
	}
	if !cleanStatus.Clean || cleanStatus.Latest == nil || cleanStatus.Latest.ID != first.Version.ID {
		t.Fatalf("workspace should be clean at restored version: %#v", cleanStatus)
	}
	history, err := service.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	if len(history) != 3 ||
		!historyContains(history, first.Version.ID) ||
		!historyContains(history, second.Version.ID) ||
		!historyContainsSource(history, VersionSourceRollbackBackup) {
		t.Fatalf("history should come from git commits, history=%#v latest=%#v", history, cleanStatus.Latest)
	}
}

func TestGoGitVersionTracksNovaDeletesWhenGitIgnored(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, ".gitignore", ".nova\n")
	writeFile(t, dir, ".nova/lore/items.json", "[]")

	first, err := service.Create("保存资料库", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	if first.Version == nil {
		t.Fatalf("expected first version")
	}
	firstFiles, err := service.commitFiles(first.Version.ID)
	if err != nil {
		t.Fatalf("commitFiles first failed: %v", err)
	}
	if _, ok := firstFiles[".nova/lore/items.json"]; !ok {
		t.Fatalf("first commit should include ignored .nova file: %v", sortedVersionFilePaths(firstFiles))
	}

	if err := os.Remove(filepath.Join(dir, ".nova", "lore", "items.json")); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	assertChange(t, status.Changes, ".nova/lore/items.json", "deleted")

	second, err := service.Create("删除资料库", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create second failed: %v", err)
	}
	secondFiles, err := service.commitFiles(second.Version.ID)
	if err != nil {
		t.Fatalf("commitFiles second failed: %v", err)
	}
	if _, ok := secondFiles[".nova/lore/items.json"]; ok {
		t.Fatalf("second commit should record ignored .nova deletion: %v", sortedVersionFilePaths(secondFiles))
	}
}

func TestGoGitVersionExcludesRunLedgers(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "chapters/ch0001.md", "第一版")
	writeFile(t, dir, ".nova/runs/run-1.jsonl", `{"type":"run_created"}`)

	first, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	files, err := service.commitFiles(first.Version.ID)
	if err != nil {
		t.Fatalf("commitFiles first failed: %v", err)
	}
	if _, ok := files[".nova/runs/run-1.jsonl"]; ok {
		t.Fatalf("run ledger should not be committed: %v", sortedVersionFilePaths(files))
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "runs", "run-1.jsonl")); err != nil {
		t.Fatalf("run ledger should remain in workspace: %v", err)
	}

	writeFile(t, dir, ".nova/runs/run-2.jsonl", `{"type":"run_finished"}`)
	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !status.Clean {
		t.Fatalf("run ledger changes should not dirty version status: %#v", status.Changes)
	}
	if _, err := service.Create("只有运行账本变化", VersionSourceManual, settings); !errors.Is(err, ErrVersionClean) {
		t.Fatalf("Create should ignore run ledger-only changes, err=%v", err)
	}
}

func TestGoGitVersionExcludesInteractiveData(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "chapters/ch0001.md", "第一版")
	writeFile(t, dir, ".nova/interactive/stories/story-1.json", `{"title":"测试故事"}`)
	writeFile(t, dir, ".nova/interactive/memory/book.json", `{"structures":[]}`)

	first, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	files, err := service.commitFiles(first.Version.ID)
	if err != nil {
		t.Fatalf("commitFiles first failed: %v", err)
	}
	if _, ok := files[".nova/interactive/stories/story-1.json"]; ok {
		t.Fatalf("interactive data should not be committed: %v", sortedVersionFilePaths(files))
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "interactive", "stories", "story-1.json")); err != nil {
		t.Fatalf("interactive data should remain in workspace: %v", err)
	}

	writeFile(t, dir, "chapters/ch0001.md", "第二版")
	if _, err := service.Create("第二版本", VersionSourceManual, settings); err != nil {
		t.Fatalf("Create second failed: %v", err)
	}

	if _, err := service.Restore(first.Version.ID, settings); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "interactive", "stories", "story-1.json")); err != nil {
		t.Fatalf("interactive data should survive version restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nova", "interactive", "memory", "book.json")); err != nil {
		t.Fatalf("interactive memory should survive version restore: %v", err)
	}

	writeFile(t, dir, ".nova/interactive/stories/story-2.json", `{"title":"新故事"}`)
	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !status.Clean {
		t.Fatalf("interactive data changes should not dirty version status: %#v", status.Changes)
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

func sortedVersionFilePaths(files map[string]versionFileData) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func historyContains(items []VersionEntry, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func historyContainsSource(items []VersionEntry, source string) bool {
	for _, item := range items {
		if item.Source == source {
			return true
		}
	}
	return false
}
