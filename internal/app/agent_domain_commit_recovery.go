package app

import (
	"context"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
	compactionapp "denova/internal/app/compaction"
	"denova/internal/book"
	"denova/internal/interactive"
)

func validateDomainCommitReconcileRequest(request agentrun.DomainCommitReconcileRequest) error {
	identity := request.Commit.Identity
	if strings.TrimSpace(string(identity.CommandID)) == "" || strings.TrimSpace(string(identity.OperationID)) == "" || identity.Cycle <= 0 {
		return fmt.Errorf("domain commit reconciliation requires a complete durable identity")
	}
	if identity.Stage != agentrun.DomainCommitInput && identity.Stage != agentrun.DomainCommitOutput {
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
		identity.Stage != agentrun.DomainCommitOutput {
		return fmt.Errorf("structural domain commit identity does not match its durable operation snapshot")
	}
	return nil
}

func (a *App) reconcileSessionDomainCommit(request agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error) {
	binding := request.Binding
	role, err := sessionRoleForDomainCommitStage(request.Commit.Identity.Stage)
	if err != nil {
		return agentrun.DomainCommitReconcileResult{}, err
	}
	dir, err := a.sessionDirectoryForBinding(request.Binding)
	if err != nil {
		return agentrun.DomainCommitReconcileResult{}, err
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
		return agentrun.DomainCommitReconcileResult{}, err
	}
	return agentrun.DomainCommitReconcileResult{
		Found: true, Revision: strconv.FormatUint(receipt.ContextRevision, 10),
	}, nil
}

func sessionRoleForDomainCommitStage(stage agentrun.DomainCommitStage) (agents.Role, error) {
	switch stage {
	case agentrun.DomainCommitInput:
		return agents.RoleUser, nil
	case agentrun.DomainCommitOutput:
		return agents.RoleAssistant, nil
	default:
		return "", fmt.Errorf("unsupported Session domain commit stage %q", stage)
	}
}

func (a *App) sessionDirectoryForBinding(binding agentrun.RuntimeBinding) (string, error) {
	if strings.TrimSpace(binding.SessionID) == "" {
		return "", fmt.Errorf("Session domain commit binding has no session id")
	}
	if binding.AgentKind == agentrun.AgentKindAutomation && strings.TrimSpace(binding.Workspace) == "" {
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
		return "", fmt.Errorf("Session domain commit binding has no workspace")
	}
	return book.NewState(workspace).SessionDir(), nil
}

func reconcileGameDomainCommit(request agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error) {
	binding := request.Binding
	if request.Commit.Identity.Stage == agentrun.DomainCommitInput {
		receipt, found, err := interactive.NewStore(binding.Workspace).FindRecentPlayerInputCommit(
			binding.StoryID,
			binding.BranchID,
			interactive.DomainCommitIdentity{
				CommandID: string(request.Commit.Identity.CommandID), OperationID: string(request.Commit.Identity.OperationID), Cycle: request.Commit.Identity.Cycle,
			},
			request.Commit.Hash,
		)
		if err != nil || !found {
			return agentrun.DomainCommitReconcileResult{}, err
		}
		return agentrun.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
	}
	if request.Commit.Identity.Stage != agentrun.DomainCommitOutput {
		return agentrun.DomainCommitReconcileResult{}, fmt.Errorf("unsupported Game domain commit stage %q", request.Commit.Identity.Stage)
	}
	receipt, found, err := interactive.NewStore(binding.Workspace).FindRecentDomainTurnCommit(
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
		return agentrun.DomainCommitReconcileResult{}, err
	}
	return agentrun.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func reconcileDirectorDomainCommit(request agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error) {
	binding := request.Binding
	if request.Commit.Identity.Stage != agentrun.DomainCommitOutput {
		return agentrun.DomainCommitReconcileResult{}, nil
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
		return agentrun.DomainCommitReconcileResult{}, err
	}
	return agentrun.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func (a *App) reconcileStructuralDomainCommit(
	ctx context.Context,
	request agentrun.DomainCommitReconcileRequest,
) (agentrun.DomainCommitReconcileResult, error) {
	snapshot := request.Structural
	if snapshot == nil {
		return agentrun.DomainCommitReconcileResult{}, nil
	}
	plan, err := agentstructural.DecodeRestorePlan(
		snapshot.Ref.RestoreDescriptor,
		request.Binding,
		snapshot.Ref.ExpectedRevision,
	)
	if err != nil {
		return agentrun.DomainCommitReconcileResult{}, fmt.Errorf("decode structural commit recovery plan: %w", err)
	}
	if request.Commit.Hash != plan.IntentHash {
		return agentrun.DomainCommitReconcileResult{}, fmt.Errorf("structural canonical commit hash does not match frozen recovery plan")
	}
	operation, err := compactionapp.RestoreOperation(agentexecution.StructuralRestoreRequest{
		Binding: request.Binding, Snapshot: *snapshot, Plan: plan,
	}, a.sessionDirectoryForBinding)
	if err != nil {
		return agentrun.DomainCommitReconcileResult{}, err
	}
	_, receipt, found, err := operation.Reconcile(ctx)
	if err != nil || !found {
		return agentrun.DomainCommitReconcileResult{}, err
	}
	return agentrun.DomainCommitReconcileResult{Found: true, Revision: receipt.Revision}, nil
}
