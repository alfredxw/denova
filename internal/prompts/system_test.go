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
	if strings.Contains(instruction, "# 当前作品状态") {
		t.Fatalf("系统提示不应直接注入动态作品状态:\n%s", instruction)
	}
}

func TestIDEWritingFlowUsesChapterStatusInsteadOfSeparateDraftDirectory(t *testing.T) {
	instruction := BuildIDEWritingFlowInstruction(SystemInstructionInput{
		Workspace: "/tmp/book",
	})

	for _, required := range []string{
		"章节初稿 -> 确认成章",
		"章节初稿直接写入 chapters/",
		"非空未确认章节为初稿",
		"作者确认后才标记为成章",
		"write_file 到 chapters/",
		"普通初稿不写入全书事实状态",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("写作流程提示缺少 %q:\n%s", required, instruction)
		}
	}
	for _, forbidden := range []string{
		"草稿" + "流程",
		"draft" + "s/",
		"Draft" + "Flow",
		"章节草稿应先写入",
	} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("写作流程提示不应包含旧草稿目录流程 %q:\n%s", forbidden, instruction)
		}
	}
	if strings.Contains(instruction, "%!(EXTRA") {
		t.Fatalf("写作流程提示存在多余 fmt 参数:\n%s", instruction)
	}
}
