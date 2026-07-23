package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
	"denova/internal/agent/session"
	"denova/internal/book"
	"denova/internal/interactive"
)

// reconcileHarnessDomainCommit is the production host adapter for the
// cross-store crash window. Every branch is query-only and reports Found only
// after matching the exact durable identity and semantic hash available in the
// corresponding canonical store.
func (a *App) reconcileHarnessDomainCommit(
	ctx context.Context,
	request runstate.DomainCommitReconcileRequest,
) (runstate.DomainCommitReconcileResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return runstate.DomainCommitReconcileResult{}, err
		}
	}
	if err := validateDomainCommitReconcileRequest(request); err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	if request.Structural != nil {
		return a.reconcileStructuralDomainCommit(ctx, request)
	}
	switch request.Binding.Profile {
	case runstate.ProfileWriting, runstate.ProfileConfigManager, runstate.ProfileImage, runstate.ProfileAutomation:
		return a.reconcileSessionDomainCommit(request)
	case runstate.ProfileGame:
		return reconcileGameDomainCommit(request)
	case runstate.ProfileDirector:
		return reconcileDirectorDomainCommit(request)
	default:
		return runstate.DomainCommitReconcileResult{}, fmt.Errorf("unsupported Agent profile %q for domain commit reconciliation", request.Binding.Profile)
	}
}

func validateDomainCommitReconcileRequest(request runstate.DomainCommitReconcileRequest) error {
	identity := request.Commit.Identity
	if strings.TrimSpace(string(identity.CommandID)) == "" || strings.TrimSpace(string(identity.OperationID)) == "" || identity.Cycle <= 0 {
		return fmt.Errorf("domain commit reconciliation requires a complete durable identity")
	}
	if identity.Stage != runstate.DomainCommitInput && identity.Stage != runstate.DomainCommitOutput {
		return fmt.Errorf("domain commit reconciliation has unsupported stage %q", identity.Stage)
	}
	if strings.TrimSpace(request.Commit.Hash) == "" {
		return fmt.Errorf("domain commit reconciliation requires an exact semantic hash")
	}
	if request.Structural == nil {
		return nil
	}
	structural := request.Structural
	if structural.Binding != request.Binding || structural.CommandID != identity.CommandID ||
		structural.OperationID != identity.OperationID || structural.Cycle != identity.Cycle ||
		identity.Stage != runstate.DomainCommitOutput {
		return fmt.Errorf("structural domain commit identity does not match its durable operation snapshot")
	}
	return nil
}

func (a *App) reconcileSessionDomainCommit(request runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
	role, err := sessionRoleForDomainCommitStage(request.Commit.Identity.Stage)
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	dir, err := a.sessionDirectoryForBinding(request.Binding)
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	receipt, found, err := session.FindStoredDomainCommit(
		dir,
		request.Binding.SessionID,
		session.DomainCommitIdentity{
			CommandID:   string(request.Commit.Identity.CommandID),
			OperationID: string(request.Commit.Identity.OperationID),
			Cycle:       request.Commit.Identity.Cycle,
		},
		role,
		request.Commit.Hash,
	)
	if err != nil || !found {
		return runstate.DomainCommitReconcileResult{}, err
	}
	return runstate.DomainCommitReconcileResult{
		Found: true, Revision: strconv.FormatUint(receipt.ContextRevision, 10),
	}, nil
}

func sessionRoleForDomainCommitStage(stage runstate.DomainCommitStage) (agent.Role, error) {
	switch stage {
	case runstate.DomainCommitInput:
		return agent.RoleUser, nil
	case runstate.DomainCommitOutput:
		return agent.RoleAssistant, nil
	default:
		return "", fmt.Errorf("unsupported Session domain commit stage %q", stage)
	}
}

func (a *App) sessionDirectoryForBinding(binding runstate.BindingRef) (string, error) {
	if strings.TrimSpace(binding.SessionID) == "" {
		return "", fmt.Errorf("Session domain commit binding has no session id")
	}
	if binding.Kind == runstate.BindingAutomation && strings.TrimSpace(binding.Workspace) == "" {
		if a == nil {
			return "", fmt.Errorf("App is unavailable for global automation reconciliation")
		}
		a.mu.RLock()
		dataDir := ""
		if a.cfg != nil {
			dataDir = strings.TrimSpace(a.cfg.DataDir())
		}
		a.mu.RUnlock()
		if dataDir == "" {
			return "", ErrAgentDataDirRequired
		}
		return filepath.Join(dataDir, "automations", "sessions"), nil
	}
	workspace := strings.TrimSpace(binding.Workspace)
	if workspace == "" {
		return "", fmt.Errorf("Session domain commit binding has no workspace")
	}
	return book.NewState(workspace).SessionDir(), nil
}

func reconcileGameDomainCommit(request runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
	if request.Commit.Identity.Stage == runstate.DomainCommitInput {
		receipt, found, err := interactive.NewStore(request.Binding.Workspace).FindPlayerInputCommit(
			request.Binding.StoryID,
			request.Binding.BranchID,
			interactive.DomainCommitIdentity{
				CommandID: string(request.Commit.Identity.CommandID), OperationID: string(request.Commit.Identity.OperationID), Cycle: request.Commit.Identity.Cycle,
			},
			request.Commit.Hash,
		)
		if err != nil || !found {
			return runstate.DomainCommitReconcileResult{}, err
		}
		return runstate.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
	}
	if request.Commit.Identity.Stage != runstate.DomainCommitOutput {
		return runstate.DomainCommitReconcileResult{}, fmt.Errorf("unsupported Game domain commit stage %q", request.Commit.Identity.Stage)
	}
	receipt, found, err := interactive.NewStore(request.Binding.Workspace).FindDomainTurnCommit(
		request.Binding.StoryID,
		request.Binding.BranchID,
		interactive.DomainCommitIdentity{
			CommandID:   string(request.Commit.Identity.CommandID),
			OperationID: string(request.Commit.Identity.OperationID),
			Cycle:       request.Commit.Identity.Cycle,
		},
		request.Commit.Hash,
	)
	if err != nil || !found {
		return runstate.DomainCommitReconcileResult{}, err
	}
	return runstate.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func reconcileDirectorDomainCommit(request runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
	if request.Commit.Identity.Stage != runstate.DomainCommitOutput {
		return runstate.DomainCommitReconcileResult{}, nil
	}
	receipt, found, err := interactive.NewStore(request.Binding.Workspace).FindDirectorPlanDomainCommit(
		request.Binding.StoryID,
		request.Binding.BranchID,
		interactive.DirectorPlanDomainCommitIdentity{
			CommandID:   string(request.Commit.Identity.CommandID),
			OperationID: string(request.Commit.Identity.OperationID),
			Cycle:       request.Commit.Identity.Cycle,
		},
		request.Commit.Hash,
	)
	if err != nil || !found {
		return runstate.DomainCommitReconcileResult{}, err
	}
	return runstate.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func (a *App) reconcileStructuralDomainCommit(
	ctx context.Context,
	request runstate.DomainCommitReconcileRequest,
) (runstate.DomainCommitReconcileResult, error) {
	snapshot := request.Structural
	if snapshot == nil {
		return runstate.DomainCommitReconcileResult{}, nil
	}
	plan, err := agent.DecodeContextStructuralRestorePlan(
		snapshot.Ref.RestoreDescriptor,
		request.Binding,
		snapshot.Ref.ExpectedRevision,
	)
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, fmt.Errorf("decode structural commit recovery plan: %w", err)
	}
	if request.Commit.Hash != plan.IntentHash {
		return runstate.DomainCommitReconcileResult{}, fmt.Errorf("structural canonical commit hash does not match frozen recovery plan")
	}
	operation, err := a.contextStructuralOperationForRestore(agent.HarnessStructuralRestoreRequest{
		Binding: request.Binding, Snapshot: *snapshot, Plan: plan,
	})
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	_, receipt, found, err := operation.Reconcile(ctx)
	if err != nil || !found {
		return runstate.DomainCommitReconcileResult{}, err
	}
	return runstate.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}
