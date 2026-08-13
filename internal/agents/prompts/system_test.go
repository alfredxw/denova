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
		"must be updated from the template with author confirmation during new-book ideation",
		"Read ideas.md and CREATOR.md first",
		"interim conclusion, open question, or tradeoff rationale",
		"CREATOR.md describes how it should be written over time and which rules always apply",
		"chapter-length goals",
		"promptly update ideas.md with edit or write",
		"update ideas.md and CREATOR.md separately with write",
		"ideas.md remains the direction guide",
		"CREATOR.md remains the highest-priority creator instruction",
		"Keep it short and scannable for unified author review",
		"preferably 800-1200 Han characters",
		"Limit each chapter arrangement to 3-5 key points",
		"ch{order:05}-{chapter}-{title}.md",
		"v{order:05}-{volume}",
		"never automatically rename old chapters",
		"Unwritten events from injected outlines and chapter-group plans are planning material",
		"must check for future-plot leakage",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("系统提示缺少 %q:\n%s", required, instruction)
		}
	}
	if strings.Contains(instruction, "# 当前作品状态") {
		t.Fatalf("系统提示不应直接注入动态作品状态:\n%s", instruction)
	}
}

func TestIDEWritingFlowKeepsChapterStatusIndependentFromStateSync(t *testing.T) {
	instruction := BuildIDEWritingFlowInstruction(SystemInstructionInput{
		Workspace: "/tmp/book",
	})

	for _, required := range []string{
		"chapter writing -> synchronize progress and character state",
		"Write chapter prose directly under chapters/",
		"non-empty unconfirmed chapter as a draft",
		"chapter status is only an editing marker",
		"does not affect next-chapter detection, context selection, or state synchronization",
		"Write the chapter with write",
		"update setting/progress.md and setting/character-states.md in the same turn",
		"without waiting for a separate author confirmation",
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
		"普通初稿不写入全书事实状态",
		"只有作者明确确认成章",
	} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("写作流程提示不应包含旧草稿目录流程 %q:\n%s", forbidden, instruction)
		}
	}
	if strings.Contains(instruction, "%!(EXTRA") {
		t.Fatalf("写作流程提示存在多余 fmt 参数:\n%s", instruction)
	}
}
