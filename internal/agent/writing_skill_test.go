package agent

import (
	"strings"
	"testing"

	"nova/config"
)

func TestResolveWritingSkillNameDefaultsAndSelection(t *testing.T) {
	if got := ResolveWritingSkillName(&config.Config{WritingSkillDefault: "novel-heavy"}, ""); got != "novel-heavy" {
		t.Fatalf("default writing skill = %s, want novel-heavy", got)
	}
	if got := ResolveWritingSkillName(&config.Config{WritingSkillDefault: "novel-heavy"}, "slow-burn"); got != "slow-burn" {
		t.Fatalf("selected writing skill = %s, want slow-burn", got)
	}
	if got := ResolveWritingSkillName(&config.Config{}, ""); got != config.DefaultWritingSkillName {
		t.Fatalf("fallback writing skill = %s, want %s", got, config.DefaultWritingSkillName)
	}
}

func TestComposeAgentInputAddsWritingSkillLoadHintWithoutSkillBody(t *testing.T) {
	composition := composeAgentInput(ChatRequest{
		Message:      "帮我分析一下 progress.md 有没有问题",
		WritingSkill: "novel-standard",
	}, nil, nil, DefaultLoopPolicy())

	for _, want := range []string{"Writing Skill 按需加载提示", "当前创作 Agent 选中的 Writing Skill 是 `novel-standard`", "当前 Agent 已启用 `skill` 工具", "调用 `skill` 工具加载 `novel-standard`", "不要假装已经读取了该 Skill 的完整说明", "不存在单独的 `writing_scope` 字段"} {
		if !strings.Contains(composition.AgentMessage, want) {
			t.Fatalf("writing skill hint missing %q:\n%s", want, composition.AgentMessage)
		}
	}
	for _, notWant := range []string{"```markdown", "SKILL.md 是本轮 IDE 创作 Agent 必须遵循"} {
		if strings.Contains(composition.AgentMessage, notWant) {
			t.Fatalf("writing skill body should not be injected, found %q:\n%s", notWant, composition.AgentMessage)
		}
	}
}
