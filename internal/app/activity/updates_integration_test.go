package activity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/buildinfo"
	"denova/internal/update"
)

func TestUpgradeNotesReadFullInstalledHistoryInRequestLanguage(t *testing.T) {
	previous := buildinfo.Version
	buildinfo.Version = "0.6.15"
	t.Cleanup(func() { buildinfo.Version = previous })
	root := t.TempDir()
	path := filepath.Join(root, "CHANGELOG.md")
	content := "## [Unreleased]\n- Not published\n"
	for version := 0; version <= 15; version++ {
		content += fmt.Sprintf("\n## [v0.6.%d] - 2026-10-20\n### Brief / 简要说明\n#### 中文\n- 中文变化 %d\n#### English\n- English change %d\n### Added / 新增\n- 中文详情 %d\n- English detail %d\n", version, version, version, version, version)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DENOVA_CHANGELOG_PATH", path)
	service := &Service{dataDir: root, updates: update.NewService()}
	for _, locale := range []string{"zh-CN", "en-US"} {
		notes, err := service.UpdateNotes(context.Background(), "v0.6.0", "v0.6.15", locale)
		if err != nil || !notes.Complete || len(notes.Releases) != 15 {
			t.Fatalf("range was truncated to the notification feed: %+v, %v", notes, err)
		}
		for index, release := range notes.Releases {
			if release.Version != fmt.Sprintf("v0.6.%d", 15-index) {
				t.Fatalf("incorrect numeric version order: %+v", notes.Releases)
			}
			if locale == "zh-CN" && (strings.Contains(release.Brief+release.Body, "English") || strings.Contains(release.Body, "Added") || !strings.Contains(release.Brief, "中文变化") || !strings.Contains(release.Body, "中文详情")) {
				t.Fatalf("incorrect Chinese release: %+v", release)
			}
			if locale == "en-US" && (strings.Contains(release.Brief+release.Body, "中文") || strings.Contains(release.Body, "新增") || !strings.Contains(release.Brief, "English change") || !strings.Contains(release.Body, "English detail")) {
				t.Fatalf("incorrect English release: %+v", release)
			}
			if strings.Contains(release.Body, "change") || strings.Contains(release.Body, "变化") {
				t.Fatalf("brief was duplicated in details: %+v", release)
			}
		}
	}
	history, err := service.UpdateNotes(context.Background(), "", "0.6.15", "en-US")
	if err != nil || !history.Complete || len(history.Releases) != 16 {
		t.Fatalf("published history = %+v, %v", history, err)
	}
	partial, err := service.UpdateNotes(context.Background(), "0.5.1", "0.6.15", "en-US")
	if err != nil || partial.Complete {
		t.Fatalf("missing baseline was reported as a complete range: %+v, %v", partial, err)
	}
	if _, err := service.UpdateNotes(context.Background(), "0.6.15", "0.6.15", "en-US"); !errors.Is(err, ErrUpdateNotesRange) {
		t.Fatalf("accepted an empty upgrade range: %v", err)
	}
}
