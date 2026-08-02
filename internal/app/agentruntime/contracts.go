package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentstructural "denova/internal/agents/context/structural"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
)

var (
	ErrNoWorkspace         = errors.New("no workspace is selected / 尚未选择工作区")
	ErrOperationActive     = errors.New("agent operation is already active")
	ErrNoActiveOperation   = errors.New("no active agent operation")
	ErrContextChanged      = errors.New("agent start context changed")
	ErrWorkspaceTransition = errors.New("workspace runtime is transitioning")
)

// Command is the application-level command contract shared by every
// conversation surface. Binding identity stays with the owning service.
type Command struct {
	Kind            agentharness.CommandKind
	CommandID       string
	OperationID     agentrun.OperationID
	TargetCommandID agentrun.CommandID
	Reason          string
	Input           agentchat.ChatRequest
}

// RecoveryRequest carries the exact durable action selected by the caller.
// Story and branch scope are used only by the interactive owner.
type RecoveryRequest struct {
	Action   agentharness.RuntimeRecoveryAction
	StoryID  string
	BranchID string
}

type RecoveryResult struct {
	Task    *apptask.Task
	Action  agentharness.RuntimeRecoveryAction
	Receipt agentrun.CommandReceipt
}

func RecoveryActionKey(action agentharness.RuntimeRecoveryAction) string {
	return strings.Join([]string{string(action.Kind), string(action.CommandID), string(action.OperationID)}, "\x00")
}

func ValidateRecoveryAction(status agentrun.RuntimeStatus, selected agentharness.RuntimeRecoveryAction) error {
	for _, action := range agentharness.RuntimeRecoveryActions(status) {
		if action == selected {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: kind=%q command_id=%q operation_id=%q",
		agentharness.ErrRecoveryActionChanged,
		selected.Kind,
		selected.CommandID,
		selected.OperationID,
	)
}

func StructuralRecoveryAction(kind agentharness.RuntimeRecoveryActionKind) (agentstructural.Action, bool) {
	switch kind {
	case agentharness.RuntimeRecoveryCompactContext:
		return agentstructural.Compact, true
	case agentharness.RuntimeRecoveryRemoveCompaction:
		return agentstructural.Remove, true
	default:
		return "", false
	}
}

// RuntimeProjection reads the durable runtime without turning an unavailable
// process-local projection into a user operation failure.
func RuntimeProjection(ctx context.Context, service *agentharness.Service, options agentrun.Options) (agentrun.RuntimeStatus, bool) {
	if service == nil {
		return agentrun.RuntimeStatus{}, false
	}
	snapshot, err := service.RuntimeRecoveryStatusProjection(ctx, options)
	if err == nil {
		return snapshot, true
	}
	if !errors.Is(err, agentharness.ErrRuntimeProjectionUnavailable) {
		slog.WarnContext(ctx, fmt.Sprintf(
			"[app/agentruntime] runtime projection unavailable kind=%s workspace=%s session_id=%s story_id=%s branch_id=%s err=%v",
			options.AgentKind, options.Workspace, options.SessionID, options.StoryID, options.BranchID, err,
		))
	}
	return agentrun.RuntimeStatus{}, false
}

// FinishedRecoveryActionStillCurrent distinguishes an idempotent replay of a
// settled action from a retry whose earlier display task failed before the
// durable state changed.
func FinishedRecoveryActionStillCurrent(
	ctx context.Context,
	task *apptask.Task,
	recovery *agentharness.RecoveryObservation,
	action agentharness.RuntimeRecoveryAction,
) (bool, error) {
	if task == nil || recovery == nil || !task.Finished() {
		return false, nil
	}
	status, err := recovery.CurrentStatus(ctx)
	if err != nil {
		return false, err
	}
	for _, current := range agentharness.RuntimeRecoveryActions(status) {
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

func CloseBindings(service *agentharness.Service, close func(*agentharness.Service) error) error {
	if service == nil {
		return nil
	}
	err := close(service)
	if err == nil || errors.Is(err, agentharness.ErrRuntimeProjectionUnavailable) {
		return nil
	}
	return fmt.Errorf("close Agent runtime binding: %w", err)
}
