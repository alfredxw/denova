package automation

import (
	"strings"
	"time"
)

const (
	legacyContinueWritingTaskID = "workspace-auto-continue-writing"
	legacyReviewTaskID          = "workspace-auto-review"
)

// BuiltinTaskTemplates returns the application-level creation recipes. The
// catalog has constant size and is never expanded per workspace.
func BuiltinTaskTemplates(locale string) []TaskTemplate {
	english := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en")
	continueName := "续写章节"
	continueDescription := "读取大纲、进度与最近章节，准备下一章；默认确认后写入文件。"
	continuePrompt := DefaultContinueWritingPrompt
	reviewName := "自动 Review"
	reviewDescription := "每 5 个新章节检查连续性、设定、节奏与语言质量。"
	reviewPrompt := DefaultReviewPrompt
	if english {
		continueName = "Continue Writing"
		continueDescription = "Read the outline, progress, and recent chapters to prepare the next chapter; confirm before writing files."
		continuePrompt = DefaultContinueWritingPromptEnglish
		reviewName = "Automatic Review"
		reviewDescription = "Review continuity, lore, pacing, and prose quality every five new chapters."
		reviewPrompt = DefaultReviewPromptEnglish
	}

	schedule := Schedule{Kind: ScheduleManual, Hour: 9, Minute: 0, Weekday: 1, DayOfMonth: 1, EveryHours: 6}
	return []TaskTemplate{
		{
			ID:          TemplateContinueWriting,
			Version:     1,
			Description: continueDescription,
			TargetKinds: []string{TargetKindWorkspace},
			Defaults: TaskTemplateDefaults{
				Enabled:             false,
				Name:                continueName,
				Template:            TemplateContinueWriting,
				Prompt:              continuePrompt,
				Schedule:            schedule,
				Triggers:            []TriggerDefinition{legacyScheduleTrigger(schedule)},
				DefaultActionPolicy: ActionPolicyAutoRun,
				WriteMode:           WriteModeConfirmWrite,
				WriteScope:          WriteScopeFile,
				OutputPolicy:        OutputPolicyRunRecordOnly,
			},
		},
		{
			ID:          TemplateReview,
			Version:     1,
			Description: reviewDescription,
			TargetKinds: []string{TargetKindWorkspace},
			Defaults: TaskTemplateDefaults{
				Enabled:  false,
				Name:     reviewName,
				Template: TemplateReview,
				Prompt:   reviewPrompt,
				Schedule: schedule,
				Triggers: []TriggerDefinition{{
					ID:               "chapter_batch_review",
					Type:             TriggerTypeChapterBatch,
					Enabled:          true,
					NotifyPolicy:     NotifyPolicyInbox,
					ChapterBatchSize: 5,
				}},
				DefaultActionPolicy: ActionPolicyAutoRun,
				WriteMode:           WriteModeReadOnly,
				WriteScope:          WriteScopeNone,
				OutputPolicy:        OutputPolicyRunRecordOnly,
			},
		},
	}
}

// legacyDefaultWorkspaceAutomations reconstructs the exact task identities
// previously inserted into every workspace. It exists only for conservative
// migration and must never be used to seed new stores.
func legacyDefaultWorkspaceAutomations(now time.Time) []Task {
	templates := BuiltinTaskTemplates("zh-CN")
	ids := []string{legacyContinueWritingTaskID, legacyReviewTaskID}
	tasks := make([]Task, 0, len(templates))
	for i, template := range templates {
		defaults := template.Defaults
		tasks = append(tasks, Task{
			ID:                  ids[i],
			Scope:               ScopeWorkspace,
			Enabled:             defaults.Enabled,
			Name:                defaults.Name,
			Template:            defaults.Template,
			Prompt:              defaults.Prompt,
			ModelProfileID:      defaults.ModelProfileID,
			Schedule:            defaults.Schedule,
			Triggers:            defaults.Triggers,
			DefaultActionPolicy: defaults.DefaultActionPolicy,
			WriteMode:           defaults.WriteMode,
			WriteScope:          defaults.WriteScope,
			OutputPolicy:        defaults.OutputPolicy,
			OutputPath:          defaults.OutputPath,
			RecentRuns:          []RunRecord{},
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return tasks
}
