package agent

import (
	"strings"
	"testing"

	"nova/config"
	"nova/internal/book"
)

func TestBuildInteractiveStoryInstructionIsIsolatedFromIDEPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	instruction := BuildInteractiveStoryInstruction(&config.Config{Workspace: state.Workspace()}, state)

	for _, forbidden := range []string{"创建章节文件", "chXX", "progress.md", "setting/outline.md"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("interactive story instruction should not contain IDE-only prompt %q:\n%s", forbidden, instruction)
		}
	}
	for _, required := range []string{"互动故事模式", "<NARRATIVE>", "<STATE_DELTA>", "禁止使用写文件工具", "write_todos", "<invoke>", "文字小说 RPG", "回合裁定循环", "可行动空间", "一致性自检"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction should contain %q:\n%s", required, instruction)
		}
	}
}
