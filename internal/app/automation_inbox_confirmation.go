package app

import (
	"context"
	apptask "denova/internal/app/task"
	"fmt"

	"denova/internal/automation"
)

type automationInboxRunStarter func(context.Context, string, string, string, string, []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error)

func (s *AutomationAppService) confirmInboxItemWithStarter(ctx context.Context, store *automation.Store, snap *automationWorkspaceSnapshot, id string, start automationInboxRunStarter) (automation.InboxActionResult, error) {
	item, err := store.GetInboxItem(id)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	task, err := store.Get(automation.CatalogTaskID(item.Scope, item.Workspace, item.TaskID))
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	runID, err := automation.InboxConfirmationRunID(item)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	claimed, _, err := store.ClaimInboxRun(ctx, item.ID, runID)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	if start == nil {
		return automation.InboxActionResult{}, fmt.Errorf("automation inbox run starter is required")
	}
	trigger := automation.TriggerInboxConfirmation
	sourceRunID := ""
	if claimed.Purpose == automation.InboxPurposeWriteConfirmation {
		trigger = automation.TriggerWriteConfirmation
		sourceRunID = claimed.SourceRunID
	}
	_, run, err := start(ctx, automationTaskStoreID(task), trigger, sourceRunID, runID, claimed.Evidence)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	if run.ID != runID {
		return automation.InboxActionResult{}, fmt.Errorf("automation inbox run identity changed: got %s want %s", run.ID, runID)
	}
	if err := validateAutomationRunRootReceipt(run); err != nil {
		return automation.InboxActionResult{}, fmt.Errorf("automation inbox run %s has no valid durable root receipt: %w", run.ID, err)
	}
	if run.RuntimeRecoveryRequired {
		return automation.InboxActionResult{}, fmt.Errorf("automation inbox run %s requires explicit runtime recovery", run.ID)
	}
	updated, err := store.CompleteInboxRun(ctx, claimed.ID, run.ID)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	return automation.InboxActionResult{Item: updated, Run: &run}, nil
}
