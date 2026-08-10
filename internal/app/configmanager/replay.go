package configmanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	chatagent "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
)

// replayDurableStart rebuilds only the reconnectable display Task. Durable
// Runtime state owns the result; no model, tool, or mutable preparation runs.
func (service *Service) replayDurableStart(
	ctx context.Context,
	runtime Runtime,
	request chatagent.ChatRequest,
	sessionID string,
	fingerprint string,
) (*apptask.Task, bool, error) {
	if runtime.ExecutionRuntime == nil {
		return nil, false, nil
	}
	options := runOptions(runtime.ProjectID, runtime.Workspace, runtime.Config.ProjectStateDir, sessionID)
	status, err := runtime.ExecutionRuntime.RuntimeStatusProjection(ctx, options)
	if err != nil {
		return nil, false, err
	}
	if !appagentruntime.StatusOwnsCommand(status, request.CommandID) {
		return nil, false, nil
	}

	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		return service.host.RegisterTask(task, runtime)
	})
	if err != nil {
		return nil, true, err
	}
	identity := apptask.StartIdentity{
		CommandID: request.CommandID, Scope: runtime.ProjectID,
		SessionID: sessionID, Fingerprint: fingerprint,
	}
	reservation, err := service.starts.Reserve(apptask.StartRecord{Identity: identity, Task: task})
	if err != nil {
		service.rollbackReplayTask(task, err)
		return nil, true, err
	}
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	accepted, err := runtime.ExecutionRuntime.Start(acceptCtx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			BookService: runtime.BookService,
			Request:     request,
			Options:     options,
		},
		Emit: task.Emit,
	})
	releaseAcceptance()
	if err != nil {
		reservation.Rollback()
		service.rollbackReplayTask(task, err)
		if errors.Is(err, agentrun.ErrInvalidCommand) {
			return nil, true, fmt.Errorf("%w: command_id=%q", apptask.ErrCommandConflict, request.CommandID)
		}
		return nil, true, err
	}
	if !accepted.Receipt().Replayed {
		err := fmt.Errorf("durable Config Manager replay unexpectedly accepted a new command")
		reservation.Rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
		service.rollbackReplayTask(task, err)
		return nil, true, err
	}
	if err := task.Start(func(runCtx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer service.host.UnregisterTask(task)
		outcome := accepted.Wait(runCtx)
		slog.InfoContext(runCtx, fmt.Sprintf(
			"[app/configmanager] replay end task_id=%s command_id=%s status=%s",
			task.ID(), request.CommandID, outcome.Status,
		))
	}); err != nil {
		reservation.Rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
		service.rollbackReplayTask(task, err)
		return nil, true, err
	}
	reservation.Commit()
	return task, true, nil
}

func (service *Service) rollbackReplayTask(task *apptask.Task, err error) {
	if task == nil {
		return
	}
	task.RejectStart(err)
	if service != nil && service.host != nil {
		service.host.UnregisterTask(task)
	}
}
