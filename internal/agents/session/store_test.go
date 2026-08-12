package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

func TestAgentMessageWireRoundTripsAllStableFields(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("message-wire")
	if err != nil {
		t.Fatal(err)
	}
	callIndex := 2
	want := &agent.Message{
		Role:                     agent.Assistant,
		Content:                  "result",
		MultiContent:             []json.RawMessage{json.RawMessage(`{"type":"text","text":"multi"}`)},
		UserInputMultiContent:    []json.RawMessage{json.RawMessage(`{"type":"input_text","text":"input"}`)},
		AssistantGenMultiContent: []json.RawMessage{json.RawMessage(`{"type":"output_text","text":"output"}`)},
		Name:                     "writer",
		ToolCalls:                []agent.ToolCall{{Index: &callIndex, ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapter.md"}`}, Extra: map[string]any{"provider": "test"}}},
		ToolCallID:               "parent-call",
		ToolName:                 "task",
		ReasoningContent:         "bounded reasoning",
		Extra:                    map[string]any{"request_id": "req-1", "cached": true},
		ResponseMeta: &agent.ResponseMeta{
			FinishReason: "tool_calls",
			Usage:        &agent.TokenUsage{PromptTokens: 10, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 4}, CompletionTokens: 5, TotalTokens: 15, CompletionTokensDetails: agent.CompletionTokensDetails{ReasoningTokens: 2}},
			LogProbs:     &agent.LogProbs{Content: []agent.LogProb{{Token: "x", LogProb: -0.1, Bytes: []int64{120}, TopLogProbs: []agent.TopLogProb{{Token: "y", LogProb: -0.2}}}}},
		},
	}
	if err := sess.Append(want); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("message-wire")
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.GetMessages()
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("Agent message wire changed across reload:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestClearMarkerKeepsHistoryAndLimitsEffectiveContext(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("清理前用户")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("清理前助手", nil)); err != nil {
		t.Fatal(err)
	}
	if err := sess.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("清理后用户")); err != nil {
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
	history := sess.History()
	if len(history) != 2 {
		t.Fatalf("旧文件历史消息数量错误: %#v", history)
	}
	if history[0].CreatedAt.IsZero() || history[1].CreatedAt.IsZero() {
		t.Fatalf("旧文件历史消息应补齐展示时间: %#v", history)
	}
	if !history[1].CreatedAt.After(history[0].CreatedAt) {
		t.Fatalf("旧文件历史消息展示时间应按文件顺序递增: %#v", history)
	}
}

func TestAssistantMessageMetadataPersistsRunID(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("写一段")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("已完成", nil), MessageMetadata{
		RunID:         "run-1",
		AgentKind:     "ide",
		AgentName:     "DenovaAgent",
		RootAgentName: "DenovaAgent",
		RunPath:       []string{"DenovaAgent"},
	}); err != nil {
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
	if len(history) != 2 {
		t.Fatalf("history entries = %d, want 2: %#v", len(history), history)
	}
	assistant := history[1]
	if assistant.Role != "assistant" || assistant.RunID != "run-1" || assistant.AgentKind != "ide" || assistant.AgentName != "DenovaAgent" || len(assistant.RunPath) != 1 {
		t.Fatalf("assistant metadata was not persisted: %#v", assistant)
	}
	effective := reloaded.GetEffectiveMessages()
	if len(effective) != 2 || effective[1].Content != "已完成" {
		t.Fatalf("metadata should not change model-visible messages: %#v", effective)
	}
}

func TestUserMessageReferencesPersistAcrossSessionReload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendWithMetadata(agent.UserMessage("请修改"), MessageMetadata{UserReferences: []agentcontext.UserReference{
		{Kind: "file", Label: "chapters/ch01.md"},
		{Kind: "review_comment", ID: "comment-1", Label: "setting/progress.md", Detail: "需要增加爽点"},
	}}); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	if len(history) != 1 || len(history[0].UserReferences) != 2 {
		t.Fatalf("reloaded user references = %#v", history)
	}
	if history[0].UserReferences[1].Detail != "需要增加爽点" {
		t.Fatalf("reloaded review detail = %#v", history[0].UserReferences)
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
	if err := sess.Append(agent.UserMessage("帮我规划下一章")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{Role: "thinking", Content: "先分析角色动机"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-1", Role: "tool_call", Name: "read", Content: "read", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayToolArgs("call-1", "read", `{"path":"chapters/1.md"}`); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("call-1", "read", "success", "章节内容"); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("规划完成", nil)); err != nil {
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
	if history[2].Role != "tool_call" || history[2].Name != "read" || history[2].Status != "success" {
		t.Fatalf("工具卡片展示状态未恢复: %#v", history[2])
	}
	if history[2].Args != `{"path":"chapters/1.md"}` || history[2].Result != "章节内容" {
		t.Fatalf("工具卡片参数和结果未恢复: %#v", history[2])
	}
}

func TestOrderedAssistantDisplaySegmentsReplaceAggregatedAssistantInHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("ordered-display")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("继续创作")); err != nil {
		t.Fatal(err)
	}
	for _, event := range []DisplayEvent{
		{ID: "run-order-thinking-1", Role: "thinking", Content: "先分析。", RunID: "run-order"},
		{ID: "run-order-assistant-1", Role: "assistant", Content: "第一段正文。", RunID: "run-order"},
		{ID: "run-order-thinking-2", Role: "thinking", Content: "再检查。", RunID: "run-order"},
		{ID: "run-order-assistant-2", Role: "assistant", Content: "第二段正文。", RunID: "run-order"},
	} {
		if err := sess.AppendDisplayEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("第一段正文。第二段正文。", nil), MessageMetadata{RunID: "run-order"}); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("ordered-display")
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	wantRoles := []string{"user", "thinking", "assistant", "thinking", "assistant"}
	if len(history) != len(wantRoles) {
		t.Fatalf("history = %#v, want roles %v", history, wantRoles)
	}
	for index, role := range wantRoles {
		if history[index].Role != role {
			t.Fatalf("history[%d] = %#v, want role %q", index, history[index], role)
		}
	}
	for _, index := range []int{1, 2, 3, 4} {
		if history[index].DisplaySegmentID == "" || history[index].DisplaySegmentID != history[index].ID {
			t.Fatalf("history[%d] lost display segment identity: %#v", index, history[index])
		}
	}
	if effective := reloaded.GetEffectiveMessages(); len(effective) != 2 || effective[1].Content != "第一段正文。第二段正文。" {
		t.Fatalf("ordered display segments must not change model context: %#v", effective)
	}
}

func TestTerminalAssistantDisplaySegmentReplacesCanonicalAssistantAfterProgress(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("terminal-display")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("删除 ideas.md")); err != nil {
		t.Fatal(err)
	}
	for _, event := range []DisplayEvent{
		{ID: "progress", Role: "assistant", DisplayPhase: DisplayPhaseProgress, Content: "我先确认文件位置。", RunID: "run-1"},
		{ID: "final", Role: "assistant", DisplayPhase: DisplayPhaseFinal, Content: "已完成，ideas.md 已删除。", RunID: "run-1"},
	} {
		if err := sess.AppendDisplayEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := sess.AppendWithMetadata(
		agent.AssistantMessage("已完成，ideas.md 已删除。", nil),
		MessageMetadata{RunID: "run-1"},
	); err != nil {
		t.Fatal(err)
	}

	history := sess.History()
	if len(history) != 3 || history[0].Role != "user" || history[1].DisplayPhase != DisplayPhaseProgress || history[2].DisplayPhase != DisplayPhaseFinal {
		t.Fatalf("resident history kept duplicate canonical assistant: %#v", history)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedStore.Close() })
	reopened, err := reopenedStore.Get("terminal-display")
	if err != nil {
		t.Fatal(err)
	}
	page, err := reopened.ReadHistoryPage(context.Background(), -1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Entries) != 3 || page.Entries[2].DisplayPhase != DisplayPhaseFinal {
		t.Fatalf("paged history kept duplicate canonical assistant: %#v", page)
	}
}

func TestIncompleteAssistantDisplaySegmentsDoNotHideCanonicalHistory(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("incomplete-display")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID: "run-incomplete-display-001-assistant", Role: "assistant", Content: "第一段。", RunID: "run-incomplete",
	}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("第一段。第二段。", nil), MessageMetadata{RunID: "run-incomplete"}); err != nil {
		t.Fatal(err)
	}

	history := sess.History()
	if len(history) != 2 || history[0].Content != "第一段。" || history[1].Content != "第一段。第二段。" {
		t.Fatalf("incomplete display projection must retain canonical fallback: %#v", history)
	}
}

func TestContextMessagesPersistInEffectiveContextButNotHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("读取第一章")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessage(agent.AssistantMessage("", []agent.ToolCall{{
		ID:   "call-read",
		Type: "function",
		Function: agent.FunctionCall{
			Name:      "read",
			Arguments: `{"path":"chapters/1.md"}`,
		},
	}})); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessage(agent.ToolMessage(agent.TextToolResult("第一章内容"), "call-read", agent.WithToolName("read"))); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("已读取", nil)); err != nil {
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
	if len(effective) != 4 {
		t.Fatalf("context messages should enter effective context: %#v", effective)
	}
	if effective[1].Role != agent.Assistant || len(effective[1].ToolCalls) != 1 || effective[2].Role != agent.ToolRole || effective[2].Content != "第一章内容" {
		t.Fatalf("context tool chain mismatch: %#v", effective)
	}
	history := reloaded.History()
	if len(history) != 2 {
		t.Fatalf("context messages should stay hidden from UI history: %#v", history)
	}
	if count := reloaded.MessageCount(); count != 2 {
		t.Fatalf("visible message count should ignore context messages, got %d", count)
	}
}

func TestHistoryNormalizesRunningToolAfterSameRunTokenUsage(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID:      "call-execute",
		Role:    "tool_call",
		Name:    "bash",
		Content: "bash",
		Status:  "running",
		RunID:   "run-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID:      "run-2",
		Role:    "tool_call",
		Name:    "bash",
		Content: "bash",
		Status:  "running",
		RunID:   "run-2",
	}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID:     "run-1",
		Role:   "token_usage",
		Name:   "token_usage",
		RunID:  "run-1",
		Status: "success",
	}); err != nil {
		t.Fatal(err)
	}

	history := sess.History()
	if history[0].Status != "success" {
		t.Fatalf("same-run completed tool should be shown as success: %#v", history[0])
	}
	if history[1].Status != "running" {
		t.Fatalf("different run without token_usage should stay running: %#v", history[1])
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
	if err := sess.Append(agent.UserMessage("委派调研")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{
		ID:                "run-1-subagent-01-researcher",
		Role:              "assistant",
		Content:           "第一段",
		RunID:             "run-1",
		AgentName:         "researcher",
		RootAgentName:     "DenovaAgent",
		RunPath:           []string{"DenovaAgent", "researcher"},
		SubAgent:          true,
		SubAgentSessionID: "run-1-subagent-01-researcher",
		SubAgentType:      "researcher",
	}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEventContent("run-1-subagent-01-researcher", "assistant", "第二段"); err != nil {
		t.Fatal(err)
	}
	if err := sess.FlushDisplayEventContent("run-1-subagent-01-researcher", "assistant"); err != nil {
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

func TestDisplayToolArgsDeltasAreBatchedAndFlushedOnFinalResult(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-1", Role: "tool_call", Name: "write", Content: "write", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	smallArgs := `{"path":"chapters/ch01.md","content":"draft"}`
	if err := sess.AppendDisplayToolArgs("call-1", "write", smallArgs); err != nil {
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
		t.Fatalf("小块流式参数应等待工具终态批量落盘: %#v", history)
	}

	if err := sess.UpdateDisplayToolResult("call-1", "write", "success", "ok"); err != nil {
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

func TestDisplayToolArgsArePersistedWithoutTruncation(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-1", Role: "tool_call", Name: "write", Content: "write", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	largeArgs := `{"path":"chapters/ch01.md","content":"` + strings.Repeat("长内容", 20*1024) + `工具输入尾部必须完整恢复"}`
	if err := sess.AppendDisplayToolArgs("call-1", "write", largeArgs); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("call-1", "write", "success", "ok"); err != nil {
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
	if history[0].Args != largeArgs {
		t.Fatalf("工具参数应完整持久化: got_bytes=%d want_bytes=%d", len(history[0].Args), len(largeArgs))
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
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-execute", Role: "tool_call", Name: "bash", Content: "bash", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("", "bash", "success", "command done"); err != nil {
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
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "call-execute", Role: "tool_call", Name: "bash", Content: "bash", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("stale-id", "bash", "success", "stale result"); err != nil {
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
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "execute-1", Role: "tool_call", Name: "bash", Content: "bash", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "execute-2", Role: "tool_call", Name: "bash", Content: "bash", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateDisplayToolResult("stale-id", "bash", "success", "ambiguous result"); err != nil {
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
	if err := sess.Append(agent.UserMessage("统计一下")); err != nil {
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
	if err := sess.Append(agent.AssistantMessage("统计完成", nil)); err != nil {
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

func TestTokenUsageDisplayEventsAreCappedPerAgent(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 12; i++ {
		if err := sess.AppendDisplayEvent(DisplayEvent{
			ID:           fmt.Sprintf("ide-run-%02d", i),
			Role:         "token_usage",
			Content:      "usage",
			RunID:        fmt.Sprintf("ide-run-%02d", i),
			AgentKind:    "ide",
			PromptTokens: i,
			TotalTokens:  i,
			ModelCalls:   1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		if err := sess.AppendDisplayEvent(DisplayEvent{
			ID:           fmt.Sprintf("config-run-%02d", i),
			Role:         "token_usage",
			Content:      "usage",
			RunID:        fmt.Sprintf("config-run-%02d", i),
			AgentKind:    "config_manager",
			PromptTokens: i,
			TotalTokens:  i,
			ModelCalls:   1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	var ideRuns []string
	var configRuns []string
	for _, entry := range reloaded.History() {
		if entry.Role != "token_usage" {
			continue
		}
		switch entry.AgentKind {
		case "ide":
			ideRuns = append(ideRuns, entry.RunID)
		case "config_manager":
			configRuns = append(configRuns, entry.RunID)
		}
	}
	if len(ideRuns) != 10 || ideRuns[0] != "ide-run-03" || ideRuns[9] != "ide-run-12" {
		t.Fatalf("ide usage should keep latest 10 runs: %#v", ideRuns)
	}
	if len(configRuns) != 3 {
		t.Fatalf("config manager usage should be capped independently: %#v", configRuns)
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
	if err := first.Append(agent.UserMessage("会话 A")); err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("会话 B")
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Append(agent.UserMessage("会话 B")); err != nil {
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

func TestListUsesProjectionWithoutMaterializingColdSessions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("投影列表标题")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessage(agent.ToolMessage(agent.TextToolResult("仅模型上下文"), "projection-list-tool")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("投影列表回复", nil)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	metas, err := reloaded.List("default")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != "default" || metas[0].Title != "投影列表标题" {
		t.Fatalf("unexpected session metadata: %#v", metas)
	}
	if !metas[0].Active || metas[0].MessageCount != 2 || metas[0].CreatedAt.IsZero() || metas[0].UpdatedAt.IsZero() {
		t.Fatalf("incomplete session metadata: %#v", metas[0])
	}
	if len(reloaded.cache) != 0 {
		t.Fatalf("listing materialized cold sessions: cached=%d", len(reloaded.cache))
	}

	if _, err := reloaded.Get("default"); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.cache) != 1 {
		t.Fatalf("opening a session should still materialize it: cached=%d", len(reloaded.cache))
	}
	if err := reloaded.Close(); err != nil {
		t.Fatal(err)
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
	if err := matching.Append(agent.UserMessage("互动故事")); err != nil {
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

func TestStoreBoundsResidentSessionsWithoutInvalidatingInFlightHandles(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrCreate("resident-000")
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= maxResidentSessions; index++ {
		if _, err := store.GetOrCreate(fmt.Sprintf("resident-%03d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.cache) != maxResidentSessions {
		t.Fatalf("resident sessions = %d, want %d", len(store.cache), maxResidentSessions)
	}
	if store.cache[first.ID] != nil {
		t.Fatal("least-recently-used session remained resident")
	}
	if err := first.Append(agent.UserMessage("still valid")); err != nil {
		t.Fatalf("evicted in-flight session handle failed: %v", err)
	}
	reopened, err := store.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	messages := reopened.GetMessages()
	if len(messages) != 1 || messages[0].Content != "still valid" {
		t.Fatalf("reopened evicted session messages = %#v", messages)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
}
