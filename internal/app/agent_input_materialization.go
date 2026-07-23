package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
	"denova/internal/agent/session"
	"denova/internal/interactive"
)

// PlanHarnessInputMaterialization derives the canonical semantic hash without
// invoking a model, tool, Runner, or process-local turn registry.
func (a *App) PlanHarnessInputMaterialization(
	ctx context.Context,
	request agent.HarnessInputMaterializationRequest,
) (runstate.InputMaterializationPlan, error) {
	switch request.Binding.Profile {
	case runstate.ProfileWriting, runstate.ProfileConfigManager, runstate.ProfileImage, runstate.ProfileAutomation:
		intent, err := a.sessionAcceptedInputIntent(ctx, request)
		if err != nil {
			return runstate.InputMaterializationPlan{}, err
		}
		return runstate.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case runstate.ProfileGame:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return runstate.InputMaterializationPlan{}, err
		}
		return runstate.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case runstate.ProfileDirector:
		return runstate.InputMaterializationPlan{}, nil
	default:
		return runstate.InputMaterializationPlan{}, fmt.Errorf("unsupported Agent profile %q for accepted input", request.Binding.Profile)
	}
}

func (a *App) MaterializeHarnessInput(
	ctx context.Context,
	request agent.HarnessInputMaterializationRequest,
	plan runstate.InputMaterializationPlan,
) (runstate.InputMaterializationReceipt, error) {
	if !plan.Required || strings.TrimSpace(plan.Hash) == "" {
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("accepted input materialization requires an exact semantic hash")
	}
	switch request.Binding.Profile {
	case runstate.ProfileWriting, runstate.ProfileConfigManager, runstate.ProfileImage, runstate.ProfileAutomation:
		intent, err := a.sessionAcceptedInputIntent(ctx, request)
		if err != nil {
			return runstate.InputMaterializationReceipt{}, err
		}
		if intent.Hash != plan.Hash {
			return runstate.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted Session input changed after planning", session.ErrDomainCommitIdentityConflict)
		}
		receipt, err := a.commitSessionAcceptedInput(ctx, request.Binding, intent)
		if err != nil {
			return runstate.InputMaterializationReceipt{}, err
		}
		return runstate.InputMaterializationReceipt{Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
	case runstate.ProfileGame:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return runstate.InputMaterializationReceipt{}, err
		}
		if intent.Hash != plan.Hash {
			return runstate.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted player input changed after planning", interactive.ErrPlayerInputIdentityConflict)
		}
		receipt, err := interactive.NewStore(request.Binding.Workspace).CommitPlayerInput(request.Binding.StoryID, intent)
		if err != nil {
			return runstate.InputMaterializationReceipt{}, err
		}
		return runstate.InputMaterializationReceipt{Revision: receipt.Revision}, nil
	case runstate.ProfileDirector:
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("Director profile has no canonical user input")
	default:
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("unsupported Agent profile %q for accepted input", request.Binding.Profile)
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
	}, agent.UserMessage(request.Message), session.MessageMetadata{
		AgentKind: request.AgentKind, UserReferences: agent.UserMessageReferencesForRequest(resolved),
	})
}

func (a *App) commitSessionAcceptedInput(
	ctx context.Context,
	binding runstate.BindingRef,
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
