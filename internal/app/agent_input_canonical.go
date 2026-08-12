package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

func (a *App) sessionCanonicalInput(
	ctx context.Context,
	request agentexecution.CanonicalInputRequest,
) (agent.CanonicalAdapter, error) {
	resolved := request.Request
	if len(resolved.ReviewFeedback) > 0 {
		workspace := strings.TrimSpace(request.Binding.Workspace)
		if projectID := strings.TrimSpace(request.Binding.ProjectID); projectID != "" {
			_, layout, err := a.resolveProject(projectID, true)
			if err != nil {
				return nil, err
			}
			workspace = layout.ContentRoot
		}
		runtime := ideChatRuntime{workspace: workspace, sess: &session.Session{ID: request.Binding.SessionID}}
		if err := (&ChatAppService{app: a}).resolveReviewFeedback(ctx, runtime, &resolved); err != nil {
			return nil, err
		}
	}
	request.Request = resolved
	return agent.CanonicalAdapterFuncs{
		CapabilityIdentity: request.Identity,
		MaterializeInputFn: func(ctx context.Context, input agent.InputCommitRequest) (agent.CommitReceipt, error) {
			intent, err := sessionCanonicalInputIntent(request, input.Hash)
			if err != nil {
				return agent.CommitReceipt{}, err
			}
			receipt, err := a.commitSessionAcceptedInput(ctx, request.Binding, intent)
			if err != nil {
				return agent.CommitReceipt{}, err
			}
			if request.Options.InputCommitEffect != nil {
				if err := request.Options.InputCommitEffect.Apply(ctx, canonicalInputEffectRequest(input.Identity, input.Hash)); err != nil {
					return agent.CommitReceipt{}, fmt.Errorf("run Denova input-commit callback: %w", err)
				}
			}
			return agent.CommitReceipt{Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
		},
		ReconcileFn: func(ctx context.Context, reconcile agent.ReconcileRequest) (agent.ReconcileResult, error) {
			result, err := a.reconcileStoredSessionCanonicalInput(request.Binding, reconcile)
			if err != nil || !result.Found || request.Options.InputCommitEffect == nil {
				return result, err
			}
			result.Found, err = request.Options.InputCommitEffect.Reconcile(ctx, canonicalInputEffectRequest(reconcile.Identity, reconcile.Hash))
			if !result.Found {
				result.Revision = ""
			}
			return result, err
		},
	}, nil
}

func canonicalInputEffectRequest(identity agent.CommitIdentity, hash string) agentrun.InputCommitEffectRequest {
	return agentrun.InputCommitEffectRequest{
		CommandID: identity.CommandID, OperationID: identity.RunID, Cycle: identity.Cycle, Hash: hash,
	}
}

func sessionCanonicalInputIntent(
	request agentexecution.CanonicalInputRequest,
	agentHash string,
) (session.DomainCommitIntent, error) {
	if request.Input.Text != agentchat.CallerView(request.Request).Message {
		return session.DomainCommitIntent{}, errors.New("accepted Session input changed after admission")
	}
	intent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(request.CommandID), OperationID: string(request.RunID), Cycle: request.Cycle,
	}, agent.UserMessage(request.Input.Text), session.MessageMetadata{
		AgentKind:      request.Options.AgentKind,
		UserReferences: cloneCanonicalUserReferences(agentchat.UserMessageReferencesForRequest(request.Request)),
		ContextOnly:    request.Request.InputVisibility == agentrun.InputModelOnly,
	})
	if err != nil {
		return session.DomainCommitIntent{}, err
	}
	return intent.WithAgentCanonicalHash(agentHash)
}

func cloneCanonicalUserReferences(references []agentcontext.UserReference) []agentcontext.UserReference {
	return append([]agentcontext.UserReference(nil), references...)
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

func (a *App) reconcileStoredSessionCanonicalInput(
	binding agentrun.RuntimeBinding,
	request agent.ReconcileRequest,
) (agent.ReconcileResult, error) {
	if request.Identity.Stage != agent.CommitInput {
		return agent.ReconcileResult{}, fmt.Errorf("provider-free Session adapter cannot reconcile canonical stage %q", request.Identity.Stage)
	}
	dir, err := a.sessionDirectoryForBinding(binding)
	if err != nil {
		return agent.ReconcileResult{}, err
	}
	receipt, found, err := session.FindStoredAgentCanonicalCommit(
		dir,
		binding.SessionID,
		session.DomainCommitIdentity{
			CommandID:   request.Identity.CommandID,
			OperationID: request.Identity.RunID,
			Cycle:       request.Identity.Cycle,
		},
		agent.User,
		request.Hash,
	)
	if err != nil || !found {
		return agent.ReconcileResult{Found: found}, err
	}
	return agent.ReconcileResult{Found: true, Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
}

func (a *App) gameCanonicalInput(
	_ context.Context,
	request agentexecution.CanonicalInputRequest,
) (agent.CanonicalAdapter, error) {
	return agent.CanonicalAdapterFuncs{
		CapabilityIdentity: request.Identity,
		MaterializeInputFn: func(_ context.Context, input agent.InputCommitRequest) (agent.CommitReceipt, error) {
			intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
				CommandID: string(request.CommandID), OperationID: string(request.RunID), Cycle: request.Cycle,
			}, request.Binding.BranchID, request.Input.Text)
			if err != nil {
				return agent.CommitReceipt{}, err
			}
			intent, err = intent.WithAgentCanonicalHash(input.Hash)
			if err != nil {
				return agent.CommitReceipt{}, err
			}
			receipt, err := interactive.NewStore(request.Binding.Workspace).CommitPlayerInput(request.Binding.StoryID, intent)
			if err != nil {
				return agent.CommitReceipt{}, err
			}
			return agent.CommitReceipt{Revision: receipt.Revision}, nil
		},
		ReconcileFn: func(_ context.Context, reconcile agent.ReconcileRequest) (agent.ReconcileResult, error) {
			if reconcile.Identity.Stage != agent.CommitInput {
				return agent.ReconcileResult{}, fmt.Errorf("provider-free Game adapter cannot reconcile canonical stage %q", reconcile.Identity.Stage)
			}
			receipt, found, err := interactive.NewStore(request.Binding.Workspace).FindRecentAgentCanonicalPlayerInputCommit(
				request.Binding.StoryID,
				request.Binding.BranchID,
				interactive.DomainCommitIdentity{
					CommandID:   reconcile.Identity.CommandID,
					OperationID: reconcile.Identity.RunID,
					Cycle:       reconcile.Identity.Cycle,
				},
				reconcile.Hash,
			)
			return agent.ReconcileResult{Found: found, Revision: receipt.Revision}, err
		},
	}, nil
}

func (a *App) sessionDirectoryForBinding(binding agentrun.RuntimeBinding) (string, error) {
	if strings.TrimSpace(binding.SessionID) == "" {
		return "", errors.New("Session canonical input binding has no session id")
	}
	if binding.AgentKind == agentrun.AgentKindAutomation && strings.TrimSpace(binding.Workspace) == "" {
		if a == nil || a.cfg == nil || strings.TrimSpace(a.cfg.DataDir()) == "" {
			return "", ErrAgentDataDirRequired
		}
		return filepath.Join(a.cfg.DataDir(), "automations", "sessions"), nil
	}
	if a != nil && a.projectRegistry != nil {
		if projectID := strings.TrimSpace(binding.ProjectID); projectID != "" {
			record, err := a.projectRegistry.Get(projectID)
			if err != nil {
				return "", err
			}
			layout, err := a.projectRegistry.EnsureState(record)
			if err != nil {
				return "", err
			}
			return layout.SessionsDir(), nil
		}
		if workspace := strings.TrimSpace(binding.Workspace); workspace != "" {
			record, found, err := a.projectRegistry.FindByPath(workspace, true)
			if err != nil {
				return "", err
			}
			if found {
				layout, err := a.projectRegistry.EnsureState(record)
				if err != nil {
					return "", err
				}
				return layout.SessionsDir(), nil
			}
		}
	}
	workspace := strings.TrimSpace(binding.Workspace)
	if workspace == "" {
		return "", errors.New("Session canonical input binding has no workspace")
	}
	return book.NewState(workspace).SessionDir(), nil
}
