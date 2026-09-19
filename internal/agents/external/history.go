package external

import (
	"context"
	"encoding/json"
	"fmt"

	"denova/config"
	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/external/journal"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	"denova/internal/agents/toolruntime"
	agent "github.com/alfredxw/denova/agent"
)

// History is a complete canonical source snapshot, not the resident UI window.
// Positions allow maintenance checkpoints to cover an exact immutable prefix.
type History struct {
	Cursor               conversationjournal.Cursor
	Revision             uint64
	Selection            config.RuntimeSelection
	Messages             []Message
	ContinuesOperationID string
	Checkpoint           *externaljournal.Checkpoint
	PriorMutations       []agenttool.Mutation
}

func CommandReceipt(ctx context.Context, sess *session.Session, commandID string) (agentrun.CommandReceipt, bool, error) {
	var receipt agentrun.CommandReceipt
	found := false
	err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		for _, operation := range state.Projection.Operations {
			if operation.CommandID != commandID {
				continue
			}
			receipt = agentrun.CommandReceipt{CommandID: agentrun.CommandID(commandID), OperationID: agentrun.OperationID(operation.ID), Cursor: agentrun.Cursor(operation.Accepted.Cursor)}
			found = true
			break
		}
		return nil
	})
	return receipt, found, err
}

func ReadHistory(ctx context.Context, sess *session.Session) (History, error) {
	var history History
	err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		history.Cursor, history.Revision, history.Selection = state.Cursor, state.Config.Revision, state.Config.Engine()
		if state.Projection.Checkpoint != nil {
			record, err := state.Read(*state.Projection.Checkpoint)
			if err != nil {
				return err
			}
			var checkpoint externaljournal.Checkpoint
			if err := json.Unmarshal(record.Data, &checkpoint); err != nil {
				return err
			}
			history.Checkpoint = &checkpoint
		}
		if err := state.Projection.RequireIdle(); err != nil {
			return err
		}
		var latest conversationjournal.Cursor
		for _, operation := range state.Projection.Operations {
			if operation.Accepted.Cursor > latest {
				latest = operation.Accepted.Cursor
				history.ContinuesOperationID = ""
				if operation.Status == externaljournal.Interrupted {
					history.ContinuesOperationID = operation.ID
				}
			}
		}
		// Interrupted continuations inherit committed domain effects as well as
		// prose, so the original post-run verification can finish after recovery.
		ancestors := map[string]bool{}
		for id := history.ContinuesOperationID; id != ""; {
			if ancestors[id] {
				return fmt.Errorf("cyclic external continuation")
			}
			operation := state.Projection.Operations[id]
			if operation == nil {
				return fmt.Errorf("external continuation source is missing")
			}
			ancestors[id] = true
			record, err := state.Read(operation.Accepted)
			if err != nil {
				return err
			}
			var accepted externaljournal.Accepted
			if err := json.Unmarshal(record.Data, &accepted); err != nil {
				return err
			}
			id = accepted.ContinuesOperationID
		}
		return state.ScanContext(func(source session.ExternalContextRecord) error {
			if source.Message != nil {
				message := source.Message
				role, content := string(message.Role), message.Content
				if message.Role == agent.ToolRole {
					role, content = "user", "Confirmed tool observation ("+message.ToolName+"):\n"+content
				}
				projected := Message{Role: role, Text: content, Cursor: uint64(source.Cursor)}
				if message.Role == agent.ToolRole {
					projected.ToolImages = message.Attachments
				} else {
					projected.Attachments = message.Attachments
				}
				history.Messages = append(history.Messages, projected)
			}
			if source.Runtime == nil || source.Runtime.Kind != externaljournal.ToolFinished {
				return nil
			}
			var finished externaljournal.FinishedTool
			if err := json.Unmarshal(source.Runtime.Data, &finished); err != nil {
				return err
			}
			if ancestors[source.Runtime.OperationID] && finished.Receipt != nil {
				for _, effect := range finished.Receipt.Effects {
					if effect.Kind != toolruntime.AgentToolMutationEffectKind {
						continue
					}
					mutation, err := toolruntime.DecodeAgentToolMutationEffect(effect)
					if err != nil {
						return err
					}
					history.PriorMutations = append(history.PriorMutations, mutation)
				}
			}
			operation := state.Projection.Operations[source.Runtime.OperationID]
			if operation == nil {
				return nil
			}
			tool := operation.Tools[finished.ExecutionID]
			// Arguments and outcome are facts, never provider-specific tool-call IDs.
			start, err := state.Read(tool.Started)
			if err != nil {
				return err
			}
			var started externaljournal.StartedTool
			if err := json.Unmarshal(start.Data, &started); err != nil {
				return err
			}
			text := fmt.Sprintf("Confirmed tool observation: %s\nArguments: %s\nSuccess: %t\nResult: %s", tool.Name, started.Arguments, finished.Success, finished.Result)
			projected := Message{Role: "user", Text: text, Cursor: uint64(source.Cursor)}
			if finished.Receipt != nil {
				projected.ToolImages = finished.Receipt.Attachments
			}
			history.Messages = append(history.Messages, projected)
			return nil
		})
	})
	return history, err
}
