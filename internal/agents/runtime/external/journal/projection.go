package externaljournal

import (
	"encoding/json"
	"errors"
	"fmt"

	"denova/internal/agents/conversationjournal"
)

// Locator contains only a canonical transaction reference, never tool arguments
// or output. All content is read from the JSONL when the product needs it.
type Locator struct {
	Cursor conversationjournal.Cursor `json:"cursor"`
	Index  int                        `json:"index"`
}

type ToolState struct {
	Name     string   `json:"name"`
	Recovery Recovery `json:"recovery"`
	Started  Locator  `json:"started"`
	Finished *Locator `json:"finished,omitempty"`
}

type Operation struct {
	ID             string               `json:"id"`
	CommandID      string               `json:"command_id"`
	InputCommandID string               `json:"input_command_id,omitempty"`
	GuidanceCount  int                  `json:"guidance_count,omitempty"`
	Fingerprint    string               `json:"fingerprint"`
	ConfigRevision uint64               `json:"config_revision"`
	Status         Status               `json:"status"`
	Accepted       Locator              `json:"accepted"`
	Closed         *Locator             `json:"closed,omitempty"`
	Tools          map[string]ToolState `json:"tools,omitempty"`
}

// Projection is a rebuildable index of execution facts. Clear removes all
// active references; older records remain canonical history, never live work.
type Projection struct {
	Operations map[string]*Operation `json:"operations,omitempty"`
	Checkpoint *Locator              `json:"checkpoint,omitempty"`
}

var ErrBusy = errors.New("external operation is running or has unresolved tools")

// RequireIdle guards both a new attempt and an explicit configuration switch.
// An interrupted attempt can still own an unanswered question or unknown write.
func (projection *Projection) RequireIdle() error {
	for _, operation := range projection.Operations {
		if operation.Status == Running {
			return ErrBusy
		}
		for _, tool := range operation.Tools {
			if tool.Finished == nil {
				return ErrBusy
			}
		}
	}
	return nil
}

func (projection *Projection) Reset() { *projection = Projection{} }

func (projection *Projection) Apply(source conversationjournal.Record) (bool, error) {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(source.Payload, &typed); err != nil {
		return false, err
	}
	if typed.Type != RecordType {
		return false, nil
	}
	var record Record
	if err := json.Unmarshal(source.Payload, &record); err != nil {
		return false, err
	}
	if record.Type != RecordType {
		return false, nil
	}
	if err := record.Validate(); err != nil {
		return true, err
	}
	if projection.Operations == nil {
		projection.Operations = map[string]*Operation{}
	}
	location := Locator{Cursor: source.Location.Cursor, Index: source.Location.RecordIndex}
	operation := projection.Operations[record.OperationID]
	if record.Kind == OperationAccepted {
		if operation != nil {
			return true, errors.New("duplicate external operation acceptance")
		}
		var accepted Accepted
		if err := json.Unmarshal(record.Data, &accepted); err != nil {
			return true, err
		}
		if err := projection.RequireIdle(); err != nil {
			return true, err
		}
		if accepted.ContinuesOperationID != "" {
			previous := projection.Operations[accepted.ContinuesOperationID]
			if previous == nil || previous.Status != Interrupted || previous.ConfigRevision != record.ConfigRevision {
				return true, errors.New("external continuation requires an interrupted operation at the same configuration revision")
			}
		}
		for _, existing := range projection.Operations {
			if existing.CommandID == accepted.CommandID {
				return true, errors.New("duplicate external command acceptance")
			}
			if existing.Status == Running {
				return true, errors.New("an external operation is already running")
			}
		}
		projection.Operations[record.OperationID] = &Operation{ID: record.OperationID, CommandID: accepted.CommandID, InputCommandID: accepted.InputCommandID, GuidanceCount: accepted.GuidanceCount, Fingerprint: accepted.Fingerprint, ConfigRevision: record.ConfigRevision, Status: Running, Accepted: location, Tools: map[string]ToolState{}}
		return true, nil
	}
	if operation == nil || operation.ConfigRevision != record.ConfigRevision {
		return true, errors.New("external record does not match an accepted operation revision")
	}
	switch record.Kind {
	case GuidanceDelivered:
		var delivered DeliveredGuidance
		if err := json.Unmarshal(record.Data, &delivered); err != nil {
			return true, err
		}
		if operation.Status != Running || delivered.Count != operation.GuidanceCount+1 {
			return true, errors.New("external guidance does not extend the active accepted prefix")
		}
		operation.GuidanceCount = delivered.Count
	case ToolStarted:
		if operation.Status != Running {
			return true, errors.New("cannot start a tool for a closed external operation")
		}
		var started StartedTool
		if err := json.Unmarshal(record.Data, &started); err != nil {
			return true, err
		}
		if _, exists := operation.Tools[started.ExecutionID]; exists {
			return true, errors.New("duplicate external execution ID")
		}
		for _, other := range projection.Operations {
			if other.ID != operation.ID {
				if _, exists := other.Tools[started.ExecutionID]; exists {
					return true, errors.New("external execution ID belongs to another operation")
				}
			}
		}
		if operation.Tools == nil {
			operation.Tools = make(map[string]ToolState)
		}
		operation.Tools[started.ExecutionID] = ToolState{Name: started.Tool, Recovery: started.Recovery, Started: location}
	case ToolFinished:
		if operation.Status != Running && operation.Status != Interrupted {
			return true, errors.New("external operation no longer accepts tool results")
		}
		var finished FinishedTool
		if err := json.Unmarshal(record.Data, &finished); err != nil {
			return true, err
		}
		tool, exists := operation.Tools[finished.ExecutionID]
		if !exists || tool.Finished != nil {
			return true, errors.New("external tool result has no pending execution")
		}
		if tool.Name == "ask" {
			if err := validateQuestionResult(finished); err != nil {
				return true, err
			}
		}
		tool.Finished = &location
		operation.Tools[finished.ExecutionID] = tool
	case OperationClosed:
		if operation.Status != Running {
			return true, errors.New("external operation is already closed")
		}
		var closed Closed
		if err := json.Unmarshal(record.Data, &closed); err != nil {
			return true, err
		}
		if closed.Status != Interrupted {
			for _, tool := range operation.Tools {
				if tool.Finished == nil {
					return true, errors.New("external operation has unsettled tools")
				}
			}
		}
		operation.Status, operation.Closed = closed.Status, &location
	case ContextCheckpoint:
		if operation.Status != Running {
			return true, errors.New("checkpoint requires an active maintenance operation")
		}
		projection.Checkpoint = &location
	case OperationAccepted:
		return true, errors.New("external acceptance was not handled")
	default:
		return true, fmt.Errorf("unhandled external runtime kind %q", record.Kind)
	}
	return true, nil
}
