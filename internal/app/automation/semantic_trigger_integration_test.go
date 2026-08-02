package automationapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/automation"
	"denova/internal/book"
	workspacechange "denova/internal/workspace/change"
)

func TestAutomationMutationEvaluatorIgnoresRequestCancelAndAppCloseDrains(t *testing.T) {
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
	if _, err := application.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Lifecycle semantic review",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:                "semantic_batch_1",
			Type:              automation.TriggerTypeSemantic,
			Enabled:           true,
			NotifyPolicy:      automation.NotifyPolicyInbox,
			SemanticCondition: "chapter is ready",
			ChapterBatchSize:  1,
		}},
	}); err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}

	started := make(chan struct{})
	canceled := make(chan struct{})
	automationService := application.automation()
	previousEvaluator := automationService.semanticEvaluator
	automationService.semanticEvaluator = func(ctx context.Context, _ *config.Config, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return "", ctx.Err()
	}
	defer func() { automationService.semanticEvaluator = previousEvaluator }()
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	application.CheckAutomationTriggersAfterWorkspaceMutation(requestCtx, "canceled_request", []string{"chapters/ch01.md"})
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("request cancellation incorrectly prevented mutation evaluation")
	}
	closed := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				closed <- fmt.Errorf("App.Close panic: %v", recovered)
			}
		}()
		application.Close()
		closed <- nil
	}()
	select {
	case <-canceled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("App.Close did not cancel the mutation evaluator")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("App.Close did not drain the mutation evaluator")
	}
}

func TestWorkspaceChangeMutationAutomationUsesCapturedWorkspaceAfterSwitch(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	nextWorkspace := filepath.Join(root, "next-workspace")
	for _, target := range []string{workspace, nextWorkspace} {
		if err := os.MkdirAll(filepath.Join(target, "chapters"), 0o755); err != nil {
			t.Fatalf("create chapter directory: %v", err)
		}
	}
	writeTestChapter(t, workspace, 1)
	novaDir := filepath.Join(root, "nova")
	app := &App{
		cfg:         &config.Config{NovaDir: novaDir, Workspace: workspace},
		workspace:   workspace,
		bookService: book.NewService(workspace),
	}
	oldStore := registerAutomationProjectForTest(t, app, workspace)
	if _, err := app.projectRegistry.EnsureBook(nextWorkspace); err != nil {
		t.Fatal(err)
	}
	nextStore := projectAutomationStoreForTest(t, novaDir, app.projectRegistry, nextWorkspace)
	app.ensureServices()
	t.Cleanup(app.Close)

	if _, err := app.CreateAutomation(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Captured semantic review",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:                "semantic_batch_1",
			Type:              automation.TriggerTypeSemantic,
			Enabled:           true,
			NotifyPolicy:      automation.NotifyPolicyInbox,
			SemanticCondition: "chapter is ready",
			ChapterBatchSize:  1,
		}},
	}); err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}

	evaluationStarted := make(chan string, 1)
	releaseEvaluation := make(chan struct{})
	automationService := app.automation()
	previousEvaluator := automationService.semanticEvaluator
	automationService.semanticEvaluator = func(ctx context.Context, cfg *config.Config, _ string) (string, error) {
		evaluationStarted <- cfg.Workspace
		select {
		case <-releaseEvaluation:
			return `{"matched":true,"confidence":0.9,"reason":"ready","title":"Ready","evidence_refs":["chapters/ch01.md"]}`, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	defer func() { automationService.semanticEvaluator = previousEvaluator }()

	if _, err := app.WithWorkspaceChangeMutation(
		context.Background(),
		workspace,
		func(*workspacechange.Service) (WorkspaceChangeMutationHooks, error) {
			return WorkspaceChangeMutationHooks{
				AutomationSource: "editor_save",
				Paths:            []string{"chapters/ch01.md"},
			}, nil
		},
	); err != nil {
		t.Fatalf("WithWorkspaceChangeMutation failed: %v", err)
	}

	select {
	case evaluatedWorkspace := <-evaluationStarted:
		if evaluatedWorkspace != workspace {
			t.Fatalf("evaluator workspace=%q want captured %q", evaluatedWorkspace, workspace)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("automation evaluation did not start")
	}

	app.mu.Lock()
	app.workspace = nextWorkspace
	app.cfg.Workspace = nextWorkspace
	app.bookService = book.NewService(nextWorkspace)
	app.mu.Unlock()
	close(releaseEvaluation)

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		inbox, err := oldStore.ListInbox()
		if err != nil {
			t.Fatalf("list captured workspace inbox: %v", err)
		}
		if len(inbox) == 1 {
			if canonicalAutomationWorkspace(inbox[0].Workspace) != canonicalAutomationWorkspace(workspace) {
				t.Fatalf("inbox workspace=%q want=%q", inbox[0].Workspace, workspace)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("captured workspace inbox was not created: %#v", inbox)
		}
		time.Sleep(5 * time.Millisecond)
	}
	newInbox, err := nextStore.ListInbox()
	if err != nil {
		t.Fatalf("list next workspace inbox: %v", err)
	}
	if len(newInbox) != 0 {
		t.Fatalf("next workspace received stale automation trigger: %#v", newInbox)
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
	registerAutomationProjectForTest(t, app, workspace)
	app.ensureServices()
	app.bookService = book.NewService(workspace)

	var calls int
	var lastInstruction string
	automationService := app.automation()
	previousEvaluator := automationService.semanticEvaluator
	automationService.semanticEvaluator = func(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
		calls++
		lastInstruction = instruction
		return `{"matched":true,"confidence":0.9,"reason":"new semantic state","title":"Semantic hit","evidence_refs":["chapters/ch03.md"]}`, nil
	}
	defer func() { automationService.semanticEvaluator = previousEvaluator }()

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
