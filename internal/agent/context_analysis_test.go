package agent

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
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
