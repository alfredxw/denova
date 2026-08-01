package chat

import (
	"strings"
	"testing"

	agentcontext "denova/internal/agents/context"
)

func TestTurnInputProjectionAddsWritingSkillLoadHintWithoutSkillBody(t *testing.T) {
	_, assembled := assembleTurnForTest(t, ChatRequest{
		Message:      "帮我分析一下 progress.md 有没有问题",
		WritingSkill: "novel-standard",
	}, nil, nil, agentcontext.DefaultBudget())
	modelMessage := finalAssembledUserMessage(t, assembled)

	for _, want := range []string{"Writing Skill 按需加载提示", "当前创作 Agent 选中的 Writing Skill 是 `novel-standard`", "当前 Agent 已启用 `skill` 工具", "调用 `skill` 工具加载 `novel-standard`", "不要假装已经读取了该 Skill 的完整说明", "不存在单独的 `writing_scope` 字段"} {
		if !strings.Contains(modelMessage, want) {
			t.Fatalf("writing skill hint missing %q:\n%s", want, modelMessage)
		}
	}
	for _, notWant := range []string{"```markdown", "SKILL.md 是本轮 IDE 创作 Agent 必须遵循"} {
		if strings.Contains(modelMessage, notWant) {
			t.Fatalf("writing skill body should not be injected, found %q:\n%s", notWant, modelMessage)
		}
	}
}
