package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
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
	task            *apptask.Task
	runtime         ideChatRuntime
	recovery        *agentexecution.RecoveryObservation
	recoveryActions map[string]agentrun.CommandReceipt

	recoveryMutationMu sync.Mutex
	recoveryMutations  []writingRecoveryMutationBatch
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
	executionRuntime := a.executionRuntime
	sess := a.session
	bookService := a.bookService
	stateRoot := ""
	projectID := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
		projectID = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	if executionRuntime == nil || sess == nil || sess.ID != sessionID {
		return nil, false, nil
	}
	runtime := ideChatRuntime{
		app: a, projectID: projectID, sess: sess, bookService: bookService,
		executionRuntime: executionRuntime, workspace: workspace, projectState: stateRoot,
	}
	options := runtime.agentOptions("")
	status, err := executionRuntime.RuntimeStatusProjection(ctx, options)
	if err != nil {
		return nil, false, err
	}
	if !agentStatusOwnsCommand(status, req.CommandID) {
		return nil, false, nil
	}

	var accepted *agentexecution.Operation
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
	accepted, err = executionRuntime.Start(acceptCtx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			BookService: bookService,
			Request:     req,
			Options:     options,
		},
		Emit: task.Emit,
	})
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
