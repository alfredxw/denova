package prompts

import (
	"strings"
	"testing"
)

func TestSystemInstructionRequiresIdeasAndCreatorDuringIdeation(t *testing.T) {
	instruction := BuildSystemInstruction(SystemInstructionInput{
		Workspace: "/tmp/book",
	})

	for _, required := range []string{
		"/tmp/book/CREATOR.md",
		"/tmp/book/ideas.md",
		"新书构思阶段也必须基于模板和作者确认更新",
		"先 read_file ideas.md 和 CREATOR.md",
		"阶段性结论和待确认点",
		"CREATOR.md 负责“这本书长期怎么写、哪些规则必须一直遵守”",
		"每章字数/篇幅目标",
		"及时 edit_file 或 write_file 更新 ideas.md",
		"先分别 write_file 更新 ideas.md 和 CREATOR.md",
		"ideas.md 继续作为方向指引",
		"CREATOR.md 继续作为每轮最高优先级创作者指令生效",
		"及时写回 `ideas.md` 方便作者统一查看",
		"内容保持短小、可扫读、方便作者评论和后续更新",
		"建议控制在 800-1200 个中文字内",
		"每章安排只写 3-5 条关键点",
		"ch{order:05}-{chapter}-{title}.md",
		"v{order:05}-{volume}",
		"不要自动重命名旧章节",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("系统提示缺少 %q:\n%s", required, instruction)
		}
	}
}
