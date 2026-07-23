package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
	"denova/internal/automation"
)

const maxRecoveredAutomationSummaryChars = 8 * 1024

type automationRuntimeProjectionFunc func(context.Context, *automationWorkspaceSnapshot, automation.Task, automation.RunRecord) (runstate.StatusSnapshot, error)

func (s *AutomationAppService) automationRuntimeProjection(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord) (runstate.StatusSnapshot, error) {
	if s.runtimeProjector != nil {
		return s.runtimeProjector(ctx, snap, task, run)
	}
	if snap == nil || snap.chatService == nil {
		return runstate.StatusSnapshot{}, agent.ErrRuntimeProjectionUnavailable
	}
	return snap.chatService.RuntimeRecoveryStatusProjection(ctx, agent.RunOptions{
		AgentKind: agent.AgentKindAutomation, TaskID: run.ID, AutomationTaskID: task.ID,
		SessionID: run.SessionID, Workspace: run.Workspace, Mode: "automation",
	})
}

// persistAcceptedAutomationRun closes the cross-domain admission window: the
// runtime command is already durable, so its receipt and a running RunRecord
// must become durable before trigger evaluation may be completed.
func persistAcceptedAutomationRun(snap *automationWorkspaceSnapshot, execution *automationAcceptedRun) error {
	if execution == nil || execution.accepted == nil {
		return fmt.Errorf("accepted automation execution is required")
	}
	receipt := execution.accepted.Receipt()
	if err := applyAutomationRootReceipt(&execution.run, receipt); err != nil {
		return fmt.Errorf("validate accepted automation run %s: %w", execution.run.ID, err)
	}
	if err := applyAutomationCurrentReceipt(&execution.run, receipt, automationRunAgentCommandID(execution.run.ID)); err != nil {
		return fmt.Errorf("persist accepted automation run %s current receipt: %w", execution.run.ID, err)
	}
	execution.run.Status = automation.RunStatusRunning
	execution.run.RuntimeAdmissionPending = false
	execution.run.RuntimeRecoveryRequired = false
	if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(execution.task), execution.run); err != nil {
		return fmt.Errorf("persist accepted automation run %s: %w", execution.run.ID, err)
	}
	return nil
}

// persistAutomationAdmissionIntent writes the initial command identity before
// Runtime.StartTurn can become durable. The exact runtime receipt replaces this
// intent after acceptance; if the process dies in between, recovery can prove
// whether Runtime accepted the command without relying on transport replay.
func persistAutomationAdmissionIntent(snap *automationWorkspaceSnapshot, task automation.Task, run *automation.RunRecord) error {
	if run == nil || strings.TrimSpace(run.ID) == "" {
		return fmt.Errorf("automation admission intent requires a run identity")
	}
	if runHasRuntimeReceiptForAdmission(*run) {
		return fmt.Errorf("automation admission intent %s already has a runtime receipt", run.ID)
	}
	run.Status = automation.RunStatusRunning
	run.RuntimeAdmissionPending = true
	run.RuntimeRecoveryRequired = false
	if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), *run); err != nil {
		return fmt.Errorf("persist automation admission intent %s: %w", run.ID, err)
	}
	return nil
}

func runHasRuntimeReceiptForAdmission(run automation.RunRecord) bool {
	return strings.TrimSpace(run.RuntimeCommandID) != "" && strings.TrimSpace(run.RuntimeOperationID) != "" && run.RuntimeReceiptCursor > 0
}

// reconcileAutomationRunReceipt projects the durable Agent journal for a
// deterministic run binding. It recovers both a missing running record (crash
// after StartTurn acceptance) and a stale running record (restart before the
// terminal RunRecord write) without submitting a second operation.
func (s *AutomationAppService) reconcileAutomationRunReceipt(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, candidate automation.RunRecord) (automation.RunRecord, bool, error) {
	status, err := s.automationRuntimeProjection(ctx, snap, task, candidate)
	if err != nil {
		if errors.Is(err, agent.ErrRuntimeProjectionUnavailable) {
			return automation.RunRecord{}, false, nil
		}
		return automation.RunRecord{}, false, err
	}
	match := automationRuntimeReceipt(status, candidate)
	commandID, operationID := match.commandID, match.operationID
	if operationID == "" {
		if candidate.RuntimeAdmissionPending && !runHasRuntimeReceiptForAdmission(candidate) {
			candidate.RuntimeAdmissionPending = false
			candidate.RuntimeRecoveryRequired = false
			candidate.Status = automation.RunStatusFailed
			candidate.Error = "Runtime 未接受自动化命令，可使用同一命令安全重试。 / Runtime did not accept the automation command; retrying the same command is safe."
			candidate.FinishedAt = time.Now().UTC()
			candidate.WriteConfirmationRequired = false
			candidate.WriteConfirmationPolicyCaptured = true
			candidate.CompletionEffectsPending = false
			candidate.CompletionEffectsCompleted = true
			if _, persistErr := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), candidate); persistErr != nil {
				return automation.RunRecord{}, false, fmt.Errorf("settle unaccepted automation admission %s: %w", candidate.ID, persistErr)
			}
			return candidate, false, nil
		}
		return automation.RunRecord{}, false, nil
	}
	pendingCommandID := strings.TrimSpace(candidate.PendingRuntimeCommandID)
	pendingReplayedCurrent := false
	if pendingCommandID == "" {
		if candidate.RuntimeOperationID != "" && candidate.RuntimeOperationID != operationID {
			return automation.RunRecord{}, false, fmt.Errorf("%w: run_id=%s runtime operation differs", automation.ErrRunIdentityConflict, candidate.ID)
		}
		if candidate.RuntimeCommandID != "" && commandID != "" && candidate.RuntimeCommandID != commandID {
			return automation.RunRecord{}, false, fmt.Errorf("%w: run_id=%s runtime command differs", automation.ErrRunIdentityConflict, candidate.ID)
		}
	} else if commandID != pendingCommandID || strings.TrimSpace(candidate.PendingRuntimeCommandFingerprint) == "" ||
		match.fingerprint != candidate.PendingRuntimeCommandFingerprint || match.receiptCursor == 0 {
		reason := fmt.Sprintf("pending successor runtime identity mismatch: command=%q fingerprint=%q receipt_cursor=%d", commandID, match.fingerprint, match.receiptCursor)
		candidate.PendingRuntimeCommandID = ""
		candidate.PendingRuntimeIntentHash = ""
		candidate.PendingRuntimeCommandFingerprint = ""
		candidate.RuntimeSuccessorConflict = reason
		if _, persistErr := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), candidate); persistErr != nil {
			return automation.RunRecord{}, false, errors.Join(fmt.Errorf("%w: run_id=%s %s", automation.ErrRunIdentityConflict, candidate.ID, reason), persistErr)
		}
		return candidate, false, fmt.Errorf("%w: run_id=%s %s", automation.ErrRunIdentityConflict, candidate.ID, reason)
	}
	if commandID == "" {
		return automation.RunRecord{}, false, fmt.Errorf("automation runtime operation %s has no durable command identity", operationID)
	}
	receiptCursor := match.receiptCursor
	if receiptCursor == 0 && pendingCommandID == "" {
		// Compatibility for journals projected before acceptance metadata was
		// added. Pending successor promotion never uses this fallback.
		receiptCursor = status.Cursor
	}
	receipt := runstate.Receipt{CommandID: runstate.CommandID(commandID), OperationID: runstate.OperationID(operationID), Cursor: receiptCursor}
	if candidate.RootRuntimeCommandID == "" && commandID == automationRunAgentCommandID(candidate.ID) {
		rootReceipt := receipt
		if legacy := automationRootReceipt(candidate); legacy.CommandID != "" {
			rootReceipt = legacy
		}
		if err := applyAutomationRootReceipt(&candidate, rootReceipt); err != nil {
			return automation.RunRecord{}, false, err
		}
		if err := applyAutomationCurrentReceipt(&candidate, receipt, automationRunAgentCommandID(candidate.ID)); err != nil {
			return automation.RunRecord{}, false, err
		}
	} else {
		if err := validateAutomationRunRootReceipt(candidate); err != nil {
			return automation.RunRecord{}, false, fmt.Errorf("reconcile automation run %s root receipt: %w", candidate.ID, err)
		}
		if pendingCommandID != "" {
			if candidate.RuntimeCommandID == commandID && candidate.RuntimeOperationID == operationID {
				pendingReplayedCurrent = true
				if uint64(receipt.Cursor) < candidate.RuntimeReceiptCursor {
					receipt.Cursor = runstate.Cursor(candidate.RuntimeReceiptCursor)
				}
				if err := applyAutomationCurrentReceipt(&candidate, receipt, pendingCommandID); err != nil {
					return automation.RunRecord{}, false, err
				}
			} else if err := advanceVerifiedAutomationCurrentReceipt(&candidate, receipt, pendingCommandID); err != nil {
				return automation.RunRecord{}, false, err
			}
			candidate.RuntimeCommandFingerprint = match.fingerprint
			candidate.RuntimeIntentHash = candidate.PendingRuntimeIntentHash
			candidate.PendingRuntimeCommandID = ""
			candidate.PendingRuntimeIntentHash = ""
			candidate.PendingRuntimeCommandFingerprint = ""
			candidate.RuntimeSuccessorConflict = ""
		} else {
			expectedCurrent := strings.TrimSpace(candidate.RuntimeCommandID)
			if expectedCurrent == "" {
				expectedCurrent = commandID
			}
			if candidate.RuntimeOperationID == operationID && uint64(receipt.Cursor) < candidate.RuntimeReceiptCursor {
				receipt.Cursor = runstate.Cursor(candidate.RuntimeReceiptCursor)
			}
			if err := applyAutomationCurrentReceipt(&candidate, receipt, expectedCurrent); err != nil {
				return automation.RunRecord{}, false, err
			}
		}
	}
	if pendingReplayedCurrent && !match.active && automationTerminalStatusMatches(candidate.Status, match.status) {
		candidate.RuntimeAdmissionPending = false
		candidate.RuntimeRecoveryRequired = false
		if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), candidate); err != nil {
			return automation.RunRecord{}, false, fmt.Errorf("persist replayed automation follow-up %s: %w", candidate.ID, err)
		}
		_, persisted, err := storeForSnapshot(snap).GetRunByID(candidate.ID)
		return persisted, err == nil, err
	}
	if match.active {
		candidate.RuntimeAdmissionPending = false
		candidate.Status = automation.RunStatusRunning
		candidate.FinishedAt = time.Time{}
		candidate.Error = ""
		candidate.RuntimeRecoveryRequired = true
	} else {
		candidate.RuntimeAdmissionPending = false
		candidate.RuntimeRecoveryRequired = false
		candidate.FinishedAt = time.Now().UTC()
		switch match.status {
		case runstate.OperationSucceeded:
			candidate.Status = automation.RunStatusSuccess
			candidate.Error = ""
			if summary := recoveredAutomationRunSummary(snap, candidate); summary != "" {
				candidate.Summary = summary
			} else if strings.TrimSpace(candidate.Summary) == "" {
				candidate.Summary = "自动化运行已从持久化运行时恢复。 / Automation run recovered from the durable runtime."
			}
		case runstate.OperationAborted:
			candidate.Status = automation.RunStatusAborted
			candidate.Error = recoveredAutomationRunError(match.reason)
		case runstate.OperationFailed, runstate.OperationInterrupted:
			candidate.Status = automation.RunStatusFailed
			candidate.Error = recoveredAutomationRunError(match.reason)
		default:
			return automation.RunRecord{}, false, fmt.Errorf("automation runtime operation %s has unsupported status %q", operationID, match.status)
		}
		if candidate.Status == automation.RunStatusSuccess {
			stageAutomationTerminalEffects(&candidate, candidate.CompletionMutationPaths)
			if candidate.RuntimeOperationID == candidate.RootRuntimeOperationID {
				if !candidate.WriteConfirmationPolicyCaptured {
					candidate.WriteConfirmationRequired = automationRunNeedsWriteConfirmation(task, candidate)
				}
			} else {
				candidate.WriteConfirmationRequired = false
			}
			candidate.WriteConfirmationPolicyCaptured = true
		} else {
			stageAutomationTerminalEffects(&candidate, candidate.CompletionMutationPaths)
		}
	}
	if _, err := storeForSnapshot(snap).AppendRun(automationTaskStoreID(task), candidate); err != nil {
		return automation.RunRecord{}, false, fmt.Errorf("persist reconciled automation run %s: %w", candidate.ID, err)
	}
	return candidate, true, nil
}

func automationTerminalStatusMatches(runStatus string, operationStatus runstate.OperationStatus) bool {
	switch operationStatus {
	case runstate.OperationSucceeded:
		return runStatus == automation.RunStatusSuccess
	case runstate.OperationAborted:
		return runStatus == automation.RunStatusAborted
	case runstate.OperationFailed, runstate.OperationInterrupted:
		return runStatus == automation.RunStatusFailed
	default:
		return false
	}
}

type automationRuntimeReceiptMatch struct {
	commandID     string
	operationID   string
	fingerprint   string
	receiptCursor runstate.Cursor
	status        runstate.OperationStatus
	reason        string
	active        bool
}

func automationRuntimeReceipt(status runstate.StatusSnapshot, candidate automation.RunRecord) automationRuntimeReceiptMatch {
	wanted := strings.TrimSpace(candidate.RuntimeOperationID)
	wantedCommand := strings.TrimSpace(candidate.RuntimeCommandID)
	pendingCommand := strings.TrimSpace(candidate.PendingRuntimeCommandID)
	if pendingCommand != "" {
		wanted = ""
		wantedCommand = pendingCommand
	}
	if wanted == "" && pendingCommand == "" {
		wanted = strings.TrimSpace(candidate.RootRuntimeOperationID)
	}
	if wantedCommand == "" {
		wantedCommand = strings.TrimSpace(candidate.RootRuntimeCommandID)
	}
	if wantedCommand == "" && strings.TrimSpace(candidate.ID) != "" {
		wantedCommand = automationRunAgentCommandID(candidate.ID)
	}
	matches := func(summary runstate.OperationSummary) bool {
		if wanted != "" {
			return string(summary.OperationID) == wanted
		}
		return wantedCommand == "" || string(summary.CommandID) == wantedCommand
	}
	if status.ActiveOperation != "" && matches(runstate.OperationSummary{OperationID: status.ActiveOperation, CommandID: status.ActiveCommandID}) {
		return automationRuntimeReceiptMatch{
			commandID: string(status.ActiveCommandID), operationID: string(status.ActiveOperation),
			fingerprint: status.ActiveCommandFingerprint, receiptCursor: status.ActiveReceiptCursor, active: true,
		}
	}
	if status.LastOperation != nil && matches(*status.LastOperation) {
		return automationRuntimeMatchFromSummary(*status.LastOperation)
	}
	for index := len(status.RecentOperations) - 1; index >= 0; index-- {
		operation := status.RecentOperations[index]
		if matches(operation) {
			return automationRuntimeMatchFromSummary(operation)
		}
	}
	return automationRuntimeReceiptMatch{}
}

func automationRuntimeMatchFromSummary(summary runstate.OperationSummary) automationRuntimeReceiptMatch {
	return automationRuntimeReceiptMatch{
		commandID: string(summary.CommandID), operationID: string(summary.OperationID),
		fingerprint: summary.CommandFingerprint, receiptCursor: summary.ReceiptCursor,
		status: summary.Status, reason: summary.Reason,
	}
}

func recoveredAutomationRunSummary(snap *automationWorkspaceSnapshot, run automation.RunRecord) string {
	if snap == nil || snap.sessionStore == nil || strings.TrimSpace(run.SessionID) == "" {
		return ""
	}
	sess, err := snap.sessionStore.Get(run.SessionID)
	if err != nil {
		return ""
	}
	history := sess.History()
	fallback := ""
	for index := len(history) - 1; index >= 0; index-- {
		entry := history[index]
		if strings.TrimSpace(entry.Role) != "assistant" || strings.TrimSpace(entry.Content) == "" {
			continue
		}
		content := trimForTriggerSnippet(entry.Content, maxRecoveredAutomationSummaryChars)
		if fallback == "" {
			fallback = content
		}
		if entry.AgentOperationID == "" || entry.AgentOperationID == run.RuntimeOperationID {
			return content
		}
	}
	return fallback
}

func recoveredAutomationRunError(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason != "" {
		return reason
	}
	return "自动化运行在恢复时未成功完成。 / Automation run did not complete successfully during recovery."
}
