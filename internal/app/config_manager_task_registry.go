package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	agents "denova/internal/agents"
	"denova/internal/book"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

// replayDurableStart rebuilds only the reconnectable display Task. The
// durable Config Manager binding owns the result; no model, tool, Session
// conversation, or mutable request preparation is needed on this path.
func (s *ConfigManagerAppService) replayDurableStart(
	ctx context.Context,
	chatService *agents.ChatService,
	bookService *book.Service,
	chatReq agents.ChatRequest,
	workspace string,
	sessionID string,
	fingerprint string,
) (*Task, bool, error) {
	if chatService == nil {
		return nil, false, nil
	}
	options := agents.RunOptions{
		AgentKind: agents.AgentKindConfigManager, SessionID: sessionID,
		Workspace: workspace, Mode: "config_manager",
	}
	status, err := chatService.RuntimeStatusProjection(ctx, options)
	if err != nil {
		return nil, false, err
	}
	if !agentStatusOwnsCommand(status, chatReq.CommandID) {
		return nil, false, nil
	}

	a := s.app
	var accepted *agents.AcceptedRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
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
	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	accepted, err = chatService.StartWithOptions(acceptCtx, nil, nil, bookService, chatReq, options, task.emit)
	releaseAcceptance()
	if err != nil {
		startReservation.rollback()
		rollbackConfigManagerReplayTask(a, task, err)
		if errors.Is(err, runstate.ErrInvalidCommand) {
			return nil, true, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, chatReq.CommandID)
		}
		return nil, true, err
	}
	if !accepted.Receipt().Replayed {
		err := fmt.Errorf("durable Config Manager replay unexpectedly accepted a new command")
		startReservation.rollback()
		task.Abort()
		_ = accepted.Wait(task.ctx)
		rollbackConfigManagerReplayTask(a, task, err)
		return nil, true, err
	}
	if err := task.Start(func(ctx context.Context, task *Task, _ func(agents.Event)) {
		defer a.unregisterWorkspaceTask(task)
		outcome := accepted.Wait(ctx)
		log.Printf("[config-manager] replay end id=%s command_id=%s status=%s", task.ID(), chatReq.CommandID, outcome.Status)
	}); err != nil {
		startReservation.rollback()
		task.Abort()
		_ = accepted.Wait(task.ctx)
		rollbackConfigManagerReplayTask(a, task, err)
		return nil, true, err
	}
	startReservation.commit()
	return task, true, nil
}

func rollbackConfigManagerReplayTask(a *App, task *Task, err error) {
	task.failBeforeStart(err)
	a.unregisterWorkspaceTask(task)
}
