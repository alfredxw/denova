package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"

	"denova/internal/agent"
	"denova/internal/agentruntime"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

// reconcileHarnessDomainCommit is the production host adapter for the
// cross-store crash window. Every branch is query-only and reports Found only
// after matching the exact durable identity and semantic hash available in the
// corresponding canonical store.
func (a *App) reconcileHarnessDomainCommit(
	ctx context.Context,
	request agentruntime.DomainCommitReconcileRequest,
) (agentruntime.DomainCommitReconcileResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return agentruntime.DomainCommitReconcileResult{}, err
		}
	}
	if err := validateDomainCommitReconcileRequest(request); err != nil {
		return agentruntime.DomainCommitReconcileResult{}, err
	}
	if request.Structural != nil {
		return a.reconcileStructuralDomainCommit(ctx, request)
	}
	switch request.Binding.Profile {
	case agentruntime.ProfileWriting, agentruntime.ProfileConfigManager, agentruntime.ProfileImage, agentruntime.ProfileAutomation:
		return a.reconcileSessionDomainCommit(request)
	case agentruntime.ProfileGame:
		return reconcileGameDomainCommit(request)
	case agentruntime.ProfileDirector:
		return reconcileDirectorDomainCommit(request)
	default:
		return agentruntime.DomainCommitReconcileResult{}, fmt.Errorf("unsupported Agent profile %q for domain commit reconciliation", request.Binding.Profile)
	}
}

func validateDomainCommitReconcileRequest(request agentruntime.DomainCommitReconcileRequest) error {
	identity := request.Commit.Identity
	if strings.TrimSpace(string(identity.CommandID)) == "" || strings.TrimSpace(string(identity.OperationID)) == "" || identity.Cycle <= 0 {
		return fmt.Errorf("domain commit reconciliation requires a complete durable identity")
	}
	if identity.Stage != agentruntime.DomainCommitInput && identity.Stage != agentruntime.DomainCommitOutput {
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
		identity.Stage != agentruntime.DomainCommitOutput {
		return fmt.Errorf("structural domain commit identity does not match its durable operation snapshot")
	}
	return nil
}

func (a *App) reconcileSessionDomainCommit(request agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
	role, err := sessionRoleForDomainCommitStage(request.Commit.Identity.Stage)
	if err != nil {
		return agentruntime.DomainCommitReconcileResult{}, err
	}
	dir, err := a.sessionDirectoryForBinding(request.Binding)
	if err != nil {
		return agentruntime.DomainCommitReconcileResult{}, err
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
		return agentruntime.DomainCommitReconcileResult{}, err
	}
	return agentruntime.DomainCommitReconcileResult{
		Found: true, Revision: strconv.FormatUint(receipt.ContextRevision, 10),
	}, nil
}

func sessionRoleForDomainCommitStage(stage agentruntime.DomainCommitStage) (schema.RoleType, error) {
	switch stage {
	case agentruntime.DomainCommitInput:
		return schema.User, nil
	case agentruntime.DomainCommitOutput:
		return schema.Assistant, nil
	default:
		return "", fmt.Errorf("unsupported Session domain commit stage %q", stage)
	}
}

func (a *App) sessionDirectoryForBinding(binding agentruntime.BindingRef) (string, error) {
	if strings.TrimSpace(binding.SessionID) == "" {
		return "", fmt.Errorf("Session domain commit binding has no session id")
	}
	if binding.Kind == agentruntime.BindingAutomation && strings.TrimSpace(binding.Workspace) == "" {
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

func reconcileGameDomainCommit(request agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
	if request.Commit.Identity.Stage == agentruntime.DomainCommitInput {
		receipt, found, err := interactive.NewStore(request.Binding.Workspace).FindPlayerInputCommit(
			request.Binding.StoryID,
			request.Binding.BranchID,
			interactive.DomainCommitIdentity{
				CommandID: string(request.Commit.Identity.CommandID), OperationID: string(request.Commit.Identity.OperationID), Cycle: request.Commit.Identity.Cycle,
			},
			request.Commit.Hash,
		)
		if err != nil || !found {
			return agentruntime.DomainCommitReconcileResult{}, err
		}
		return agentruntime.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
	}
	if request.Commit.Identity.Stage != agentruntime.DomainCommitOutput {
		return agentruntime.DomainCommitReconcileResult{}, fmt.Errorf("unsupported Game domain commit stage %q", request.Commit.Identity.Stage)
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
		return agentruntime.DomainCommitReconcileResult{}, err
	}
	return agentruntime.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func reconcileDirectorDomainCommit(request agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
	if request.Commit.Identity.Stage != agentruntime.DomainCommitOutput {
		return agentruntime.DomainCommitReconcileResult{}, nil
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
		return agentruntime.DomainCommitReconcileResult{}, err
	}
	return agentruntime.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func (a *App) reconcileStructuralDomainCommit(
	ctx context.Context,
	request agentruntime.DomainCommitReconcileRequest,
) (agentruntime.DomainCommitReconcileResult, error) {
	snapshot := request.Structural
	if snapshot == nil {
		return agentruntime.DomainCommitReconcileResult{}, nil
	}
	plan, err := agent.DecodeContextStructuralRestorePlan(
		snapshot.Ref.RestoreDescriptor,
		request.Binding,
		snapshot.Ref.ExpectedRevision,
	)
	if err != nil {
		return agentruntime.DomainCommitReconcileResult{}, fmt.Errorf("decode structural commit recovery plan: %w", err)
	}
	if request.Commit.Hash != plan.IntentHash {
		return agentruntime.DomainCommitReconcileResult{}, fmt.Errorf("structural canonical commit hash does not match frozen recovery plan")
	}
	operation, err := a.contextStructuralOperationForRestore(agent.HarnessStructuralRestoreRequest{
		Binding: request.Binding, Snapshot: *snapshot, Plan: plan,
	})
	if err != nil {
		return agentruntime.DomainCommitReconcileResult{}, err
	}
	_, receipt, found, err := operation.Reconcile(ctx)
	if err != nil || !found {
		return agentruntime.DomainCommitReconcileResult{}, err
	}
	return agentruntime.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}
