package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/automation"
	"nova/internal/book"
)

func TestAutomationCheckCreatesRetryableInboxWhenAutoRunCannotStart(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
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
	if items[0].Status != automation.InboxStatusPending || items[0].ActionPolicy != automation.ActionPolicyConfirm {
		t.Fatalf("unexpected inbox item: %#v", items[0])
	}
	if !strings.Contains(items[0].Summary, "自动执行启动失败") {
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
	app.ensureServices()
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
	if _, err := app.automation().store().DismissInboxItem(itemsFromFirstBatch(t, app, task.ID)); err != nil {
		t.Fatalf("dismiss first batch inbox: %v", err)
	}
	savedTask, err := app.automation().store().Get(task.ID)
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

func TestAutomationMutationCallbackChecksAgentChapterWrites(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestChapter(t, workspace, 1)
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
	app.ensureServices()
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

	callback := app.automationMutationCallback("agent_test")
	callback(context.Background(), []agent.ToolMutation{{
		ToolName: "write_file",
		Target:   filepath.Join(workspace, "chapters", "ch01.md"),
	}}, agent.PostRunVerification{Status: "ok", Mutations: 1})

	inbox := waitForAutomationInbox(t, app, 1)
	if inbox[0].TaskID != task.ID || inbox[0].TriggerID != "chapter_batch_1" {
		t.Fatalf("unexpected inbox after agent mutation callback: %#v", inbox)
	}
}

func TestAutomationSemanticTriggerChecksOnlyCompletedChapterBatches(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		writeTestChapter(t, workspace, i)
	}
	app := &App{cfg: &config.Config{NovaDir: filepath.Join(root, "nova"), Workspace: workspace}, workspace: workspace}
	app.ensureServices()
	app.bookService = book.NewService(workspace)

	var calls int
	var lastInstruction string
	previousEvaluator := semanticTriggerEvaluator
	semanticTriggerEvaluator = func(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
		calls++
		lastInstruction = instruction
		return `{"matched":true,"confidence":0.9,"reason":"new semantic state","title":"Semantic hit","evidence_refs":["chapters/ch03.md"]}`, nil
	}
	defer func() { semanticTriggerEvaluator = previousEvaluator }()

	task, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Semantic batch",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:                "semantic_batch_3",
			Type:              automation.TriggerTypeSemantic,
			Enabled:           true,
			NotifyPolicy:      automation.NotifyPolicyInbox,
			SemanticCondition: "新角色登场",
			ChapterBatchSize:  3,
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}

	items, err := app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers before semantic batch failed: %v", err)
	}
	if len(items) != 0 || calls != 0 {
		t.Fatalf("semantic trigger should not evaluate before batch boundary items=%#v calls=%d", items, calls)
	}

	writeTestChapter(t, workspace, 3)
	items, err = app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers at semantic batch failed: %v", err)
	}
	if len(items) != 1 || calls != 1 {
		t.Fatalf("semantic batch item count/calls = %d/%d, want 1/1", len(items), calls)
	}
	if len(items[0].Evidence) != 3 || items[0].Evidence[0].Ref != "chapters/ch01.md" || items[0].Evidence[2].Ref != "chapters/ch03.md" {
		t.Fatalf("semantic evidence should be scoped to first batch: %#v", items[0].Evidence)
	}
	if !strings.Contains(lastInstruction, "chapters/ch03.md") || !strings.Contains(lastInstruction, "content_excerpt=") {
		t.Fatalf("semantic instruction should include bounded chapter batch content:\n%s", lastInstruction)
	}

	items, err = app.CheckAutomationTriggers(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CheckAutomationTriggers duplicate semantic batch failed: %v", err)
	}
	if len(items) != 0 || calls != 1 {
		t.Fatalf("same semantic batch should not re-evaluate items=%#v calls=%d", items, calls)
	}
}

func TestAutomationWriteModeToolConstraints(t *testing.T) {
	readOnly := constrainAutomationTools(config.Config{}, automation.WriteModeReadOnly, automation.WriteScopeNone)
	readOnlyTools := config.ResolveAgentTools(&readOnly, config.AgentKindAutomation)
	if readOnlyTools.FileWrite || readOnlyTools.LoreWrite {
		t.Fatalf("read_only should disable writes: %#v", readOnlyTools)
	}

	fileOnly := constrainAutomationTools(config.Config{}, automation.WriteModeAutoWrite, automation.WriteScopeFile)
	fileOnlyTools := config.ResolveAgentTools(&fileOnly, config.AgentKindAutomation)
	if !fileOnlyTools.FileWrite || fileOnlyTools.LoreWrite {
		t.Fatalf("file scope tools = %#v, want file write only", fileOnlyTools)
	}

	loreAndFile := constrainAutomationTools(config.Config{}, automation.WriteModeAutoWrite, automation.WriteScopeLoreAndFile)
	loreAndFileTools := config.ResolveAgentTools(&loreAndFile, config.AgentKindAutomation)
	if !loreAndFileTools.FileWrite || !loreAndFileTools.LoreWrite {
		t.Fatalf("lore_and_file tools = %#v, want both write tools", loreAndFileTools)
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

	cfg := app.automation().runtimeConfigForTask(automation.Task{ModelProfileID: "fast"})
	resolved := config.ResolveAgentModel(&cfg, config.AgentKindAutomation)
	if resolved.ProfileID != "fast" || resolved.OpenAIModel != "fast-model" {
		t.Fatalf("resolved model = %#v, want fast profile", resolved)
	}

	cfg = app.automation().runtimeConfigForTask(automation.Task{})
	resolved = config.ResolveAgentModel(&cfg, config.AgentKindAutomation)
	if resolved.ProfileID != "default" || resolved.OpenAIModel != "base-model" {
		t.Fatalf("resolved inherited model = %#v, want default base model", resolved)
	}

	cfg = app.automation().runtimeConfigForTask(automation.Task{Template: automation.TemplateReview})
	if cfg.MaxIteration != 0 {
		t.Fatalf("review max iteration should stay unlimited by default, got %d", cfg.MaxIteration)
	}
	maxIteration := 20
	if err := config.WriteSettingsFile(config.UserConfigPath(app.cfg.NovaDir), config.Settings{MaxIteration: &maxIteration}); err != nil {
		t.Fatal(err)
	}
	cfg = app.automation().runtimeConfigForTask(automation.Task{Template: automation.TemplateReview})
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
