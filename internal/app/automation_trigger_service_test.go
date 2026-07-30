package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/automation"
	"denova/internal/book"
)

func TestAutomationCheckCreatesRetryableInboxWhenAutoRunCannotStart(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
	registerAutomationProjectForTest(t, app, workspace)
	app.ensureServices()

	now := time.Now()
	task, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Read-only schedule",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Schedule:   automation.Schedule{Kind: automation.ScheduleManual, Hour: now.Hour(), Minute: now.Minute()},
		Triggers: []automation.TriggerDefinition{{
			ID:           "schedule",
			Type:         automation.TriggerTypeSchedule,
			Enabled:      true,
			NotifyPolicy: automation.NotifyPolicyInbox,
			Schedule:     automation.Schedule{Kind: automation.ScheduleDaily, Hour: now.Hour(), Minute: now.Minute()},
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}

	items, err := app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("inbox count = %d, want 1", len(items))
	}
	if items[0].Status != automation.InboxStatusPending || items[0].ActionPolicy != automation.ActionPolicyAutoRun {
		t.Fatalf("unexpected inbox item: %#v", items[0])
	}
	if !strings.Contains(items[0].ActionError, "自动执行启动失败") {
		t.Fatalf("failed auto-run inbox should include retryable error summary: %#v", items[0])
	}
	if runs := app.RunDueAutomations(context.Background(), now); len(runs) != 0 {
		t.Fatalf("same trigger should not run twice, got %#v", runs)
	}
}

func TestAutomationCheckSkipsInboxForSilentScheduleTrigger(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
	registerAutomationProjectForTest(t, app, workspace)
	app.ensureServices()

	now := time.Now()
	task, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Silent read-only",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Schedule:   automation.Schedule{Kind: automation.ScheduleManual, Hour: now.Hour(), Minute: now.Minute()},
		Triggers: []automation.TriggerDefinition{{
			ID:           "schedule",
			Type:         automation.TriggerTypeSchedule,
			Enabled:      true,
			NotifyPolicy: automation.NotifyPolicySilent,
			Schedule:     automation.Schedule{Kind: automation.ScheduleDaily, Hour: now.Hour(), Minute: now.Minute()},
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}

	items, err := app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("silent trigger should not create inbox: %#v", items)
	}
	inbox, err := app.AutomationInbox()
	if err != nil {
		t.Fatalf("AutomationInbox failed: %v", err)
	}
	if len(inbox) != 0 {
		t.Fatalf("silent trigger should keep inbox empty: %#v", inbox)
	}
}

func TestAutomationChapterBatchTriggerCreatesInboxAtBatchBoundaries(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		writeTestChapter(t, workspace, i)
	}
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
	registerAutomationProjectForTest(t, app, workspace)
	app.ensureServices()
	t.Cleanup(app.Close)
	app.bookService = book.NewService(workspace)

	task, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Batch review",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:               "chapter_batch_5",
			Type:             automation.TriggerTypeChapterBatch,
			Enabled:          true,
			NotifyPolicy:     automation.NotifyPolicyInbox,
			ChapterBatchSize: 5,
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}

	items, err := app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers before batch failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items before batch = %#v, want none", items)
	}

	writeTestChapter(t, workspace, 5)
	items, err = app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers at first batch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("first batch item count = %d, want 1", len(items))
	}
	if got := len(items[0].Evidence); got != 5 {
		t.Fatalf("evidence count = %d, want 5", got)
	}
	if items[0].Evidence[4].Ref != "chapters/ch05.md" {
		t.Fatalf("last evidence ref = %q, want chapters/ch05.md", items[0].Evidence[4].Ref)
	}

	items, err = app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers duplicate batch failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("duplicate batch should not match again, got %#v", items)
	}
	if err := os.WriteFile(filepath.Join(workspace, "chapters", "ch05.md"), []byte("# Chapter 5\n\nThis chapter was edited after review and should not retrigger the same batch.\n"), 0o644); err != nil {
		t.Fatalf("update chapter 5: %v", err)
	}
	items, err = app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers after chapter edit failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("same batch should not retrigger after chapter metadata changes, got %#v", items)
	}
	if _, err := app.automation().storeAllWorkspaces().DismissInboxItem(itemsFromFirstBatch(t, app, task.ID)); err != nil {
		t.Fatalf("dismiss first batch inbox: %v", err)
	}
	savedTask, err := app.automation().storeAllWorkspaces().Get(task.ID)
	if err != nil {
		t.Fatalf("load saved task: %v", err)
	}
	savedTask.TriggerState = map[string]automation.TriggerState{}
	if _, err := app.UpdateAutomation(task.ID, savedTask); err != nil {
		t.Fatalf("clear trigger state: %v", err)
	}
	items, err = app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers after state loss failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("same batch should not retrigger after trigger state loss, got %#v", items)
	}
	inbox, err := app.AutomationInbox()
	if err != nil {
		t.Fatalf("AutomationInbox failed: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("duplicate batch should not create another inbox item: %#v", inbox)
	}

	for i := 6; i <= 10; i++ {
		writeTestChapter(t, workspace, i)
	}
	items, err = app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers at second batch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("second batch item count = %d, want 1", len(items))
	}
	if items[0].Evidence[0].Ref != "chapters/ch06.md" || items[0].Evidence[4].Ref != "chapters/ch10.md" {
		t.Fatalf("second batch evidence = %#v", items[0].Evidence)
	}
}

func TestAutomationMutationCheckRunsOnlyContentTriggersForChapterWrites(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestChapter(t, workspace, 1)
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
	registerAutomationProjectForTest(t, app, workspace)
	app.ensureServices()
	app.bookService = book.NewService(workspace)

	now := time.Now()
	if _, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Due schedule",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:           "schedule",
			Type:         automation.TriggerTypeSchedule,
			Enabled:      true,
			NotifyPolicy: automation.NotifyPolicyInbox,
			Schedule:     automation.Schedule{Kind: automation.ScheduleDaily, Hour: now.Hour(), Minute: now.Minute()},
		}},
	}); err != nil {
		t.Fatalf("CreateAutomation schedule failed: %v", err)
	}
	batchTask, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Batch review",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:               "chapter_batch_1",
			Type:             automation.TriggerTypeChapterBatch,
			Enabled:          true,
			NotifyPolicy:     automation.NotifyPolicyInbox,
			ChapterBatchSize: 1,
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation batch failed: %v", err)
	}

	items, err := app.automation().CheckContentTriggersForWorkspaceMutation(context.Background(), "test_mutation", []string{"setting/progress.md"})
	if err != nil {
		t.Fatalf("non-chapter mutation check failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("non-chapter mutation should not check triggers: %#v", items)
	}

	items, err = app.automation().CheckContentTriggersForWorkspaceMutation(context.Background(), "test_mutation", []string{"chapters/ch01.md"})
	if err != nil {
		t.Fatalf("chapter mutation check failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("content trigger item count = %d, want 1: %#v", len(items), items)
	}
	if items[0].TaskID != batchTask.ID || items[0].TriggerID != "chapter_batch_1" {
		t.Fatalf("unexpected content trigger item: %#v", items[0])
	}
	inbox, err := app.AutomationInbox()
	if err != nil {
		t.Fatalf("AutomationInbox failed: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("schedule trigger should not run from chapter mutation, inbox=%#v", inbox)
	}
}

func TestAutomationMutationCallbackDoesNotEvaluateBeforeDurableHostEffect(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestChapter(t, workspace, 1)
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
	registerAutomationProjectForTest(t, app, workspace)
	app.ensureServices()
	t.Cleanup(app.Close)
	app.bookService = book.NewService(workspace)

	task, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Agent batch review",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:               "chapter_batch_1",
			Type:             automation.TriggerTypeChapterBatch,
			Enabled:          true,
			NotifyPolicy:     automation.NotifyPolicyInbox,
			ChapterBatchSize: 1,
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}
	started := make(chan struct{}, 1)
	app.automationTriggers.processOverride = func(context.Context, *AutomationAppService, *automationWorkspaceSnapshot, string) error {
		started <- struct{}{}
		return nil
	}

	callback := app.automationMutationCallback("agent_test")
	callback(context.Background(), []agents.ToolMutation{{
		ToolName: "write",
		Target:   filepath.Join(workspace, "chapters", "ch01.md"),
	}}, agents.PostRunVerification{Status: "ok", Mutations: 1})

	select {
	case <-started:
		t.Fatal("pre-output OnMutationsVerified callback evaluated Automation")
	case <-time.After(50 * time.Millisecond):
	}
	inbox, err := app.AutomationInbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 0 {
		t.Fatalf("pre-output callback created inbox items for task %s: %#v", task.ID, inbox)
	}
}

func TestUserScheduleLastRunOnlySuppressesItsOwnWorkspace(t *testing.T) {
	root := t.TempDir()
	workspaceA := filepath.Join(root, "a")
	workspaceB := filepath.Join(root, "b")
	for _, workspace := range []string{workspaceA, workspaceB} {
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	task := automation.Task{
		ID:    "shared-user-task",
		Scope: automation.ScopeUser,
		LastRun: &automation.RunRecord{
			Workspace: workspaceA,
			StartedAt: now.Add(-10 * time.Minute),
		},
	}
	trigger := automation.TriggerDefinition{
		ID:   "hourly",
		Type: automation.TriggerTypeSchedule,
		Schedule: automation.Schedule{
			Kind:       automation.ScheduleEveryHours,
			EveryHours: 1,
		},
	}
	serviceA := automationRegistryTestService(&App{})
	snapA := automationRegistryTestSnapshot(workspaceA)
	if _, _, matched, err := serviceA.evaluateScheduleTrigger(snapA, now, task, trigger, automation.TriggerState{}); err != nil || matched {
		t.Fatalf("workspace A schedule matched=%v err=%v, want suppressed by its recent run", matched, err)
	}
	serviceB := automationRegistryTestService(&App{})
	snapB := automationRegistryTestSnapshot(workspaceB)
	if _, _, matched, err := serviceB.evaluateScheduleTrigger(snapB, now, task, trigger, automation.TriggerState{}); err != nil || !matched {
		t.Fatalf("workspace B schedule matched=%v err=%v, want independent first run", matched, err)
	}
}

func TestAutomationMutationChecksCoalesceRapidSavesWithoutDuplicateInbox(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestChapter(t, workspace, 1)
	application := &App{
		cfg:         &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace},
		workspace:   workspace,
		bookService: book.NewService(workspace),
	}
	registerAutomationProjectForTest(t, application, workspace)
	application.ensureServices()
	defer application.Close()
	task, err := application.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Rapid-save review",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:               "chapter_batch_1",
			Type:             automation.TriggerTypeChapterBatch,
			Enabled:          true,
			NotifyPolicy:     automation.NotifyPolicyInbox,
			ChapterBatchSize: 1,
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}
	for i := 0; i < 32; i++ {
		application.CheckAutomationTriggersAfterWorkspaceMutation(context.Background(), "rapid_save", []string{"chapters/ch01.md"})
	}
	items := waitForAutomationInbox(t, application, 1)
	if items[0].TaskID != task.ID {
		t.Fatalf("unexpected inbox item: %#v", items[0])
	}
}

func TestUserAutomationTriggerStateAndInboxAreWorkspaceScoped(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaces := []string{filepath.Join(root, "one"), filepath.Join(root, "two")}
	for _, workspace := range workspaces {
		if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestChapter(t, workspace, 1)
	}
	application := &App{cfg: &config.Config{NovaDir: userDir}}
	application.ensureServices()
	defer application.Close()
	service := &AutomationAppService{app: application}
	snapshots := make([]*automationWorkspaceSnapshot, 0, len(workspaces))
	for _, workspace := range workspaces {
		snapshots = append(snapshots, &automationWorkspaceSnapshot{
			workspace:   workspace,
			novaDir:     userDir,
			cfg:         config.Config{NovaDir: userDir, Workspace: workspace},
			bookService: book.NewService(workspace),
		})
	}
	task, err := service.Create(automation.Task{
		Scope:      automation.ScopeUser,
		Enabled:    true,
		Name:       "Shared user review",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:               "chapter_batch_1",
			Type:             automation.TriggerTypeChapterBatch,
			ActionPolicy:     automation.ActionPolicyNotifyOnly,
			Enabled:          true,
			NotifyPolicy:     automation.NotifyPolicyInbox,
			ChapterBatchSize: 1,
		}},
	})
	if err != nil {
		t.Fatalf("create user automation: %v", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(snapshots))
	for _, snap := range snapshots {
		wg.Add(1)
		go func(snap *automationWorkspaceSnapshot) {
			defer wg.Done()
			var processErr error
			defer func() {
				if recovered := recover(); recovered != nil {
					processErr = fmt.Errorf("workspace trigger check panic: %v", recovered)
				}
				errs <- processErr
			}()
			_, _, processErr = service.processContentTriggers(context.Background(), snap, time.Now().UTC(), "test")
		}(snap)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("workspace trigger check failed: %v", err)
		}
	}
	fingerprints := map[string]bool{}
	for index, snap := range snapshots {
		items, err := storeForSnapshot(snap).ListInbox()
		if err != nil {
			t.Fatalf("workspace %d Inbox failed: %v", index, err)
		}
		if len(items) != 1 || items[0].TaskID != task.ID || items[0].Workspace != workspaces[index] {
			t.Fatalf("workspace %d inbox = %#v", index, items)
		}
		fingerprints[items[0].Fingerprint] = true
	}
	if len(fingerprints) != len(workspaces) {
		t.Fatalf("user-scope fingerprints collided across workspaces: %#v", fingerprints)
	}
	saved, err := automation.NewStore(userDir, "").Get(task.ID)
	if err != nil {
		t.Fatalf("load shared user task: %v", err)
	}
	if len(saved.TriggerState) != len(workspaces) {
		t.Fatalf("trigger state count = %d, want %d: %#v", len(saved.TriggerState), len(workspaces), saved.TriggerState)
	}
}

func TestAutomationWriteModeToolConstraints(t *testing.T) {
	readOnly := constrainAutomationTools(config.Config{AgentTools: config.AgentToolSettings{Automation: config.AgentToolOverride{
		config.AgentToolShell: true, config.AgentToolBrowser: true,
	}}}, automation.WriteModeReadOnly, automation.WriteScopeNone)
	readOnlyTools := config.ResolveAgentTools(&readOnly, config.AgentKindAutomation)
	if readOnlyTools.Allows(config.AgentToolWorkspaceWrite) || readOnlyTools.Allows(config.AgentToolLoreWrite) {
		t.Fatalf("read_only should disable writes: %#v", readOnlyTools)
	}
	for _, capability := range []string{
		config.AgentToolShell, config.AgentToolBrowser, config.AgentToolWebSearch,
		config.AgentToolWebFetch, config.AgentToolDelegation,
	} {
		if !readOnlyTools.Allows(capability) {
			t.Fatalf("read_only should preserve %s capability: %#v", capability, readOnlyTools)
		}
	}
	if path, err := (&AutomationAppService{}).writeOptionalOutput(nil, automation.Task{
		OutputPolicy: automation.OutputPolicyOptionalFile, OutputPath: "reports/daily.md",
	}, "summary", config.Config{}, automation.WriteModeReadOnly, automation.WriteScopeNone); err != nil || path != "" {
		t.Fatalf("read_only automatic file output path=%q error=%v", path, err)
	}

	fileOnly := constrainAutomationTools(config.Config{}, automation.WriteModeAutoWrite, automation.WriteScopeFile)
	fileOnlyTools := config.ResolveAgentTools(&fileOnly, config.AgentKindAutomation)
	if !fileOnlyTools.Allows(config.AgentToolWorkspaceWrite) || fileOnlyTools.Allows(config.AgentToolLoreWrite) {
		t.Fatalf("file scope tools = %#v, want file write only", fileOnlyTools)
	}

	loreAndFile := constrainAutomationTools(config.Config{}, automation.WriteModeAutoWrite, automation.WriteScopeLoreAndFile)
	loreAndFileTools := config.ResolveAgentTools(&loreAndFile, config.AgentKindAutomation)
	if !loreAndFileTools.Allows(config.AgentToolWorkspaceWrite) || !loreAndFileTools.Allows(config.AgentToolLoreWrite) {
		t.Fatalf("lore_and_file tools = %#v, want both write tools", loreAndFileTools)
	}

	global := constrainGlobalAutomationTools(config.Config{})
	globalTools := config.ResolveAgentTools(&global, config.AgentKindAutomation)
	if globalTools.Allows(config.AgentToolWorkspaceRead) || globalTools.Allows(config.AgentToolWorkspaceWrite) || globalTools.Allows(config.AgentToolShell) || globalTools.Allows(config.AgentToolLoreRead) || globalTools.Allows(config.AgentToolLoreWrite) {
		t.Fatalf("global automation exposed workspace tools: %#v", globalTools)
	}
	if !globalTools.Allows(config.AgentToolSkills) || !globalTools.Allows(config.AgentToolTodo) || !globalTools.Allows(config.AgentToolWebSearch) {
		t.Fatalf("global automation omitted user-level tools: %#v", globalTools)
	}

	firstRun := automation.RunRecord{Trigger: automation.TriggerCondition}
	mode, scope := effectiveAutomationWriteModeScope(automation.Task{WriteMode: automation.WriteModeConfirmWrite, WriteScope: automation.WriteScopeFile}, firstRun)
	if mode != automation.WriteModeReadOnly || scope != automation.WriteScopeNone {
		t.Fatalf("confirm_write first run = %s/%s, want read_only/none", mode, scope)
	}
	confirmedRun := automation.RunRecord{Trigger: automation.TriggerWriteConfirmation}
	mode, scope = effectiveAutomationWriteModeScope(automation.Task{WriteMode: automation.WriteModeConfirmWrite, WriteScope: automation.WriteScopeFile}, confirmedRun)
	if mode != automation.WriteModeAutoWrite || scope != automation.WriteScopeFile {
		t.Fatalf("confirm_write write run = %s/%s, want auto_write/file", mode, scope)
	}
}

func TestAutomationRuntimeConfigUsesTaskModelProfile(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	app := &App{
		cfg: &config.Config{
			NovaDir:     filepath.Join(root, "nova"),
			Workspace:   workspace,
			OpenAIModel: "base-model",
			ModelProfiles: []config.ModelProfileSettings{{
				ID:          "fast",
				Name:        "Fast",
				OpenAIModel: "fast-model",
			}},
		},
		workspace: workspace,
	}
	app.ensureServices()
	snap := app.automationSnapshot()
	if snap == nil {
		t.Fatal("automation snapshot is nil")
	}

	cfg := runtimeConfigForTask(snap, automation.Task{ModelProfileID: "fast"})
	resolved := config.ResolveAgentModel(&cfg, config.AgentKindAutomation)
	if resolved.ProfileID != "fast" || resolved.OpenAIModel != "fast-model" {
		t.Fatalf("resolved model = %#v, want fast profile", resolved)
	}

	cfg = runtimeConfigForTask(snap, automation.Task{})
	resolved = config.ResolveAgentModel(&cfg, config.AgentKindAutomation)
	if resolved.ProfileID != "default" || resolved.OpenAIModel != "base-model" {
		t.Fatalf("resolved inherited model = %#v, want default base model", resolved)
	}

	cfg = runtimeConfigForTask(snap, automation.Task{Template: automation.TemplateReview})
	if cfg.MaxIteration != 0 {
		t.Fatalf("review max iteration should stay unlimited by default, got %d", cfg.MaxIteration)
	}
	maxIteration := 20
	if err := config.WriteSettingsFile(config.UserConfigPath(app.cfg.NovaDir), config.Settings{MaxIteration: &maxIteration}); err != nil {
		t.Fatal(err)
	}
	snap = app.automationSnapshot()
	if snap == nil {
		t.Fatal("automation snapshot is nil after settings write")
	}
	cfg = runtimeConfigForTask(snap, automation.Task{Template: automation.TemplateReview})
	if cfg.MaxIteration != maxIteration {
		t.Fatalf("review max iteration = %d, want explicit configured value %d", cfg.MaxIteration, maxIteration)
	}
}

func TestAutomationReviewMessageTargetsTriggeredChapters(t *testing.T) {
	service := &AutomationAppService{}
	task := automation.Task{
		Name:         "自动 Review",
		Template:     automation.TemplateReview,
		Prompt:       automation.DefaultReviewPrompt,
		WriteMode:    automation.WriteModeReadOnly,
		WriteScope:   automation.WriteScopeNone,
		OutputPolicy: automation.OutputPolicyRunRecordOnly,
	}
	run := automation.RunRecord{
		Trigger: automation.TriggerCondition,
		TriggerEvidence: []automation.TriggerEvidence{{
			Source:  "chapter_batch",
			Title:   "第 5 章",
			Ref:     "chapters/ch05.md",
			Snippet: "batch=1 words=3200 updated=2026-06-15T20:00:00Z",
		}},
	}

	message := service.buildAutomationUserMessage(task, run, automation.WriteModeReadOnly, automation.WriteScopeNone)
	for _, want := range []string{
		"本次触发范围",
		"chapters/ch05.md",
		"对本次触发范围中的新增章节做自动 Review",
		"只评审这些新增章节",
		"不要把全书当作被评审正文",
		"CREATOR.md",
		"长期大纲",
		"角色设定与状态",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("automation review message missing %q:\n%s", want, message)
		}
	}
}

func TestAutomationMessageDoesNotFallbackToTemplatePrompt(t *testing.T) {
	service := &AutomationAppService{}
	task := automation.Task{
		Name:         "Empty prompt review",
		Template:     automation.TemplateReview,
		WriteMode:    automation.WriteModeReadOnly,
		WriteScope:   automation.WriteScopeNone,
		OutputPolicy: automation.OutputPolicyRunRecordOnly,
	}
	message := service.buildAutomationUserMessage(task, automation.RunRecord{Trigger: automation.TriggerManual}, automation.WriteModeReadOnly, automation.WriteScopeNone)
	if strings.Contains(message, "对本次触发范围中的新增章节做自动 Review") {
		t.Fatalf("empty task prompt should not fallback to template-specific prompt:\n%s", message)
	}
	if !strings.Contains(message, automation.GenericTaskPrompt) {
		t.Fatalf("empty task prompt should use generic fallback:\n%s", message)
	}
}

func TestGlobalAutomationMessageDoesNotRequestWorkspaceContext(t *testing.T) {
	service := &AutomationAppService{}
	task := automation.Task{
		Name:         "Global research",
		Target:       automation.ExecutionTarget{Kind: automation.TargetKindUser},
		Template:     automation.TemplateCustomPrompt,
		Prompt:       "检索公开资料并整理摘要",
		WriteMode:    automation.WriteModeReadOnly,
		WriteScope:   automation.WriteScopeNone,
		OutputPolicy: automation.OutputPolicyRunRecordOnly,
	}
	message := service.buildAutomationUserMessage(task, automation.RunRecord{Trigger: automation.TriggerManual}, automation.WriteModeReadOnly, automation.WriteScopeNone)
	if strings.Contains(message, "读取完成任务所需的工作区文件") {
		t.Fatalf("global task message requested workspace files:\n%s", message)
	}
	if !strings.Contains(message, "全局任务") || !strings.Contains(message, "用户级") {
		t.Fatalf("global task message did not state its execution boundary:\n%s", message)
	}
}

func writeTestChapter(t *testing.T, workspace string, index int) {
	t.Helper()
	path := filepath.Join(workspace, "chapters", fmt.Sprintf("ch%02d.md", index))
	content := fmt.Sprintf("# Chapter %d\n\nThis chapter has enough text to count as written.\n", index)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write chapter %d: %v", index, err)
	}
}

func waitForAutomationInbox(t *testing.T, app *App, want int) []automation.TriggerInboxItem {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var inbox []automation.TriggerInboxItem
	for time.Now().Before(deadline) {
		var err error
		inbox, err = app.AutomationInbox()
		if err != nil {
			t.Fatalf("AutomationInbox failed: %v", err)
		}
		if len(inbox) == want {
			return inbox
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("automation inbox count = %d, want %d: %#v", len(inbox), want, inbox)
	return nil
}

func itemsFromFirstBatch(t *testing.T, app *App, taskID string) string {
	t.Helper()
	inbox, err := app.AutomationInbox()
	if err != nil {
		t.Fatalf("AutomationInbox failed: %v", err)
	}
	for _, item := range inbox {
		if item.TaskID == taskID && item.TriggerID == "chapter_batch_5" {
			return item.ID
		}
	}
	t.Fatalf("first batch inbox item not found: %#v", inbox)
	return ""
}
