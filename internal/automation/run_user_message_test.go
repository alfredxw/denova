package automation

import (
	"strings"
	"testing"
)

func TestBuildRunUserMessageIncludesScopeAndEvidence(t *testing.T) {
	task := Task{
		Name: "每日回顾", Target: ExecutionTarget{Kind: TargetKindWorkspace},
		Prompt: "请回顾今日新增章节并按需修改。",
	}
	run := RunRecord{
		Trigger: TriggerCondition,
		TriggerEvidence: []TriggerEvidence{{
			Source: "chapter", Title: "新增第三章", Ref: "chapters/ch03.md", Snippet: "开头",
		}},
	}
	message := BuildRunUserMessage(task, run, "")
	for _, expected := range []string{
		"任务名称：每日回顾",
		"触发来源：" + TriggerCondition,
		"[chapter] 新增第三章 — chapters/ch03.md",
		"请回顾今日新增章节并按需修改。",
		"请你自行使用可用工具读取完成任务所需的工作区文件",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message missing %q:\n%s", expected, message)
		}
	}
}

func TestBuildRunUserMessageLegacyConfirmationIncludesConfirmedSummary(t *testing.T) {
	task := Task{Name: "确认继续", Target: ExecutionTarget{Kind: TargetKindWorkspace}}
	run := RunRecord{Trigger: TriggerWriteConfirmation}
	message := BuildRunUserMessage(task, run, "已确认方案正文")
	for _, expected := range []string{
		"用户已经确认继续处理上一轮方案",
		"已确认方案摘要：\n已确认方案正文\n",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("confirmation message missing %q:\n%s", expected, message)
		}
	}
}

func TestBuildRunUserMessageUsesGenericPromptAndUserScopeRestriction(t *testing.T) {
	task := Task{Name: "用户任务", Target: ExecutionTarget{Kind: TargetKindUser}}
	run := RunRecord{Trigger: TriggerSchedule}
	message := BuildRunUserMessage(task, run, "")
	if !strings.Contains(message, GenericTaskPrompt) {
		t.Fatalf("empty prompt should fall back to GenericTaskPrompt:\n%s", message)
	}
	if !strings.Contains(message, "这是用户全局任务，没有书籍工作区") {
		t.Fatalf("user-scope restriction missing:\n%s", message)
	}
}
