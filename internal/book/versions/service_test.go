package versions

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestDefaultAutoSettingsEnablesAutomaticVersionsEveryTenMinutes(t *testing.T) {
	settings := DefaultAutoSettings()
	if !settings.TimedEnabled {
		t.Fatal("automatic versions should default on")
	}
	if settings.TimedIntervalMinutes != 10 {
		t.Fatalf("automatic version interval=%d want=10", settings.TimedIntervalMinutes)
	}
}

func TestScheduleAutoVersionDebouncesWorkspaceChanges(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	service.autoVersionIdleDelay = 30 * time.Millisecond
	t.Cleanup(service.Close)
	settings := DefaultAutoSettings()

	writeFile(t, dir, "chapters/ch0001.md", "第一段")
	service.ScheduleAutoVersion(settings)
	time.Sleep(20 * time.Millisecond)
	writeFile(t, dir, "chapters/ch0001.md", "第一段，继续写作")
	service.ScheduleAutoVersion(settings)
	time.Sleep(20 * time.Millisecond)

	history, err := service.History(10)
	if err != nil {
		t.Fatalf("History before idle failed: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("automatic version should wait for the latest idle period: %#v", history)
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(history) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		history, err = service.History(10)
		if err != nil {
			t.Fatalf("History after idle failed: %v", err)
		}
	}
	if len(history) != 1 || history[0].Source != VersionSourceTimer {
		t.Fatalf("expected one automatic version after idle, got %#v", history)
	}
}

func TestScheduleAutoVersionCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	service.autoVersionIdleDelay = 30 * time.Millisecond
	t.Cleanup(service.Close)
	settings := DefaultAutoSettings()

	writeFile(t, dir, "chapters/ch0001.md", "不会自动创建版本")
	service.ScheduleAutoVersion(settings)
	settings.TimedEnabled = false
	service.ConfigureAutoVersion(settings)
	time.Sleep(60 * time.Millisecond)

	history, err := service.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("disabled automatic versions should not create history: %#v", history)
	}
}

func TestMaybeCreateTimedHonorsConfiguredMinimumInterval(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	settings.TimedIntervalMinutes = 20

	writeFile(t, dir, "chapters/ch0001.md", "第一版")
	first, err := service.MaybeCreateTimed(settings)
	if err != nil || first.Version == nil {
		t.Fatalf("create first automatic version: result=%#v err=%v", first, err)
	}
	writeFile(t, dir, "chapters/ch0001.md", "第二版")
	second, err := service.MaybeCreateTimed(settings)
	if err != nil {
		t.Fatalf("check second automatic version: %v", err)
	}
	if !second.Skipped || second.RetryAfter < 19*time.Minute {
		t.Fatalf("configured interval should defer the second version: %#v", second)
	}
}

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
	if len(history) != 2 ||
		!historyContains(history, first.Version.ID) ||
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

func TestGoGitVersionExcludesAndPreservesReviewJournals(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "chapters/ch0001.md", "第一版")
	writeFile(t, dir, ".denova/changes/ledger.jsonl", `{"type":"change_applied"}`)
	writeFile(t, dir, ".nova/changes/legacy.jsonl", `{"type":"comment_added"}`)
	writeFile(t, dir, ".denova/reviews/ledger.jsonl", `{"type":"comments_upserted"}`)
	writeFile(t, dir, ".nova/reviews/legacy.jsonl", `{"type":"comments_upserted"}`)

	first, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	files, err := service.commitFiles(first.Version.ID)
	if err != nil {
		t.Fatalf("commitFiles first failed: %v", err)
	}
	for _, path := range []string{
		".denova/changes/ledger.jsonl",
		".nova/changes/legacy.jsonl",
		".denova/reviews/ledger.jsonl",
		".nova/reviews/legacy.jsonl",
	} {
		if _, ok := files[path]; ok {
			t.Fatalf("review journal must not be committed: path=%s files=%v", path, sortedVersionFilePaths(files))
		}
	}

	writeFile(t, dir, "chapters/ch0001.md", "第二版")
	if _, err := service.Restore(first.Version.ID, settings); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if got := readFile(t, dir, ".denova/changes/ledger.jsonl"); got != `{"type":"change_applied"}` {
		t.Fatalf("current change journal should survive restore: %q", got)
	}
	if got := readFile(t, dir, ".nova/changes/legacy.jsonl"); got != `{"type":"comment_added"}` {
		t.Fatalf("legacy change journal should survive restore: %q", got)
	}
	if got := readFile(t, dir, ".denova/reviews/ledger.jsonl"); got != `{"type":"comments_upserted"}` {
		t.Fatalf("current document review journal should survive restore: %q", got)
	}
	if got := readFile(t, dir, ".nova/reviews/legacy.jsonl"); got != `{"type":"comments_upserted"}` {
		t.Fatalf("legacy document review journal should survive restore: %q", got)
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
		t.Fatalf("legacy interactive data should survive version restore: %v", err)
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

func TestGoGitVersionRestorePathsKeepsCurrentHead(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "chapters/ch0001.md", "第一版")
	writeFile(t, dir, "setting/progress.md", "进度一")

	first, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	writeFile(t, dir, "chapters/ch0001.md", "第二版")
	writeFile(t, dir, "chapters/ch0002.md", "新增章节")
	if err := os.Remove(filepath.Join(dir, "setting", "progress.md")); err != nil {
		t.Fatal(err)
	}
	second, err := service.Create("第二版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create second failed: %v", err)
	}

	plan, err := service.RestorePlan(first.Version.ID, []string{"chapters/ch0001.md", "setting/progress.md", "chapters/ch0002.md"}, settings)
	if err != nil {
		t.Fatalf("RestorePlan paths failed: %v", err)
	}
	if plan.Scope != VersionRestoreScopePaths || plan.WillCreateBackup || len(plan.Changes) != 3 {
		t.Fatalf("unexpected restore plan: %#v", plan)
	}

	result, err := service.RestoreWithPaths(first.Version.ID, []string{"chapters/ch0001.md", "setting/progress.md", "chapters/ch0002.md"}, settings)
	if err != nil {
		t.Fatalf("RestoreWithPaths failed: %v", err)
	}
	if result.Scope != VersionRestoreScopePaths || result.BackupVersion != nil || len(result.RestoredPaths) != 3 {
		t.Fatalf("unexpected restore result: %#v", result)
	}
	if got := readFile(t, dir, "chapters/ch0001.md"); got != "第一版" {
		t.Fatalf("restored modified file = %q", got)
	}
	if got := readFile(t, dir, "setting/progress.md"); got != "进度一" {
		t.Fatalf("restored deleted file = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "chapters", "ch0002.md")); !os.IsNotExist(err) {
		t.Fatalf("file missing in target should be removed, err=%v", err)
	}

	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status after path restore failed: %v", err)
	}
	if status.Latest == nil || status.Latest.ID != second.Version.ID {
		t.Fatalf("path restore should not move current version: %#v", status.Latest)
	}
	assertChange(t, status.Changes, "chapters/ch0001.md", "modified")
	assertChange(t, status.Changes, "setting/progress.md", "added")
	assertChange(t, status.Changes, "chapters/ch0002.md", "deleted")

	history, err := service.History(10)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("path restore should not create a version, history=%#v", history)
	}
}

func TestGoGitVersionRestoreRejectsExcludedPath(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "chapters/ch0001.md", "第一版")
	first, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	if _, err := service.RestorePlan(first.Version.ID, []string{".denova/interactive/stories/story.json"}, settings); err == nil {
		t.Fatalf("RestorePlan should reject excluded paths")
	}
}

func TestGoGitVersionRestoreIgnoredLorePath(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, ".gitignore", ".nova\n")
	writeFile(t, dir, ".nova/lore/items.json", `["old"]`)
	first, err := service.Create("初始资料库", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	writeFile(t, dir, ".nova/lore/items.json", `["new"]`)
	second, err := service.Create("更新资料库", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("Create second failed: %v", err)
	}

	if _, err := service.RestoreWithPaths(first.Version.ID, []string{".nova/lore/items.json"}, settings); err != nil {
		t.Fatalf("RestoreWithPaths lore failed: %v", err)
	}
	if got := readFile(t, dir, ".nova/lore/items.json"); got != `["old"]` {
		t.Fatalf("restored lore = %q", got)
	}
	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Latest == nil || status.Latest.ID != second.Version.ID {
		t.Fatalf("lore path restore should not move current version: %#v", status.Latest)
	}
	assertChange(t, status.Changes, ".nova/lore/items.json", "modified")
}

func TestGoGitVersionRestorePathsRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "linked/secret.md", "versioned")
	version, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}

	if _, err := service.RestoreWithPaths(version.Version.ID, []string{"linked/secret.md"}, settings); err == nil {
		t.Fatal("RestoreWithPaths should reject a target path that escapes through a symlink")
	}
	data, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside" {
		t.Fatalf("outside file was modified by restore: %q", data)
	}
}

func TestGoGitVersionRestorePathsRollsBackOnApplyFailure(t *testing.T) {
	dir := t.TempDir()
	service := NewService(dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "a.md", "versioned-a")
	writeFile(t, dir, "zdir/b.md", "versioned-b")
	version, err := service.Create("初始版本", VersionSourceManual, settings)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.md", "current-a")
	if err := os.RemoveAll(filepath.Join(dir, "zdir")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "zdir", "not-a-directory")

	if _, err := service.RestoreWithPaths(version.Version.ID, []string{"a.md", "zdir/b.md"}, settings); err == nil {
		t.Fatal("RestoreWithPaths should fail when a later target parent is not a directory")
	}
	if got := readFile(t, dir, "a.md"); got != "current-a" {
		t.Fatalf("failed restore left an earlier path partially restored: %q", got)
	}
	if got := readFile(t, dir, "zdir"); got != "not-a-directory" {
		t.Fatalf("failed restore changed the blocking path: %q", got)
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
