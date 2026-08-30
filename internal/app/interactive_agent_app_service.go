package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"fmt"
	"log/slog"
	"strings"

	agentcompaction "denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive"
)

// StartInteractiveTask 启动游戏模式 Agent 任务，输出写回 interactive/story。
func (a *App) StartInteractiveTask(ctx context.Context, request InteractiveAgentStartRequest) *apptask.Task {
	task, _ := a.StartInteractiveTaskWithError(ctx, request)
	return task
}

func (s *InteractiveAppService) StartInteractiveTask(ctx context.Context, request InteractiveAgentStartRequest) *apptask.Task {
	task, _ := s.StartInteractiveTaskWithError(ctx, request)
	return task
}

func (a *App) StartInteractiveTaskWithError(ctx context.Context, request InteractiveAgentStartRequest) (*apptask.Task, error) {
	return a.interactiveService().StartInteractiveTaskWithError(ctx, request)
}

func (s *InteractiveAppService) StartInteractiveTaskWithError(ctx context.Context, request InteractiveAgentStartRequest) (*apptask.Task, error) {
	return s.startInteractiveTask(ctx, request)
}

func (a *App) AnalyzeInteractiveContext(storyID, branchID, message string, styleScenes []string, locale string) (agentchat.ContextAnalysis, error) {
	return a.interactiveService().AnalyzeInteractiveContext(storyID, branchID, message, styleScenes, locale)
}

func (s *InteractiveAppService) AnalyzeInteractiveContext(storyID, branchID, message string, styleScenes []string, locale string) (agentchat.ContextAnalysis, error) {
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	operation, err := a.acquireWorkspaceOperation(context.Background(), workspace, true)
	if err != nil {
		return agentchat.ContextAnalysis{}, err
	}
	defer operation.Release()
	ctx := operation.Context()
	cycle, err := s.prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
		StoryID: storyID, BranchID: branchID, Message: message,
		StyleScenes: append([]string(nil), styleScenes...), Locale: locale,
	})
	if err != nil {
		return agentchat.ContextAnalysis{}, err
	}
	inspection, err := cycle.executionRuntime.Inspect(ctx, agentexecution.Cycle{
		Definition: cycle.definition, Conversation: cycle.conversation,
		BookService: cycle.bookService, Request: cycle.request, Options: cycle.options(""),
	})
	if err != nil {
		return agentchat.ContextAnalysis{}, err
	}
	return agentchat.BuildInteractiveInspectedContextAnalysis(
		&cycle.runtimeCfg, cycle.systemPrompt, inspection,
	), nil
}

func (a *App) CompactInteractiveContext(ctx context.Context, storyID, branchID string) (agentcompaction.Result, error) {
	return a.interactiveService().CompactInteractiveContext(ctx, storyID, branchID)
}

func (s *InteractiveAppService) CompactInteractiveContext(ctx context.Context, storyID, branchID string) (agentcompaction.Result, error) {
	return s.executeInteractiveContextCompaction(ctx, storyID, branchID, "")
}

func (a *App) CompactInteractiveContextCommand(ctx context.Context, storyID, branchID, commandID string) (agentcompaction.Result, error) {
	return a.interactiveService().executeInteractiveContextCompaction(ctx, storyID, branchID, commandID)
}

func (a *App) RemoveInteractiveContextCompaction(storyID, branchID string) (bool, error) {
	return a.interactiveService().RemoveInteractiveContextCompaction(storyID, branchID)
}

func (s *InteractiveAppService) RemoveInteractiveContextCompaction(storyID, branchID string) (bool, error) {
	return s.executeInteractiveContextCompactionRemoval(context.Background(), storyID, branchID, "")
}

func (a *App) RemoveInteractiveContextCompactionCommand(ctx context.Context, storyID, branchID, commandID string) (bool, error) {
	return a.interactiveService().executeInteractiveContextCompactionRemoval(ctx, storyID, branchID, commandID)
}

func (s *InteractiveAppService) startInteractiveTask(ctx context.Context, request InteractiveAgentStartRequest) (*apptask.Task, error) {
	// Root admission is deliberately exclusive. It closes the gap between the
	// process-local replay lookup and durable StartTurn acceptance, so concurrent
	// retries cannot allocate two display tasks or prepare two model cycles.
	s.admission.Lock()
	defer s.admission.Unlock()
	identity, err := s.resolveInteractiveStart(request)
	if err != nil {
		return nil, err
	}
	if replay, ok, err := s.starts.replay(identity); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}

	a := s.app
	a.mu.RLock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil || a.executionRuntime == nil {
		a.mu.RUnlock()
		slog.InfoContext(ctx, "[interactive-agent-task] Cannot start task without a selected workspace")
		return nil, ErrNoWorkspace
	}
	workspace := a.workspace
	a.mu.RUnlock()
	if workspace != identity.workspace {
		return nil, ErrAgentContextChanged
	}
	operation, err := a.acquireWorkspaceOperation(ctx, identity.workspace, true)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	ctx = operation.Context()
	a.mu.RLock()
	transitioning := a.workspaceTransition
	contextChanged := a.workspace != identity.workspace
	operationActive := a.activeInteractiveRun != nil && a.activeInteractiveRun.task != nil && !a.activeInteractiveRun.task.Finished()
	a.mu.RUnlock()
	if transitioning {
		return nil, ErrWorkspaceTransition
	}
	if contextChanged {
		return nil, ErrAgentContextChanged
	}
	if operationActive {
		return nil, ErrAgentOperationActive
	}

	cycle, err := s.prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
		CommandID: identity.request.CommandID,
		StoryID:   identity.request.StoryID, BranchID: identity.request.BranchID, Message: identity.request.Message,
		ResumeInterruptionID: identity.request.ResumeInterruptionID,
		StyleScenes:          identity.request.StyleScenes, Locale: identity.request.Locale,
		RegenerateFromTurnID: identity.request.RegenerateFromTurnID,
		AttachmentIDs:        identity.request.AttachmentIDs, AttachedFiles: identity.request.AttachedFiles,
	})
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-agent-task] prepare cycle failed command_id=%s story_id=%s branch_id=%s err=%v", identity.request.CommandID, identity.request.StoryID, identity.request.BranchID, err))
		return nil, err
	}
	if cycle.workspace != identity.workspace || cycle.storyID != identity.request.StoryID || cycle.branchID != identity.request.BranchID {
		return nil, ErrAgentContextChanged
	}
	// Preserve the caller snapshot captured before mutable style/default
	// resolution, while retaining the first accepted cycle's server-owned rules.
	resolvedRequest := cycle.request
	cycle.request = identity.chatRequest
	cycle.request.StyleRules = resolvedRequest.StyleRules
	var accepted *agentexecution.Operation
	runAccepted := func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent-task] run begin id=%s command_id=%s story_id=%s branch_id=%s rewind_turn_id=%s message_len=%d style_scenes=%d", task.ID(), identity.request.CommandID, cycle.storyID, cycle.branchID, identity.request.RegenerateFromTurnID, len(identity.request.Message), len(identity.request.StyleScenes)))
		outcome := accepted.Wait(ctx)
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent-task] run end id=%s command_id=%s outcome=%s status=%s", task.ID(), identity.request.CommandID, outcome.Status, task.Status()))
	}
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != cycle.workspace || a.interactive != cycle.store || a.bookState != cycle.state || a.executionRuntime != cycle.executionRuntime {
			return ErrAgentContextChanged
		}
		if a.activeInteractiveRun != nil && a.activeInteractiveRun.task != nil && !a.activeInteractiveRun.task.Finished() {
			return ErrAgentOperationActive
		}
		if err := a.registerWorkspaceTaskLocked(task, cycle.workspace, true); err != nil {
			return err
		}
		a.activeInteractiveRun = &interactiveTaskRun{task: task, info: identity.taskInfo(task.ID())}
		return nil
	})
	if err != nil {
		return nil, err
	}
	cycle.bindCommit(task.Emit)
	options := cycle.options(task.ID())
	options.TurnID = identity.request.RegenerateFromTurnID
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	accepted, err = cycle.executionRuntime.Start(acceptCtx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			Definition:   cycle.definition,
			Conversation: cycle.conversation,
			BookService:  cycle.bookService,
			Request:      cycle.request,
			Options:      options,
		},
		Emit: task.Emit,
	})
	releaseAcceptance()
	if err != nil {
		task.RejectStart(err)
		a.unregisterWorkspaceTask(task)
		a.mu.Lock()
		if a.activeInteractiveRun != nil && a.activeInteractiveRun.task == task {
			a.activeInteractiveRun = nil
		}
		a.mu.Unlock()
		return nil, err
	}
	if err := task.Start(runAccepted); err != nil {
		task.Abort()
		_ = accepted.Wait(task.Context())
		task.Finish()
		a.unregisterWorkspaceTask(task)
		a.mu.Lock()
		if a.activeInteractiveRun != nil && a.activeInteractiveRun.task == task {
			a.activeInteractiveRun = nil
		}
		a.mu.Unlock()
		return nil, err
	}
	if err := s.starts.remember(identity, task); err != nil {
		// Durable acceptance already succeeded. Keep the original task alive and
		// surface the registry invariant rather than starting a second cycle.
		return nil, err
	}
	return task, nil
}

func emitInteractiveTurnPersisted(store *interactive.Store, storyID string, conversation *interactiveapp.Conversation, emit func(agentrun.Event)) *interactive.Snapshot {
	snapshot, err := emitInteractiveTurnPersistedResult(store, storyID, conversation, emit)
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent-task] emit persisted turn failed story_id=%s err=%v", storyID, err))
	}
	return snapshot
}

func emitInteractiveTurnPersistedResult(store *interactive.Store, storyID string, conversation *interactiveapp.Conversation, emit func(agentrun.Event)) (*interactive.Snapshot, error) {
	if store == nil || conversation == nil || emit == nil {
		return nil, fmt.Errorf("interactive turn persistence projection is unavailable")
	}
	turn, _, ok := conversation.LastTurnForState()
	if !ok || strings.TrimSpace(turn.ID) == "" {
		return nil, fmt.Errorf("interactive turn was not persisted")
	}
	snapshot, err := store.Snapshot(storyID, turn.BranchID)
	if err != nil {
		return nil, fmt.Errorf("load persisted interactive turn snapshot: %w", err)
	}
	persistedTurn := turn
	for _, snapshotTurn := range snapshot.Turns {
		if snapshotTurn.ID == turn.ID {
			persistedTurn = snapshotTurn
			break
		}
	}
	event := InteractiveTurnPersistedEvent{
		StoryID:           storyID,
		BranchID:          snapshot.BranchID,
		TurnCount:         snapshot.TurnCount,
		Turn:              persistedTurn,
		BranchPlan:        snapshot.BranchPlan,
		State:             snapshot.State,
		Graph:             snapshot.Graph,
		Branches:          snapshot.Graph.Branches,
		ContextCompaction: conversation.AgentCompactionProjection(snapshot),
	}
	event.Turn.Attachments = attachmentDescriptors(event.Turn.Attachments)
	emit(agentrun.Event{Type: "interactive_turn_persisted", Data: event})
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent-task] emitted persisted turn story_id=%s branch_id=%s turn_id=%s", storyID, snapshot.BranchID, persistedTurn.ID))
	return &snapshot, nil
}
