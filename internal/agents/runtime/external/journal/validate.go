package externaljournal

import "errors"

// Validate checks only index structure. Locators are still resolved against
// canonical records before any answer or recovery decision consumes content.
func (projection *Projection) Validate() error {
	commands, executions := map[string]bool{}, map[string]bool{}
	running := 0
	for id, operation := range projection.Operations {
		if operation == nil || id == "" || operation.ID != id || operation.CommandID == "" || commands[operation.CommandID] || operation.Fingerprint == "" || operation.ConfigRevision == 0 || !validLocator(operation.Accepted) {
			return errors.New("invalid external operation index")
		}
		commands[operation.CommandID] = true
		switch operation.Status {
		case Running:
			running++
			if operation.Closed != nil {
				return errors.New("running external operation has a close locator")
			}
		case Completed, Failed, Cancelled, Interrupted:
			if operation.Closed == nil || !validLocator(*operation.Closed) {
				return errors.New("closed external operation has no canonical locator")
			}
		default:
			return errors.New("unknown external operation status")
		}
		for executionID, tool := range operation.Tools {
			if executionID == "" || executions[executionID] || tool.Name == "" || !validLocator(tool.Started) {
				return errors.New("invalid external tool index")
			}
			executions[executionID] = true
			switch tool.Recovery {
			case ReadOnly, ReceiptVerifiable, NonReplayable:
			default:
				return errors.New("unknown external tool recovery class")
			}
			if tool.Finished != nil && !validLocator(*tool.Finished) {
				return errors.New("invalid external tool result locator")
			}
			if tool.Finished == nil && operation.Status != Running && operation.Status != Interrupted {
				return errors.New("settled external operation has an unknown tool result")
			}
		}
	}
	if running > 1 {
		return errors.New("multiple external operations are active")
	}
	if projection.Checkpoint != nil && !validLocator(*projection.Checkpoint) {
		return errors.New("invalid external checkpoint locator")
	}
	return nil
}

func validLocator(locator Locator) bool { return locator.Cursor > 0 && locator.Index >= 0 }
