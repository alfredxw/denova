package automationapp

import (
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/automation"
)

// validateAutomationReceipt is the single admission-receipt validator. A
// durable command is usable as a completion barrier only when every identity
// component is present and the command is exactly the caller's expectation.
func validateAutomationReceipt(receipt agentrun.CommandReceipt, expectedCommandID string) error {
	expectedCommandID = strings.TrimSpace(expectedCommandID)
	if expectedCommandID == "" || string(receipt.CommandID) != expectedCommandID ||
		strings.TrimSpace(string(receipt.OperationID)) == "" || receipt.Cursor == 0 {
		return fmt.Errorf(
			"automation runtime receipt mismatch: command=%q expected=%q operation=%q cursor=%d",
			receipt.CommandID, expectedCommandID, receipt.OperationID, receipt.Cursor,
		)
	}
	return nil
}

func automationRootReceipt(run automation.RunRecord) agentrun.CommandReceipt {
	return agentrun.CommandReceipt{
		CommandID:   agentrun.CommandID(strings.TrimSpace(run.RootRuntimeCommandID)),
		OperationID: agentrun.OperationID(strings.TrimSpace(run.RootRuntimeOperationID)),
		Cursor:      agentrun.Cursor(run.RootRuntimeReceiptCursor),
	}
}

func validateAutomationRunRootReceipt(run automation.RunRecord) error {
	return validateAutomationReceipt(automationRootReceipt(run), automationRunAgentCommandID(run.ID))
}

func applyAutomationRootReceipt(run *automation.RunRecord, receipt agentrun.CommandReceipt) error {
	if run == nil {
		return fmt.Errorf("automation run is required")
	}
	expected := automationRunAgentCommandID(run.ID)
	if err := validateAutomationReceipt(receipt, expected); err != nil {
		return err
	}
	if existing := automationRootReceipt(*run); existing.CommandID != "" {
		if existing.CommandID != receipt.CommandID || existing.OperationID != receipt.OperationID || existing.Cursor != receipt.Cursor {
			return fmt.Errorf("%w: run_id=%s root runtime receipt changed", automation.ErrRunIdentityConflict, run.ID)
		}
	}
	run.RootRuntimeCommandID = string(receipt.CommandID)
	run.RootRuntimeOperationID = string(receipt.OperationID)
	run.RootRuntimeReceiptCursor = uint64(receipt.Cursor)
	return nil
}

func applyAutomationCurrentReceipt(run *automation.RunRecord, receipt agentrun.CommandReceipt, expectedCommandID string) error {
	if run == nil {
		return fmt.Errorf("automation run is required")
	}
	if err := validateAutomationReceipt(receipt, expectedCommandID); err != nil {
		return err
	}
	if currentCommandID := strings.TrimSpace(run.RuntimeCommandID); currentCommandID != "" {
		if currentCommandID != string(receipt.CommandID) || strings.TrimSpace(run.RuntimeOperationID) != string(receipt.OperationID) {
			return fmt.Errorf("%w: run_id=%s current runtime operation changed outside successor admission", automation.ErrRunIdentityConflict, run.ID)
		}
		if run.RuntimeReceiptCursor > uint64(receipt.Cursor) {
			return fmt.Errorf("%w: run_id=%s current runtime receipt cursor regressed", automation.ErrRunIdentityConflict, run.ID)
		}
	}
	run.RuntimeCommandID = string(receipt.CommandID)
	run.RuntimeOperationID = string(receipt.OperationID)
	run.RuntimeReceiptCursor = uint64(receipt.Cursor)
	return nil
}

// advanceAutomationCurrentReceipt is the only transition allowed to replace a
// current operation. The successor must be newer than the persisted receipt;
// reconciliation of the same operation continues through
// applyAutomationCurrentReceipt above.
func advanceAutomationCurrentReceipt(run *automation.RunRecord, receipt agentrun.CommandReceipt, expectedCommandID string) error {
	if run == nil {
		return fmt.Errorf("automation run is required")
	}
	if err := validateAutomationReceipt(receipt, expectedCommandID); err != nil {
		return err
	}
	if err := validateAutomationRunRootReceipt(*run); err != nil {
		return err
	}
	if strings.TrimSpace(run.RuntimeCommandID) != "" {
		if run.RuntimeCommandID == string(receipt.CommandID) || run.RuntimeOperationID == string(receipt.OperationID) {
			return fmt.Errorf("%w: run_id=%s successor receipt reuses current identity", automation.ErrRunIdentityConflict, run.ID)
		}
		if run.RuntimeReceiptCursor >= uint64(receipt.Cursor) {
			return fmt.Errorf("%w: run_id=%s successor receipt is not newer", automation.ErrRunIdentityConflict, run.ID)
		}
	}
	run.RuntimeCommandID = string(receipt.CommandID)
	run.RuntimeOperationID = string(receipt.OperationID)
	run.RuntimeReceiptCursor = uint64(receipt.Cursor)
	return nil
}

// applyAutomationFollowUpReceipt handles both sides of a cold idempotent retry:
// a replay of the already-current operation refreshes that same receipt, while
// a replay accepted before the RunRecord write promotes the pending successor.
func applyAutomationFollowUpReceipt(run *automation.RunRecord, receipt agentrun.CommandReceipt, expectedCommandID, fingerprint string) error {
	if run == nil {
		return fmt.Errorf("automation run is required")
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return fmt.Errorf("automation follow-up runtime fingerprint is required")
	}
	if receipt.Replayed && run.RuntimeCommandID == string(receipt.CommandID) && run.RuntimeOperationID == string(receipt.OperationID) {
		if current := strings.TrimSpace(run.RuntimeCommandFingerprint); current != "" && current != fingerprint {
			return fmt.Errorf("%w: run_id=%s replayed successor fingerprint changed", automation.ErrRunIdentityConflict, run.ID)
		}
		if err := applyAutomationCurrentReceipt(run, receipt, expectedCommandID); err != nil {
			return err
		}
		run.RuntimeCommandFingerprint = fingerprint
		return nil
	}
	if err := advanceAutomationCurrentReceipt(run, receipt, expectedCommandID); err != nil {
		return err
	}
	run.RuntimeCommandFingerprint = fingerprint
	return nil
}
