package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"denova/internal/agentruntime"
)

func (h *chatHarness) waitForOperation(
	caller context.Context,
	harness *agentruntime.Harness,
	observation agentruntime.Observation,
	receipt agentruntime.Receipt,
	outcomes <-chan RunOutcome,
	emit func(Event),
) RunOutcome {
	var terminal *RunOutcome
	var settlement *agentruntime.OperationSettledEvent
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
			case agentruntime.OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					captured := payload
					settlement = &captured
				}
			case agentruntime.OperationInterruptedEvent:
				if payload.OperationID == receipt.OperationID {
					err := errors.New(payload.Reason)
					failed := outcomeFromOutput(RunOutcomeFailed, err, payload.Reason, "", "")
					terminal = &failed
					settled := agentruntime.OperationSettledEvent{OperationID: payload.OperationID, Status: agentruntime.OperationInterrupted}
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
			_, err := harness.Submit(h.lifecycle, agentruntime.Abort{
				ID:          agentruntime.CommandID(newHarnessIdentity("command")),
				OperationID: receipt.OperationID,
				Reason:      reason,
			})
			if err != nil &&
				!errors.Is(err, agentruntime.ErrInvalidCommand) &&
				!errors.Is(err, agentruntime.ErrStaleOperation) &&
				!errors.Is(err, agentruntime.ErrDomainCommitRejected) {
				emitHarnessError(emit, err)
				return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), outputContent(terminal), outputThinking(terminal))
			}
		case <-h.lifecycle.Done():
			err := h.lifecycle.Err()
			if err == nil {
				err = agentruntime.ErrRuntimeClosed
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
	if settlement.Status == agentruntime.OperationSucceeded {
		if content, thinking, ok := settledAssistantOutput(h.lifecycle, harness, receipt.OperationID); ok {
			outcome.Content = content
			outcome.Thinking = thinking
		}
	}
	return outcome
}

func replayedTerminalFromSnapshot(
	snapshot agentruntime.StateSnapshot,
	receipt agentruntime.Receipt,
) (*RunOutcome, *agentruntime.OperationSettledEvent) {
	summary := replayedOperationSummary(snapshot, receipt)
	if summary == nil {
		return nil, nil
	}
	content, thinking := assistantOutputFromMessages(snapshot.Messages, receipt.OperationID)
	settlement := &agentruntime.OperationSettledEvent{
		OperationID: summary.OperationID, Status: summary.Status, Reason: summary.Reason,
	}
	outcome := outcomeFromOperationStatus(summary.Status, summary.Reason, content, thinking)
	return &outcome, settlement
}

func replayedOperationSummary(snapshot agentruntime.StateSnapshot, receipt agentruntime.Receipt) *agentruntime.OperationSummary {
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
	harness *agentruntime.Harness,
	operationID agentruntime.OperationID,
	settlement agentruntime.OperationSettledEvent,
) RunOutcome {
	content, thinking, _ := settledAssistantOutput(ctx, harness, operationID)
	return outcomeFromOperationStatus(settlement.Status, settlement.Reason, content, thinking)
}

func outcomeFromOperationStatus(status agentruntime.OperationStatus, reason, content, thinking string) RunOutcome {
	switch status {
	case agentruntime.OperationSucceeded:
		return outcomeFromOutput(RunOutcomeCompleted, nil, "", content, thinking)
	case agentruntime.OperationAborted:
		return outcomeFromOutput(RunOutcomeAborted, nil, strings.TrimSpace(reason), content, thinking)
	case agentruntime.OperationFailed, agentruntime.OperationInterrupted:
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

func assistantOutputFromMessages(messages []agentruntime.Message, operationID agentruntime.OperationID) (string, string) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Operation == operationID && message.Role == agentruntime.RoleAssistant {
			return message.Content, message.Thinking
		}
	}
	return "", ""
}

func outcomeForSettlement(outcome RunOutcome, status agentruntime.OperationStatus) RunOutcome {
	switch status {
	case agentruntime.OperationSucceeded:
		outcome.Status = RunOutcomeCompleted
		outcome.Error = nil
		outcome.Reason = ""
		return outcome
	case agentruntime.OperationAborted:
		outcome.Status = RunOutcomeAborted
		outcome.Error = nil
		return outcome
	case agentruntime.OperationFailed, agentruntime.OperationInterrupted:
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

func settledAssistantOutput(ctx context.Context, harness *agentruntime.Harness, operationID agentruntime.OperationID) (string, string, bool) {
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
		if message.Operation == operationID && message.Role == agentruntime.RoleAssistant {
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
