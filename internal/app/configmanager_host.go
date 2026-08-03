package app

import (
	"context"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	appagentruntime "denova/internal/app/agentruntime"
	configmanagerapp "denova/internal/app/configmanager"
	apptask "denova/internal/app/task"
	"denova/internal/book"
)

type configManagerHost struct {
	app *App
}

func (host configManagerHost) ResolveAsk(
	ctx context.Context,
	target *session.Session,
	projectID, workspace, askID, status string,
	answers []agentconversation.HostAskAnswer,
	cancelReason string,
) (agentconversation.HostAskResolution, error) {
	if host.app == nil {
		return agentconversation.HostAskResolution{}, appagentruntime.ErrNoWorkspace
	}
	return host.app.resolveAgentAsk(ctx, target, projectID, workspace, askID, status, answers, cancelReason)
}

func (host configManagerHost) Snapshot() configmanagerapp.Runtime {
	if host.app == nil {
		return configmanagerapp.Runtime{}
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	var cfg config.Config
	if host.app.cfg != nil {
		cfg = *host.app.cfg
	}
	return configmanagerapp.Runtime{
		Config: cfg, Workspace: host.app.workspace, State: host.app.bookState,
		SessionStore: host.app.sessionStore, BookService: host.app.bookService,
		VersionService: host.app.versionService, ChatService: host.app.chatService,
		ProjectRegistry: host.app.projectRegistry,
	}
}

func (host configManagerHost) AcquireWorkspaceOperation(ctx context.Context, workspace string) (configmanagerapp.Operation, error) {
	if host.app == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	return host.app.acquireWorkspaceOperation(ctx, workspace, true)
}

func (host configManagerHost) IsCurrent(expected configmanagerapp.Runtime) bool {
	if host.app == nil {
		return false
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	return !host.app.workspaceTransition &&
		lifecycleWorkspaceKey(host.app.workspace) == lifecycleWorkspaceKey(expected.Workspace) &&
		host.app.bookState == expected.State && host.app.sessionStore == expected.SessionStore &&
		host.app.bookService == expected.BookService && host.app.versionService == expected.VersionService &&
		host.app.chatService == expected.ChatService
}

func (host configManagerHost) RegisterTask(task *apptask.Task, expected configmanagerapp.Runtime) error {
	if host.app == nil {
		return appagentruntime.ErrNoWorkspace
	}
	host.app.mu.Lock()
	defer host.app.mu.Unlock()
	if host.app.workspaceTransition || lifecycleWorkspaceKey(host.app.workspace) != lifecycleWorkspaceKey(expected.Workspace) ||
		host.app.bookState != expected.State || host.app.sessionStore != expected.SessionStore ||
		host.app.bookService != expected.BookService || host.app.versionService != expected.VersionService ||
		host.app.chatService != expected.ChatService {
		return appagentruntime.ErrContextChanged
	}
	return host.app.registerWorkspaceTaskLocked(task, expected.Workspace, true)
}

func (host configManagerHost) UnregisterTask(task *apptask.Task) {
	if host.app != nil {
		host.app.unregisterWorkspaceTask(task)
	}
}

func (host configManagerHost) OnVerifiedMutations(
	ctx context.Context,
	source string,
	versionService *book.VersionService,
	cfg config.Config,
	mutations []agenttool.Mutation,
	verification agenttool.Verification,
) {
	if host.app == nil {
		return
	}
	host.app.verifiedWorkspaceMutationCallback(
		source, versionService, versionAutoSettingsForConfig(&cfg),
	)(ctx, mutations, verification)
}
