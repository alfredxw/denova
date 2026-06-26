package app

import (
	"testing"

	"nova/config"
	"nova/internal/agent"
)

func TestApplyWritingSkillRuntimePolicyResolvesDefaultNameOnly(t *testing.T) {
	runtime := &ideChatRuntime{cfg: config.Config{
		WritingSkillDefault: "novel-heavy",
		SubAgents: []config.SubAgentConfig{{
			ID:           "researcher",
			Description:  "Reads context.",
			SystemPrompt: "Return notes.",
		}},
	}}
	req := &agent.ChatRequest{Message: "帮我分析一下 progress.md 有没有问题"}

	if err := applyWritingSkillRuntimePolicy(runtime, req); err != nil {
		t.Fatal(err)
	}
	if req.WritingSkill != "novel-heavy" {
		t.Fatalf("writing skill = %s, want novel-heavy", req.WritingSkill)
	}
	if len(runtime.cfg.SubAgents) != 1 || runtime.cfg.SubAgents[0].ID != "researcher" {
		t.Fatalf("writing skill selection should not mutate subagents: %+v", runtime.cfg.SubAgents)
	}
}

func TestApplyWritingSkillRuntimePolicyKeepsCustomSkillAsDynamicHintOnly(t *testing.T) {
	runtime := &ideChatRuntime{cfg: config.Config{WritingSkillDefault: "novel-standard"}}
	req := &agent.ChatRequest{Message: "写一个雨夜重逢的场景", WritingSkill: "slow-burn"}

	if err := applyWritingSkillRuntimePolicy(runtime, req); err != nil {
		t.Fatal(err)
	}
	if req.WritingSkill != "slow-burn" {
		t.Fatalf("writing skill = %s, want slow-burn", req.WritingSkill)
	}
	if runtime.cfg.GeneralSubAgents.IDE != nil || len(runtime.cfg.SubAgents) != 0 {
		t.Fatalf("writing skill selection should not mutate agent config: %+v", runtime.cfg)
	}
}
