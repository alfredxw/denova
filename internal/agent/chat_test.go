package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/internal/book"
	"nova/internal/session"
)

func TestMergeToolCalls(t *testing.T) {
	idx := 0
	calls := mergeToolCalls(nil, []schema.ToolCall{
		{Index: &idx, Function: schema.FunctionCall{Name: "write_file", Arguments: `{"path":`}},
	})
	calls = mergeToolCalls(calls, []schema.ToolCall{
		{Index: &idx, Function: schema.FunctionCall{Arguments: `"chapters/ch01.md"}`}},
	})

	if len(calls) != 1 {
		t.Fatalf("期望 1 个 tool call，实际: %d", len(calls))
	}
	if calls[0].Function.Name != "write_file" {
		t.Fatalf("工具名称未合并: %s", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"path":"chapters/ch01.md"}` {
		t.Fatalf("工具参数未合并: %s", calls[0].Function.Arguments)
	}
}

func TestMergeToolCallsHandlesSparseIndexes(t *testing.T) {
	idx := 2
	calls := mergeToolCalls(nil, []schema.ToolCall{
		{Index: &idx, ID: "call-2", Function: schema.FunctionCall{Name: "edit_file", Arguments: `{"path":`}},
	})
	calls = mergeToolCalls(calls, []schema.ToolCall{
		{Index: &idx, Function: schema.FunctionCall{Arguments: `"chapters/ch02.md"}`}},
	})

	if len(calls) != 3 {
		t.Fatalf("稀疏 index 应补齐切片长度，实际: %d", len(calls))
	}
	if calls[2].ID != "call-2" || calls[2].Function.Name != "edit_file" {
		t.Fatalf("工具元信息未按 index 保留: %#v", calls[2])
	}
	if calls[2].Function.Arguments != `{"path":"chapters/ch02.md"}` {
		t.Fatalf("工具参数未按 index 合并: %s", calls[2].Function.Arguments)
	}
}

func TestParseWriteLoreItemsToolResultReturnsChangedIDs(t *testing.T) {
	itemIDs, deletedIDs := parseWriteLoreItemsToolResult("write_lore_items", strings.Join([]string{
		"message: 已更新资料库",
		`item_ids: ["char_hero","world_rule"]`,
		`deleted_ids: ["old_note"]`,
	}, "\n"))

	if got := strings.Join(itemIDs, ","); got != "char_hero,world_rule" {
		t.Fatalf("未解析写入资料 ID: %v", itemIDs)
	}
	if got := strings.Join(deletedIDs, ","); got != "old_note" {
		t.Fatalf("未解析删除资料 ID: %v", deletedIDs)
	}
}

func TestComposeAgentInputDoesNotInjectImagePresetContext(t *testing.T) {
	composition := composeAgentInput(ChatRequest{
		Message:       "给当前章节生成插画",
		ImagePresetID: "realistic",
		ImagePreset: ImagePresetContext{
			ID:                "realistic",
			Name:              "写实",
			AgentSystemPrompt: "系统理解规则。",
			ToolRequestPrompt: "真实光影和摄影感。",
		},
	}, nil, nil, DefaultLoopPolicy())
	if strings.Contains(composition.AgentMessage, "真实光影和摄影感") || strings.Contains(composition.AgentMessage, "图像方案预设") {
		t.Fatalf("image preset should not be injected into turn message:\n%s", composition.AgentMessage)
	}
	if composition.ContextLog != nil && strings.Contains(composition.ContextLog.String(), "图像方案预设") {
		t.Fatalf("context log should not record image preset as turn context:\n%s", composition.ContextLog.String())
	}
}

func TestAppendReferenceContextDedupesAndReportsReadFailure(t *testing.T) {
	workspace := t.TempDir()
	mustWriteTestFile(t, workspace, "chapters/ch01.md", "第一章正文")
	service := book.NewService(workspace)

	got := appendReferenceContext(service, "请参考", []string{
		"chapters/ch01.md",
		"chapters/ch01.md",
		"chapters/missing.md",
	})

	assertContains(t, got, "请参考")
	assertContains(t, got, "以下是用户引用的文件")
	assertContains(t, got, "## @chapters/ch01.md")
	assertContains(t, got, "```markdown\n第一章正文\n```")
	assertContains(t, got, "## @chapters/missing.md")
	assertContains(t, got, "读取失败：")
	if count := strings.Count(got, "## @chapters/ch01.md"); count != 1 {
		t.Fatalf("重复引用应去重，实际出现 %d 次\n%s", count, got)
	}
}

func TestAppendSelectionContextIncludesFileAndLineRange(t *testing.T) {
	got := appendSelectionContext("修改这段", []TextSelectionRef{
		{
			FileName:  "chapters/ch03.md",
			StartLine: 12,
			EndLine:   18,
			Content:   "选中的正文",
		},
	})

	assertContains(t, got, "修改这段")
	assertContains(t, got, "以下是用户在编辑器中选中的文本片段")
	assertContains(t, got, "## 选中内容来自 chapters/ch03.md:L12-L18")
	assertContains(t, got, "```\n选中的正文\n```")
}

func TestAppendPlanModeInstructionUsesStructuredPlanningProtocol(t *testing.T) {
	got := appendPlanModeInstruction("重构章节")

	assertContains(t, got, "[Plan Mode / 规划模式]")
	assertContains(t, got, "不要直接执行")
	assertContains(t, got, "<plan_questions>")
	assertContains(t, got, "<proposed_plan>")
	assertContains(t, got, "# 计划标题")
	assertContains(t, got, "## Summary")
	assertContains(t, got, "## Key Changes")
	assertContains(t, got, "用户需求：\n重构章节")
	if strings.Contains(got, "Tests、Assumptions") || strings.Contains(got, "Test Plan") {
		t.Fatalf("Plan Mode 最终方案模板不应强制输出测试或假设小节:\n%s", got)
	}
}

func TestAppendContextBoundaryInstructionEmphasizesCurrentRequest(t *testing.T) {
	got := appendContextBoundaryInstruction("帮我写第三章")

	assertContains(t, got, "[上下文边界]")
	assertContains(t, got, "当前用户请求是“这次要做什么”")
	assertContains(t, got, "已确认的小说状态")
	assertContains(t, got, "背景是什么")
	assertContains(t, got, "历史对话只能辅助理解")
	assertContains(t, got, "以当前请求为准")
	assertContains(t, got, "本轮请求：\n帮我写第三章")
}

func TestStyleRulesSystemInstructionEmitsSceneAndStyles(t *testing.T) {
	got := styleRulesSystemInstruction([]StyleRule{
		{Scene: "激烈打斗", StyleContents: []string{"短句留白", "强冲突快节奏"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
		{Scene: "", StyleContents: []string{"无效内容"}},     // 应被跳过
		{Scene: "空风格", StyleContents: []string{"", " "}}, // 空内容应被跳过
	})

	assertContains(t, got, "## 场景化风格规则")
	assertContains(t, got, "场景：激烈打斗")
	assertContains(t, got, "短句留白")
	assertContains(t, got, "强冲突快节奏")
	assertContains(t, got, "场景：日常对话")
	assertContains(t, got, "温吞对白")
	assertContains(t, got, "选出最贴近的场景")
	assertContains(t, got, "完全忽略以上规则")
	if strings.Contains(got, "read_file") {
		t.Fatalf("场景风格内容不应要求 read_file：\n%s", got)
	}
	if strings.Contains(got, "无效内容") {
		t.Fatalf("空 scene 的规则应被跳过，但仍包含无效内容：\n%s", got)
	}
}

func TestBuildInterruptedResumeMessageIncludesInterruptedContext(t *testing.T) {
	got := buildInterruptedResumeMessage("继续", &session.Interruption{
		UserMessage:      "写第一章",
		AssistantContent: "已经写出的片段",
		Reason:           "runner error",
	})

	assertContains(t, got, "[异常中断恢复]")
	assertContains(t, got, "用户当前要求继续")
	assertContains(t, got, "写第一章")
	assertContains(t, got, "已经写出的片段")
	assertContains(t, got, "runner error")
}

func TestShouldResumeInterruptedRequestOnlyMatchesExplicitContinue(t *testing.T) {
	if !shouldResumeInterruptedRequest("继续") {
		t.Fatal("明确的继续请求应触发异常恢复")
	}
	if !shouldResumeInterruptedRequest("继续刚才的任务") {
		t.Fatal("继续刚才的任务应触发异常恢复")
	}
	if shouldResumeInterruptedRequest("帮我写下一章") {
		t.Fatal("普通请求不应触发异常恢复")
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("期望包含 %q\n实际内容:\n%s", want, got)
	}
}

func mustWriteTestFile(t *testing.T, workspace, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("创建测试目录失败: %v", err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
}
