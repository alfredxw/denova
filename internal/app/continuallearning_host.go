package app

import (
	"context"
	"log/slog"

	chatagent "denova/internal/agents/chat"
	"denova/internal/agents/trajectory"
	agentchatapp "denova/internal/app/agentchat"
	continuallearningapp "denova/internal/app/continuallearning"
	apptask "denova/internal/app/task"
	projectdomain "denova/internal/project"
)

type continualLearningHost struct{ app *App }

func (host continualLearningHost) Runtime() continuallearningapp.Runtime {
	if host.app == nil {
		return continuallearningapp.Runtime{}
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	runtime := continuallearningapp.Runtime{}
	if host.app.cfg != nil {
		runtime.Config = *host.app.cfg
	}
	return runtime
}

func (host continualLearningHost) TrajectorySources(ctx context.Context) ([]trajectory.Source, error) {
	if host.app == nil {
		return nil, ErrNoWorkspace
	}
	sources, issues, err := host.app.globalTrajectorySources(ctx)
	for _, issue := range issues {
		slog.WarnContext(ctx, "[trajectory] skip unavailable Project source", "project_id", issue.ProjectID, "error", issue.Message)
	}
	return sources, err
}

func (host continualLearningHost) StartHarnessTurn(ctx context.Context, request continuallearningapp.HarnessTurnRequest) (*apptask.Task, error) {
	if host.app == nil {
		return nil, ErrNoWorkspace
	}
	turn, err := host.app.AgentChat().AcceptTurn(ctx, agentchatapp.TurnRequest{
		Binding: agentchatapp.Binding{ProjectID: projectdomain.HarnessProjectID, SessionID: request.SessionID},
		ChatRequest: chatagent.CaptureChatRequestCallerInput(chatagent.ChatRequest{
			CommandID: request.CommandID,
			Message:   request.Message,
			Locale:    request.Locale,
		}),
		Policy: agentchatapp.TurnPolicy{
			Origin:       "harness_schedule",
			OriginID:     request.CommandID,
			SessionTitle: "Scheduled Harness maintenance / 定时 Harness 维护",
			BusyPolicy:   agentchatapp.TurnBusyReject,
		},
	})
	if err != nil {
		return nil, err
	}
	if !turn.Replayed() {
		if err := turn.Start(); err != nil {
			return nil, err
		}
	}
	return turn.Task(), nil
}
