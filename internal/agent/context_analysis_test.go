package agent

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/prompts"
)

func TestInteractiveContextAnalysisLabelsDynamicContextAtFinalMessage(t *testing.T) {
	analysis, err := BuildInteractiveStoryContextAnalysis(
		&config.Config{},
		nil,
		prompts.InteractiveStorySystemInstructionInput{},
		nil,
		ChatRequest{Message: "我点燃火把"},
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
	if first := analysis.ContextMessages[0]; first.Source != "最近互动回合" || strings.Contains(first.Title, "故事状态与记忆") {
		t.Fatalf("first message should be recent history, got: %#v", first)
	}
	last := analysis.ContextMessages[len(analysis.ContextMessages)-1]
	if last.Source != "本轮互动指令" || last.Title != "本轮互动指令与动态上下文" {
		t.Fatalf("final message should carry runtime context label, got: %#v", last)
	}
	if !strings.Contains(last.Content, "[本轮动态上下文]") || !strings.Contains(last.Content, "当前互动状态快照") {
		t.Fatalf("final message should include dynamic context content: %#v", last)
	}
}
