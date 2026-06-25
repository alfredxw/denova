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

func TestDisplayEventsPersistOutsideEffectiveContext(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("帮我规划下一章")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{Role: "thinking", Content: "先分析角色动机"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-1", Role: "tool_call", Name: "read_file", Content: "read_file", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayToolArgs("call-1", "read_file", `{"path":"chapters/1.md"}`); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("call-1", "read_file", "success", "章节内容"); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.AssistantMessage("规划完成", nil)); err != nil {
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
	effective := reloaded.GetEffectiveMessages()
	if len(effective) != 2 {
		t.Fatalf("展示事件不应进入 Agent 有效上下文: %#v", effective)
	}
	history := reloaded.History()
	if len(history) != 4 {
		t.Fatalf("历史应包含 user/thinking/tool/assistant: %#v", history)
	}
	if history[1].Role != "thinking" || history[1].Content != "先分析角色动机" {
		t.Fatalf("thinking 展示事件未恢复: %#v", history[1])
	}
	if history[2].Role != "tool_call" || history[2].Name != "read_file" || history[2].Status != "success" {
		t.Fatalf("工具卡片展示状态未恢复: %#v", history[2])
	}
	if history[2].Args != `{"path":"chapters/1.md"}` || history[2].Result != "章节内容" {
		t.Fatalf("工具卡片参数和结果未恢复: %#v", history[2])
	}
}

func TestSubAgentAssistantDisplayChunksPersistOutsideEffectiveContext(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("委派调研")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID:                "run-1-subagent-01-researcher",
		Role:              "assistant",
		Content:           "第一段",
		RunID:             "run-1",
		AgentName:         "researcher",
		RootAgentName:     "NovaAgent",
		RunPath:           []string{"NovaAgent", "researcher"},
		SubAgent:          true,
		SubAgentSessionID: "run-1-subagent-01-researcher",
		SubAgentType:      "researcher",
	}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEventContent("run-1-subagent-01-researcher", "assistant", "第二段"); err != nil {
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
	if effective := reloaded.GetEffectiveMessages(); len(effective) != 1 {
		t.Fatalf("SubAgent 展示正文不应进入有效上下文: %#v", effective)
	}
	history := reloaded.History()
	if len(history) != 2 {
		t.Fatalf("历史应包含 user/subagent display: %#v", history)
	}
	if got := history[1].Content; got != "第一段第二段" {
		t.Fatalf("SubAgent 展示正文未合并恢复: %q", got)
	}
	if !history[1].SubAgent || history[1].SubAgentSessionID != "run-1-subagent-01-researcher" || history[1].SubAgentType != "researcher" {
		t.Fatalf("SubAgent metadata 未恢复: %#v", history[1])
	}
}

func TestDisplayToolArgsDeltasArePersistedOnFinalResult(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-1", Role: "tool_call", Name: "write_file", Content: "write_file", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	smallArgs := `{"path":"chapters/ch01.md","content":"draft"}`
	if err := sess.AppendDisplayToolArgs("call-1", "write_file", smallArgs); err != nil {
		t.Fatal(err)
	}
	if history := sess.History(); len(history) != 1 || history[0].Args != smallArgs {
		t.Fatalf("内存历史应实时累积工具参数: %#v", history)
	}

	reloadedBeforeResult, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	beforeResult, err := reloadedBeforeResult.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	if history := beforeResult.History(); len(history) != 1 || history[0].Args != "" {
		t.Fatalf("小块工具参数不应每帧落盘: %#v", history)
	}

	if err := sess.UpdateDisplayToolResult("call-1", "write_file", "success", "ok"); err != nil {
		t.Fatal(err)
	}
	reloadedAfterResult, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	afterResult, err := reloadedAfterResult.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	history := afterResult.History()
	if len(history) != 1 || history[0].Args != smallArgs || history[0].Result != "ok" || history[0].Status != "success" {
		t.Fatalf("工具结束时应落盘完整工具卡片状态: %#v", history)
	}
}

func TestDisplayToolArgsPreviewIsBounded(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-1", Role: "tool_call", Name: "write_file", Content: "write_file", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	largeArgs := `{"path":"chapters/ch01.md","content":"` + strings.Repeat("长内容", displayToolArgsPreviewBytes) + `"}`
	if err := sess.AppendDisplayToolArgs("call-1", "write_file", largeArgs); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("call-1", "write_file", "success", "ok"); err != nil {
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
	history := reloaded.History()
	if len(history) != 1 {
		t.Fatalf("历史应只包含工具展示事件: %#v", history)
	}
	if len(history[0].Args) > displayToolArgsPreviewBytes {
		t.Fatalf("工具参数预览应有硬上限: %d", len(history[0].Args))
	}
	if !strings.HasSuffix(history[0].Args, displayToolArgsTruncatedHint) {
		t.Fatalf("超长工具参数应标记为已截断: %q", history[0].Args[len(history[0].Args)-80:])
	}
}

func TestUpdateDisplayToolResultFallsBackToNameWhenIDMissing(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-execute", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("", "execute", "success", "command done"); err != nil {
		t.Fatal(err)
	}

	history := sess.History()
	if len(history) != 1 {
		t.Fatalf("历史应只包含工具展示事件: %#v", history)
	}
	if history[0].Status != "success" || history[0].Result != "command done" {
		t.Fatalf("id 缺失时应按唯一工具名更新工具卡片: %#v", history[0])
	}
}

func TestUpdateDisplayToolResultDoesNotFallbackWhenIDDiffers(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-execute", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("stale-id", "execute", "success", "stale result"); err != nil {
		t.Fatal(err)
	}

	history := sess.History()
	if len(history) != 1 {
		t.Fatalf("历史应只包含工具展示事件: %#v", history)
	}
	if history[0].Result == "stale result" || history[0].Status != "running" {
		t.Fatalf("id 不一致时不应按工具名更新工具卡片: %#v", history[0])
	}
}

func TestUpdateDisplayToolResultDoesNotFallbackWhenNameIsAmbiguous(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "execute-1", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "execute-2", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("stale-id", "execute", "success", "ambiguous result"); err != nil {
		t.Fatal(err)
	}

	for _, message := range sess.History() {
		if message.Result == "ambiguous result" || message.Status != "running" {
			t.Fatalf("同名工具调用存在歧义时不应按工具名误更新: %#v", message)
		}
	}
}

func TestTokenUsageDisplayEventPersistsOutsideEffectiveContext(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("统计一下")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID:                   "run-1",
		Role:                 "token_usage",
		Content:              "cache_hit_rate=50.0%",
		RunID:                "run-1",
		AgentKind:            "ide",
		PromptTokens:         2000,
		CachedPromptTokens:   1000,
		UncachedPromptTokens: 1000,
		CacheHitRate:         0.5,
		CompletionTokens:     300,
		ReasoningTokens:      40,
		TotalTokens:          2300,
		ModelCalls:           2,
		GeneratedBytes:       128,
		UsageCalls: []TokenUsageCall{
			{Index: 1, PromptTokens: 800, CachedPromptTokens: 400, UncachedPromptTokens: 400, CacheHitRate: 0.5, CompletionTokens: 120, ReasoningTokens: 10, TotalTokens: 920},
			{Index: 2, PromptTokens: 1200, CachedPromptTokens: 600, UncachedPromptTokens: 600, CacheHitRate: 0.5, CompletionTokens: 180, ReasoningTokens: 30, TotalTokens: 1380},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.AssistantMessage("统计完成", nil)); err != nil {
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
	effective := reloaded.GetEffectiveMessages()
	if len(effective) != 2 {
		t.Fatalf("usage display event must not enter effective context: %#v", effective)
	}
	history := reloaded.History()
	if len(history) != 3 {
		t.Fatalf("history should include token usage display event: %#v", history)
	}
	usage := history[1]
	if usage.Role != "token_usage" || usage.RunID != "run-1" || usage.PromptTokens != 2000 || usage.CachedPromptTokens != 1000 {
		t.Fatalf("usage event was not restored: %#v", usage)
	}
	if usage.UncachedPromptTokens != 1000 {
		t.Fatalf("uncached prompt tokens were not restored: %#v", usage)
	}
	if usage.CacheHitRate != 0.5 || usage.TotalTokens != 2300 || usage.ModelCalls != 2 || usage.GeneratedBytes != 128 {
		t.Fatalf("usage metrics were not restored: %#v", usage)
	}
	if len(usage.UsageCalls) != 2 || usage.UsageCalls[1].PromptTokens != 1200 || usage.UsageCalls[1].CachedPromptTokens != 600 {
		t.Fatalf("usage call details were not restored: %#v", usage.UsageCalls)
	}
	if usage.UsageCalls[1].UncachedPromptTokens != 600 {
		t.Fatalf("usage call uncached tokens were not restored: %#v", usage.UsageCalls)
	}
}

func TestContextCompactionPersistsOutsideVisibleHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("第一轮")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.AssistantMessage("第一答", nil)); err != nil {
		t.Fatal(err)
	}
	record, err := sess.AppendContextCompaction(ContextCompaction{
		AgentKind:           "ide",
		Summary:             "保留目标和决定",
		SourceStartIndex:    0,
		SourceEndIndex:      2,
		RetainedTurns:       8,
		TokensBefore:        900,
		TokensAfter:         120,
		ContextWindowTokens: 1000,
		Threshold:           0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Epoch != 1 {
		t.Fatalf("compaction epoch = %d, want 1", record.Epoch)
	}
	if len(sess.GetEffectiveMessages()) != 2 {
		t.Fatalf("compaction must not alter effective raw messages: %#v", sess.GetEffectiveMessages())
	}
	if history := sess.History(); len(history) != 2 {
		t.Fatalf("compaction must not appear in user-visible history: %#v", history)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	latest, ok := reloaded.LatestContextCompaction("ide")
	if !ok {
		t.Fatal("expected reloaded compaction record")
	}
	if latest.Summary != "保留目标和决定" || latest.SourceEndIndex != 2 {
		t.Fatalf("unexpected reloaded compaction: %#v", latest)
	}
	if history := reloaded.History(); len(history) != 2 {
		t.Fatalf("reloaded visible history should stay raw: %#v", history)
	}
}

func TestContextCompactionRemovalRestoresRawHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("第一轮")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.AssistantMessage("第一答", nil)); err != nil {
		t.Fatal(err)
	}
	record, err := sess.AppendContextCompaction(ContextCompaction{
		AgentKind:           "ide",
		Summary:             "旧摘要",
		SourceStartIndex:    0,
		SourceEndIndex:      2,
		RetainedTurns:       8,
		TokensBefore:        900,
		TokensAfter:         120,
		ContextWindowTokens: 1000,
		Threshold:           0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	removal, removed, err := sess.RemoveLatestContextCompaction("ide", "user_rejected")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected active compaction to be removed")
	}
	if removal.CompactionID != record.ID || removal.SourceEndIndex != record.SourceEndIndex {
		t.Fatalf("unexpected removal record: %#v for compaction %#v", removal, record)
	}
	if _, ok := sess.LatestContextCompaction("ide"); ok {
		t.Fatal("removed compaction should not be active")
	}
	if latestRemoval, ok := sess.LatestContextCompactionRemoval("ide"); !ok || latestRemoval.CompactionID != record.ID {
		t.Fatalf("expected latest removal for record %s, got %#v ok=%v", record.ID, latestRemoval, ok)
	}
	if history := sess.History(); len(history) != 2 {
		t.Fatalf("visible history should stay raw after removal: %#v", history)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.LatestContextCompaction("ide"); ok {
		t.Fatal("removed compaction should stay inactive after reload")
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
