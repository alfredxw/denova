package conversation

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

func TestCompactionIncrementalSourceReadsCanonicalHistoryBeforeResidentWindow(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("canonical-compaction-source")
	if err != nil {
		t.Fatal(err)
	}
	total := 209
	messages := make([]*agent.Message, total)
	for index := 0; index < total; index++ {
		messages[index] = agent.UserMessage(fmt.Sprintf("source-%03d", index))
	}
	if err := sess.AppendContextMessages(messages...); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess)
	source, checkpoint, start, end, err := conversation.compactionIncrementalSource(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint != "" || start != 0 || end != total || len(source) != total {
		t.Fatalf("source = checkpoint:%q range:[%d,%d) len:%d", checkpoint, start, end, len(source))
	}
	if source[0].Content != "source-000" || source[len(source)-1].Content != "source-208" {
		t.Fatalf("canonical compaction endpoints = %q ... %q", source[0].Content, source[len(source)-1].Content)
	}
}

func TestCompactionIncrementalSourceExcludesOnlyAnActualLatestUser(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("compaction-latest-user")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessages(
		agent.UserMessage("previous user"),
		agent.AssistantMessage("assistant result", nil),
	); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess)
	source, _, _, end, err := conversation.compactionIncrementalSource(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if end != 2 || len(source) != 2 || source[1].Content != "assistant result" {
		t.Fatalf("assistant tail was mistaken for current user: end=%d source=%#v", end, source)
	}
	if err := sess.AppendContextMessage(agent.UserMessage("current user")); err != nil {
		t.Fatal(err)
	}
	source, _, _, end, err = conversation.compactionIncrementalSource(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if end != 2 || len(source) != 2 || source[1].Content != "assistant result" {
		t.Fatalf("latest user was not excluded: end=%d source=%#v", end, source)
	}
}

func TestSessionConversationKeepsFullEffectiveHistoryBeforeCompaction(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if err := sess.Append(agent.UserMessage("user " + string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
		if err := sess.Append(agent.AssistantMessage("assistant "+string(rune('0'+i)), nil)); err != nil {
			t.Fatal(err)
		}
	}
	conversation := NewSessionConversation(sess)
	history, err := assembleAndCommitModelContextForTest(conversation, "user 5", "agent user 5")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 9 {
		t.Fatalf("history length = %d, want 9", len(history))
	}
	want := []string{
		"user 1", "assistant 1",
		"user 2", "assistant 2",
		"user 3", "assistant 3",
		"user 4", "assistant 4",
		"agent user 5",
	}
	for i := range want {
		if history[i].Content != want[i] {
			t.Fatalf("history[%d] = %q, want %q; all=%#v", i, history[i].Content, want[i], history)
		}
	}
}

func TestSessionConversationPersistsUserMessageReferencesOutsideModelContent(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess)
	references := []agentcontext.UserReference{
		{Kind: "file", Label: "chapters/ch01.md"},
		{Kind: "review_comment", ID: "comment-1", Label: "setting/progress.md", Detail: "需要增加爽点"},
	}

	history, err := assembleAndCommitModelContextForTest(conversation, "请统一修改", "请统一修改", references...)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Content != "请统一修改" {
		t.Fatalf("display references must not be injected into model content: %#v", history)
	}
	visible := sess.History()
	if len(visible) != 1 || len(visible[0].UserReferences) != 2 {
		t.Fatalf("user references were not persisted: %#v", visible)
	}
	if visible[0].UserReferences[1].Detail != "需要增加爽点" {
		t.Fatalf("review comment display detail was lost: %#v", visible[0].UserReferences)
	}
}

func TestSessionConversationPrependsDynamicContextInsideFinalUserMessageOnly(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("旧用户请求")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("旧助手回复", nil)); err != nil {
		t.Fatal(err)
	}

	conversation := NewSessionConversationForAgentWithRuntimeContext(
		sess,
		&config.Config{},
		config.AgentKindIDE,
		"本轮动态作品状态",
		"## 大纲\n\n主角进入废城。",
	)
	history, err := assembleAndCommitModelContextForTest(conversation, "继续写", "继续写")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3: %#v", len(history), history)
	}
	final := history[len(history)-1].Content
	dynamicIndex := strings.Index(final, "# 本轮动态作品状态")
	requestIndex := strings.Index(final, "# 本轮用户请求（最高优先级）")
	if dynamicIndex < 0 || requestIndex < 0 || dynamicIndex >= requestIndex {
		t.Fatalf("final model message should place dynamic context before the current request:\n%s", final)
	}
	if !strings.Contains(final, "主角进入废城") || !strings.HasSuffix(strings.TrimSpace(final), "继续写") {
		t.Fatalf("final model message missing dynamic state or bottom request:\n%s", final)
	}
	visible := sess.History()
	if got := visible[len(visible)-1].Content; got != "继续写" {
		t.Fatalf("visible session history should keep original user message, got %q", got)
	}
	if sources := conversation.ContextSourceSummary(); !strings.Contains(sources, `source="workspace.runtime.dynamic"`) || !strings.Contains(sources, `placement="final_user_prefix"`) || !strings.Contains(sources, `purpose="provide turn-scoped workspace state for the current request"`) {
		t.Fatalf("runtime context source summary missing dynamic context: %s", sources)
	}
}

func TestSessionConversationPrependsStableContextBeforeHistory(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("旧用户请求")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("旧助手回复", nil)); err != nil {
		t.Fatal(err)
	}

	conversation := NewSessionConversationForAgentWithRuntimeContexts(
		sess,
		&config.Config{},
		config.AgentKindIDE,
		"稳定作品上下文",
		"## 当前大纲\n\n主角进入废城。",
		"本轮动态作品状态",
		"## 当前进度\n\n刚抵达废城。",
	)
	history, err := assembleAndCommitModelContextForTest(conversation, "继续写", "继续写")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 4 {
		t.Fatalf("history length = %d, want 4: %#v", len(history), history)
	}
	if !strings.Contains(history[0].Content, "# 稳定作品上下文") || !strings.Contains(history[0].Content, "主角进入废城") {
		t.Fatalf("first model message should be stable context: %s", history[0].Content)
	}
	if history[1].Content != "旧用户请求" || history[2].Content != "旧助手回复" {
		t.Fatalf("stable context should precede persisted history: %#v", messageContents(history))
	}
	if !strings.Contains(history[3].Content, "# 本轮动态作品状态") || !strings.HasSuffix(strings.TrimSpace(history[3].Content), "继续写") {
		t.Fatalf("final model message should contain dynamic context then request: %s", history[3].Content)
	}
	if visible := sess.History(); len(visible) != 3 || visible[2].Content != "继续写" {
		t.Fatalf("visible session history should only include raw user request: %#v", visible)
	}
	if sources := conversation.ContextSourceSummary(); !strings.Contains(sources, `source="workspace.runtime.stable"`) || !strings.Contains(sources, `placement="leading_message"`) || !strings.Contains(sources, `source="workspace.runtime.dynamic"`) || !strings.Contains(sources, `placement="final_user_prefix"`) {
		t.Fatalf("runtime context source summary missing stable/dynamic locations: %s", sources)
	}
}

func TestSessionConversationKeepsStableContextBeforeCompactionSummary(t *testing.T) {
	summarize := func(context.Context, *config.Config, agentcompaction.SummaryRequest, func(int, string)) (string, error) {
		return "压缩摘要：旧对话已合并。", nil
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []*agent.Message{
		agent.UserMessage(strings.Repeat("第一轮旧用户请求 ", 700)),
		agent.AssistantMessage(strings.Repeat("第一轮旧助手回复 ", 700), nil),
		agent.UserMessage("第二轮旧用户请求"),
		agent.AssistantMessage("第二轮旧助手回复", nil),
	} {
		if err := sess.Append(message); err != nil {
			t.Fatal(err)
		}
	}
	conversation := NewSessionConversationForAgentWithRuntimeContexts(
		sess,
		&config.Config{},
		config.AgentKindIDE,
		"稳定作品上下文",
		"## 当前大纲\n\n主角进入废城。",
		"本轮动态作品状态",
		"## 当前进度\n\n刚抵达废城。",
	)
	history, err := assembleAndCommitModelContextForTest(conversation, "继续写", "继续写")
	if err != nil {
		t.Fatal(err)
	}
	compacted, result, err := conversation.CompactContextIfNeeded(context.Background(), coldCompactionTestInput(agentcompaction.Input{
		Messages: history,
		Force:    true,
	}, summarize))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered {
		t.Fatalf("expected compaction to trigger: %#v", result)
	}
	if len(compacted) < 3 {
		t.Fatalf("compacted messages too short: %#v", compacted)
	}
	if !strings.Contains(compacted[0].Content, "# 稳定作品上下文") {
		t.Fatalf("stable context should remain first after compaction: %#v", messageContents(compacted))
	}
	if !agentcontext.IsCompactionSummaryMessage(compacted[1]) {
		t.Fatalf("compaction summary should follow stable context: %#v", messageContents(compacted))
	}
}

func TestSessionConversationPreparesIncrementalCompactionWithoutAdvancingCanonicalCheckpoint(t *testing.T) {
	var capturedTranscript string
	summarize := func(_ context.Context, _ *config.Config, request agentcompaction.SummaryRequest, _ func(int, string)) (string, error) {
		capturedTranscript = strings.Join(messageContents(request.Messages), "\n")
		return "新压缩摘要：旧目标与新增进展都已合并。", nil
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("已压缩用户 1 ", 700)),
		agent.AssistantMessage(strings.Repeat("已压缩助手 1 ", 700), nil),
		agent.UserMessage("新增用户 2"),
		agent.AssistantMessage("新增助手 2", nil),
	}
	for _, msg := range messages {
		if err := sess.Append(msg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindIDE, agentcompaction.Result{
			Epoch: 1, Summary: "旧压缩摘要：用户 1 已处理。", RetainedTurns: 1,
		}),
		SourceStartIndex: 0,
		SourceEndIndex:   2,
	}); err != nil {
		t.Fatal(err)
	}

	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	projection, err := conversation.SnapshotContextCompaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := messageContents(projection.Source), []string{"新增用户 2", "新增助手 2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("incremental source = %#v, want %#v", got, want)
	}
	_, result, err := conversation.CompactContextIfNeeded(context.Background(), coldCompactionTestInput(agentcompaction.Input{
		Messages:       sess.GetEffectiveMessages(),
		Force:          true,
		KeepLatestUser: true,
	}, summarize))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered {
		t.Fatalf("expected compaction to trigger: %#v", result)
	}
	if !strings.Contains(capturedTranscript, "旧压缩摘要：用户 1 已处理。") ||
		!strings.Contains(capturedTranscript, "新增用户 2") || !strings.Contains(capturedTranscript, "新增助手 2") {
		t.Fatalf("incremental summary request = %q", capturedTranscript)
	}
	if record, ok := sess.LatestContextCompaction(config.AgentKindIDE); !ok || record.SourceStartIndex != 0 || record.SourceEndIndex != 2 || record.Epoch != 1 {
		t.Fatalf("transient compaction must not advance canonical checkpoint before a structural command: ok=%v record=%#v", ok, record)
	}
}

func TestSessionConversationUsesCompactionSummaryRetainedTailAndAppendedMessages(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := sess.Append(agent.UserMessage("user " + string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
		if err := sess.Append(agent.AssistantMessage("assistant "+string(rune('0'+i)), nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindIDE, agentcompaction.Result{
			Summary: "用户目标：继续写作。", RetainedTurns: 2,
		}),
		SourceStartIndex: 0,
		SourceEndIndex:   2,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	conversation := NewSessionConversationForAgent(sess, cfg, config.AgentKindIDE)
	history, err := assembleAndCommitModelContextForTest(conversation, "user 3", "agent user 3")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 6 {
		t.Fatalf("history length = %d, want 6: %#v", len(history), history)
	}
	if !agentcontext.IsCompactionSummaryMessage(history[0]) || history[0].Role != agent.Assistant {
		t.Fatalf("first message should be compaction summary: %#v", history[0])
	}
	if history[1].Content != "user 1" || history[2].Content != "assistant 1" || history[3].Content != "user 2" || history[4].Content != "assistant 2" || history[5].Content != "agent user 3" {
		t.Fatalf("unexpected compacted history tail: %#v", history)
	}
	if visible := sess.History(); len(visible) != 5 {
		t.Fatalf("visible raw history should include only raw messages and current user: %#v", visible)
	}
}

func TestSessionConversationKeepsPostCompactionTurnsUntilNextCompaction(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if err := sess.Append(agent.UserMessage("user " + string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
		if err := sess.Append(agent.AssistantMessage("assistant "+string(rune('0'+i)), nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindIDE, agentcompaction.Result{
			Summary: "用户目标：继续写作。", RetainedTurns: 1,
		}),
		SourceStartIndex: 0,
		SourceEndIndex:   4,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	conversation := NewSessionConversationForAgent(sess, cfg, config.AgentKindIDE)
	history, err := assembleAndCommitModelContextForTest(conversation, "user 6", "agent user 6")
	if err != nil {
		t.Fatal(err)
	}
	got := messageContents(history)
	want := []string{
		history[0].Content,
		"user 2",
		"assistant 2",
		"user 3",
		"assistant 3",
		"user 4",
		"assistant 4",
		"user 5",
		"assistant 5",
		"agent user 6",
	}
	if !agentcontext.IsCompactionSummaryMessage(history[0]) {
		t.Fatalf("first message should be compaction summary: %#v", history[0])
	}
	if len(got) != len(want) {
		t.Fatalf("history length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func messageContents(messages []*agent.Message) []string {
	contents := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		contents = append(contents, msg.Content)
	}
	return contents
}
