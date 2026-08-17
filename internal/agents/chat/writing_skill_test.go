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

	for _, want := range []string{"On-demand Writing Skill Loading", "The Writing Skill selected for the current creative Agent is `novel-standard`", "current Agent has the `skill` tool enabled", "call `skill` to load `novel-standard`", "do not claim to have read its complete instructions", "there is no separate `writing_scope` field"} {
		if !strings.Contains(modelMessage, want) {
			t.Fatalf("writing skill hint missing %q:\n%s", want, modelMessage)
		}
	}
	for _, notWant := range []string{"```markdown", "SKILL.md is mandatory for this IDE creative Agent turn"} {
		if strings.Contains(modelMessage, notWant) {
			t.Fatalf("writing skill body should not be injected, found %q:\n%s", notWant, modelMessage)
		}
	}
}
