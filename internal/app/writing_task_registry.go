package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// writingTaskRun binds the reconnectable display task to the exact immutable
// runtime snapshot admitted for its root operation. Typed commands never
// reconstruct a binding from whichever session happens to be selected later.
type writingTaskRun struct {
	task               *apptask.Task
	runtime            ideChatRuntime
	recovery           *agentharness.RecoveryObservation
	recoveryActions    map[string]agentrun.CommandReceipt
	recoveryStructural bool

	recoveryMutationMu sync.Mutex
	recoveryMutations  []writingRecoveryMutationBatch

	// A recovered structural commit can settle before the long-lived selected
	// Session successfully reloads canonical state. Keep its display Task alive
	// until the exact recovery POST closes this latch.
	recoveryRefreshReady chan struct{}
	recoveryRefreshOnce  sync.Once
}

func (run *writingTaskRun) resolveRecoveryRefresh() {
	if run == nil || run.recoveryRefreshReady == nil {
		return
	}
	run.recoveryRefreshOnce.Do(func() { close(run.recoveryRefreshReady) })
}

func (run *writingTaskRun) waitForRecoveryRefresh(ctx context.Context) bool {
	if run == nil || run.recoveryRefreshReady == nil {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-run.recoveryRefreshReady:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *ChatAppService) replayDurableWritingStart(
	ctx context.Context,
	req agentchat.ChatRequest,
	workspace string,
	sessionID string,
	fingerprint string,
) (*apptask.Task, bool, error) {
	a := s.app
	a.mu.RLock()
	chatService := a.chatService
	sess := a.session
	bookService := a.bookService
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	a.mu.RUnlock()
	if chatService == nil || sess == nil || sess.ID != sessionID {
		return nil, false, nil
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, SessionID: sessionID,
		StateRoot: stateRoot, Workspace: workspace, Mode: "ide",
	}
	status, err := chatService.RuntimeStatusProjection(ctx, options)
	if err != nil {
		return nil, false, err
	}
	if !agentStatusOwnsCommand(status, req.CommandID) {
		return nil, false, nil
	}

	runtime := ideChatRuntime{
		app: a, sess: sess, bookService: bookService,
		chatService: chatService, workspace: workspace, projectState: stateRoot,
	}
	var accepted *agentharness.AcceptedRun
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != workspace || a.session != sess {
			return ErrAgentContextChanged
		}
		if a.activeTask != nil && !a.activeTask.Finished() {
			return ErrAgentOperationActive
		}
		if err := a.registerWorkspaceTaskLocked(task, workspace, true); err != nil {
			return err
		}
		a.activeTask = task
		a.activeWritingRun = &writingTaskRun{task: task, runtime: runtime}
		return nil
	})
	if err != nil {
		return nil, true, err
	}
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	accepted, err = chatService.StartWithOptions(acceptCtx, nil, nil, bookService, req, options, task.Emit)
	releaseAcceptance()
	if err != nil {
		rollbackWritingReplayTask(a, task, err)
		if errors.Is(err, agentrun.ErrInvalidCommand) {
			return nil, true, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, req.CommandID)
		}
		return nil, true, err
	}
	if !accepted.Receipt().Replayed {
		err := fmt.Errorf("durable Writing replay unexpectedly accepted a new command")
		task.Abort()
		_ = accepted.Wait(task.Context())
		rollbackWritingReplayTask(a, task, err)
		return nil, true, err
	}
	if err := task.Start(func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		outcome := accepted.Wait(ctx)
		slog.InfoContext(ctx, fmt.Sprintf("[agent-task] replay end id=%s command_id=%s status=%s", task.ID(), req.CommandID, outcome.Status))
	}); err != nil {
		task.Abort()
		_ = accepted.Wait(task.Context())
		rollbackWritingReplayTask(a, task, err)
		return nil, true, err
	}
	if err := s.starts.Remember(agentStartRecord(
		req.CommandID, workspace, sessionID, fingerprint, task,
	)); err != nil {
		return nil, true, err
	}
	return task, true, nil
}

func agentStatusOwnsCommand(status agentrun.RuntimeStatus, commandID string) bool {
	return appagentruntime.StatusOwnsCommand(status, commandID)
}

func rollbackWritingReplayTask(a *App, task *apptask.Task, err error) {
	task.RejectStart(err)
	a.unregisterWorkspaceTask(task)
	a.mu.Lock()
	if a.activeTask == task {
		a.activeTask = nil
	}
	if a.activeWritingRun != nil && a.activeWritingRun.task == task {
		a.activeWritingRun = nil
	}
	a.mu.Unlock()
}

func (run *writingTaskRun) matchesCurrent(a *App) bool {
	if run == nil || run.task == nil || a == nil || a.session == nil {
		return false
	}
	return run.runtime.workspace == a.workspace && run.runtime.sess == a.session
}

var (
	// ErrAgentCommandIDRequired is returned before any display task, model, or
	// canonical side effect is allocated for a root Agent request without caller identity.
	ErrAgentCommandIDRequired = apptask.ErrCommandIDRequired
	// ErrAgentCommandConflict means one caller identity was reused for a
	// different payload or lifecycle binding.
	ErrAgentCommandConflict = apptask.ErrCommandConflict
	// ErrAgentReplayCapacity means every bounded display replay slot is owned by
	// live work. Admission fails before the durable Runtime command is submitted.
	ErrAgentReplayCapacity = apptask.ErrReplayCapacity
)

func agentStartIdentity(commandID, scope, sessionID, fingerprint string) apptask.StartIdentity {
	return apptask.StartIdentity{
		CommandID: commandID, Scope: scope, SessionID: sessionID, Fingerprint: fingerprint,
	}
}

func agentStartRecord(commandID, scope, sessionID, fingerprint string, task *apptask.Task) apptask.StartRecord {
	return apptask.StartRecord{
		Identity: agentStartIdentity(commandID, scope, sessionID, fingerprint),
		Task:     task,
	}
}
