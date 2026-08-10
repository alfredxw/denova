package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"strings"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func (a *App) sessionAcceptedInputIntent(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
) (session.DomainCommitIntent, error) {
	binding := request.Binding
	resolved := request.Request
	if len(resolved.ReviewFeedback) > 0 {
		workspace := strings.TrimSpace(binding.Workspace)
		if projectID := strings.TrimSpace(binding.ProjectID); projectID != "" {
			_, layout, err := a.resolveProject(projectID, true)
			if err != nil {
				return session.DomainCommitIntent{}, err
			}
			workspace = layout.ContentRoot
		}
		runtime := ideChatRuntime{
			workspace: workspace,
			sess:      &session.Session{ID: binding.SessionID},
		}
		if err := (&ChatAppService{app: a}).resolveReviewFeedback(ctx, runtime, &resolved); err != nil {
			return session.DomainCommitIntent{}, err
		}
	}
	return session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(request.Identity.CommandID), OperationID: string(request.Identity.OperationID), Cycle: request.Identity.Cycle,
	}, agents.UserMessage(request.Message), session.MessageMetadata{
		AgentKind: request.AgentKind, UserReferences: agentchat.UserMessageReferencesForRequest(resolved),
		ContextOnly: request.Request.InputVisibility == agentrun.InputModelOnly,
	})
}

func (a *App) commitSessionAcceptedInput(
	ctx context.Context,
	binding agentrun.RuntimeBinding,
	intent session.DomainCommitIntent,
) (session.DomainCommitReceipt, error) {
	if a != nil {
		a.mu.RLock()
		workspace := strings.TrimSpace(a.workspace)
		store := a.sessionStore
		a.mu.RUnlock()
		if store != nil && workspace != "" && workspace == strings.TrimSpace(binding.Workspace) {
			sess, err := store.Get(binding.SessionID)
			if err != nil {
				return session.DomainCommitReceipt{}, err
			}
			return sess.CommitDomainMessageContext(ctx, intent)
		}
	}
	dir, err := a.sessionDirectoryForBinding(binding)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	return session.CommitStoredDomainMessage(ctx, dir, binding.SessionID, intent)
}

func gameAcceptedInputIntent(request agentexecution.InputMaterializationRequest) (interactive.PlayerInputIntent, error) {
	binding := request.Binding
	return interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(request.Identity.CommandID), OperationID: string(request.Identity.OperationID), Cycle: request.Identity.Cycle,
	}, binding.BranchID, request.Message)
}
