package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"denova/config"
	externaljournal "denova/internal/agents/external/journal"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	"denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
	workspacechange "denova/internal/workspace/change"
	agent "github.com/alfredxw/denova/agent"
)

// Reconcile reads original domain receipts for abandoned operations. It never
// invokes a tool or treats a missing receipt as evidence that a write failed.
// Call after Recover, under product admission, before preparing new history.
func Reconcile(ctx context.Context, sess *session.Session, workspace, stateRoot string) error {
	type pending struct {
		operation string
		revision  uint64
		started   externaljournal.StartedTool
	}
	var calls []pending
	var revision uint64
	var incarnation string
	if err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		revision, incarnation = state.Config.Revision, state.Incarnation
		for _, operation := range state.Projection.Operations {
			if operation.Status != externaljournal.Interrupted {
				continue
			}
			for _, tool := range operation.Tools {
				if tool.Finished != nil || tool.Name == "ask" {
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
				calls = append(calls, pending{operation.ID, operation.ConfigRevision, started})
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, call := range calls {
		finished := externaljournal.FinishedTool{ExecutionID: call.started.ExecutionID}
		switch call.started.Recovery {
		case externaljournal.ReadOnly:
			finished.Result = "The previous read was interrupted before its result was saved. Request a fresh read if this information is still needed."
		case externaljournal.ReceiptVerifiable:
			if call.started.Tool != "write" && call.started.Tool != "edit" {
				continue
			}
			changes, err := workspacechange.ForWorkspaceAt(workspace, stateRoot)
			if err != nil {
				return err
			}
			group, err := changes.GetGroup(ctx, call.operation)
			if err != nil {
				var domainErr *workspacechange.Error
				if errors.As(err, &domainErr) && domainErr.Code == workspacechange.ErrorCodeNotFound {
					continue
				}
				return err
			}
			var matches []workspacechange.ChangeSet
			for _, change := range group.ChangeSets {
				if change.ToolCallID == call.started.ExecutionID && change.RunID == call.operation && change.SessionID == sess.ID {
					matches = append(matches, change)
				}
			}
			// write/edit commit one change set per invocation. An ambiguous or
			// partial multi-action receipt cannot establish a completed tool.
			if len(matches) != 1 || matches[0].ApplyState != workspacechange.ApplyStateApplied {
				continue
			}
			finished, err = recoveredWrite(call.started, matches[0])
			if err != nil {
				return err
			}
		case externaljournal.NonReplayable:
			continue
		}
		record, err := externaljournal.NewRecord(externaljournal.ToolFinished, call.operation, call.revision, finished)
		if err != nil {
			return err
		}
		if err := sess.UpdateExternal(ctx, revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
			if state.Incarnation != incarnation {
				return session.ExternalTransaction{}, session.ErrContextRevisionConflict
			}
			operation := state.Projection.Operations[call.operation]
			if operation == nil || operation.Status != externaljournal.Interrupted {
				return session.ExternalTransaction{}, session.ErrContextRevisionConflict
			}
			tool, found := operation.Tools[call.started.ExecutionID]
			if !found {
				return session.ExternalTransaction{}, session.ErrContextRevisionConflict
			}
			if tool.Finished != nil {
				return session.ExternalTransaction{}, nil
			}
			return session.ExternalTransaction{Records: []externaljournal.Record{record}}, nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func recoveredWrite(started externaljournal.StartedTool, change workspacechange.ChangeSet) (externaljournal.FinishedTool, error) {
	body, err := workspacechange.MarshalToolReceipt(change)
	if err != nil {
		return externaljournal.FinishedTool{}, err
	}
	effect, present, err := toolruntime.AgentToolMutationEffect(agenttool.ExecutionRecord{
		ToolName: started.Tool, ExecutionID: started.ExecutionID, Status: "success", Target: change.Path,
		ChangeGroupID: change.GroupID, ReviewThreadID: change.ReviewThreadID, ChangeSetID: change.ID,
		BaseRevision: change.BaseRevision, Revision: change.Revision, ReviewStatus: change.ReviewStatus, ApplyState: change.ApplyState,
		MutationReceiptSchema: workspacechange.ToolResultSchema,
		Descriptor:            producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable),
	})
	if err != nil {
		return externaljournal.FinishedTool{}, err
	}
	if !present {
		return externaljournal.FinishedTool{}, fmt.Errorf("recovered %s receipt has no committed mutation", started.Tool)
	}
	receipt := &externaljournal.ToolReceipt{Details: json.RawMessage(body), Effects: []agent.Effect{effect}}
	return externaljournal.FinishedTool{ExecutionID: started.ExecutionID, Success: true, Result: workspacechange.ToolReceiptForModel(started.Tool, body), Receipt: receipt}, nil
}
