package app

import (
	"context"

	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"
	conversationapp "denova/internal/app/conversation"
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
	inspected, err := conversationapp.InspectPrepared(ctx, sharedConversationRuntime(runtime), req, agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, StateRoot: runtime.projectState,
		Workspace: runtime.workspace, SessionID: runtime.sess.ID, Mode: "ide",
	})
	if err != nil {
		return agentchat.ContextAnalysis{}, err
	}
	return agentchat.BuildInspectedContextAnalysis(
		&runtime.cfg, agentrun.AgentKindIDE, "ide", inspected.Composition, inspected.Inspection,
	), nil
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
