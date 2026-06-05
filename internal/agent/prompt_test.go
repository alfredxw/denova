package agent

import (
	"strings"
	"testing"

	"nova/config"
	"nova/internal/book"
	"nova/internal/prompts"
)

func TestBuildInteractiveStoryInstructionIsIsolatedFromIDEPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	instruction := BuildInteractiveStoryInstruction(&config.Config{Workspace: state.Workspace()}, state, prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           "classic",
		StoryTellerName:         "经典叙事者",
		StoryTellerDescription:  "平衡叙事",
		StoryTellerSystemPrompt: "你是一位经典叙事者。",
	})

	for _, forbidden := range []string{"创建章节文件", "chXX", "progress.md", "setting/outline.md"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("interactive story instruction should not contain IDE-only prompt %q:\n%s", forbidden, instruction)
		}
	}
	for _, required := range []string{"互动故事模式", "<NARRATIVE>", "<HOT_STATE>", "<STATE_DELTA>", "禁止使用写文件工具", "write_todos", "<invoke>", "文字小说 RPG", "回合裁定循环", "可选择", "一致性自检"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction should contain %q:\n%s", required, instruction)
		}
	}
	if !strings.Contains(instruction, "讲述者系统规则") || !strings.Contains(instruction, "经典叙事者") {
		t.Fatalf("interactive story instruction should include teller system rules:\n%s", instruction)
	}
}

func TestPromptStateSectionSourceIncludesCharacterStates(t *testing.T) {
	if got := promptStateSectionSource("角色状态"); got != "setting/character-states.md" {
		t.Fatalf("角色状态来源 = %q", got)
	}
}
