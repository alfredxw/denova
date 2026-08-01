package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"fmt"
	"strconv"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

// PlanHarnessInputMaterialization derives the canonical semantic hash without
// invoking a model, tool, Runner, or process-local turn registry.
func (a *App) PlanHarnessInputMaterialization(
	ctx context.Context,
	request agentharness.InputMaterializationRequest,
) (agentrun.InputMaterializationPlan, error) {
	binding := request.Binding
	switch binding.AgentKind {
	case agentrun.AgentKindGeneral, agentrun.AgentKindIDE, agentrun.AgentKindConfigManager, agentrun.AgentKindImage, agentrun.AgentKindAutomation:
		intent, err := a.sessionAcceptedInputIntent(ctx, request)
		if err != nil {
			return agentrun.InputMaterializationPlan{}, err
		}
		return agentrun.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case agentrun.AgentKindInteractiveStory:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return agentrun.InputMaterializationPlan{}, err
		}
		return agentrun.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case config.AgentKindInteractiveDirector:
		return agentrun.InputMaterializationPlan{}, nil
	default:
		return agentrun.InputMaterializationPlan{}, fmt.Errorf("unsupported Agent kind %q for accepted input", binding.AgentKind)
	}
}

func (a *App) MaterializeHarnessInput(
	ctx context.Context,
	request agentharness.InputMaterializationRequest,
	plan agentrun.InputMaterializationPlan,
) (agentrun.InputMaterializationReceipt, error) {
	if !plan.Required || strings.TrimSpace(plan.Hash) == "" {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("accepted input materialization requires an exact semantic hash")
	}
	binding := request.Binding
	switch binding.AgentKind {
	case agentrun.AgentKindGeneral, agentrun.AgentKindIDE, agentrun.AgentKindConfigManager, agentrun.AgentKindImage, agentrun.AgentKindAutomation:
		intent, err := a.sessionAcceptedInputIntent(ctx, request)
		if err != nil {
			return agentrun.InputMaterializationReceipt{}, err
		}
		if intent.Hash != plan.Hash {
			return agentrun.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted Session input changed after planning", session.ErrDomainCommitIdentityConflict)
		}
		receipt, err := a.commitSessionAcceptedInput(ctx, request.Binding, intent)
		if err != nil {
			return agentrun.InputMaterializationReceipt{}, err
		}
		return agentrun.InputMaterializationReceipt{Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
	case agentrun.AgentKindInteractiveStory:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return agentrun.InputMaterializationReceipt{}, err
		}
		if intent.Hash != plan.Hash {
			return agentrun.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted player input changed after planning", interactive.ErrPlayerInputIdentityConflict)
		}
		receipt, err := interactive.NewStore(binding.Workspace).CommitPlayerInput(binding.StoryID, intent)
		if err != nil {
			return agentrun.InputMaterializationReceipt{}, err
		}
		return agentrun.InputMaterializationReceipt{Revision: receipt.Revision}, nil
	case config.AgentKindInteractiveDirector:
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("Director profile has no canonical user input")
	default:
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("unsupported Agent kind %q for accepted input", binding.AgentKind)
	}
}

func (a *App) sessionAcceptedInputIntent(
	ctx context.Context,
	request agentharness.InputMaterializationRequest,
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

func gameAcceptedInputIntent(request agentharness.InputMaterializationRequest) (interactive.PlayerInputIntent, error) {
	binding := request.Binding
	return interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(request.Identity.CommandID), OperationID: string(request.Identity.OperationID), Cycle: request.Identity.Cycle,
	}, binding.BranchID, request.Message)
}
