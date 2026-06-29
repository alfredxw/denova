package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/book"
	"nova/internal/prompts"
	"nova/internal/session"
)

func TestInteractiveContextAnalysisLabelsDynamicContextAtFinalMessage(t *testing.T) {
	analysis, err := BuildInteractiveStoryContextAnalysis(
		&config.Config{},
		nil,
		prompts.InteractiveStorySystemInstructionInput{},
		nil,
		ChatRequest{Message: "我点燃火把"},
		nil,
		func(originalMessage, agentMessage string) ([]*schema.Message, error) {
			return []*schema.Message{
				schema.UserMessage("我推开门"),
				schema.AssistantMessage("门后传来风声。", nil),
				schema.UserMessage(agentMessage + "\n\n[本轮动态上下文]\n## 当前互动状态快照(JSON)\n{}"),
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.ContextMessages) != 3 {
		t.Fatalf("context message count = %d, want 3", len(analysis.ContextMessages))
	}
	if first := analysis.ContextMessages[0]; first.Source != "互动历史回合" || strings.Contains(first.Title, "故事状态与记忆") {
		t.Fatalf("first message should be interactive history, got: %#v", first)
	}
	last := analysis.ContextMessages[len(analysis.ContextMessages)-1]
	if last.Source != "本轮互动指令" || last.Title != "本轮互动指令与动态上下文" {
		t.Fatalf("final message should carry runtime context label, got: %#v", last)
	}
	if !strings.Contains(last.Content, "[本轮动态上下文]") || !strings.Contains(last.Content, "当前互动状态快照") {
		t.Fatalf("final message should include dynamic context content: %#v", last)
	}
}

func TestInteractiveContextAnalysisUsesConfiguredContextWindow(t *testing.T) {
	contextWindow := 650000
	analysis, err := BuildInteractiveStoryContextAnalysis(
		&config.Config{OpenAIContextWindowTokens: contextWindow},
		nil,
		prompts.InteractiveStorySystemInstructionInput{},
		nil,
		ChatRequest{Message: "继续"},
		nil,
		func(originalMessage, agentMessage string) ([]*schema.Message, error) {
			return []*schema.Message{schema.UserMessage(agentMessage)}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.ContextWindowTokens != contextWindow {
		t.Fatalf("context window tokens = %d, want %d", analysis.ContextWindowTokens, contextWindow)
	}
}

func TestInteractiveContextAnalysisShowsDirectNarrativeOutputProtocol(t *testing.T) {
	analysis, err := BuildInteractiveStoryContextAnalysis(
		&config.Config{},
		nil,
		prompts.InteractiveStorySystemInstructionInput{},
		nil,
		ChatRequest{Message: "继续"},
		nil,
		func(originalMessage, agentMessage string) ([]*schema.Message, error) {
			return []*schema.Message{schema.UserMessage(agentMessage)}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var outputProtocol *ContextAnalysisPart
	for i := range analysis.SystemPromptParts {
		part := &analysis.SystemPromptParts[i]
		if part.ID == "output_protocol" {
			outputProtocol = part
			break
		}
	}
	if outputProtocol == nil {
		t.Fatalf("output protocol part missing: %#v", analysis.SystemPromptParts)
	}
	if !strings.Contains(outputProtocol.Content, "只输出本回合可展示在故事舞台上的故事正文") {
		t.Fatalf("output protocol should describe direct narrative text: %#v", outputProtocol)
	}
	if strings.Contains(outputProtocol.Content, "<NARRATIVE>") {
		t.Fatalf("output protocol should not require narrative XML wrapper: %#v", outputProtocol)
	}
}

func TestIDEContextAnalysisShowsStyleRulesAsSystemPromptParts(t *testing.T) {
	analysis, err := BuildIDEContextAnalysis(
		&config.Config{},
		nil,
		IDEStoryTeller{
			ID:         "classic",
			StyleRules: []StyleRule{{Scene: "激烈打斗", StyleContents: []string{"短句留白"}}},
		},
		nil,
		nil,
		0,
		nil,
		nil,
		ChatRequest{
			Message:    "续写第三章",
			StyleRules: []StyleRule{{Scene: "激烈打斗", StyleContents: []string{"短句留白"}}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var foundSystemPart bool
	for _, part := range analysis.SystemPromptParts {
		if part.Title == "场景化风格规则：激烈打斗" && strings.Contains(part.Content, "短句留白") {
			foundSystemPart = true
		}
	}
	if !foundSystemPart {
		t.Fatalf("style rule should be a system prompt part: %#v", analysis.SystemPromptParts)
	}
	if len(analysis.ContextMessages) == 0 {
		t.Fatal("context messages should not be empty")
	}
	final := analysis.ContextMessages[len(analysis.ContextMessages)-1].Content
	if strings.Contains(final, "场景化风格规则") || strings.Contains(final, "短句留白") {
		t.Fatalf("style rule should not be appended to final user message:\n%s", final)
	}
}

func TestInteractiveContextAnalysisShowsStyleRulesAsSystemPromptParts(t *testing.T) {
	analysis, err := BuildInteractiveStoryContextAnalysis(
		&config.Config{},
		nil,
		prompts.InteractiveStorySystemInstructionInput{
			StoryTellerID: "classic",
			StyleRules:    []prompts.StyleRule{{Scene: "日常对话", StyleContents: []string{"克制对白"}}},
		},
		nil,
		ChatRequest{
			Message:    "我和守卫交谈",
			StyleRules: []StyleRule{{Scene: "日常对话", StyleContents: []string{"克制对白"}}},
		},
		nil,
		func(originalMessage, agentMessage string) ([]*schema.Message, error) {
			return []*schema.Message{schema.UserMessage(agentMessage)}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var foundSystemPart bool
	for _, part := range analysis.SystemPromptParts {
		if part.Title == "场景化风格规则：日常对话" && strings.Contains(part.Content, "克制对白") {
			foundSystemPart = true
		}
	}
	if !foundSystemPart {
		t.Fatalf("style rule should be a system prompt part: %#v", analysis.SystemPromptParts)
	}
	final := analysis.ContextMessages[len(analysis.ContextMessages)-1].Content
	if strings.Contains(final, "场景化风格规则") || strings.Contains(final, "克制对白") {
		t.Fatalf("style rule should not be appended to final interactive message:\n%s", final)
	}
}

func TestIDEContextAnalysisKeepsPostCompactionMessages(t *testing.T) {
	messages := []*schema.Message{
		schema.UserMessage("user 1"),
		schema.AssistantMessage("assistant 1", nil),
		schema.UserMessage("user 2"),
		schema.AssistantMessage("assistant 2", nil),
		schema.UserMessage("user 3"),
		schema.AssistantMessage("assistant 3", nil),
	}
	compaction := &session.ContextCompaction{
		Epoch:          1,
		Summary:        "压缩摘要：保留早期约束。",
		SourceEndIndex: 2,
		RetainedTurns:  1,
	}
	cfg := &config.Config{}

	analysisMessages := buildIDEAnalysisMessages(cfg, messages, len(messages), compaction)
	got := messageContents(analysisMessages)
	want := []string{
		analysisMessages[0].Content,
		"user 1",
		"assistant 1",
		"user 2",
		"assistant 2",
		"user 3",
		"assistant 3",
	}
	if !isContextCompactionMessage(analysisMessages[0]) {
		t.Fatalf("first message should be compaction summary: %#v", analysisMessages[0])
	}
	if len(got) != len(want) {
		t.Fatalf("analysis messages = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("analysis message %d = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestIDEContextAnalysisSplitsStableAndDynamicWorkspaceState(t *testing.T) {
	dir := t.TempDir()
	state := book.NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	if err := os.MkdirAll(state.SettingDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.SettingDir(), "outline.md"), []byte("## 第一卷\n\n主角进入废城。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.SettingDir(), "progress.md"), []byte("当前进度：抵达废城入口。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.SettingDir(), book.CharacterStatesFileName), []byte("林川：警惕，轻伤。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.ChapterGroupDir(), "group01-废城.md"), []byte("章节组：探索废城。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chapters", "ch0001-开局.md"), []byte("第一章正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := book.NewLoreStore(dir).Create(book.LoreItemInput{
		ID:         "hero",
		Type:       "character",
		Name:       "林川",
		Importance: "major",
		LoadMode:   book.LoreLoadModeResident,
		Content:    "## 角色小标题\n\n林川长期设定。",
	}); err != nil {
		t.Fatalf("create lore item failed: %v", err)
	}

	analysis, err := BuildIDEContextAnalysis(
		&config.Config{Workspace: dir},
		state,
		IDEStoryTeller{},
		nil,
		nil,
		0,
		nil,
		nil,
		ChatRequest{
			Message: "继续写",
			IDEContext: IDEContextRef{
				CurrentFile: "chapters/ch0001-开局.md",
				OpenFiles:   []string{"chapters/ch0001-开局.md", "setting/progress.md"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, part := range analysis.SystemPromptParts {
		if part.Source == ".nova/lore/items.json" {
			t.Fatalf("workspace state should not be part of system prompt sources: %#v", part)
		}
		if part.Title == "角色小标题" || part.Source == "作品状态注入" {
			t.Fatalf("context analysis should not split workspace state by markdown headings: %#v", part)
		}
	}
	if len(analysis.ContextMessages) == 0 {
		t.Fatal("context analysis should include final model messages")
	}
	first := analysis.ContextMessages[0]
	if first.Source != "稳定作品上下文" || first.Title != "稳定作品上下文" {
		t.Fatalf("first message should carry stable workspace state: %#v", first)
	}
	for _, want := range []string{"# 稳定作品上下文", "主角进入废城", "## 角色小标题", "林川长期设定"} {
		if !strings.Contains(first.Content, want) {
			t.Fatalf("stable message missing %q:\n%s", want, first.Content)
		}
	}
	for _, notWant := range []string{"当前进度：抵达废城入口", "章节组：探索废城", "chapters/ch0001-开局.md", "林川：警惕"} {
		if strings.Contains(first.Content, notWant) {
			t.Fatalf("stable message should not include dynamic state %q:\n%s", notWant, first.Content)
		}
	}
	final := analysis.ContextMessages[len(analysis.ContextMessages)-1]
	if final.Source != "本轮上下文" || final.Title != "动态作品状态与本轮用户请求" {
		t.Fatalf("final message should carry dynamic workspace state label: %#v", final)
	}
	for _, want := range []string{"# 本轮动态作品状态", "章节组：探索废城", "chapters/ch0001-开局.md", "当前进度：抵达废城入口", "林川：警惕", "## IDE 当前状态", "当前聚焦文件：chapters/ch0001-开局.md", "当前打开文件：chapters/ch0001-开局.md、setting/progress.md", "# 本轮用户请求（最高优先级）"} {
		if !strings.Contains(final.Content, want) {
			t.Fatalf("final message missing %q:\n%s", want, final.Content)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(final.Content), "继续写") {
		t.Fatalf("final message should keep current request at the bottom:\n%s", final.Content)
	}
}
