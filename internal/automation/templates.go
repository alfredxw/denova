package automation

import (
	"strings"
	"time"
)

const (
	legacyContinueWritingTaskID = "workspace-auto-continue-writing"
	legacyReviewTaskID          = "workspace-auto-review"
	legacyReviewPrompt          = "对本次触发范围中的新增章节做自动 Review。若触发范围包含章节路径，只评审这些新增章节，不要把全书当作被评审正文；可读取必要前文、CREATOR.md、大纲、进度、角色状态和资料库作为对照依据。重点检查新增章节是否符合任务要求/用户 Prompt、CREATOR.md、长期大纲、角色设定与状态、世界观和已有连续性；评估剧情推进、人物行为动机、设定一致性、节奏、语言质量和可读性。按严重程度输出问题、证据位置、影响和可执行改进建议；执行模式不允许写入时只输出 Review 和修订方案。"
)

// BuiltinTaskTemplates returns the application-level creation recipes. The
// catalog has constant size and is never expanded per workspace.
func BuiltinTaskTemplates(locale string) []TaskTemplate {
	english := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en")
	continueName := "续写章节"
	continueDescription := "读取大纲、进度与最近章节，并根据 Prompt 续写下一章。"
	continuePrompt := DefaultContinueWritingPrompt
	reviewName := "自动 Review"
	reviewDescription := "每 5 个新章节检查连续性、设定、节奏与语言质量。"
	reviewPrompt := DefaultReviewPrompt
	if english {
		continueName = "Continue Writing"
		continueDescription = "Read the outline, progress, and recent chapters, then continue the next chapter according to the prompt."
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
				SessionStrategy:     SessionStrategyPerRun,
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
				SessionStrategy:     SessionStrategyPerRun,
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
		prompt := defaults.Prompt
		if ids[i] == legacyReviewTaskID {
			// Preserve the exact former seed so migration can still identify and
			// remove untouched tasks after the current template prompt evolves.
			prompt = legacyReviewPrompt
		}
		tasks = append(tasks, Task{
			ID:                  ids[i],
			Scope:               ScopeWorkspace,
			Enabled:             defaults.Enabled,
			Name:                defaults.Name,
			Template:            defaults.Template,
			Prompt:              prompt,
			ModelProfileID:      defaults.ModelProfileID,
			Schedule:            defaults.Schedule,
			Triggers:            defaults.Triggers,
			DefaultActionPolicy: defaults.DefaultActionPolicy,
			SessionStrategy:     defaults.SessionStrategy,
			RecentRuns:          []RunRecord{},
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return tasks
}
