package automation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2 user-created tasks", len(tasks))
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

func TestStoreListDoesNotCreateWorkspaceAutomationFile(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	tasks, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("task count = %d, want no implicit tasks", len(tasks))
	}
	path := filepath.Join(workspace, ".denova", "automations", "tasks.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("read-only List should not create %s: %v", path, err)
	}
}

func TestStoreGetRunByIDResolvesRunAcrossScopes(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	store := NewStore(userDir, workspace)

	userTask, err := store.Create(Task{Scope: ScopeUser, Name: "User task", Template: TemplateCustomPrompt})
	if err != nil {
		t.Fatalf("Create user task failed: %v", err)
	}
	userRun := RunRecord{ID: "run-user", TaskID: userTask.ID, Scope: ScopeUser, Trigger: TriggerManual, Status: RunStatusSuccess}
	if _, err := store.AppendRun(userTask.ID, userRun); err != nil {
		t.Fatalf("AppendRun user failed: %v", err)
	}

	workspaceTask, err := store.Create(Task{Scope: ScopeWorkspace, Name: "Workspace task", Template: TemplateReview})
	if err != nil {
		t.Fatalf("Create workspace task failed: %v", err)
	}
	workspaceRun := RunRecord{ID: "run-workspace", TaskID: workspaceTask.ID, Scope: ScopeWorkspace, Trigger: TriggerManual, Status: RunStatusSuccess}
	if _, err := store.AppendRun(workspaceTask.ID, workspaceRun); err != nil {
		t.Fatalf("AppendRun workspace failed: %v", err)
	}

	for _, runID := range []string{"run-user", "run-workspace"} {
		if _, run, err := store.GetRunByID(runID); err != nil {
			t.Fatalf("GetRunByID(%q) failed: %v", runID, err)
		} else if run.ID != runID {
			t.Fatalf("GetRunByID(%q) returned run %q", runID, run.ID)
		}
	}
	if _, _, err := store.GetRunByID("run-missing"); err == nil {
		t.Fatal("GetRunByID for unknown run returned nil error")
	}
	if _, _, err := store.GetRunByID("  "); err == nil {
		t.Fatal("GetRunByID for empty run id returned nil error")
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

func TestStoreConcurrentUserScopeCreatesDoNotLoseTasks(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaces := []string{filepath.Join(root, "one"), filepath.Join(root, "two")}
	const count = 24
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := NewStore(userDir, workspaces[index%len(workspaces)]).Create(Task{
				Scope:    ScopeUser,
				Name:     "Concurrent user task",
				Template: TemplateCustomPrompt,
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create failed: %v", err)
		}
	}
	tasks, err := NewStore(userDir, "").List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != count {
		t.Fatalf("task count = %d, want %d", len(tasks), count)
	}
	data, err := os.ReadFile(filepath.Join(userDir, "automations", "tasks.json"))
	if err != nil {
		t.Fatalf("read tasks JSON: %v", err)
	}
	var persisted storeFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("tasks JSON is invalid after concurrent writes: %v", err)
	}
	if len(persisted.Tasks) != count {
		t.Fatalf("persisted task count = %d, want %d", len(persisted.Tasks), count)
	}
}

func TestStoreConcurrentAppendRunPreservesEveryRun(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	userDir := filepath.Join(root, "user")
	store := NewStore(userDir, workspace)
	task, err := store.Create(Task{Scope: ScopeWorkspace, Name: "Review", Template: TemplateReview})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	const count = 12
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := NewStore(userDir, workspace).AppendRun(task.ID, RunRecord{
				ID:      fmt.Sprintf("run-%02d", index),
				TaskID:  task.ID,
				Scope:   ScopeWorkspace,
				Trigger: TriggerManual,
				Status:  RunStatusSuccess,
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent AppendRun failed: %v", err)
		}
	}
	updated, err := store.Get(task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(updated.RecentRuns) != count {
		t.Fatalf("recent run count = %d, want %d", len(updated.RecentRuns), count)
	}
	seen := make(map[string]bool, count)
	for _, run := range updated.RecentRuns {
		seen[run.ID] = true
	}
	for i := 0; i < count; i++ {
		if !seen[fmt.Sprintf("run-%02d", i)] {
			t.Fatalf("run-%02d was lost: %#v", i, updated.RecentRuns)
		}
	}
}

func TestStoreConcurrentUserInboxWritesRemainWorkspaceScoped(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaces := []string{filepath.Join(root, "one"), filepath.Join(root, "two")}
	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := NewStore(userDir, workspaces[index%2]).CreateInboxItem(TriggerInboxItem{
				TaskID:       "shared-user-task",
				TriggerID:    "batch",
				Scope:        ScopeUser,
				ActionPolicy: ActionPolicyConfirm,
				NotifyPolicy: NotifyPolicyInbox,
				Title:        fmt.Sprintf("Item %d", index),
				Fingerprint:  fmt.Sprintf("fp-%d", index),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent CreateInboxItem failed: %v", err)
		}
	}
	for _, workspace := range workspaces {
		items, err := NewStore(userDir, workspace).ListInbox()
		if err != nil {
			t.Fatalf("ListInbox(%s) failed: %v", workspace, err)
		}
		if len(items) != count/2 {
			t.Fatalf("workspace %s inbox count = %d, want %d", workspace, len(items), count/2)
		}
		for _, item := range items {
			if canonicalStoreRoot(item.Workspace) != canonicalStoreRoot(workspace) {
				t.Fatalf("workspace %s received foreign inbox item: %#v", workspace, item)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(userDir, "automations", "inbox.json"))
	if err != nil {
		t.Fatalf("read inbox JSON: %v", err)
	}
	var persisted inboxFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("inbox JSON is invalid after concurrent writes: %v", err)
	}
	if len(persisted.Items) != count {
		t.Fatalf("persisted inbox count = %d, want %d", len(persisted.Items), count)
	}
}

func TestStoreRemovesPristineLegacyWorkspaceSeeds(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	now := time.Now().UTC()
	tasks := legacyDefaultWorkspaceAutomations(now)
	tasks[0].Prompt = "" // Version 1 seeds did not persist editable prompts.
	if err := store.writeScopeFile(ScopeWorkspace, storeFile{SeedVersion: 1, Tasks: tasks}); err != nil {
		t.Fatalf("write legacy seed file failed: %v", err)
	}

	migrated, err := store.List()
	if err != nil {
		t.Fatalf("List after legacy seed failed: %v", err)
	}
	if len(migrated) != 0 {
		t.Fatalf("pristine legacy seeds should be removed: %#v", migrated)
	}
}

func TestStorePreservesEditedOrUsedLegacyWorkspaceSeeds(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	now := time.Now().UTC()
	tasks := legacyDefaultWorkspaceAutomations(now)
	tasks[0].Prompt = "用户修改后的续写规则"
	tasks[0].UpdatedAt = now.Add(time.Minute)
	tasks[1].RecentRuns = []RunRecord{{ID: "run-1", TaskID: tasks[1].ID, Status: RunStatusSuccess}}
	if err := store.writeScopeFile(ScopeWorkspace, storeFile{SeedVersion: 2, Tasks: tasks}); err != nil {
		t.Fatalf("write legacy seed file failed: %v", err)
	}

	migrated, err := store.List()
	if err != nil {
		t.Fatalf("List after legacy seed failed: %v", err)
	}
	if len(migrated) != 2 {
		t.Fatalf("edited and used legacy tasks must be preserved: %#v", migrated)
	}
	if got := taskByID(migrated, legacyContinueWritingTaskID); got == nil || got.Prompt != "用户修改后的续写规则" {
		t.Fatalf("edited continue-writing task changed during migration: %#v", got)
	}
	if got := taskByID(migrated, legacyReviewTaskID); got == nil || len(got.RecentRuns) != 1 {
		t.Fatalf("used review task changed during migration: %#v", got)
	}
}

func TestStorePreservesSeedLikeTaskWithoutLegacyFileMarker(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	tasks := legacyDefaultWorkspaceAutomations(time.Now().UTC())[:1]
	if err := store.writeScopeFile(ScopeWorkspace, storeFile{Tasks: tasks}); err != nil {
		t.Fatalf("write unversioned task file failed: %v", err)
	}

	listed, err := store.List()
	if err != nil {
		t.Fatalf("List unversioned task file failed: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != legacyContinueWritingTaskID {
		t.Fatalf("unversioned user-owned task must be preserved: %#v", listed)
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

func TestTaskUnmarshalMigratesLegacyWritePolicy(t *testing.T) {
	tests := []struct {
		name      string
		policy    string
		wantMode  string
		wantScope string
	}{
		{"read-only", WritePolicyReadOnly, WriteModeReadOnly, WriteScopeNone},
		{"file", WritePolicyAllowFileWrite, WriteModeAutoWrite, WriteScopeFile},
		{"lore", WritePolicyAllowLoreWrite, WriteModeAutoWrite, WriteScopeLore},
		{"both", WritePolicyAllowLoreAndFileWrite, WriteModeAutoWrite, WriteScopeLoreAndFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"id":"task-1","scope":"workspace","name":"Write","template":"review","write_policy":%q}`, tt.policy)
			var task Task
			if err := json.Unmarshal([]byte(raw), &task); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if task.WriteMode != tt.wantMode || task.WriteScope != tt.wantScope {
				t.Fatalf("write config = %s/%s, want %s/%s", task.WriteMode, task.WriteScope, tt.wantMode, tt.wantScope)
			}
			normalized, err := NormalizeTask(task)
			if err != nil {
				t.Fatalf("NormalizeTask failed: %v", err)
			}
			if normalized.WriteMode != tt.wantMode || normalized.WriteScope != tt.wantScope {
				t.Fatalf("normalized write config = %s/%s, want %s/%s", normalized.WriteMode, normalized.WriteScope, tt.wantMode, tt.wantScope)
			}
			if normalized.DefaultActionPolicy != ActionPolicyAutoRun {
				t.Fatalf("default action = %q, want auto_run derived from execution mode", normalized.DefaultActionPolicy)
			}
		})
	}
}

func TestTaskMarshalOmitsLegacyWritePolicy(t *testing.T) {
	task, err := NormalizeTask(Task{Scope: ScopeWorkspace, Name: "Write", Template: TemplateReview, WriteMode: WriteModeAutoWrite, WriteScope: WriteScopeFile})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("re-decode failed: %v", err)
	}
	if _, exists := fields["write_policy"]; exists {
		t.Fatalf("serialized task still contains the retired write_policy field: %s", data)
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
