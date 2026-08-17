package automation

import (
	"fmt"
	"strings"
)

// BuildRunUserMessage assembles the user prompt injected into an automation
// run. Prompt construction is domain logic (it depends on the task definition
// and trigger evidence), so it lives in the automation package rather than the
// app orchestration layer.
//
// confirmedSummary is the already-resolved summary of the source run when this
// run is a write-confirmation follow-up; the app layer performs the lookup so
// this function stays free of storage concerns.
func BuildRunUserMessage(task Task, run RunRecord, confirmedSummary string) string {
	var sb strings.Builder
	sb.WriteString("Execute this Denova automation task.\n\n")
	sb.WriteString(fmt.Sprintf("Task name: %s\n", task.Name))
	sb.WriteString(fmt.Sprintf("Trigger source: %s\n", run.Trigger))
	if len(run.TriggerEvidence) > 0 {
		sb.WriteString("\nTrigger scope (bounded evidence; prioritize this new material):\n")
		for _, item := range run.TriggerEvidence {
			sb.WriteString(FormatTriggerEvidenceLine(item))
		}
	}
	if run.Trigger == TriggerWriteConfirmation {
		sb.WriteString("\nThe user confirmed continuation of the previous proposal.\n")
		if summary := strings.TrimSpace(confirmedSummary); summary != "" {
			sb.WriteString("Confirmed proposal summary:\n")
			sb.WriteString(summary)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\nUser prompt:\n")
	if task.Prompt != "" {
		sb.WriteString(task.Prompt)
	} else {
		sb.WriteString(GenericTaskPrompt)
	}
	if task.Target.Kind == TargetKindUser {
		sb.WriteString("\n\nThis is a user-global task with no book workspace. Use only user-level Skills, Todo, or Web capabilities enabled for this run. Do not read or modify work files, lore, or Project state.")
	} else {
		sb.WriteString("\n\nUse available tools to read the workspace files, lore, and state required by the task. Locate the relevant scope before reading or writing.")
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
