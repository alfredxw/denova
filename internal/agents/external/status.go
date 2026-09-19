package external

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"denova/config"
	"denova/internal/agents/conversation"
	externaljournal "denova/internal/agents/external/journal"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

// Status projects the same product control vocabulary without creating a
// Native Run. The bool reports ownership, independently of projection errors.
func (service *Service) Status(ctx context.Context, projectID string, sess *session.Session) (agentrun.RuntimeStatus, bool, error) {
	view := agentrun.RuntimeStatus{Phase: agentrun.RunPhaseIdle}
	if sess == nil {
		return view, false, nil
	}
	external := false
	if err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		external = state.Config.Engine().Kind != config.RuntimeNative
		return nil
	}); err != nil {
		return view, external, err
	}
	if !external {
		return view, false, nil
	}
	if err := service.Recover(ctx, projectID, sess); err != nil {
		return view, true, err
	}
	err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		view.Cursor = agentrun.Cursor(state.Cursor)
		operations := make([]*externaljournal.Operation, 0, len(state.Projection.Operations))
		for _, operation := range state.Projection.Operations {
			operations = append(operations, operation)
		}
		sort.Slice(operations, func(i, j int) bool { return operations[i].Accepted.Cursor < operations[j].Accepted.Cursor })
		if len(operations) == 0 {
			return nil
		}
		last := operations[len(operations)-1]
		if last.Status == externaljournal.Running {
			view.Phase = agentrun.RunPhaseRunning
			view.ActiveCycle = 1
			view.ActiveOperation, view.ActiveCommandID = agentrun.OperationID(last.ID), agentrun.CommandID(last.CommandID)
			view.ActiveCommandFingerprint, view.ActiveReceiptCursor = last.Fingerprint, agentrun.Cursor(last.Accepted.Cursor)
		} else {
			status := agentrun.OperationFailed
			switch last.Status {
			case externaljournal.Completed:
				status = agentrun.OperationSucceeded
			case externaljournal.Cancelled:
				status = agentrun.OperationAborted
			case externaljournal.Interrupted:
				status = agentrun.OperationInterrupted
			case externaljournal.Failed:
			default:
				return errors.New("invalid external operation status")
			}
			view.LastOperation = &agentrun.OperationSummary{OperationID: agentrun.OperationID(last.ID), CommandID: agentrun.CommandID(last.CommandID), CommandFingerprint: last.Fingerprint, ReceiptCursor: agentrun.Cursor(last.Accepted.Cursor), Status: status}
		}
		for _, operation := range operations {
			for executionID, tool := range operation.Tools {
				if tool.Finished != nil {
					continue
				}
				view.OpenToolCalls = append(view.OpenToolCalls, agentrun.OpenToolCall{CallID: executionID, Name: tool.Name, OperationID: agentrun.OperationID(operation.ID)})
				if tool.Name != "ask" {
					continue
				}
				record, err := state.Read(tool.Started)
				if err != nil {
					return err
				}
				var started externaljournal.StartedTool
				if err := json.Unmarshal(record.Data, &started); err != nil {
					return err
				}
				question, err := QuestionRequest(executionID, started.Arguments)
				if err != nil {
					return err
				}
				view.PendingInteractions = append(view.PendingInteractions, question)
			}
		}
		return nil
	})
	return view, true, err
}

// ResolveAsk falls through only when the original Ask identity is not in this
// journal lane. An existing conflicting answer must never reach another engine.
func (service *Service) ResolveAsk(ctx context.Context, projectID string, sess *session.Session, askID, status string, answers []conversation.HostAskAnswer, reason string) (conversation.HostAskResolution, bool, error) {
	var cancel *string
	switch status {
	case session.AskAnswered:
	case session.AskCancelled:
		cancel = &reason
	default:
		return conversation.HostAskResolution{}, true, errors.New("invalid question resolution status")
	}
	result, err := service.Interactions.Resolve(ctx, projectID, sess, askID, answers, cancel)
	return result, !errors.Is(err, ErrAskNotFound), err
}
