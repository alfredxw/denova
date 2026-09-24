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
