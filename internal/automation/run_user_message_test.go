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
		"Task name: 每日回顾",
		"Trigger source: " + TriggerCondition,
		"[chapter] 新增第三章 — chapters/ch03.md",
		"请回顾今日新增章节并按需修改。",
		"Use available tools to read the workspace files, lore, and state required by the task",
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
		"The user confirmed continuation of the previous proposal",
		"Confirmed proposal summary:\n已确认方案正文\n",
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
	if !strings.Contains(message, "This is a user-global task with no book workspace") {
		t.Fatalf("user-scope restriction missing:\n%s", message)
	}
}
