package app

import (
	"context"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	appconversation "denova/internal/app/conversation"
	"denova/internal/app/task"
)

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
		chatService: app.chatService, workspace: app.workspace, versionService: app.versionService,
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
