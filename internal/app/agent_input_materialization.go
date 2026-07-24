package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/interactive"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

// PlanHarnessInputMaterialization derives the canonical semantic hash without
// invoking a model, tool, Runner, or process-local turn registry.
func (a *App) PlanHarnessInputMaterialization(
	ctx context.Context,
	request agents.HarnessInputMaterializationRequest,
) (runstate.InputMaterializationPlan, error) {
	binding, err := agents.ParseRuntimeBinding(request.Binding)
	if err != nil {
		return runstate.InputMaterializationPlan{}, err
	}
	switch binding.AgentKind {
	case agents.AgentKindIDE, agents.AgentKindConfigManager, agents.AgentKindImage, agents.AgentKindAutomation:
		intent, err := a.sessionAcceptedInputIntent(ctx, request)
		if err != nil {
			return runstate.InputMaterializationPlan{}, err
		}
		return runstate.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case agents.AgentKindInteractiveStory:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return runstate.InputMaterializationPlan{}, err
		}
		return runstate.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
	case config.AgentKindInteractiveDirector:
		return runstate.InputMaterializationPlan{}, nil
	default:
		return runstate.InputMaterializationPlan{}, fmt.Errorf("unsupported Agent kind %q for accepted input", binding.AgentKind)
	}
}

func (a *App) MaterializeHarnessInput(
	ctx context.Context,
	request agents.HarnessInputMaterializationRequest,
	plan runstate.InputMaterializationPlan,
) (runstate.InputMaterializationReceipt, error) {
	if !plan.Required || strings.TrimSpace(plan.Hash) == "" {
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("accepted input materialization requires an exact semantic hash")
	}
	binding, err := agents.ParseRuntimeBinding(request.Binding)
	if err != nil {
		return runstate.InputMaterializationReceipt{}, err
	}
	switch binding.AgentKind {
	case agents.AgentKindIDE, agents.AgentKindConfigManager, agents.AgentKindImage, agents.AgentKindAutomation:
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
	case agents.AgentKindInteractiveStory:
		intent, err := gameAcceptedInputIntent(request)
		if err != nil {
			return runstate.InputMaterializationReceipt{}, err
		}
		if intent.Hash != plan.Hash {
			return runstate.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted player input changed after planning", interactive.ErrPlayerInputIdentityConflict)
		}
		receipt, err := interactive.NewStore(binding.Workspace).CommitPlayerInput(binding.StoryID, intent)
		if err != nil {
			return runstate.InputMaterializationReceipt{}, err
		}
		return runstate.InputMaterializationReceipt{Revision: receipt.Revision}, nil
	case config.AgentKindInteractiveDirector:
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("Director profile has no canonical user input")
	default:
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("unsupported Agent kind %q for accepted input", binding.AgentKind)
	}
}

func (a *App) sessionAcceptedInputIntent(
	ctx context.Context,
	request agents.HarnessInputMaterializationRequest,
) (session.DomainCommitIntent, error) {
	binding, err := agents.ParseRuntimeBinding(request.Binding)
	if err != nil {
		return session.DomainCommitIntent{}, err
	}
	resolved := request.Request
	if len(resolved.ReviewFeedback) > 0 {
		runtime := ideChatRuntime{
			workspace: binding.Workspace,
			sess:      &session.Session{ID: binding.SessionID},
		}
		if err := (&ChatAppService{app: a}).resolveReviewFeedback(ctx, runtime, &resolved); err != nil {
			return session.DomainCommitIntent{}, err
		}
	}
	return session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(request.Identity.CommandID), OperationID: string(request.Identity.OperationID), Cycle: request.Identity.Cycle,
	}, agents.UserMessage(request.Message), session.MessageMetadata{
		AgentKind: request.AgentKind, UserReferences: agents.UserMessageReferencesForRequest(resolved),
	})
}

func (a *App) commitSessionAcceptedInput(
	ctx context.Context,
	binding runstate.BindingRef,
	intent session.DomainCommitIntent,
) (session.DomainCommitReceipt, error) {
	productBinding, err := agents.ParseRuntimeBinding(binding)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	if a != nil {
		a.mu.RLock()
		workspace := strings.TrimSpace(a.workspace)
		store := a.sessionStore
		a.mu.RUnlock()
		if store != nil && workspace != "" && workspace == strings.TrimSpace(productBinding.Workspace) {
			sess, err := store.Get(productBinding.SessionID)
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
	return session.CommitStoredDomainMessage(ctx, dir, productBinding.SessionID, intent)
}

func gameAcceptedInputIntent(request agents.HarnessInputMaterializationRequest) (interactive.PlayerInputIntent, error) {
	binding, err := agents.ParseRuntimeBinding(request.Binding)
	if err != nil {
		return interactive.PlayerInputIntent{}, err
	}
	return interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(request.Identity.CommandID), OperationID: string(request.Identity.OperationID), Cycle: request.Identity.Cycle,
	}, binding.BranchID, request.Message)
}
