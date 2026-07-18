package automation

import (
	"strings"
	"testing"
)

func TestBuildRunUserMessageIncludesScopeAndEvidence(t *testing.T) {
	task := Task{
		Name: "每日回顾", OutputPolicy: "summary",
		WriteMode: WriteModeAutoWrite, Target: ExecutionTarget{Kind: TargetKindWorkspace},
		Prompt: "请回顾今日新增章节。",
	}
	run := RunRecord{
		Trigger: TriggerCondition,
		TriggerEvidence: []TriggerEvidence{{
			Source: "chapter", Title: "新增第三章", Ref: "chapters/ch03.md", Snippet: "开头",
		}},
	}
	message := BuildRunUserMessage(task, run, WriteModeAutoWrite, WriteScopeFile, "")
	for _, expected := range []string{
		"任务名称：每日回顾",
		"触发来源：" + TriggerCondition,
		"执行模式：" + WriteModeAutoWrite,
		"写入范围：" + WriteScopeFile,
		"输出策略：summary",
		"[chapter] 新增第三章 — chapters/ch03.md",
		"请回顾今日新增章节。",
		"请你自行使用可用工具读取完成任务所需的工作区文件",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message missing %q:\n%s", expected, message)
		}
	}
}

func TestBuildRunUserMessageWriteConfirmationIncludesConfirmedSummary(t *testing.T) {
	task := Task{Name: "确认写入", WriteMode: WriteModeConfirmWrite, Target: ExecutionTarget{Kind: TargetKindWorkspace}}
	run := RunRecord{Trigger: TriggerWriteConfirmation}
	message := BuildRunUserMessage(task, run, WriteModeConfirmWrite, WriteScopeFile, "已确认方案正文")
	for _, expected := range []string{
		"写入确认：用户已经确认执行上一轮只读方案",
		"已确认方案摘要：\n已确认方案正文\n",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("confirmation message missing %q:\n%s", expected, message)
		}
	}
}

func TestBuildRunUserMessageConfirmWriteModeForcesReadOnly(t *testing.T) {
	task := Task{Name: "待确认", WriteMode: WriteModeConfirmWrite, Target: ExecutionTarget{Kind: TargetKindWorkspace}}
	run := RunRecord{Trigger: TriggerCondition}
	message := BuildRunUserMessage(task, run, WriteModeConfirmWrite, WriteScopeFile, "")
	if !strings.Contains(message, "写入确认模式：本轮强制只读") {
		t.Fatalf("confirm-write message missing read-only instruction:\n%s", message)
	}
	if strings.Contains(message, "已确认方案摘要") {
		t.Fatalf("non-confirmation run should not include confirmed summary:\n%s", message)
	}
}

func TestBuildRunUserMessageUsesGenericPromptAndUserScopeRestriction(t *testing.T) {
	task := Task{Name: "用户任务", WriteMode: WriteModeAutoWrite, Target: ExecutionTarget{Kind: TargetKindUser}}
	run := RunRecord{Trigger: TriggerSchedule}
	message := BuildRunUserMessage(task, run, WriteModeAutoWrite, WriteScopeNone, "")
	if !strings.Contains(message, GenericTaskPrompt) {
		t.Fatalf("empty prompt should fall back to GenericTaskPrompt:\n%s", message)
	}
	if !strings.Contains(message, "这是用户全局任务，没有书籍工作区") {
		t.Fatalf("user-scope restriction missing:\n%s", message)
	}
}
