package automation

import (
	"fmt"
	"strings"
)

// BuildRunUserMessage assembles the user prompt injected into an automation
// run. Prompt construction is domain logic (it depends on the task template,
// write mode, trigger evidence and output policy), so it lives in the
// automation package rather than the app orchestration layer.
//
// confirmedSummary is the already-resolved summary of the source run when this
// run is a write-confirmation follow-up; the app layer performs the lookup so
// this function stays free of storage concerns.
func BuildRunUserMessage(task Task, run RunRecord, writeMode, writeScope, confirmedSummary string) string {
	var sb strings.Builder
	sb.WriteString("执行 Denova 自动化任务。\n\n")
	sb.WriteString(fmt.Sprintf("任务名称：%s\n", task.Name))
	sb.WriteString(fmt.Sprintf("触发来源：%s\n", run.Trigger))
	sb.WriteString(fmt.Sprintf("执行模式：%s\n", writeMode))
	sb.WriteString(fmt.Sprintf("写入范围：%s\n", writeScope))
	sb.WriteString(fmt.Sprintf("输出策略：%s\n", task.OutputPolicy))
	if task.OutputPath != "" {
		sb.WriteString(fmt.Sprintf("输出文件：%s\n", task.OutputPath))
	}
	if len(run.TriggerEvidence) > 0 {
		sb.WriteString("\n本次触发范围（有界证据，优先处理这些新增内容）：\n")
		for _, item := range run.TriggerEvidence {
			sb.WriteString(FormatTriggerEvidenceLine(item))
		}
	}
	if run.Trigger == TriggerWriteConfirmation {
		sb.WriteString("\n写入确认：用户已经确认执行上一轮只读方案。请只在写入范围内落实方案，不要扩大修改范围。\n")
		if summary := strings.TrimSpace(confirmedSummary); summary != "" {
			sb.WriteString("已确认方案摘要：\n")
			sb.WriteString(summary)
			sb.WriteString("\n")
		}
	} else if task.WriteMode == WriteModeConfirmWrite {
		sb.WriteString("\n写入确认模式：本轮强制只读。请输出具体写入方案/修订建议，包括建议修改的路径、资料库项和原因；不要实际写入。用户确认后会启动第二个写入 run。\n")
	}
	sb.WriteString("\n用户 Prompt：\n")
	if task.Prompt != "" {
		sb.WriteString(task.Prompt)
	} else {
		sb.WriteString(GenericTaskPrompt)
	}
	if task.Target.Kind == TargetKindUser {
		sb.WriteString("\n\n这是用户全局任务，没有书籍工作区。只使用本轮启用的用户级 Skills、Todo 或 Web 能力；不得读取或修改作品文件、资料库和项目状态。")
	} else {
		sb.WriteString("\n\n请你自行使用可用工具读取完成任务所需的工作区文件、资料库和状态；先定位范围，再读取和写入。")
	}
	return sb.String()
}

func FormatTriggerEvidenceLine(item TriggerEvidence) string {
	source := strings.TrimSpace(item.Source)
	if source == "" {
		source = "unknown"
	}
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "(untitled)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- [%s] %s", source, title))
	if ref := strings.TrimSpace(item.Ref); ref != "" {
		sb.WriteString(fmt.Sprintf(" — %s", ref))
	}
	sb.WriteString("\n")
	if snippet := strings.TrimSpace(item.Snippet); snippet != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", snippet))
	}
	return sb.String()
}
