package automation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreSeparatesUserAndWorkspaceTasks(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspace := filepath.Join(root, "book")
	if err := os.MkdirAll(filepath.Join(workspace, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewStore(userDir, workspace)

	userTask, err := store.Create(Task{Scope: ScopeUser, Name: "User task", Template: TemplateCustomPrompt})
	if err != nil {
		t.Fatalf("create user task: %v", err)
	}
	workspaceTask, err := store.Create(Task{Scope: ScopeWorkspace, Name: "Workspace task", Template: TemplateReview})
	if err != nil {
		t.Fatalf("create workspace task: %v", err)
	}
	if _, err := os.Stat(filepath.Join(userDir, "automations", "tasks.json")); err != nil {
		t.Fatalf("user tasks not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".nova", "automations", "tasks.json")); err != nil {
		t.Fatalf("workspace tasks not written: %v", err)
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 4 {
		t.Fatalf("task count = %d, want 4", len(tasks))
	}
	if !hasTask(tasks, "workspace-auto-continue-writing") || !hasTask(tasks, "workspace-auto-review") {
		t.Fatalf("default workspace automations missing: %#v", tasks)
	}

	userOnly, err := NewStore(userDir, "").List()
	if err != nil {
		t.Fatalf("list user-only: %v", err)
	}
	if len(userOnly) != 1 || userOnly[0].ID != userTask.ID {
		t.Fatalf("user-only tasks = %#v, want %s", userOnly, userTask.ID)
	}
	if _, err := NewStore(userDir, "").Get(workspaceTask.ID); err == nil {
		t.Fatalf("workspace task should not be visible without workspace")
	}
}

func TestStoreSeedsDefaultWorkspaceAutomationsDisabled(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	tasks, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want seeded defaults only", len(tasks))
	}
	continueTask := taskByID(tasks, "workspace-auto-continue-writing")
	if continueTask == nil || continueTask.Enabled || continueTask.Template != TemplateContinueWriting || continueTask.WriteMode != WriteModeConfirmWrite || continueTask.WriteScope != WriteScopeFile {
		t.Fatalf("unexpected continue writing seed: %#v", continueTask)
	}
	if !strings.Contains(continueTask.Prompt, "续写下一章") || !strings.Contains(continueTask.Prompt, "CREATOR.md") {
		t.Fatalf("continue writing seed prompt should be editable task config, got %q", continueTask.Prompt)
	}
	reviewTask := taskByID(tasks, "workspace-auto-review")
	if reviewTask == nil || reviewTask.Enabled || reviewTask.Template != TemplateReview || reviewTask.WriteMode != WriteModeReadOnly {
		t.Fatalf("unexpected review seed: %#v", reviewTask)
	}
	if !strings.Contains(reviewTask.Prompt, "新增章节") || !strings.Contains(reviewTask.Prompt, "不要把全书当作被评审正文") {
		t.Fatalf("review seed prompt should target new chapters, got %q", reviewTask.Prompt)
	}
	if len(reviewTask.Triggers) != 1 || reviewTask.Triggers[0].Type != TriggerTypeChapterBatch || reviewTask.Triggers[0].ChapterBatchSize != 5 {
		t.Fatalf("unexpected review seed trigger: %#v", reviewTask.Triggers)
	}
}

func TestStoreAppendRunUpdatesExistingRun(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "nova"), workspace)
	task, err := store.Create(Task{Scope: ScopeWorkspace, Name: "Review", Template: TemplateReview})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	first := RunRecord{ID: "run-1", TaskID: task.ID, Scope: ScopeWorkspace, Trigger: TriggerManual, Status: RunStatusSuccess, Summary: "first"}
	if _, err := store.AppendRun(task.ID, first); err != nil {
		t.Fatalf("AppendRun first failed: %v", err)
	}
	second := first
	second.Summary = "second"
	updated, err := store.AppendRun(task.ID, second)
	if err != nil {
		t.Fatalf("AppendRun second failed: %v", err)
	}
	if len(updated.RecentRuns) != 1 {
		t.Fatalf("recent runs = %#v, want one updated run", updated.RecentRuns)
	}
	if updated.RecentRuns[0].Summary != "second" || updated.LastRun == nil || updated.LastRun.Summary != "second" {
		t.Fatalf("run was not updated in place: %#v last=%#v", updated.RecentRuns, updated.LastRun)
	}
}

func TestStoreMigratesSeededDefaultAutomationPrompts(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	if _, err := store.List(); err != nil {
		t.Fatalf("initial List failed: %v", err)
	}
	tasks, err := store.readScope(ScopeWorkspace)
	if err != nil {
		t.Fatalf("read workspace tasks failed: %v", err)
	}
	for i := range tasks {
		if tasks[i].ID == "workspace-auto-review" || tasks[i].ID == "workspace-auto-continue-writing" {
			tasks[i].Prompt = ""
		}
	}
	if err := store.writeScopeFile(ScopeWorkspace, storeFile{SeedVersion: 1, Tasks: tasks}); err != nil {
		t.Fatalf("write legacy seed file failed: %v", err)
	}

	migrated, err := store.List()
	if err != nil {
		t.Fatalf("List after legacy seed failed: %v", err)
	}
	if prompt := taskByID(migrated, "workspace-auto-review").Prompt; !strings.Contains(prompt, "新增章节") {
		t.Fatalf("review prompt was not migrated: %q", prompt)
	}
	if prompt := taskByID(migrated, "workspace-auto-continue-writing").Prompt; !strings.Contains(prompt, "续写下一章") {
		t.Fatalf("continue prompt was not migrated: %q", prompt)
	}
}

func TestStoreDoesNotReseedDeletedDefaultWorkspaceAutomation(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	if _, err := store.List(); err != nil {
		t.Fatalf("initial List failed: %v", err)
	}
	if err := store.Delete("workspace-auto-review"); err != nil {
		t.Fatalf("Delete default review failed: %v", err)
	}
	tasks, err := store.List()
	if err != nil {
		t.Fatalf("second List failed: %v", err)
	}
	if hasTask(tasks, "workspace-auto-review") {
		t.Fatalf("deleted default review should not be reseeded: %#v", tasks)
	}
	if !hasTask(tasks, "workspace-auto-continue-writing") {
		t.Fatalf("unrelated default should remain: %#v", tasks)
	}
}

func TestNormalizeScheduleBuildsCronShape(t *testing.T) {
	tests := []struct {
		name     string
		schedule Schedule
		wantCron string
	}{
		{"daily", Schedule{Kind: ScheduleDaily, Hour: 9, Minute: 30}, "30 9 * * *"},
		{"weekly", Schedule{Kind: ScheduleWeekly, Weekday: 2, Hour: 8, Minute: 5}, "5 8 * * 2"},
		{"monthly", Schedule{Kind: ScheduleMonthly, DayOfMonth: 12, Hour: 7, Minute: 0}, "0 7 12 * *"},
		{"every-hours", Schedule{Kind: ScheduleEveryHours, EveryHours: 6, Minute: 15}, "15 */6 * * *"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeSchedule(tt.schedule)
			if err != nil {
				t.Fatalf("NormalizeSchedule failed: %v", err)
			}
			if got.Cron != tt.wantCron {
				t.Fatalf("cron = %q, want %q", got.Cron, tt.wantCron)
			}
		})
	}
}

func TestDueHandlesStructuredSchedules(t *testing.T) {
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	last := now.Add(-25 * time.Hour)
	task := Task{
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleDaily, Hour: 9, Minute: 0},
		LastRun:  &RunRecord{StartedAt: last},
	}
	if !Due(now, task) {
		t.Fatalf("daily task should be due")
	}
	task.Enabled = false
	if Due(now, task) {
		t.Fatalf("disabled task should not be due")
	}
	task.Enabled = true
	task.Schedule = Schedule{Kind: ScheduleManual}
	if Due(now, task) {
		t.Fatalf("manual task should not be due")
	}
}

func TestNormalizeTaskAcceptsContinueWritingTemplate(t *testing.T) {
	task, err := NormalizeTask(Task{Scope: ScopeWorkspace, Name: "Continue", Template: TemplateContinueWriting})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.Template != TemplateContinueWriting {
		t.Fatalf("template = %q, want %q", task.Template, TemplateContinueWriting)
	}
}

func TestNormalizeTaskTrimsModelProfileID(t *testing.T) {
	task, err := NormalizeTask(Task{Scope: ScopeWorkspace, Name: "Profile", Template: TemplateReview, ModelProfileID: " fast "})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.ModelProfileID != "fast" {
		t.Fatalf("model profile id = %q, want fast", task.ModelProfileID)
	}
}

func TestNormalizeTaskMigratesLegacyWritePolicy(t *testing.T) {
	tests := []struct {
		name       string
		policy     string
		wantMode   string
		wantScope  string
		wantPolicy string
	}{
		{"read-only", WritePolicyReadOnly, WriteModeReadOnly, WriteScopeNone, WritePolicyReadOnly},
		{"file", WritePolicyAllowFileWrite, WriteModeAutoWrite, WriteScopeFile, WritePolicyAllowFileWrite},
		{"lore", WritePolicyAllowLoreWrite, WriteModeAutoWrite, WriteScopeLore, WritePolicyAllowLoreWrite},
		{"both", WritePolicyAllowLoreAndFileWrite, WriteModeAutoWrite, WriteScopeLoreAndFile, WritePolicyAllowLoreAndFileWrite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := NormalizeTask(Task{Scope: ScopeWorkspace, Name: "Write", Template: TemplateReview, WritePolicy: tt.policy})
			if err != nil {
				t.Fatalf("NormalizeTask failed: %v", err)
			}
			if task.WriteMode != tt.wantMode || task.WriteScope != tt.wantScope || task.WritePolicy != tt.wantPolicy {
				t.Fatalf("write config = %s/%s/%s, want %s/%s/%s", task.WriteMode, task.WriteScope, task.WritePolicy, tt.wantMode, tt.wantScope, tt.wantPolicy)
			}
			if task.DefaultActionPolicy != ActionPolicyAutoRun {
				t.Fatalf("default action = %q, want auto_run derived from execution mode", task.DefaultActionPolicy)
			}
		})
	}
}

func TestNormalizeTaskAcceptsChapterBatchTrigger(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:    ScopeWorkspace,
		Name:     "Batch review",
		Template: TemplateReview,
		Triggers: []TriggerDefinition{{
			Type:    TriggerTypeChapterBatch,
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if len(task.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(task.Triggers))
	}
	trigger := task.Triggers[0]
	if trigger.Type != TriggerTypeChapterBatch || trigger.ChapterBatchSize != 5 || trigger.NotifyPolicy != NotifyPolicyInbox {
		t.Fatalf("unexpected chapter batch trigger: %#v", trigger)
	}
}

func TestNormalizeTaskMigratesLegacyScheduleToTaskLevelSilentAutoRun(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:    ScopeWorkspace,
		Name:     "Legacy schedule",
		Template: TemplateReview,
		Schedule: Schedule{Kind: ScheduleDaily, Hour: 10, Minute: 5},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.DefaultActionPolicy != ActionPolicyAutoRun {
		t.Fatalf("default action = %q, want auto_run", task.DefaultActionPolicy)
	}
	if len(task.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(task.Triggers))
	}
	trigger := task.Triggers[0]
	if trigger.Type != TriggerTypeSchedule || !trigger.Enabled {
		t.Fatalf("legacy trigger = %#v, want enabled schedule", trigger)
	}
	if trigger.ActionPolicy != "" || trigger.NotifyPolicy != NotifyPolicySilent {
		t.Fatalf("legacy trigger policy = %s/%s, want empty/silent", trigger.ActionPolicy, trigger.NotifyPolicy)
	}
}

func TestNormalizeTaskClearsLegacyTriggerActionAndDerivesTaskActionFromWriteMode(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:               ScopeWorkspace,
		Name:                "Saved legacy schedule",
		Template:            TemplateReview,
		DefaultActionPolicy: ActionPolicyConfirm,
		WriteMode:           WriteModeConfirmWrite,
		WriteScope:          WriteScopeFile,
		Triggers: []TriggerDefinition{{
			Type:         TriggerTypeSchedule,
			Enabled:      true,
			ActionPolicy: ActionPolicyAutoRun,
			NotifyPolicy: NotifyPolicySilent,
			Schedule:     Schedule{Kind: ScheduleDaily, Hour: 10, Minute: 5},
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.DefaultActionPolicy != ActionPolicyAutoRun {
		t.Fatalf("default action = %q, want auto_run", task.DefaultActionPolicy)
	}
	if task.Triggers[0].ActionPolicy != "" {
		t.Fatalf("trigger action should be cleared, got %q", task.Triggers[0].ActionPolicy)
	}
}

func TestNormalizeTaskMigratesLegacyCharacterTriggerToSemantic(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:    ScopeWorkspace,
		Name:     "Legacy semantic",
		Template: TemplateReview,
		Triggers: []TriggerDefinition{{
			Type:    "interactive_new_character",
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if len(task.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(task.Triggers))
	}
	trigger := task.Triggers[0]
	if trigger.Type != TriggerTypeSemantic || !strings.Contains(trigger.SemanticCondition, "新") {
		t.Fatalf("legacy trigger not migrated to semantic: %#v", trigger)
	}
}

func TestEffectiveTriggerPolicies(t *testing.T) {
	task := Task{DefaultActionPolicy: ActionPolicyNotifyOnly, WriteMode: WriteModeReadOnly, WriteScope: WriteScopeNone}
	trigger := TriggerDefinition{Type: TriggerTypeSchedule, NotifyPolicy: NotifyPolicySilent}
	if got := EffectiveActionPolicy(task, trigger); got != ActionPolicyAutoRun {
		t.Fatalf("effective action = %q, want auto_run derived from execution mode", got)
	}
	if got := EffectiveNotifyPolicy(task, trigger); got != NotifyPolicySilent {
		t.Fatalf("schedule notify = %q, want silent", got)
	}
	trigger.ActionPolicy = ActionPolicyConfirm
	if got := EffectiveActionPolicy(task, trigger); got != ActionPolicyAutoRun {
		t.Fatalf("trigger action override should be ignored, got %q", got)
	}
	task.DefaultActionPolicy = ActionPolicyConfirm
	if got := EffectiveNotifyPolicy(task, trigger); got != NotifyPolicySilent {
		t.Fatalf("execution mode should not force inbox notify, got %q", got)
	}
}

func TestStoreInboxLifecycle(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	item, err := store.CreateInboxItem(TriggerInboxItem{
		TaskID:       "auto-1",
		TriggerID:    "schedule",
		Scope:        ScopeWorkspace,
		ActionPolicy: ActionPolicyConfirm,
		NotifyPolicy: NotifyPolicyInbox,
		Title:        "Review ready",
		Summary:      "A chapter is ready.",
		Fingerprint:  "fp-1",
	})
	if err != nil {
		t.Fatalf("CreateInboxItem failed: %v", err)
	}
	if item.ID == "" || item.Status != InboxStatusPending {
		t.Fatalf("unexpected item after create: %#v", item)
	}
	if _, ok, err := store.FindOpenInboxItem("auto-1", "schedule", "fp-1"); err != nil || !ok {
		t.Fatalf("FindOpenInboxItem ok=%v err=%v", ok, err)
	}
	read, err := store.MarkInboxItemRead(item.ID)
	if err != nil {
		t.Fatalf("MarkInboxItemRead failed: %v", err)
	}
	if read.ReadAt == nil {
		t.Fatalf("read_at should be set")
	}
	confirmed, err := store.ConfirmInboxItem(item.ID, "run-1")
	if err != nil {
		t.Fatalf("ConfirmInboxItem failed: %v", err)
	}
	if confirmed.Status != InboxStatusConfirmed || confirmed.RunID != "run-1" || confirmed.HandledAt == nil {
		t.Fatalf("unexpected confirmed item: %#v", confirmed)
	}
	if _, ok, err := store.FindOpenInboxItem("auto-1", "schedule", "fp-1"); err != nil || ok {
		t.Fatalf("confirmed item should not remain open ok=%v err=%v", ok, err)
	}
}

func TestTriggerContextBoundAndSemanticEvaluation(t *testing.T) {
	ctx := BoundedTriggerContext(TriggerContext{
		Source:  strings.Repeat("s", 300),
		Summary: strings.Repeat("一", 2000),
		Evidence: []TriggerEvidence{{
			Source:  "chapter",
			Title:   strings.Repeat("t", 500),
			Ref:     strings.Repeat("r", 500),
			Snippet: strings.Repeat("x", 2000),
		}},
	})
	if len([]rune(ctx.Source)) > 120 || len([]rune(ctx.Summary)) > 1000 {
		t.Fatalf("context source/summary not bounded: %#v", ctx)
	}
	if len(ctx.Evidence) != 1 || len([]rune(ctx.Evidence[0].Snippet)) > 1200 {
		t.Fatalf("evidence not bounded: %#v", ctx.Evidence)
	}
	eval, err := ParseSemanticEvaluation(`{"matched":true,"confidence":0.82,"reason":"ok","title":"Hit","evidence_refs":[" a ",""]}`)
	if err != nil {
		t.Fatalf("ParseSemanticEvaluation failed: %v", err)
	}
	if !eval.Matched || eval.Confidence != 0.82 || len(eval.EvidenceRefs) != 1 || eval.EvidenceRefs[0] != "a" {
		t.Fatalf("unexpected semantic eval: %#v", eval)
	}
}

func hasTask(tasks []Task, id string) bool {
	return taskByID(tasks, id) != nil
}

func taskByID(tasks []Task, id string) *Task {
	for i := range tasks {
		if tasks[i].ID == id {
			return &tasks[i]
		}
	}
	return nil
}
