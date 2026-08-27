package conversation

import (
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

func TestSessionConversationKeepsRichRawHistoryForAgentOwnedMaintenance(t *testing.T) {
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
	requestIndex := strings.Index(final, "# Current User Request (Highest Priority)")
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
	if sources := conversation.ContextSourceSummary(); !strings.Contains(sources, `source="workspace.runtime.dynamic"`) || !strings.Contains(sources, `placement="final_user_prefix"`) || !strings.Contains(sources, `purpose="provide turn-scoped workspace sources for the current request"`) {
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

func TestWorkspaceRuntimeContextUsesReplaceableStablePrefix(t *testing.T) {
	conversation := &SessionConversation{
		stableContextTitle: "Stable Workspace",
		stableContext:      "current workspace snapshot",
	}
	fragments := conversation.runtimeContextFragments()
	if len(fragments) != 1 {
		t.Fatalf("runtime fragments = %#v", fragments)
	}
	fragment := fragments[0]
	if fragment.Stability != agent.ContextStablePrefix || fragment.Placement != agentcontext.PlacementLeadingMessage || fragment.StateID != "" {
		t.Fatalf("stable workspace fragment = %#v", fragment)
	}
}

func TestSessionConversationKeepsStableContextBeforeAgentTranscript(t *testing.T) {
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
	if len(history) < 2 || !strings.Contains(history[0].Content, "# 稳定作品上下文") {
		t.Fatalf("stable context should remain before the Agent-owned transcript: %#v", messageContents(history))
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
