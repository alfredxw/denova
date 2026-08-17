package app

import (
	"context"
	"strings"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	appconversation "denova/internal/app/conversation"
	"denova/internal/app/task"
)

// agentOptions is the single foreground Writing runtime route. ProjectID is
// product event metadata; the public Agent Session remains workspace/session
// scoped in agentrun.RuntimeBindingForOptions.
func (runtime ideChatRuntime) agentOptions(taskID string) agentrun.Options {
	return agentrun.Options{
		AgentKind: agentrun.AgentKindIDE,
		ProjectID: strings.TrimSpace(runtime.projectID),
		StateRoot: strings.TrimSpace(runtime.projectState),
		TaskID:    strings.TrimSpace(taskID),
		SessionID: runtime.sess.ID,
		Workspace: strings.TrimSpace(runtime.workspace),
		Mode:      "ide",
	}
}

func (service *ChatAppService) prepareIDEChatRuntime(ctx context.Context, request agentchat.ChatRequest) (ideChatRuntime, agentchat.ChatRequest, error) {
	app := service.app
	app.mu.Lock()
	if app.session == nil || app.bookState == nil || app.cfg == nil {
		app.mu.Unlock()
		return ideChatRuntime{}, request, ErrNoWorkspace
	}
	runtime := ideChatRuntime{
		app: app, projectID: app.cfg.ProjectID, projectType: ProjectTypeBook,
		projectState: app.cfg.ProjectStateDir, agentKind: config.AgentKindIDE,
		sess: app.session, state: app.bookState, bookService: app.bookService,
		executionRuntime: app.executionRuntime, workspace: app.workspace, versionService: app.versionService,
		cfg: *app.cfg,
	}
	runtime.cfg.Workspace = runtime.workspace
	app.mu.Unlock()
	return service.prepareIDEChatRuntimeSnapshot(ctx, runtime, request)
}

func (service *ChatAppService) prepareProjectChatRuntimeSnapshot(ctx context.Context, runtime ideChatRuntime, request agentchat.ChatRequest) (ideChatRuntime, agentchat.ChatRequest, error) {
	prepared, resolved, err := appconversation.Prepare(ctx, sharedConversationRuntime(runtime), request)
	if err != nil {
		return ideChatRuntime{}, request, err
	}
	return applySharedConversationRuntime(runtime, prepared), resolved, nil
}

func (service *ChatAppService) prepareGeneralChatRuntimeSnapshot(ctx context.Context, runtime ideChatRuntime, request agentchat.ChatRequest) (ideChatRuntime, agentchat.ChatRequest, error) {
	return service.prepareProjectChatRuntimeSnapshot(ctx, runtime, request)
}

func (service *ChatAppService) prepareIDEChatRuntimeSnapshot(ctx context.Context, runtime ideChatRuntime, request agentchat.ChatRequest) (ideChatRuntime, agentchat.ChatRequest, error) {
	return service.prepareProjectChatRuntimeSnapshot(ctx, runtime, request)
}

// applyWritingSkillRuntimePolicy remains a narrow test seam around the shared
// Writing/AgentChat policy. Production preparation calls the shared package.
func applyWritingSkillRuntimePolicy(ctx context.Context, runtime *ideChatRuntime, request *agentchat.ChatRequest) error {
	if runtime == nil {
		return nil
	}
	shared := sharedConversationRuntime(*runtime)
	if err := appconversation.ApplyWritingSkillPolicy(ctx, &shared, request); err != nil {
		return err
	}
	*runtime = applySharedConversationRuntime(*runtime, shared)
	return nil
}

func (app *App) ActiveTask() *task.Task {
	return app.chat().ActiveTask()
}

func (service *ChatAppService) ActiveTask() *task.Task {
	app := service.app
	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.activeWritingRun != nil && !app.activeWritingRun.matchesCurrent(app) {
		return nil
	}
	return app.activeTask
}

// ActiveTaskForSession returns a display Task only for the exact foreground
// Writing Session requested by the browser.
func (app *App) ActiveTaskForSession(sessionID string) (*task.Task, error) {
	return app.chat().ActiveTaskForSession(sessionID)
}

func (service *ChatAppService) ActiveTaskForSession(sessionID string) (*task.Task, error) {
	service.admission.RLock()
	defer service.admission.RUnlock()
	if err := service.confirmSelectedSessionID(sessionID); err != nil {
		return nil, err
	}
	app := service.app
	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.activeWritingRun != nil && !app.activeWritingRun.matchesCurrent(app) {
		return nil, ErrAgentContextChanged
	}
	if app.activeWritingRun != nil {
		if app.activeWritingRun.runtime.sess == nil || strings.TrimSpace(app.activeWritingRun.runtime.sess.ID) != strings.TrimSpace(sessionID) {
			return nil, ErrAgentContextChanged
		}
	}
	return app.activeTask, nil
}
