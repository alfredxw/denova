package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"

	"denova/internal/book"
)

// replayDurableStart rebuilds only the reconnectable display Task. The
// durable Config Manager binding owns the result; no model, tool, Session
// conversation, or mutable request preparation is needed on this path.
func (s *ConfigManagerAppService) replayDurableStart(
	ctx context.Context,
	chatService *agentharness.Service,
	bookService *book.Service,
	chatReq agentchat.ChatRequest,
	workspace string,
	sessionID string,
	fingerprint string,
) (*apptask.Task, bool, error) {
	if chatService == nil {
		return nil, false, nil
	}
	stateRoot := ""
	if s != nil && s.app != nil {
		s.app.mu.RLock()
		if s.app.cfg != nil {
			stateRoot = s.app.cfg.ProjectStateDir
		}
		s.app.mu.RUnlock()
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindConfigManager, SessionID: sessionID,
		StateRoot: stateRoot, Workspace: workspace, Mode: "config_manager",
	}
	status, err := chatService.RuntimeStatusProjection(ctx, options)
	if err != nil {
		return nil, false, err
	}
	if !agentStatusOwnsCommand(status, chatReq.CommandID) {
		return nil, false, nil
	}

	a := s.app
	var accepted *agentharness.AcceptedRun
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != workspace || a.chatService != chatService {
			return ErrAgentContextChanged
		}
		return a.registerWorkspaceTaskLocked(task, workspace, true)
	})
	if err != nil {
		return nil, true, err
	}
	startReservation, err := s.starts.reserve(writingStartRecord{
		commandID: chatReq.CommandID, workspace: workspace, sessionID: sessionID,
		fingerprint: fingerprint, task: task,
	})
	if err != nil {
		rollbackConfigManagerReplayTask(a, task, err)
		return nil, true, err
	}
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	accepted, err = chatService.StartWithOptions(acceptCtx, nil, nil, bookService, chatReq, options, task.Emit)
	releaseAcceptance()
	if err != nil {
		startReservation.rollback()
		rollbackConfigManagerReplayTask(a, task, err)
		if errors.Is(err, agentrun.ErrInvalidCommand) {
			return nil, true, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, chatReq.CommandID)
		}
		return nil, true, err
	}
	if !accepted.Receipt().Replayed {
		err := fmt.Errorf("durable Config Manager replay unexpectedly accepted a new command")
		startReservation.rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
		rollbackConfigManagerReplayTask(a, task, err)
		return nil, true, err
	}
	if err := task.Start(func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		outcome := accepted.Wait(ctx)
		slog.InfoContext(ctx, fmt.Sprintf("[config-manager] replay end id=%s command_id=%s status=%s", task.ID(), chatReq.CommandID, outcome.Status))
	}); err != nil {
		startReservation.rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
		rollbackConfigManagerReplayTask(a, task, err)
		return nil, true, err
	}
	startReservation.commit()
	return task, true, nil
}

func rollbackConfigManagerReplayTask(a *App, task *apptask.Task, err error) {
	task.RejectStart(err)
	a.unregisterWorkspaceTask(task)
}
