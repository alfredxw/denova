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
	if len(items) != 1 {
		t.Fatalf("messages length = %d, want 1", len(items))
	}
	if !strings.HasPrefix(items[0].ID, "changelog:v0.1.17:") || items[0].PublishedAt != "2026-06-27" {
		t.Fatalf("version message = %#v", items[0])
	}
}

func TestParseChangelogMessagesForLocaleFiltersBilingualContent(t *testing.T) {
	content := `# Changelog

## [v0.2.0] - 2026-07-01

### Brief / 简要说明

#### 中文

- 中文简要第一条。
- 中文简要第二条。

#### English

- English brief first item.
- English brief second item.

### Added

- 消息中心只展示中文更新。
- Message center only shows English updates.

### Fixed

- 中文：修复中文摘要。
- English: Fixed the English summary.
`
	zhItems := parseChangelogMessagesForLocale(content, "zh-CN")
	enItems := parseChangelogMessagesForLocale(content, "en-US")
	if len(zhItems) != 1 || len(enItems) != 1 {
		t.Fatalf("localized message lengths = zh %d, en %d", len(zhItems), len(enItems))
	}
	if zhItems[0].ID != enItems[0].ID {
		t.Fatalf("localized ids differ: zh %q, en %q", zhItems[0].ID, enItems[0].ID)
	}
	if zhItems[0].Summary != "中文简要第一条。" {
		t.Fatalf("zh summary = %q", zhItems[0].Summary)
	}
	if enItems[0].Summary != "English brief first item." {
		t.Fatalf("en summary = %q", enItems[0].Summary)
	}
	if strings.Contains(zhItems[0].Body, "English") || strings.Contains(zhItems[0].Body, "Message center") || strings.Contains(zhItems[0].Body, "Brief") {
		t.Fatalf("zh body leaked English content:\n%s", zhItems[0].Body)
	}
	if strings.Contains(enItems[0].Body, "中文") || strings.Contains(enItems[0].Body, "消息中心") || strings.Contains(enItems[0].Body, "简要说明") {
		t.Fatalf("en body leaked Chinese content:\n%s", enItems[0].Body)
	}
	if !strings.Contains(zhItems[0].Body, "### 简要说明") || !strings.Contains(zhItems[0].Body, "### 新增") || !strings.Contains(zhItems[0].Body, "### 修复") {
		t.Fatalf("zh body did not localize headings:\n%s", zhItems[0].Body)
	}
	if !strings.Contains(enItems[0].Body, "### Brief") || !strings.Contains(enItems[0].Body, "### Added") || !strings.Contains(enItems[0].Body, "### Fixed") {
		t.Fatalf("en body did not localize headings:\n%s", enItems[0].Body)
	}
}

func TestServiceMarksReadPersistently(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte(`## [Unreleased]

### Added

- 第一条消息。

## [v0.2.0] - 2026-07-01

### Added

- 正式发布消息。
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
	if list.Items[0].Title != "v0.2.0" {
		t.Fatalf("unreleased changelog should be skipped: %#v", list.Items[0])
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

func TestServiceReadStateIsSharedAcrossLocales(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte(`## [v0.2.0] - 2026-07-01

### Brief / 简要说明

#### 中文

- 中文简要。

#### English

- English brief.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithChangelog(filepath.Join(dir, "nova"), changelog)
	zhList, err := service.ListForLocale("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	enList, err := service.ListForLocale("en-US")
	if err != nil {
		t.Fatal(err)
	}
	if len(zhList.Items) != 1 || len(enList.Items) != 1 || zhList.Items[0].ID != enList.Items[0].ID {
		t.Fatalf("localized lists = zh %#v, en %#v", zhList, enList)
	}
	if _, err := service.MarkReadForLocale(zhList.Items[0].ID, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	next, err := service.ListForLocale("en-US")
	if err != nil {
		t.Fatal(err)
	}
	if next.UnreadCount != 0 || len(next.Items) != 1 || next.Items[0].ReadAt == nil {
		t.Fatalf("read state should be shared across locales: %#v", next)
	}
}

func TestServiceMarksAllReadPersistently(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte(`## [Unreleased]

### Added

- 第一条消息。

## [v0.2.0] - 2026-07-01

### Added

- 正式发布消息。

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
