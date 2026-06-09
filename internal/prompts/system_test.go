package prompts

import (
	"strings"
	"testing"
)

func TestSystemInstructionRequiresCreatorDuringBrainstorm(t *testing.T) {
	instruction := BuildSystemInstruction(SystemInstructionInput{
		Workspace: "/tmp/book",
	})

	for _, required := range []string{
		"/tmp/book/CREATOR.md",
		"书籍脑暴阶段也必须基于模板和作者确认更新",
		"先 read_file brainstorm.md 和 CREATOR.md",
		"CREATOR.md 负责“这本书长期怎么写、哪些规则必须一直遵守”",
		"每章字数/篇幅目标",
		"先分别 write_file 更新 brainstorm.md 和 CREATOR.md",
		"CREATOR.md 继续作为每轮最高优先级创作者指令生效",
		"先写回 `brainstorm.md` 和 `CREATOR.md`",
		"内容保持短小、可扫读、方便作者评论和后续更新",
		"建议控制在 800-1200 个中文字内",
		"每章安排只写 3-5 条关键点",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("系统提示缺少 %q:\n%s", required, instruction)
		}
	}
}
