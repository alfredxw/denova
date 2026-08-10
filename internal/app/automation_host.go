package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agentchatapp "denova/internal/app/agentchat"
	automationapp "denova/internal/app/automation"
	appTask "denova/internal/app/task"
	"denova/internal/automation"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

// automationHost is composition glue: it exposes only immutable runtime
// snapshots and lifecycle admission to the automation module.
type automationHost struct {
	app *App
}

func (host automationHost) CurrentWorkspace() string {
	if host.app == nil {
		return ""
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	return host.app.workspace
}

func (host automationHost) CurrentRuntime() (automationapp.Runtime, error) {
	if host.app == nil {
		return automationapp.Runtime{}, automationapp.ErrNoWorkspace
	}
	host.app.mu.RLock()
	if strings.TrimSpace(host.app.workspace) == "" {
		host.app.mu.RUnlock()
		return automationapp.Runtime{}, automationapp.ErrNoWorkspace
	}
	if host.app.cfg == nil {
		host.app.mu.RUnlock()
		return automationapp.Runtime{}, fmt.Errorf("application runtime configuration is unavailable")
	}
	cfg := *host.app.cfg
	runtime := automationapp.Runtime{
		ProjectID:        cfg.ProjectID,
		ProjectType:      projectdomain.TypeBook,
		StateRoot:        cfg.ProjectStateDir,
		Workspace:        host.app.workspace,
		DataDir:          cfg.DataDir(),
		Config:           cfg,
		BookState:        host.app.bookState,
		BookService:      host.app.bookService,
		SessionStore:     host.app.sessionStore,
		ExecutionRuntime: host.app.executionRuntime,
	}
	host.app.mu.RUnlock()
	fresh, err := refreshConversationRuntimeConfig(runtime.Config, runtime.Workspace, runtime.StateRoot)
	if err != nil {
		return automationapp.Runtime{}, fmt.Errorf("refresh automation runtime configuration: %w", err)
	}
	runtime.Config = fresh
	return runtime, nil
}

func (host automationHost) BaseRuntime() automationapp.Runtime {
	if host.app == nil {
		return automationapp.Runtime{}
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	var cfg config.Config
	if host.app.cfg != nil {
		cfg = *host.app.cfg
	}
	return automationapp.Runtime{
		DataDir:          cfg.DataDir(),
		Config:           cfg,
		ExecutionRuntime: host.app.executionRuntime,
	}
}

func (host automationHost) ResolveTarget(target automation.ExecutionTarget) (automation.ExecutionTarget, error) {
	if target.Kind == automation.TargetKindUser {
		return automation.ExecutionTarget{Kind: automation.TargetKindUser}, nil
	}
	if host.app == nil {
		return automation.ExecutionTarget{}, fmt.Errorf("project registry is unavailable")
	}
	if projectID := strings.TrimSpace(target.ProjectID); projectID != "" {
		if record, _, err := host.app.resolveProject(projectID, true); err == nil {
			return automation.ExecutionTarget{
				Kind: automation.TargetKindWorkspace, ProjectID: record.ID, Workspace: record.WorkspacePath,
			}, nil
		}
	}
	record, _, err := host.app.resolveProjectByWorkspace(target.Workspace)
	if err != nil {
		return automation.ExecutionTarget{}, err
	}
	return automation.ExecutionTarget{
		Kind: automation.TargetKindWorkspace, ProjectID: record.ID, Workspace: record.WorkspacePath,
	}, nil
}

func (host automationHost) RuntimeForTarget(ctx context.Context, target automation.ExecutionTarget) (automationapp.Runtime, error) {
	if host.app == nil {
		return automationapp.Runtime{}, fmt.Errorf("application runtime is unavailable")
	}
	resolved, err := host.ResolveTarget(target)
	if err != nil {
		return automationapp.Runtime{}, err
	}
	if current, currentErr := host.CurrentRuntime(); currentErr == nil &&
		(current.ProjectID == resolved.ProjectID || lifecycleWorkspaceKey(current.Workspace) == lifecycleWorkspaceKey(resolved.Workspace)) {
		return current, nil
	}
	project, err := host.app.AgentChat().ProjectRuntime(ctx, resolved.ProjectID)
	if err != nil {
		return automationapp.Runtime{}, fmt.Errorf("build automation project runtime: %w", err)
	}
	runtime := project.Conversation
	return automationapp.Runtime{
		ProjectID:        runtime.ProjectID,
		ProjectType:      runtime.ProjectType,
		StateRoot:        runtime.ProjectState,
		Workspace:        runtime.Workspace,
		DataDir:          runtime.Config.DataDir(),
		Config:           runtime.Config,
		BookState:        runtime.State,
		BookService:      runtime.BookService,
		SessionStore:     project.SessionStore,
		ExecutionRuntime: runtime.ExecutionRuntime,
	}, nil
}

func (host automationHost) Catalog() (automationapp.Catalog, error) {
	base := host.BaseRuntime()
	catalog := automationapp.Catalog{DataDir: base.DataDir, CurrentWorkspace: host.CurrentWorkspace()}
	if host.app == nil || host.app.projectRegistry == nil {
		return catalog, nil
	}
	records, err := host.app.projectRegistry.List(false)
	if err != nil {
		return catalog, err
	}
	catalog.Projects = make([]automation.ProjectLocation, 0, len(records))
	for _, record := range records {
		layout, layoutErr := host.app.projectRegistry.Layout(record)
		if layoutErr != nil {
			slog.WarnContext(context.Background(), fmt.Sprintf("[automation] skip Project with unavailable state project_id=%s err=%v", record.ID, layoutErr))
			continue
		}
		catalog.Projects = append(catalog.Projects, automation.ProjectLocation{
			ProjectID: record.ID,
			Workspace: record.WorkspacePath,
			StateRoot: layout.StateRoot,
		})
	}
	return catalog, nil
}

func (host automationHost) AcquireRootOperation(ctx context.Context) (automationapp.Operation, error) {
	return host.app.acquireRootOperation(ctx)
}

func (host automationHost) AcquireProjectOperation(ctx context.Context, projectID string) (automationapp.Operation, error) {
	return host.app.AcquireProjectOperation(ctx, projectID)
}

func (host automationHost) AcquireWorkspaceOperation(ctx context.Context, workspace string) (automationapp.Operation, error) {
	return host.app.acquireWorkspaceOperation(ctx, workspace, false)
}

func (host automationHost) AcceptProjectConversationTurn(
	ctx context.Context,
	task *appTask.Task,
	turn automationapp.ProjectConversationTurn,
	emit func(agentrun.Event),
) (automationapp.ProjectConversationExecution, error) {
	if host.app == nil {
		return nil, fmt.Errorf("application runtime is unavailable")
	}
	projectID := strings.TrimSpace(turn.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("automation execution requires a target Project")
	}
	busyPolicy := agentchatapp.TurnBusyReject
	if turn.SessionStrategy == automation.SessionStrategyPerTask {
		busyPolicy = agentchatapp.TurnBusyWait
	}
	return host.app.AgentChat().AcceptTurn(ctx, agentchatapp.TurnRequest{
		Binding: agentchatapp.Binding{ProjectID: projectID, SessionID: turn.SessionID},
		ChatRequest: agentchatapp.ChatRequest{
			CommandID: turn.CommandID,
			Message:   turn.Message,
		},
		Task: task,
		Policy: agentchatapp.TurnPolicy{
			Origin:         agentchatapp.TurnOriginAutomation,
			OriginID:       turn.AutomationTaskID,
			TraceID:        turn.RunID,
			SessionTitle:   turn.SessionTitle,
			ModelProfileID: turn.ModelProfileID,
			BusyPolicy:     busyPolicy,
		},
		Emit: emit,
	})
}

func (host automationHost) RegisterTask(task *appTask.Task, workspace string) error {
	if host.app == nil {
		return fmt.Errorf("application runtime is unavailable")
	}
	host.app.mu.Lock()
	defer host.app.mu.Unlock()
	if strings.TrimSpace(workspace) != "" {
		return host.app.registerWorkspaceTaskLocked(task, workspace, false)
	}
	if err := host.app.initializeLifecycleLocked(); err != nil {
		return err
	}
	return host.app.registerOwnedTaskLocked(task, "", host.app.rootScope)
}

func (host automationHost) UnregisterTask(task *appTask.Task) {
	if host.app != nil {
		host.app.unregisterWorkspaceTask(task)
	}
}

// automationMutationCallback is notification-only. Durable HostEffect
// reconciliation remains the sole trigger authority for tool mutations.
func (a *App) automationMutationCallback(_ string) func(context.Context, []agenttool.Mutation, agenttool.Verification) {
	return func(context.Context, []agenttool.Mutation, agenttool.Verification) {
		a.Automation().SignalReconciliation()
	}
}

// verifiedWorkspaceMutationCallback schedules versioning only after the same
// verified mutation event has signaled Automation reconciliation.
func (a *App) verifiedWorkspaceMutationCallback(
	source string,
	versionService *book.VersionService,
	settings book.VersionAutoSettings,
) func(context.Context, []agenttool.Mutation, agenttool.Verification) {
	automationCallback := a.automationMutationCallback(source)
	return func(ctx context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
		automationCallback(ctx, mutations, verification)
		if len(mutations) > 0 {
			scheduleAutoVersion(versionService, settings)
		}
	}
}
