package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
)

var (
	ErrNoWorkspace         = errors.New("no workspace is selected / 尚未选择工作区")
	ErrNoActiveOperation   = errors.New("no active agent operation")
	ErrContextChanged      = errors.New("agent start context changed")
	ErrWorkspaceTransition = errors.New("workspace runtime is transitioning")
)

// RecoveryRequest carries the exact live attachment selected by the caller.
// Story and branch scope are used only by the interactive owner.
type RecoveryRequest struct {
	Action   agentexecution.RuntimeRecoveryAction
	StoryID  string
	BranchID string
}

type RecoveryResult struct {
	Task    *apptask.Task
	Action  agentexecution.RuntimeRecoveryAction
	Receipt agentrun.CommandReceipt
}

// RuntimeProjection reads live Run state without turning an unavailable
// process-local projection into a user operation failure.
func RuntimeProjection(ctx context.Context, service *agentexecution.Runtime, options agentrun.Options) (agentrun.RuntimeStatus, bool) {
	if service == nil {
		return agentrun.RuntimeStatus{}, false
	}
	snapshot, err := service.RuntimeStatusProjection(ctx, options)
	if err == nil {
		return snapshot, true
	}
	if !errors.Is(err, agentexecution.ErrRuntimeProjectionUnavailable) {
		slog.WarnContext(ctx, fmt.Sprintf(
			"[app/agentruntime] runtime projection unavailable kind=%s workspace=%s session_id=%s story_id=%s branch_id=%s err=%v",
			options.AgentKind, options.Workspace, options.SessionID, options.StoryID, options.BranchID, err,
		))
	}
	return agentrun.RuntimeStatus{}, false
}

// FinishedRecoveryActionStillCurrent distinguishes a repeated attachment to a
// settled display task from a retry whose earlier display task failed while
// the Run remained active.
func FinishedRecoveryActionStillCurrent(
	ctx context.Context,
	task *apptask.Task,
	recovery *agentexecution.RecoveryObservation,
	action agentexecution.RuntimeRecoveryAction,
) (bool, error) {
	if task == nil || recovery == nil || !task.Finished() {
		return false, nil
	}
	status, err := recovery.CurrentStatus(ctx)
	if err != nil {
		return false, err
	}
	for _, current := range agentexecution.RuntimeRecoveryActions(status) {
		if current == action {
			return true, nil
		}
	}
	return false, nil
}

func StatusOwnsCommand(status agentrun.RuntimeStatus, commandID string) bool {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return false
	}
	if string(status.ActiveCommandID) == commandID {
		return true
	}
	if status.LastOperation != nil && string(status.LastOperation.CommandID) == commandID {
		return true
	}
	for index := len(status.RecentOperations) - 1; index >= 0; index-- {
		if string(status.RecentOperations[index].CommandID) == commandID {
			return true
		}
	}
	return false
}

func AbortAndWait(ctx context.Context, task *apptask.Task) error {
	if task == nil {
		return nil
	}
	task.Abort()
	select {
	case <-task.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func CloseBindings(service *agentexecution.Runtime, close func(*agentexecution.Runtime) error) error {
	if service == nil {
		return nil
	}
	err := close(service)
	if err == nil || errors.Is(err, agentexecution.ErrRuntimeProjectionUnavailable) {
		return nil
	}
	return fmt.Errorf("close Agent runtime binding: %w", err)
}
