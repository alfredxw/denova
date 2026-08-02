package configmanager

import (
	"context"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
)

func (service *Service) ConversationConfig(request Request) (conversationconfig.Snapshot, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeConfig, sessionID, err := service.conversationRuntime(request)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	_, snapshot, err := agentconversation.GetOrCreateSession(
		store, sessionID, &runtimeConfig, config.AgentKindConfigManager,
	)
	return snapshot, err
}

func (service *Service) PatchConversationConfig(request Request, patch conversationconfig.Patch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeConfig, sessionID, err := service.conversationRuntime(request)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	runtime := service.host.Snapshot()
	workspace := runtime.Workspace
	if active := latestStartTask(&service.starts, workspace, sessionID).Task; active != nil && !active.Finished() {
		return conversationconfig.Snapshot{}, appagentruntime.ErrOperationActive
	}
	if recovery := service.recoveries.current(workspace, sessionID); recovery != nil && recovery.task != nil && !recovery.task.Finished() {
		return conversationconfig.Snapshot{}, appagentruntime.ErrOperationActive
	}
	sess, current, err := agentconversation.GetOrCreateSession(
		store, sessionID, &runtimeConfig, config.AgentKindConfigManager,
	)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeConfig, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

func (service *Service) AnswerAsk(ctx context.Context, request Request, askID string, answers []agentconversation.HostAskAnswer) (agentconversation.HostAskResolution, error) {
	return service.resolveAsk(ctx, request, askID, session.AskAnswered, answers, "")
}

func (service *Service) CancelAsk(ctx context.Context, request Request, askID, reason string) (agentconversation.HostAskResolution, error) {
	return service.resolveAsk(ctx, request, askID, session.AskCancelled, nil, reason)
}

func (service *Service) resolveAsk(
	ctx context.Context,
	request Request,
	askID, status string,
	answers []agentconversation.HostAskAnswer,
	cancelReason string,
) (agentconversation.HostAskResolution, error) {
	sess, err := service.conversationSession(request)
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	return agentconversation.ResolveAsk(ctx, sess, askID, status, answers, cancelReason)
}
