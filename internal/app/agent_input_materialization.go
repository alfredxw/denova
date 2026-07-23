package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"

	"denova/internal/agent"
	"denova/internal/agentruntime"
	"denova/internal/interactive"
	"denova/internal/session"
)

// PlanHarnessInputMaterialization derives the canonical semantic hash without
// invoking a model, tool, Runner, or process-local turn registry.
func (a *App) PlanHarnessInputMaterialization(
	ctx context.Context,
	request agent.HarnessInputMaterializationRequest,
) (agentruntime.InputMaterializationPlan, error) {
	switch request.Binding.Profile {
	case agentruntime.ProfileWriting, agentruntime.ProfileConfigManager, agentruntime.ProfileImage, agentruntime.ProfileAutomation:
		intent, err := a.sessionAcceptedInputIntent(ctx, request)
		if err != nil {
			return agentruntime.InputMaterializationPlan{}, err
		}
		return agentruntime.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case agentruntime.ProfileGame:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return agentruntime.InputMaterializationPlan{}, err
		}
		return agentruntime.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case agentruntime.ProfileDirector:
		return agentruntime.InputMaterializationPlan{}, nil
	default:
		return agentruntime.InputMaterializationPlan{}, fmt.Errorf("unsupported Agent profile %q for accepted input", request.Binding.Profile)
	}
}

func (a *App) MaterializeHarnessInput(
	ctx context.Context,
	request agent.HarnessInputMaterializationRequest,
	plan agentruntime.InputMaterializationPlan,
) (agentruntime.InputMaterializationReceipt, error) {
	if !plan.Required || strings.TrimSpace(plan.Hash) == "" {
		return agentruntime.InputMaterializationReceipt{}, fmt.Errorf("accepted input materialization requires an exact semantic hash")
	}
	switch request.Binding.Profile {
	case agentruntime.ProfileWriting, agentruntime.ProfileConfigManager, agentruntime.ProfileImage, agentruntime.ProfileAutomation:
		intent, err := a.sessionAcceptedInputIntent(ctx, request)
		if err != nil {
			return agentruntime.InputMaterializationReceipt{}, err
		}
		if intent.Hash != plan.Hash {
			return agentruntime.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted Session input changed after planning", session.ErrDomainCommitIdentityConflict)
		}
		receipt, err := a.commitSessionAcceptedInput(ctx, request.Binding, intent)
		if err != nil {
			return agentruntime.InputMaterializationReceipt{}, err
		}
		return agentruntime.InputMaterializationReceipt{Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
	case agentruntime.ProfileGame:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return agentruntime.InputMaterializationReceipt{}, err
		}
		if intent.Hash != plan.Hash {
			return agentruntime.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted player input changed after planning", interactive.ErrPlayerInputIdentityConflict)
		}
		receipt, err := interactive.NewStore(request.Binding.Workspace).CommitPlayerInput(request.Binding.StoryID, intent)
		if err != nil {
			return agentruntime.InputMaterializationReceipt{}, err
		}
		return agentruntime.InputMaterializationReceipt{Revision: receipt.Revision}, nil
	case agentruntime.ProfileDirector:
		return agentruntime.InputMaterializationReceipt{}, fmt.Errorf("Director profile has no canonical user input")
	default:
		return agentruntime.InputMaterializationReceipt{}, fmt.Errorf("unsupported Agent profile %q for accepted input", request.Binding.Profile)
	}
}

func (a *App) sessionAcceptedInputIntent(
	ctx context.Context,
	request agent.HarnessInputMaterializationRequest,
) (session.DomainCommitIntent, error) {
	resolved := request.Request
	if len(resolved.ReviewFeedback) > 0 {
		runtime := ideChatRuntime{
			workspace: request.Binding.Workspace,
			sess:      &session.Session{ID: request.Binding.SessionID},
		}
		if err := (&ChatAppService{app: a}).resolveReviewFeedback(ctx, runtime, &resolved); err != nil {
			return session.DomainCommitIntent{}, err
		}
	}
	return session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(request.Identity.CommandID), OperationID: string(request.Identity.OperationID), Cycle: request.Identity.Cycle,
	}, schema.UserMessage(request.Message), session.MessageMetadata{
		AgentKind: request.AgentKind, UserReferences: agent.UserMessageReferencesForRequest(resolved),
	})
}

func (a *App) commitSessionAcceptedInput(
	ctx context.Context,
	binding agentruntime.BindingRef,
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

func gameAcceptedInputIntent(request agent.HarnessInputMaterializationRequest) (interactive.PlayerInputIntent, error) {
	return interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(request.Identity.CommandID), OperationID: string(request.Identity.OperationID), Cycle: request.Identity.Cycle,
	}, request.Binding.BranchID, request.Message)
}
