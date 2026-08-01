package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"strings"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
)

func (a *App) AnalyzeContext(ctx context.Context, req agentchat.ChatRequest) (agentchat.ContextAnalysis, error) {
	return a.chat().AnalyzeContext(ctx, req)
}

func (s *ChatAppService) AnalyzeContext(ctx context.Context, req agentchat.ChatRequest) (agentchat.ContextAnalysis, error) {
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return agentchat.ContextAnalysis{}, err
	}
	defer operation.Release()
	ctx = operation.Context()
	runtime, req, err := s.prepareIDEChatRuntime(ctx, req)
	if err != nil {
		return agentchat.ContextAnalysis{}, err
	}
	var pending *session.Interruption
	if shouldResume := strings.TrimSpace(req.Message); shouldResume != "" {
		pending = runtime.sess.PendingInterruption()
	}
	var compaction *session.ContextCompaction
	if record, ok := runtime.sess.LatestContextCompaction(config.AgentKindIDE); ok {
		compaction = &record
	}
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.state, req.IDEContext)
	conversation := agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)
	return agentchat.BuildIDEContextAnalysis(&runtime.cfg, runtime.state, runtime.ideTeller, runtime.bookService, compaction, pending, req, conversation)
}

func (a *App) CompactContext(ctx context.Context) (agentcompaction.Result, error) {
	return a.chat().CompactContext(ctx)
}

func (s *ChatAppService) CompactContext(ctx context.Context) (agentcompaction.Result, error) {
	return s.executeWritingContextCompaction(ctx, "")
}

func (a *App) CompactContextCommand(ctx context.Context, commandID string) (agentcompaction.Result, error) {
	return a.chat().executeWritingContextCompaction(ctx, commandID)
}

func (a *App) RemoveContextCompaction() (bool, error) {
	return a.chat().RemoveContextCompaction()
}

func (s *ChatAppService) RemoveContextCompaction() (bool, error) {
	return s.executeWritingContextCompactionRemoval(context.Background(), "")
}

func (a *App) RemoveContextCompactionCommand(ctx context.Context, commandID string) (bool, error) {
	return a.chat().executeWritingContextCompactionRemoval(ctx, commandID)
}
