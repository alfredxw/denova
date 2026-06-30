package messages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChangelogMessages(t *testing.T) {
	content := `# Changelog

## [Unreleased]

### Added

- 消息中心展示更新日志。

## [v0.1.17] - 2026-06-27

### Fixed

- 修复互动图像预览。
`
	items := parseChangelogMessages(content)
	if len(items) != 2 {
		t.Fatalf("messages length = %d, want 2", len(items))
	}
	if !strings.HasPrefix(items[0].ID, "changelog:unreleased:") || items[0].Title != "Unreleased" {
		t.Fatalf("unreleased message = %#v", items[0])
	}
	if items[0].Summary != "消息中心展示更新日志。" {
		t.Fatalf("summary = %q", items[0].Summary)
	}
	if !strings.HasPrefix(items[1].ID, "changelog:v0.1.17:") || items[1].PublishedAt != "2026-06-27" {
		t.Fatalf("version message = %#v", items[1])
	}
}

func TestServiceMarksReadPersistently(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte(`## [Unreleased]

### Added

- 第一条消息。
`), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithChangelog(filepath.Join(dir, "nova"), changelog)
	list, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if list.UnreadCount != 1 || len(list.Items) != 1 || list.Items[0].ReadAt != nil {
		t.Fatalf("initial list = %#v", list)
	}
	read, err := service.MarkRead(list.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.ReadAt == nil {
		t.Fatalf("read_at not set: %#v", read)
	}
	secondRead, err := service.MarkRead(list.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondRead.ReadAt == nil || !secondRead.ReadAt.Equal(*read.ReadAt) {
		t.Fatalf("idempotent read = %#v, want read_at %v", secondRead, read.ReadAt)
	}
	next, err := NewServiceWithChangelog(filepath.Join(dir, "nova"), changelog).List()
	if err != nil {
		t.Fatal(err)
	}
	if next.UnreadCount != 0 || len(next.Items) != 1 || next.Items[0].ReadAt == nil {
		t.Fatalf("persisted list = %#v", next)
	}
	if _, err := service.MarkRead("changelog:missing"); err == nil {
		t.Fatalf("missing message should fail")
	}
}

func TestServiceMarksAllReadPersistently(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte(`## [Unreleased]

### Added

- 第一条消息。

## [v0.1.17] - 2026-06-27

### Fixed

- 第二条消息。
`), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithChangelog(filepath.Join(dir, "nova"), changelog)
	result, err := service.MarkAllRead()
	if err != nil {
		t.Fatal(err)
	}
	if result.UnreadCount != 0 || len(result.Items) != 2 {
		t.Fatalf("mark all result = %#v", result)
	}
	for _, item := range result.Items {
		if item.ReadAt == nil {
			t.Fatalf("message should be read: %#v", item)
		}
	}
	next, err := NewServiceWithChangelog(filepath.Join(dir, "nova"), changelog).List()
	if err != nil {
		t.Fatal(err)
	}
	if next.UnreadCount != 0 || len(next.Items) != 2 {
		t.Fatalf("persisted mark all = %#v", next)
	}
}

func TestServiceListIgnoresMissingChangelog(t *testing.T) {
	service := NewServiceWithChangelog(t.TempDir(), filepath.Join(t.TempDir(), "missing.md"))
	list, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 || list.UnreadCount != 0 {
		t.Fatalf("missing changelog list = %#v", list)
	}
}
