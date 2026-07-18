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
	if !strings.HasPrefix(items[0].ID, "changelog:v0.1.17:") || items[0].PublishedAt != "2026-06-27T00:00:00Z" {
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

func TestServiceChangelogForLocaleIgnoresMissingChangelog(t *testing.T) {
	service := NewServiceWithChangelog(t.TempDir(), filepath.Join(t.TempDir(), "missing.md"))
	items, err := service.ChangelogForLocale("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("missing changelog items = %#v", items)
	}
}

func TestServiceReadStateIsEmptyForFreshService(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte("## [v0.2.0] - 2026-07-01\n\n### Added\n\n- 正式发布消息。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithChangelog(filepath.Join(dir, "nova"), changelog)
	state, err := service.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 0 {
		t.Fatalf("fresh read state = %#v, want empty", state)
	}
}

func TestServiceMarkReadPersistsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte("## [v0.2.0] - 2026-07-01\n\n### Added\n\n- 正式发布消息。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	novaDir := filepath.Join(dir, "nova")
	service := NewServiceWithChangelog(novaDir, changelog)

	items, err := service.ChangelogForLocale("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("changelog items = %d, want 1", len(items))
	}
	id := items[0].ID

	first, err := service.MarkRead(id)
	if err != nil {
		t.Fatal(err)
	}
	if first.IsZero() {
		t.Fatalf("first read time is zero")
	}

	// Idempotent: second read returns the same time.
	second, err := service.MarkRead(id)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Equal(second) {
		t.Fatalf("idempotent read = %v, want %v", second, first)
	}

	// Persisted across service instances.
	next := NewServiceWithChangelog(novaDir, changelog)
	state, err := next.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if rt, ok := state[id]; !ok || !rt.Equal(first) {
		t.Fatalf("persisted read state = %#v, want %v", state, first)
	}

	// Empty id fails.
	if _, err := service.MarkRead(""); err == nil {
		t.Fatalf("empty id should fail")
	}
}

func TestServiceReadStateIsSharedAcrossLocales(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte("## [v0.2.0] - 2026-07-01\n\n### Brief / 简要说明\n\n#### 中文\n\n- 中文简要。\n\n#### English\n\n- English brief.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	novaDir := filepath.Join(dir, "nova")
	service := NewServiceWithChangelog(novaDir, changelog)

	zhItems, err := service.ChangelogForLocale("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	enItems, err := service.ChangelogForLocale("en-US")
	if err != nil {
		t.Fatal(err)
	}
	if len(zhItems) != 1 || len(enItems) != 1 || zhItems[0].ID != enItems[0].ID {
		t.Fatalf("localized items = zh %#v, en %#v", zhItems, enItems)
	}

	if _, err := service.MarkRead(zhItems[0].ID); err != nil {
		t.Fatal(err)
	}

	// Read state is shared: a new service sees the mark regardless of locale.
	next := NewServiceWithChangelog(novaDir, changelog)
	state, err := next.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state[zhItems[0].ID]; !ok {
		t.Fatalf("read state should be shared across locales: %#v", state)
	}
}

func TestServiceMarkAllReadPersists(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(changelog, []byte("## [Unreleased]\n\n### Added\n\n- 第一条消息。\n\n## [v0.2.0] - 2026-07-01\n\n### Added\n\n- 正式发布消息。\n\n## [v0.1.17] - 2026-06-27\n\n### Fixed\n\n- 第二条消息。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	novaDir := filepath.Join(dir, "nova")
	service := NewServiceWithChangelog(novaDir, changelog)

	items, err := service.ChangelogForLocale("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("changelog items = %d, want 2", len(items))
	}

	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	if err := service.MarkAllRead(ids); err != nil {
		t.Fatal(err)
	}

	// Persisted across service instances.
	next := NewServiceWithChangelog(novaDir, changelog)
	state, err := next.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, ok := state[id]; !ok {
			t.Fatalf("id %s not in read state: %#v", id, state)
		}
	}
}

func TestServiceMarkAllReadIgnoresEmptyIDs(t *testing.T) {
	dir := t.TempDir()
	service := NewServiceWithChangelog(filepath.Join(dir, "nova"), filepath.Join(dir, "missing.md"))
	if err := service.MarkAllRead([]string{"", "  ", "valid-id"}); err != nil {
		t.Fatal(err)
	}
	state, err := service.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state["valid-id"]; !ok {
		t.Fatalf("valid-id should be marked read: %#v", state)
	}
	if len(state) != 1 {
		t.Fatalf("read state should have 1 entry: %#v", state)
	}
}

// TestServiceMarkReadWorksForDynamicIDs verifies that read state works for
// any id (changelog or dynamic), since read state is keyed by id alone.
func TestServiceMarkReadWorksForDynamicIDs(t *testing.T) {
	dir := t.TempDir()
	service := NewServiceWithChangelog(filepath.Join(dir, "nova"), filepath.Join(dir, "missing.md"))

	readAt, err := service.MarkRead("automation-run:run-1")
	if err != nil {
		t.Fatal(err)
	}
	if readAt.IsZero() {
		t.Fatalf("read time is zero for dynamic id")
	}

	state, err := service.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if rt, ok := state["automation-run:run-1"]; !ok || rt.IsZero() {
		t.Fatalf("dynamic id not in read state: %#v", state)
	}
}
