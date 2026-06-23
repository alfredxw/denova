package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/session"
)

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
		if err := sess.Append(schema.UserMessage("user " + string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
		if err := sess.Append(schema.AssistantMessage("assistant "+string(rune('0'+i)), nil)); err != nil {
			t.Fatal(err)
		}
	}
	conversation := NewSessionConversation(sess)
	history, err := conversation.PrepareMessages("user 5", "agent user 5")
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

func TestSessionConversationAppendsRuntimeContextToFinalUserMessageOnly(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("旧用户请求")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.AssistantMessage("旧助手回复", nil)); err != nil {
		t.Fatal(err)
	}

	conversation := NewSessionConversationForAgentWithRuntimeContext(
		sess,
		&config.Config{},
		config.AgentKindIDE,
		"本轮动态作品状态",
		"## 大纲\n\n主角进入废城。",
	)
	history, err := conversation.PrepareMessages("继续写", "继续写")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3: %#v", len(history), history)
	}
	final := history[len(history)-1].Content
	if !strings.HasPrefix(final, "继续写") || !strings.Contains(final, "# 本轮动态作品状态") || !strings.Contains(final, "主角进入废城") {
		t.Fatalf("final model message should append runtime context at the bottom:\n%s", final)
	}
	visible := sess.History()
	if got := visible[len(visible)-1].Content; got != "继续写" {
		t.Fatalf("visible session history should keep original user message, got %q", got)
	}
	if sources := conversation.ContextSourceSummary(); !strings.Contains(sources, "本轮动态上下文") || !strings.Contains(sources, "appended_to_final_user_message") {
		t.Fatalf("runtime context source summary missing dynamic context: %s", sources)
	}
}

func TestSessionConversationCompactsOnlyMessagesAfterPreviousCompaction(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()

	var capturedExisting string
	var capturedSource []*schema.Message
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, existingMemory string, source []*schema.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		capturedExisting = existingMemory
		capturedSource = source
		return "新压缩摘要：旧目标与新增进展都已合并。", 200, nil
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	messages := []*schema.Message{
		schema.UserMessage("已压缩用户 1"),
		schema.AssistantMessage("已压缩助手 1", nil),
		schema.UserMessage("新增用户 2"),
		schema.AssistantMessage("新增助手 2", nil),
	}
	for _, msg := range messages {
		if err := sess.Append(msg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		AgentKind:        config.AgentKindIDE,
		Epoch:            1,
		Summary:          "旧压缩摘要：用户 1 已处理。",
		SourceStartIndex: 0,
		SourceEndIndex:   2,
		RetainedTurns:    1,
	}); err != nil {
		t.Fatal(err)
	}

	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	_, result, err := conversation.CompactContextIfNeeded(context.Background(), ContextCompactionInput{
		Messages:       sess.GetEffectiveMessages(),
		Force:          true,
		KeepLatestUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered {
		t.Fatalf("expected compaction to trigger: %#v", result)
	}
	if capturedExisting != "旧压缩摘要：用户 1 已处理。" {
		t.Fatalf("existing memory = %q", capturedExisting)
	}
	got := messageContents(capturedSource)
	want := []string{"新增用户 2", "新增助手 2"}
	if len(got) != len(want) {
		t.Fatalf("source len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("source[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
	if record, ok := sess.LatestContextCompaction(config.AgentKindIDE); !ok || record.SourceStartIndex != 2 || record.SourceEndIndex != 4 {
		t.Fatalf("new compaction should record incremental source range, got ok=%v record=%#v", ok, record)
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
		if err := sess.Append(schema.UserMessage("user " + string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
		if err := sess.Append(schema.AssistantMessage("assistant "+string(rune('0'+i)), nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		AgentKind:        config.AgentKindIDE,
		Summary:          "用户目标：继续写作。",
		SourceStartIndex: 0,
		SourceEndIndex:   2,
		RetainedTurns:    2,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	conversation := NewSessionConversationForAgent(sess, cfg, config.AgentKindIDE)
	history, err := conversation.PrepareMessages("user 3", "agent user 3")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 6 {
		t.Fatalf("history length = %d, want 6: %#v", len(history), history)
	}
	if !isContextCompactionMessage(history[0]) || history[0].Role != schema.User {
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
		if err := sess.Append(schema.UserMessage("user " + string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
		if err := sess.Append(schema.AssistantMessage("assistant "+string(rune('0'+i)), nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		AgentKind:        config.AgentKindIDE,
		Summary:          "用户目标：继续写作。",
		SourceStartIndex: 0,
		SourceEndIndex:   4,
		RetainedTurns:    1,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	conversation := NewSessionConversationForAgent(sess, cfg, config.AgentKindIDE)
	history, err := conversation.PrepareMessages("user 6", "agent user 6")
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
	if !isContextCompactionMessage(history[0]) {
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

func messageContents(messages []*schema.Message) []string {
	contents := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		contents = append(contents, msg.Content)
	}
	return contents
}
