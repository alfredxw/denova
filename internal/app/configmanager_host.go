package app

import (
	"context"

	"denova/config"
	agenttool "denova/internal/agents/tool"
	appagentruntime "denova/internal/app/agentruntime"
	configmanagerapp "denova/internal/app/configmanager"
	apptask "denova/internal/app/task"
	"denova/internal/book"
)

type configManagerHost struct {
	app *App
}

func (host configManagerHost) ProjectRuntime(ctx context.Context, projectID string) (configmanagerapp.Runtime, error) {
	if host.app == nil {
		return configmanagerapp.Runtime{}, appagentruntime.ErrNoWorkspace
	}
	projectRuntime, err := host.app.AgentChat().ProjectRuntime(ctx, projectID)
	if err != nil {
		return configmanagerapp.Runtime{}, err
	}
	runtime := projectRuntime.Conversation
	host.app.mu.RLock()
	registry := host.app.projectRegistry
	host.app.mu.RUnlock()
	return configmanagerapp.Runtime{
		ProjectID: projectID, Config: runtime.Config, Workspace: runtime.Workspace, State: runtime.State,
		SessionStore: projectRuntime.SessionStore, BookService: runtime.BookService,
		VersionService: runtime.VersionService, ExecutionRuntime: runtime.ExecutionRuntime,
		ProjectRegistry: registry,
	}, nil
}

func (host configManagerHost) AcquireProjectOperation(ctx context.Context, projectID string) (configmanagerapp.Operation, error) {
	if host.app == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	return host.app.AcquireProjectOperation(ctx, projectID)
}

func (host configManagerHost) IsCurrent(expected configmanagerapp.Runtime) bool {
	if host.app == nil {
		return false
	}
	_, layout, err := host.app.resolveProject(expected.ProjectID, true)
	return err == nil && projectLayoutMatchesRuntime(
		layout, expected.ProjectID, expected.Workspace, expected.Config.ProjectStateDir,
	)
}

func (host configManagerHost) RegisterTask(task *apptask.Task, expected configmanagerapp.Runtime) error {
	if host.app == nil {
		return appagentruntime.ErrNoWorkspace
	}
	if !host.IsCurrent(expected) {
		return appagentruntime.ErrContextChanged
	}
	return host.app.registerProjectTask(
		task, expected.ProjectID, expected.Workspace, expected.Config.ProjectStateDir,
	)
}

func (host configManagerHost) UnregisterTask(task *apptask.Task) {
	if host.app != nil {
		host.app.unregisterProjectTask(task)
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
