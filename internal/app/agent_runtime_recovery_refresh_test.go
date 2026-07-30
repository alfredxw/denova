package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
)

func TestRecoveredStructuralSessionRefreshRemainsRetryableAfterTerminal(t *testing.T) {
	action := agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: "compact-refresh",
		OperationID: "operation-compact-refresh",
	}
	service := &ChatAppService{}
	service.markRecoveryRefreshPending("/book", "session-1", action)

	canonicalGeneration := 0
	refreshCalls := 0
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	refresh := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("refresh inherited display cancellation: %v", err)
		}
		refreshCalls++
		if refreshCalls == 1 {
			return errors.New("temporary journal read failure")
		}
		canonicalGeneration = 2
		return nil
	}

	if matched, err := service.retryRecoveryRefresh(canceled, "/book", "session-1", action, refresh); !matched || err == nil {
		t.Fatalf("first refresh = matched=%t err=%v", matched, err)
	}
	if matched, err := service.retryRecoveryRefresh(context.Background(), "/book", "session-1", agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryRemoveCompaction, CommandID: action.CommandID, OperationID: action.OperationID,
	}, refresh); matched || err != nil {
		t.Fatalf("different recovery action consumed obligation: matched=%t err=%v", matched, err)
	}
	if matched, err := service.retryRecoveryRefresh(context.Background(), "/book", "session-1", action, refresh); !matched || err != nil {
		t.Fatalf("same action retry = matched=%t err=%v", matched, err)
	}
	if canonicalGeneration != 2 {
		t.Fatalf("next writing turn would observe stale generation %d", canonicalGeneration)
	}
	if matched, err := service.retryRecoveryRefresh(context.Background(), "/book", "session-1", action, refresh); matched || err != nil {
		t.Fatalf("completed obligation reran: matched=%t err=%v", matched, err)
	}
	if refreshCalls != 2 {
		t.Fatalf("refresh calls = %d, want 2", refreshCalls)
	}
	service.markRecoveryRefreshPending("/book", "session-1", action)
	if retried, err := service.retryAnyRecoveryRefresh(context.Background(), "/book", "session-1", refresh); !retried || err != nil {
		t.Fatalf("new StartTurn admission refresh = retried=%t err=%v", retried, err)
	}
	if refreshCalls != 3 {
		t.Fatalf("next StartTurn did not close refresh obligation: calls=%d", refreshCalls)
	}

	// The production path returns the exact durable structural receipt after
	// the retry rather than resubmitting a terminal runtime command.
	run := &writingTaskRun{recoveryActions: map[string]agents.CommandReceipt{
		recoveryActionKey(action): {CommandID: action.CommandID, OperationID: action.OperationID, Replayed: true},
	}}
	receipt := run.recoveryActions[recoveryActionKey(action)]
	if receipt.CommandID != action.CommandID || receipt.OperationID != action.OperationID || !receipt.Replayed {
		t.Fatalf("retry lost durable structural receipt: %#v", receipt)
	}
}

func TestRecoveredStructuralRefreshKeepsOneObservableTaskUntilExactRetry(t *testing.T) {
	root := t.TempDir()
	workspace := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: root, Workspace: workspace, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	service := application.chat()
	application.mu.RLock()
	selected := application.session
	chat := application.chatService
	bookService := application.bookService
	selectedWorkspace := application.workspace
	application.mu.RUnlock()
	options := agents.RunOptions{
		AgentKind: agents.AgentKindIDE, Workspace: selectedWorkspace,
		SessionID: selected.ID, Mode: "ide",
	}

	// Give RecoveryObservation a real durable terminal to replay after the
	// application-level projection refresh latch is resolved.
	seeded, err := chat.StartWithOptions(
		context.Background(),
		newInteractiveReplayRunner(t, &interactiveReplayModel{message: agents.AssistantMessage("seeded terminal", nil)}),
		&interactiveReplayConversation{}, bookService,
		agents.ChatRequest{CommandID: "refresh-protocol-seed", Message: "seed runtime terminal"},
		options, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := seeded.Wait(context.Background()); outcome.Status != agents.RunOutcomeCompleted {
		t.Fatalf("seeded outcome = %#v", outcome)
	}
	recovery, err := chat.OpenRecoveryObservation(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	initial := recovery.InitialStatus()
	if initial.Phase != agents.RunPhaseIdle || initial.LastOperation == nil {
		recovery.Close()
		t.Fatalf("seeded runtime status = %#v", initial)
	}
	action := agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: initial.LastOperation.CommandID,
		OperationID: initial.LastOperation.OperationID,
	}

	// Append through another Session instance, as a cold structural restorer
	// does. The selected in-memory Session intentionally remains stale.
	layout, err := application.projectLayoutForWorkspace(selectedWorkspace)
	if err != nil {
		recovery.Close()
		t.Fatal(err)
	}
	externalStore, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		recovery.Close()
		t.Fatal(err)
	}
	external, err := externalStore.Get(selected.ID)
	if err != nil {
		recovery.Close()
		t.Fatal(err)
	}
	const canonicalMessage = "canonical state committed by recovered structural operation"
	if err := external.Append(agents.UserMessage(canonicalMessage)); err != nil {
		recovery.Close()
		t.Fatal(err)
	}
	if historyContainsContent(selected.History(), canonicalMessage) {
		recovery.Close()
		t.Fatal("selected Session unexpectedly refreshed before recovery retry")
	}

	controlEmitted := make(chan struct{})
	var run *writingTaskRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		application.mu.Lock()
		defer application.mu.Unlock()
		run = &writingTaskRun{
			task: task,
			runtime: ideChatRuntime{
				app: application, sess: selected, bookService: bookService,
				chatService: chat, workspace: selectedWorkspace,
			},
			recovery:             recovery,
			recoveryActions:      map[string]agents.CommandReceipt{recoveryActionKey(action): {CommandID: action.CommandID, OperationID: action.OperationID, Cursor: initial.Cursor, Replayed: true}},
			recoveryRefreshReady: make(chan struct{}),
		}
		application.activeTask = task
		application.activeWritingRun = run
		return nil
	})
	if err != nil {
		recovery.Close()
		t.Fatal(err)
	}
	service.markRecoveryRefreshPending(selectedWorkspace, selected.ID, action)
	if err := task.Start(func(taskCtx context.Context, _ *Task, emit func(agents.Event)) {
		defer recovery.Close()
		emitWritingRecoveryRefreshRequired(emit, action, initial.Cursor)
		close(controlEmitted)
		if !run.waitForRecoveryRefresh(taskCtx) {
			return
		}
		recovery.Wait(taskCtx, emit)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-controlEmitted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("refresh failure did not keep an observable recovery Task")
	}

	view := application.WritingAgentActiveView(context.Background())
	if view.Task == nil || view.Task.ID != task.ID() || view.Task.Finished ||
		!view.RuntimeProjectionOK || !view.Runtime.RecoveryPaused || len(view.RecoveryActions) != 1 || view.RecoveryActions[0] != action {
		t.Fatalf("pending refresh active projection = %#v", view)
	}
	if _, startErr := service.StartTaskWithError(context.Background(), agents.ChatRequest{
		CommandID: "start-must-not-cross-refresh", Message: "must remain fenced",
	}); !errors.Is(startErr, ErrAgentOperationActive) {
		t.Fatalf("Start crossed live refresh obligation: %v", startErr)
	}
	if task.Finished() {
		t.Fatal("refresh failure terminalized the display Task")
	}

	retried, err := application.RecoverWritingAgent(context.Background(), AgentRuntimeRecoveryRequest{Action: action})
	if err != nil {
		t.Fatal(err)
	}
	if retried.Task != task || !retried.Receipt.Replayed {
		t.Fatalf("exact refresh retry replaced Task or receipt: %#v", retried)
	}
	select {
	case <-task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("exact refresh retry did not terminalize the original Task")
	}
	if task.Status() != TaskDone {
		t.Fatalf("refreshed Task status = %s", task.Status())
	}
	events, subscription := task.Subscribe()
	task.Unsubscribe(subscription)
	if countInteractiveTaskEvents(events, agents.RuntimeRecoveryRequiredEventType) != 1 || countInteractiveTaskEvents(events, "done") != 1 {
		t.Fatalf("refresh retry Task events = %#v", events)
	}
	if !historyContainsContent(selected.History(), canonicalMessage) {
		t.Fatalf("exact retry left selected Session stale: %#v", selected.History())
	}
	if _, pending := service.pendingRecoveryRefreshAction(selectedWorkspace, selected.ID); pending {
		t.Fatal("successful exact retry left refresh action projected")
	}
	settled := application.WritingAgentActiveView(context.Background())
	if settled.Task == nil || !settled.Task.Finished || settled.Task.Status != TaskDone || settled.Runtime.RecoveryPaused || len(settled.RecoveryActions) != 0 {
		t.Fatalf("settled refresh projection = %#v", settled)
	}

	// A fresh Start is admitted only after canonical rehydrate. It may later
	// fail against the deliberately fake model, but it cannot remain blocked by
	// the resolved refresh fence.
	newTask, startErr := service.StartTaskWithError(context.Background(), agents.ChatRequest{
		CommandID: "start-after-refresh", Message: "canonical session is now safe",
	})
	if errors.Is(startErr, ErrAgentOperationActive) || errors.Is(startErr, agents.ErrRecoveryRequired) {
		t.Fatalf("resolved refresh still blocked Start: %v", startErr)
	}
	if startErr == nil && newTask != nil {
		newTask.Abort()
		select {
		case <-newTask.Done():
		case <-time.After(500 * time.Millisecond):
			t.Fatal("cleanup Start did not stop")
		}
	}
}

func historyContainsContent(history []session.HistoryEntry, content string) bool {
	for _, entry := range history {
		if entry.Content == content {
			return true
		}
	}
	return false
}

func TestWritingStartRejectsRecoveryTaskBeforeRefreshObligationIsMarked(t *testing.T) {
	application := &App{workspace: filepath.Clean(t.TempDir())}
	selectedStore, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectedStore.Create("selected")
	if err != nil {
		t.Fatal(err)
	}
	application.session = selected
	service := &ChatAppService{app: application}
	application.chatApp = service
	started := make(chan struct{})
	task := NewTask(func(ctx context.Context, _ *Task, _ func(agents.Event)) {
		close(started)
		<-ctx.Done()
	})
	<-started
	application.activeTask = task
	application.activeWritingRun = &writingTaskRun{
		task: task, runtime: ideChatRuntime{app: application, sess: selected, workspace: application.workspace},
	}

	if _, startErr := service.StartTaskWithError(context.Background(), agents.ChatRequest{
		CommandID: "start-racing-refresh-mark", Message: "must not cross",
	}); !errors.Is(startErr, ErrAgentOperationActive) {
		t.Fatalf("Start entered refresh-before-mark window: %v", startErr)
	}
	service.markRecoveryRefreshPending(application.workspace, selected.ID, agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: "compact-racing-start", OperationID: "operation-racing-start",
	})
	task.Abort()
	<-task.Done()
	if retried, retryErr := service.retryAnyRecoveryRefresh(context.Background(), application.workspace, selected.ID, selected.RefreshCanonical); !retried || retryErr != nil {
		t.Fatalf("cleanup refresh obligation = retried=%t err=%v", retried, retryErr)
	}
}

func TestManualStructuralCommandsCannotRaceActiveStructuralRecoveryTask(t *testing.T) {
	workspace := filepath.Clean(t.TempDir())
	selectedStore, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectedStore.Create("selected")
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: workspace, session: selected}
	service := &ChatAppService{app: application}
	application.chatApp = service
	started := make(chan struct{})
	task := NewTask(func(ctx context.Context, _ *Task, _ func(agents.Event)) {
		close(started)
		<-ctx.Done()
	})
	<-started
	application.activeTask = task
	application.activeWritingRun = &writingTaskRun{
		task: task,
		runtime: ideChatRuntime{
			app: application, sess: selected, workspace: workspace,
		},
		recoveryStructural: true,
	}
	exact := agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: "compact-exact-owner",
		OperationID: "operation-exact-owner",
	}
	service.markRecoveryRefreshPending(workspace, selected.ID, exact)

	if _, compactErr := service.executeWritingContextCompaction(context.Background(), "manual-compact-race"); !errors.Is(compactErr, ErrAgentOperationActive) {
		t.Fatalf("manual compact raced structural recovery: %v", compactErr)
	}
	if _, removeErr := service.executeWritingContextCompactionRemoval(context.Background(), "manual-remove-race"); !errors.Is(removeErr, ErrAgentOperationActive) {
		t.Fatalf("manual remove raced structural recovery: %v", removeErr)
	}
	// Even a defensive generic mark from a direct path cannot erase the exact
	// identity that keeps the recovery Task and projection retryable.
	service.markRecoveryRefreshPending(workspace, selected.ID, agents.RuntimeRecoveryAction{Kind: agents.RuntimeRecoveryCompactContext})
	if projected, ok := service.pendingRecoveryRefreshAction(workspace, selected.ID); !ok || projected != exact {
		t.Fatalf("manual structural path replaced exact refresh action: %#v ok=%t", projected, ok)
	}
	if task.Finished() {
		t.Fatal("manual structural command drained the exact recovery Task")
	}

	task.Abort()
	<-task.Done()
	if retried, retryErr := service.retryAnyRecoveryRefresh(context.Background(), workspace, selected.ID, selected.RefreshCanonical); !retried || retryErr != nil {
		t.Fatalf("cleanup refresh obligation = retried=%t err=%v", retried, retryErr)
	}
}

func TestFreshWorkspaceGenerationClearsPriorRefreshObligation(t *testing.T) {
	root := t.TempDir()
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: root, Workspace: workspaceA, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	service := application.chat()
	application.mu.RLock()
	canonicalA := application.workspace
	sessionA := application.session.ID
	application.mu.RUnlock()
	action := agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: "stale-generation-compact",
		OperationID: "stale-generation-operation",
	}
	service.markRecoveryRefreshPending(canonicalA, sessionA, action)
	if projected, ok := service.pendingRecoveryRefreshAction(canonicalA, sessionA); !ok || projected != action {
		t.Fatalf("seeded refresh obligation = %#v ok=%t", projected, ok)
	}

	if _, err := application.SwitchWorkspace(context.Background(), workspaceB); err != nil {
		t.Fatal(err)
	}
	if _, err := application.SwitchWorkspace(context.Background(), workspaceA); err != nil {
		t.Fatal(err)
	}
	application.mu.RLock()
	reloadedWorkspace := application.workspace
	reloadedSession := application.session
	application.mu.RUnlock()
	if reloadedWorkspace != canonicalA || reloadedSession == nil || reloadedSession.ID != sessionA {
		t.Fatalf("reloaded binding = workspace=%q session=%#v", reloadedWorkspace, reloadedSession)
	}
	if projected, ok := service.pendingRecoveryRefreshAction(canonicalA, sessionA); ok {
		t.Fatalf("fresh workspace generation retained stale refresh action %#v", projected)
	}
	view := application.WritingAgentActiveView(context.Background())
	if !view.RuntimeProjectionOK || view.Runtime.RecoveryPaused || len(view.RecoveryActions) != 0 {
		t.Fatalf("fresh workspace projection retained stale recovery: %#v", view)
	}
}

func TestFinishedRecoveryTaskCannotBypassPendingSessionRefresh(t *testing.T) {
	root := t.TempDir()
	workspace := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: root, Workspace: workspace, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	service := application.chat()
	application.mu.RLock()
	selectedWorkspace := application.workspace
	selected := application.session
	application.mu.RUnlock()
	started := make(chan struct{})
	task := NewTask(func(ctx context.Context, _ *Task, _ func(agents.Event)) {
		close(started)
		<-ctx.Done()
	})
	<-started
	task.Abort()
	<-task.Done()
	application.mu.Lock()
	application.activeTask = task
	application.activeWritingRun = &writingTaskRun{
		task: task, runtime: ideChatRuntime{app: application, sess: selected, workspace: selectedWorkspace},
	}
	application.mu.Unlock()
	service.markRecoveryRefreshPending(selectedWorkspace, selected.ID, agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: "finished-task-refresh",
		OperationID: "finished-task-operation",
	})

	layout, err := application.projectLayoutForWorkspace(selectedWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	selectedPath := filepath.Join(layout.SessionsDir(), selected.ID+".jsonl")
	unavailablePath := selectedPath + ".temporarily-unavailable"
	if err := os.Rename(selectedPath, unavailablePath); err != nil {
		t.Fatal(err)
	}
	if _, startErr := service.StartTaskWithError(context.Background(), agents.ChatRequest{
		CommandID: "start-after-canceled-refresh", Message: "must remain fenced",
	}); startErr == nil || errors.Is(startErr, ErrAgentOperationActive) {
		t.Fatalf("finished recovery Task bypassed pending refresh: %v", startErr)
	}
	if _, pending := service.pendingRecoveryRefreshAction(selectedWorkspace, selected.ID); !pending {
		t.Fatal("failed Start consumed pending refresh obligation")
	}
	if err := os.Rename(unavailablePath, selectedPath); err != nil {
		t.Fatal(err)
	}
	newTask, startErr := service.StartTaskWithError(context.Background(), agents.ChatRequest{
		CommandID: "start-after-canceled-refresh-retry", Message: "refresh is available",
	})
	if errors.Is(startErr, ErrAgentOperationActive) || errors.Is(startErr, agents.ErrRecoveryRequired) {
		t.Fatalf("successful admission refresh remained fenced: %v", startErr)
	}
	if _, pending := service.pendingRecoveryRefreshAction(selectedWorkspace, selected.ID); pending {
		t.Fatal("successful Start admission left pending refresh obligation")
	}
	if startErr == nil && newTask != nil {
		newTask.Abort()
		select {
		case <-newTask.Done():
		case <-time.After(500 * time.Millisecond):
			t.Fatal("cleanup Start did not stop")
		}
	}
}
