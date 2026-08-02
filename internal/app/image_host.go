package app

import (
	"context"
	"fmt"
	"strings"

	imageapp "denova/internal/app/image"
)

// imageHost is the narrow workspace-generation boundary used by the shared
// writing/game image service.
type imageHost struct {
	app *App
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
		Workspace: workspace, Config: *app.cfg, BookState: app.bookState,
		BookService: app.bookService, Interactive: app.interactive,
		SessionStore: app.sessionStore, ChatService: app.chatService,
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

func imageRuntimeMatches(app *App, runtime *imageapp.Runtime) bool {
	if app == nil || runtime == nil {
		return false
	}
	app.mu.RLock()
	defer app.mu.RUnlock()
	return lifecycleWorkspaceKey(app.workspace) == lifecycleWorkspaceKey(runtime.Workspace) &&
		app.bookState == runtime.BookState && app.bookService == runtime.BookService &&
		app.interactive == runtime.Interactive && app.sessionStore == runtime.SessionStore &&
		app.chatService == runtime.ChatService
}
