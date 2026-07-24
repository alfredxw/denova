package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func (h *chatHarness) waitForOperation(
	caller context.Context,
	harness *runstate.Harness,
	observation runstate.Observation,
	receipt runstate.Receipt,
	outcomes <-chan RunOutcome,
	emit func(Event),
) RunOutcome {
	var terminal *RunOutcome
	var settlement *runstate.OperationSettledEvent
	if receipt.Replayed {
		terminal, settlement = replayedTerminalFromSnapshot(observation.Snapshot, receipt)
		if terminal == nil && observation.Snapshot.ActiveOperation != receipt.OperationID {
			err := fmt.Errorf("durable outcome for replayed agent operation %s is no longer retained", receipt.OperationID)
			return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), "", "")
		}
	}
	callerDone := caller.Done()
	events := observation.Events
	errorsCh := observation.Errors
	abortSent := false

	for terminal == nil || settlement == nil {
		select {
		case outcome, ok := <-outcomes:
			if !ok {
				outcomes = nil
				continue
			}
			captured := outcome
			terminal = &captured
			outcomes = nil
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			switch payload := event.Payload.(type) {
			case runstate.OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					captured := payload
					settlement = &captured
				}
			case runstate.OperationInterruptedEvent:
				if payload.OperationID == receipt.OperationID {
					err := errors.New(payload.Reason)
					failed := outcomeFromOutput(RunOutcomeFailed, err, payload.Reason, "", "")
					terminal = &failed
					settled := runstate.OperationSettledEvent{OperationID: payload.OperationID, Status: runstate.OperationInterrupted}
					settlement = &settled
				}
			}
		case observationErr, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			if observationErr != nil {
				emitHarnessError(emit, observationErr)
				return outcomeFromOutput(RunOutcomeFailed, observationErr, observationErr.Error(), outputContent(terminal), outputThinking(terminal))
			}
		case <-callerDone:
			callerDone = nil
			if abortSent || settlement != nil {
				continue
			}
			abortSent = true
			reason := "agent caller canceled"
			if err := caller.Err(); err != nil {
				reason = err.Error()
			}
			_, err := harness.Submit(h.lifecycle, runstate.Abort{
				ID:          runstate.CommandID(newHarnessIdentity("command")),
				OperationID: receipt.OperationID,
				Reason:      reason,
			})
			if err != nil &&
				!errors.Is(err, runstate.ErrInvalidCommand) &&
				!errors.Is(err, runstate.ErrStaleOperation) &&
				!errors.Is(err, runstate.ErrDomainCommitRejected) {
				emitHarnessError(emit, err)
				return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), outputContent(terminal), outputThinking(terminal))
			}
		case <-h.lifecycle.Done():
			err := h.lifecycle.Err()
			if err == nil {
				err = runstate.ErrRuntimeClosed
			}
			return outcomeFromOutput(RunOutcomeAborted, err, err.Error(), outputContent(terminal), outputThinking(terminal))
		}
		if events == nil && errorsCh == nil && settlement == nil {
			err := fmt.Errorf("agent observation closed before operation %s settled", receipt.OperationID)
			emitHarnessError(emit, err)
			return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), outputContent(terminal), outputThinking(terminal))
		}
		if settlement != nil && terminal == nil {
			if receipt.Replayed {
				replayed := replayedOutcomeForSettlement(h.lifecycle, harness, receipt.OperationID, *settlement)
				terminal = &replayed
				continue
			}
			// harnessEngine publishes Outcome before returning EngineDone, so a
			// normal settlement always has a buffered value here.
			select {
			case outcome := <-outcomes:
				captured := outcome
				terminal = &captured
				outcomes = nil
			default:
				err := fmt.Errorf("agent operation %s settled without an engine outcome", receipt.OperationID)
				failed := outcomeFromOutput(RunOutcomeFailed, err, err.Error(), "", "")
				terminal = &failed
			}
		}
	}
	outcome := outcomeForSettlement(*terminal, settlement.Status)
	if settlement.Status == runstate.OperationSucceeded {
		if content, thinking, ok := settledAssistantOutput(h.lifecycle, harness, receipt.OperationID); ok {
			outcome.Content = content
			outcome.Thinking = thinking
		}
	}
	return outcome
}

func replayedTerminalFromSnapshot(
	snapshot runstate.StateSnapshot,
	receipt runstate.Receipt,
) (*RunOutcome, *runstate.OperationSettledEvent) {
	summary := replayedOperationSummary(snapshot, receipt)
	if summary == nil {
		return nil, nil
	}
	content, thinking := assistantOutputFromMessages(snapshot.Messages, receipt.OperationID)
	settlement := &runstate.OperationSettledEvent{
		OperationID: summary.OperationID, Status: summary.Status, Reason: summary.Reason,
	}
	outcome := outcomeFromOperationStatus(summary.Status, summary.Reason, content, thinking)
	return &outcome, settlement
}

func replayedOperationSummary(snapshot runstate.StateSnapshot, receipt runstate.Receipt) *runstate.OperationSummary {
	for index := len(snapshot.RecentOperations) - 1; index >= 0; index-- {
		summary := snapshot.RecentOperations[index]
		if summary.OperationID == receipt.OperationID && summary.CommandID == receipt.CommandID {
			return &summary
		}
	}
	if snapshot.LastOperation != nil && snapshot.LastOperation.OperationID == receipt.OperationID && snapshot.LastOperation.CommandID == receipt.CommandID {
		return snapshot.LastOperation
	}
	return nil
}

func replayedOutcomeForSettlement(
	ctx context.Context,
	harness *runstate.Harness,
	operationID runstate.OperationID,
	settlement runstate.OperationSettledEvent,
) RunOutcome {
	content, thinking, _ := settledAssistantOutput(ctx, harness, operationID)
	return outcomeFromOperationStatus(settlement.Status, settlement.Reason, content, thinking)
}

func outcomeFromOperationStatus(status runstate.OperationStatus, reason, content, thinking string) RunOutcome {
	switch status {
	case runstate.OperationSucceeded:
		return outcomeFromOutput(RunOutcomeCompleted, nil, "", content, thinking)
	case runstate.OperationAborted:
		return outcomeFromOutput(RunOutcomeAborted, nil, strings.TrimSpace(reason), content, thinking)
	case runstate.OperationFailed, runstate.OperationInterrupted:
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "agent operation failed"
		}
		return outcomeFromOutput(RunOutcomeFailed, errors.New(reason), reason, content, thinking)
	default:
		err := fmt.Errorf("unsupported agent operation status %q", status)
		return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), content, thinking)
	}
}

func assistantOutputFromMessages(messages []runstate.Message, operationID runstate.OperationID) (string, string) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Operation == operationID && message.Role == runstate.RoleAssistant {
			return message.Content, message.Thinking
		}
	}
	return "", ""
}

func outcomeForSettlement(outcome RunOutcome, status runstate.OperationStatus) RunOutcome {
	switch status {
	case runstate.OperationSucceeded:
		outcome.Status = RunOutcomeCompleted
		outcome.Error = nil
		outcome.Reason = ""
		return outcome
	case runstate.OperationAborted:
		outcome.Status = RunOutcomeAborted
		outcome.Error = nil
		return outcome
	case runstate.OperationFailed, runstate.OperationInterrupted:
		if outcome.Error == nil {
			reason := strings.TrimSpace(outcome.Reason)
			if reason == "" {
				reason = "agent operation failed"
			}
			outcome.Error = errors.New(reason)
		}
		outcome.Status = RunOutcomeFailed
		return outcome
	default:
		err := fmt.Errorf("unsupported agent operation status %q", status)
		return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), outcome.Content, outcome.Thinking)
	}
}

func settledAssistantOutput(ctx context.Context, harness *runstate.Harness, operationID runstate.OperationID) (string, string, bool) {
	if harness == nil {
		return "", "", false
	}
	observationCtx, cancel := context.WithCancel(ctx)
	observation, err := harness.ObserveFromNow(observationCtx)
	if err != nil {
		cancel()
		return "", "", false
	}
	cancel()
	for index := len(observation.Snapshot.Messages) - 1; index >= 0; index-- {
		message := observation.Snapshot.Messages[index]
		if message.Operation == operationID && message.Role == runstate.RoleAssistant {
			return message.Content, message.Thinking, true
		}
	}
	return "", "", false
}

func emitHarnessError(emit func(Event), err error) {
	if emit != nil && err != nil {
		emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
	}
}

func outputContent(outcome *RunOutcome) string {
	if outcome == nil {
		return ""
	}
	return outcome.Content
}

func outputThinking(outcome *RunOutcome) string {
	if outcome == nil {
		return ""
	}
	return outcome.Thinking
}
