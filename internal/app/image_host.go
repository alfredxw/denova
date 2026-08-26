package app

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
	imageapp "denova/internal/app/image"
)

// imageHost is the narrow workspace-generation boundary used by the shared
// writing/game image service.
type imageHost struct {
	app *App
}

func (host imageHost) ImageConfigSnapshot() config.Config {
	if host.app == nil {
		return config.Config{}
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	if host.app.cfg == nil {
		return config.Config{}
	}
	snapshot := *host.app.cfg
	snapshot.ImageAPIEndpoints = append([]config.ImageAPIEndpointSettings(nil), host.app.cfg.ImageAPIEndpoints...)
	snapshot.ImageAPIProfiles = append([]config.ImageAPIProfileSettings(nil), host.app.cfg.ImageAPIProfiles...)
	return snapshot
}

func (host imageHost) AcquireImageRuntime(ctx context.Context, expectedWorkspace string) (*imageapp.Runtime, error) {
	app := host.app
	if app == nil {
		return nil, ErrNoWorkspace
	}
	app.mu.RLock()
	workspace := strings.TrimSpace(app.workspace)
	if workspace == "" || app.bookService == nil || app.cfg == nil {
		app.mu.RUnlock()
		return nil, ErrNoWorkspace
	}
	if strings.TrimSpace(expectedWorkspace) != "" && lifecycleWorkspaceKey(expectedWorkspace) != lifecycleWorkspaceKey(workspace) {
		app.mu.RUnlock()
		return nil, fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, workspace)
	}
	runtime := &imageapp.Runtime{
		ProjectID: app.cfg.ProjectID, Workspace: workspace, Config: *app.cfg, BookState: app.bookState,
		BookService: app.bookService, Interactive: app.interactive,
		SessionStore: app.sessionStore, ExecutionRuntime: app.executionRuntime,
	}
	app.mu.RUnlock()

	operation, err := app.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	runtime.Operation = operation
	if !imageRuntimeMatches(app, runtime) {
		runtime.Release()
		return nil, ErrWorkspaceChanged
	}
	runtime.Config.Workspace = workspace
	return runtime, nil
}

// AcquireProjectImageRuntime resolves every adapter from stable Project
// identity while a Project-generation lease prevents relink/archive races.
func (host imageHost) AcquireProjectImageRuntime(ctx context.Context, projectID string) (*imageapp.Runtime, error) {
	app := host.app
	if app == nil {
		return nil, ErrNoWorkspace
	}
	operation, err := app.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	projectRuntime, err := app.AgentChat().ProjectRuntime(operation.Context(), projectID)
	if err != nil {
		operation.Release()
		return nil, err
	}
	conversation := projectRuntime.Conversation
	if conversation.State == nil || conversation.BookService == nil || conversation.ExecutionRuntime == nil {
		operation.Release()
		return nil, fmt.Errorf("Project %q is not a Book Project", projectID)
	}
	layout := operation.Layout()
	if strings.TrimSpace(conversation.ProjectID) != strings.TrimSpace(projectID) ||
		lifecycleWorkspaceKey(conversation.Workspace) != lifecycleWorkspaceKey(layout.ContentRoot) {
		operation.Release()
		return nil, ErrWorkspaceChanged
	}
	return &imageapp.Runtime{
		Operation: operation, ProjectID: conversation.ProjectID, Workspace: conversation.Workspace,
		Config: conversation.Config, BookState: conversation.State, BookService: conversation.BookService,
		SessionStore: projectRuntime.SessionStore, ExecutionRuntime: conversation.ExecutionRuntime,
	}, nil
}

func imageRuntimeMatches(app *App, runtime *imageapp.Runtime) bool {
	if app == nil || runtime == nil {
		return false
	}
	app.mu.RLock()
	defer app.mu.RUnlock()
	return lifecycleWorkspaceKey(app.workspace) == lifecycleWorkspaceKey(runtime.Workspace) &&
		app.bookState == runtime.BookState && app.bookService == runtime.BookService &&
		app.interactive == runtime.Interactive && app.sessionStore == runtime.SessionStore &&
		app.executionRuntime == runtime.ExecutionRuntime
}
