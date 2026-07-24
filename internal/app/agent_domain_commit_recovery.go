package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
)

// reconcileHarnessDomainCommit is the production host adapter for the
// cross-store crash window. Every branch is query-only and reports Found only
// after matching the exact durable identity and semantic hash available in the
// corresponding canonical store.
func (a *App) reconcileHarnessDomainCommit(
	ctx context.Context,
	request agents.DomainCommitReconcileRequest,
) (agents.DomainCommitReconcileResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return agents.DomainCommitReconcileResult{}, err
		}
	}
	if err := validateDomainCommitReconcileRequest(request); err != nil {
		return agents.DomainCommitReconcileResult{}, err
	}
	if request.Structural != nil {
		return a.reconcileStructuralDomainCommit(ctx, request)
	}
	binding := request.Binding
	switch binding.AgentKind {
	case agents.AgentKindIDE, agents.AgentKindConfigManager, agents.AgentKindImage, agents.AgentKindAutomation:
		return a.reconcileSessionDomainCommit(request)
	case agents.AgentKindInteractiveStory:
		return reconcileGameDomainCommit(request)
	case config.AgentKindInteractiveDirector:
		return reconcileDirectorDomainCommit(request)
	default:
		return agents.DomainCommitReconcileResult{}, fmt.Errorf("unsupported Agent kind %q for domain commit reconciliation", binding.AgentKind)
	}
}

func validateDomainCommitReconcileRequest(request agents.DomainCommitReconcileRequest) error {
	identity := request.Commit.Identity
	if strings.TrimSpace(string(identity.CommandID)) == "" || strings.TrimSpace(string(identity.OperationID)) == "" || identity.Cycle <= 0 {
		return fmt.Errorf("domain commit reconciliation requires a complete durable identity")
	}
	if identity.Stage != agents.DomainCommitInput && identity.Stage != agents.DomainCommitOutput {
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
		identity.Stage != agents.DomainCommitOutput {
		return fmt.Errorf("structural domain commit identity does not match its durable operation snapshot")
	}
	return nil
}

func (a *App) reconcileSessionDomainCommit(request agents.DomainCommitReconcileRequest) (agents.DomainCommitReconcileResult, error) {
	binding := request.Binding
	role, err := sessionRoleForDomainCommitStage(request.Commit.Identity.Stage)
	if err != nil {
		return agents.DomainCommitReconcileResult{}, err
	}
	dir, err := a.sessionDirectoryForBinding(request.Binding)
	if err != nil {
		return agents.DomainCommitReconcileResult{}, err
	}
	receipt, found, err := session.FindStoredDomainCommit(
		dir,
		binding.SessionID,
		session.DomainCommitIdentity{
			CommandID:   string(request.Commit.Identity.CommandID),
			OperationID: string(request.Commit.Identity.OperationID),
			Cycle:       request.Commit.Identity.Cycle,
		},
		role,
		request.Commit.Hash,
	)
	if err != nil || !found {
		return agents.DomainCommitReconcileResult{}, err
	}
	return agents.DomainCommitReconcileResult{
		Found: true, Revision: strconv.FormatUint(receipt.ContextRevision, 10),
	}, nil
}

func sessionRoleForDomainCommitStage(stage agents.DomainCommitStage) (agents.Role, error) {
	switch stage {
	case agents.DomainCommitInput:
		return agents.RoleUser, nil
	case agents.DomainCommitOutput:
		return agents.RoleAssistant, nil
	default:
		return "", fmt.Errorf("unsupported Session domain commit stage %q", stage)
	}
}

func (a *App) sessionDirectoryForBinding(binding agents.RuntimeBinding) (string, error) {
	if strings.TrimSpace(binding.SessionID) == "" {
		return "", fmt.Errorf("Session domain commit binding has no session id")
	}
	if binding.AgentKind == agents.AgentKindAutomation && strings.TrimSpace(binding.Workspace) == "" {
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

func reconcileGameDomainCommit(request agents.DomainCommitReconcileRequest) (agents.DomainCommitReconcileResult, error) {
	binding := request.Binding
	if request.Commit.Identity.Stage == agents.DomainCommitInput {
		receipt, found, err := interactive.NewStore(binding.Workspace).FindPlayerInputCommit(
			binding.StoryID,
			binding.BranchID,
			interactive.DomainCommitIdentity{
				CommandID: string(request.Commit.Identity.CommandID), OperationID: string(request.Commit.Identity.OperationID), Cycle: request.Commit.Identity.Cycle,
			},
			request.Commit.Hash,
		)
		if err != nil || !found {
			return agents.DomainCommitReconcileResult{}, err
		}
		return agents.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
	}
	if request.Commit.Identity.Stage != agents.DomainCommitOutput {
		return agents.DomainCommitReconcileResult{}, fmt.Errorf("unsupported Game domain commit stage %q", request.Commit.Identity.Stage)
	}
	receipt, found, err := interactive.NewStore(binding.Workspace).FindDomainTurnCommit(
		binding.StoryID,
		binding.BranchID,
		interactive.DomainCommitIdentity{
			CommandID:   string(request.Commit.Identity.CommandID),
			OperationID: string(request.Commit.Identity.OperationID),
			Cycle:       request.Commit.Identity.Cycle,
		},
		request.Commit.Hash,
	)
	if err != nil || !found {
		return agents.DomainCommitReconcileResult{}, err
	}
	return agents.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func reconcileDirectorDomainCommit(request agents.DomainCommitReconcileRequest) (agents.DomainCommitReconcileResult, error) {
	binding := request.Binding
	if request.Commit.Identity.Stage != agents.DomainCommitOutput {
		return agents.DomainCommitReconcileResult{}, nil
	}
	receipt, found, err := interactive.NewStore(binding.Workspace).FindDirectorPlanDomainCommit(
		binding.StoryID,
		binding.BranchID,
		interactive.DirectorPlanDomainCommitIdentity{
			CommandID:   string(request.Commit.Identity.CommandID),
			OperationID: string(request.Commit.Identity.OperationID),
			Cycle:       request.Commit.Identity.Cycle,
		},
		request.Commit.Hash,
	)
	if err != nil || !found {
		return agents.DomainCommitReconcileResult{}, err
	}
	return agents.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func (a *App) reconcileStructuralDomainCommit(
	ctx context.Context,
	request agents.DomainCommitReconcileRequest,
) (agents.DomainCommitReconcileResult, error) {
	snapshot := request.Structural
	if snapshot == nil {
		return agents.DomainCommitReconcileResult{}, nil
	}
	plan, err := agents.DecodeContextStructuralRestorePlan(
		snapshot.Ref.RestoreDescriptor,
		request.Binding,
		snapshot.Ref.ExpectedRevision,
	)
	if err != nil {
		return agents.DomainCommitReconcileResult{}, fmt.Errorf("decode structural commit recovery plan: %w", err)
	}
	if request.Commit.Hash != plan.IntentHash {
		return agents.DomainCommitReconcileResult{}, fmt.Errorf("structural canonical commit hash does not match frozen recovery plan")
	}
	operation, err := a.contextStructuralOperationForRestore(agents.HarnessStructuralRestoreRequest{
		Binding: request.Binding, Snapshot: *snapshot, Plan: plan,
	})
	if err != nil {
		return agents.DomainCommitReconcileResult{}, err
	}
	_, receipt, found, err := operation.Reconcile(ctx)
	if err != nil || !found {
		return agents.DomainCommitReconcileResult{}, err
	}
	return agents.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}
