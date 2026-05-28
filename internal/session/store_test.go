package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestClearMarkerKeepsHistoryAndLimitsEffectiveContext(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("清理前用户")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.AssistantMessage("清理前助手", nil)); err != nil {
		t.Fatal(err)
	}
	if err := sess.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("清理后用户")); err != nil {
		t.Fatal(err)
	}

	all := sess.GetMessages()
	if len(all) != 3 {
		t.Fatalf("clear 不应删除历史消息，实际消息数: %d", len(all))
	}
	effective := sess.GetEffectiveMessages()
	if len(effective) != 1 || effective[0].Content != "清理后用户" {
		t.Fatalf("有效上下文应只包含 clear 后消息: %#v", effective)
	}
	history := sess.History()
	if len(history) != 4 || history[2].Type != "clear" {
		t.Fatalf("历史中应保留 clear 分界: %#v", history)
	}
}

func TestLoadLegacyJSONLWithoutClearMarkerUsesFullHistory(t *testing.T) {
	dir := t.TempDir()
	legacy := strings.Join([]string{
		`{"type":"session","id":"legacy","created_at":"2026-01-01T00:00:00Z"}`,
		`{"role":"user","content":"旧问题"}`,
		`{"role":"assistant","content":"旧回答"}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "legacy.jsonl"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}

	effective := sess.GetEffectiveMessages()
	if len(effective) != 2 {
		t.Fatalf("旧文件无 clear 标记时应全部作为有效上下文: %d", len(effective))
	}
	if got := sess.Title(); got != "旧问题" {
		t.Fatalf("旧文件应从首条用户消息推导标题: %s", got)
	}
}

func TestMultipleSessionsAreIsolatedAndActiveSessionPersists(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Append(schema.UserMessage("会话 A")); err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("会话 B")
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Append(schema.UserMessage("会话 B")); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveID(second.ID); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	active, err := reloaded.GetActiveOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != second.ID {
		t.Fatalf("应恢复最近激活会话: want=%s got=%s", second.ID, active.ID)
	}
	if active.GetMessages()[0].Content != "会话 B" {
		t.Fatalf("激活会话上下文不应串到其他会话: %#v", active.GetMessages())
	}

	metas, err := reloaded.List(active.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("应列出两个会话: %#v", metas)
	}
	if !metas[0].Active {
		t.Fatalf("会话列表应标记当前激活会话: %#v", metas)
	}
}

func TestDeleteRejectsOnlySession(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("default"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("default"); err == nil {
		t.Fatal("删除唯一会话应失败")
	}
}

func TestListAndDeleteByPrefixForInteractiveSessions(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("default"); err != nil {
		t.Fatal(err)
	}
	matching, err := store.GetOrCreate("interactive-story-st_001-main")
	if err != nil {
		t.Fatal(err)
	}
	if err := matching.Append(schema.UserMessage("互动故事")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("interactive-story-st_002-main"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("interactive-setting-main"); err != nil {
		t.Fatal(err)
	}

	metas, err := store.ListByPrefix("interactive-story-st_001-")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != "interactive-story-st_001-main" {
		t.Fatalf("unexpected prefix sessions: %#v", metas)
	}

	if err := store.DeleteByPrefix("interactive-story-st_001-"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("interactive-story-st_001-main"); err == nil {
		t.Fatal("matching interactive session should be deleted")
	}
	if _, err := store.Get("interactive-story-st_002-main"); err != nil {
		t.Fatalf("other story session should remain: %v", err)
	}
	if _, err := store.Get("default"); err != nil {
		t.Fatalf("default session should remain: %v", err)
	}
}

func TestInterruptionPersistsPendingRecordAndCanResolve(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.MarkInterrupted("写第一章", "已经写出的片段", "runner error"); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	pending := reloaded.PendingInterruption()
	if pending == nil {
		t.Fatal("异常中断标识应在重载后保留")
	}
	if pending.UserMessage != "写第一章" || pending.AssistantContent != "已经写出的片段" || pending.Reason != "runner error" {
		t.Fatalf("异常中断信息不完整: %#v", pending)
	}

	if err := reloaded.ResolveInterruption(pending.ID); err != nil {
		t.Fatal(err)
	}
	reloadedAgain, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	if got := reloadedAgain.PendingInterruption(); got != nil {
		t.Fatalf("已解决的中断不应继续待恢复: %#v", got)
	}
}
