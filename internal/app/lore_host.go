package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agentmodeltask "denova/internal/agents/modeltask"
	"denova/internal/app/task"
	booklore "denova/internal/book/lore"
)

// loreHost is the stable-Project boundary for lore operations. Foreground
// navigation is intentionally absent: callers supply Project identity and the
// host derives its content directory and durable state layout.
type loreHost struct {
	app *App
}

func (host loreHost) WithLoreStore(ctx context.Context, projectID string, action func(*booklore.Store) error) (string, error) {
	if action == nil {
		return "", errors.New("lore action is nil")
	}
	if host.app == nil {
		return "", ErrNoWorkspace
	}
	operation, err := host.app.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return "", err
	}
	defer operation.Release()
	workspace := operation.Layout().ContentRoot
	if err := action(booklore.NewStore(workspace)); err != nil {
		return "", err
	}
	return workspace, nil
}

func (host loreHost) RegisterLoreTask(backgroundTask *task.Task, projectID string) (string, error) {
	if host.app == nil {
		return "", ErrNoWorkspace
	}
	projectID = strings.TrimSpace(projectID)
	_, layout, err := host.app.resolveProject(projectID, true)
	if err != nil {
		return "", err
	}
	if err := host.app.registerProjectTask(backgroundTask, projectID, layout.ContentRoot, layout.StateRoot); err != nil {
		return "", err
	}
	return layout.ContentRoot, nil
}

func (host loreHost) UnregisterLoreTask(backgroundTask *task.Task) {
	if host.app != nil {
		host.app.unregisterProjectTask(backgroundTask)
	}
}

func (host loreHost) ClassifyLoreItems(ctx context.Context, projectID string, inputs []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error) {
	if host.app == nil {
		return nil, ErrNoWorkspace
	}
	operation, err := host.app.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	runtime, err := host.app.AgentChat().ProjectRuntime(operation.Context(), projectID)
	if err != nil {
		return nil, err
	}
	runtimeConfig := runtime.Conversation.Config
	result, err := agentmodeltask.ClassifyLoreItems(operation.Context(), &runtimeConfig, inputs)
	inputJSON, _ := json.Marshal(inputs)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] lore semantic classification failed project_id=%s items=%d err=%v", projectID, len(inputs), err))
		_ = persistAgentCallInStore(runtime.SessionStore, config.AgentKindToolAgent, string(inputJSON), "Execution failed: "+err.Error())
		return nil, err
	}
	outputJSON, _ := json.Marshal(result)
	if persistErr := persistAgentCallInStore(runtime.SessionStore, config.AgentKindToolAgent, string(inputJSON), string(outputJSON)); persistErr != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] persist lore classification call failed project_id=%s err=%v", projectID, persistErr))
	}
	return result, nil
}
