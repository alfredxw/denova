package session

import (
	"errors"
	"fmt"
	"strings"

	"denova/internal/conversationjournal"
)

const (
	// Root context-window controllers permit one unresolved checkpoint. The
	// Agent cap bounds malformed canonical metadata without coupling this
	// storage package to the product Agent registry.
	maxContextWindowProjectionAgents      = 16
	maxProjectedCheckpointsPerAgent       = 1
	contextWindowProjectionOverflowReason = "context window projection exceeds its bounded state limit"
)

// contextOperationLocator binds one current structural operation to its exact
// canonical transaction and absolute message position. The operation itself is
// retained because an old checkpoint boundary may no longer be in the resident
// 200-message materialization.
type contextOperationLocator struct {
	Cursor          conversationjournal.Cursor `json:"cursor"`
	RecordIndex     int                        `json:"record_index,omitempty"`
	OperationIndex  int                        `json:"operation_index,omitempty"`
	MessageIndex    int                        `json:"message_index"`
	ContextRevision uint64                     `json:"context_revision,omitempty"`
	Operation       ContextOperation           `json:"operation"`
}

// agentContextWindowProjection contains only the current state for one Agent:
// unresolved checkpoints and the newest applied rewind since the latest clear.
type agentContextWindowProjection struct {
	AgentKind         string                    `json:"agent_kind"`
	ActiveCheckpoints []contextOperationLocator `json:"active_checkpoints,omitempty"`
	LatestRewind      *contextOperationLocator  `json:"latest_rewind,omitempty"`
}

func (projection *sessionJournalProjection) rememberContextOperations(
	location conversationjournal.Location,
	messageIndex int,
	contextRevision uint64,
	operations []ContextOperation,
) {
	for operationIndex, operation := range operations {
		if operation.MessageCount < projection.ClearAfter {
			continue
		}
		state := projection.contextWindowState(operation.AgentKind, true)
		if state == nil {
			projection.ContextWindowProjectionInvalid = true
			continue
		}
		switch operation.Kind {
		case ContextOperationCheckpoint:
			located := contextOperationLocator{
				Cursor: location.Cursor, RecordIndex: location.RecordIndex, OperationIndex: operationIndex,
				MessageIndex: messageIndex, ContextRevision: contextRevision,
				Operation: operation,
			}
			replaced := false
			for index := range state.ActiveCheckpoints {
				if state.ActiveCheckpoints[index].Operation.CheckpointID == operation.CheckpointID {
					state.ActiveCheckpoints[index] = located
					replaced = true
					break
				}
			}
			if replaced {
				continue
			}
			if len(state.ActiveCheckpoints) >= maxProjectedCheckpointsPerAgent {
				projection.ContextWindowProjectionInvalid = true
				continue
			}
			state.ActiveCheckpoints = append(state.ActiveCheckpoints, located)
		case ContextOperationRewind:
			state.LatestRewind = &contextOperationLocator{
				Cursor: location.Cursor, RecordIndex: location.RecordIndex, OperationIndex: operationIndex,
				MessageIndex: messageIndex, ContextRevision: contextRevision,
				Operation: operation,
			}
			for index, active := range state.ActiveCheckpoints {
				if active.Operation.CheckpointID != operation.CheckpointID {
					continue
				}
				state.ActiveCheckpoints = append([]contextOperationLocator(nil), state.ActiveCheckpoints[:index]...)
				break
			}
		}
	}
}

func checkpointOperationFromRewind(operation ContextOperation) ContextOperation {
	return ContextOperation{
		Kind: ContextOperationCheckpoint, AgentKind: operation.AgentKind,
		CheckpointID: operation.CheckpointID, Purpose: operation.Purpose,
		MessageCount: operation.MessageCount, Boundary: operation.Boundary,
	}
}

func (projection *sessionJournalProjection) contextWindowState(agentKind string, create bool) *agentContextWindowProjection {
	agentKind = strings.TrimSpace(agentKind)
	if agentKind == "" {
		return nil
	}
	for index := range projection.ContextWindows {
		if projection.ContextWindows[index].AgentKind == agentKind {
			return &projection.ContextWindows[index]
		}
	}
	if !create || len(projection.ContextWindows) >= maxContextWindowProjectionAgents {
		return nil
	}
	projection.ContextWindows = append(projection.ContextWindows, agentContextWindowProjection{AgentKind: agentKind})
	return &projection.ContextWindows[len(projection.ContextWindows)-1]
}

func (projection *sessionJournalProjection) resetContextWindows() {
	projection.ContextWindows = nil
	projection.ContextWindowProjectionInvalid = false
}

func (projection *sessionJournalProjection) activeContextCheckpoints(agentKind string) ([]ContextOperation, error) {
	if projection == nil {
		return nil, nil
	}
	if projection.ContextWindowProjectionInvalid {
		return nil, errors.New(contextWindowProjectionOverflowReason)
	}
	state := projection.contextWindowState(agentKind, false)
	if state == nil {
		return nil, nil
	}
	if err := projection.validateContextWindowState(*state); err != nil {
		return nil, err
	}
	result := make([]ContextOperation, 0, len(state.ActiveCheckpoints))
	for _, located := range state.ActiveCheckpoints {
		operation, err := cloneValidatedContextOperation(located.Operation)
		if err != nil {
			return nil, fmt.Errorf("active context checkpoint %q has an invalid durable boundary: %w", located.Operation.CheckpointID, err)
		}
		result = append(result, operation)
	}
	return result, nil
}

func (projection *sessionJournalProjection) latestContextWindowProjection(agentKind string) (ContextWindowProjection, bool, error) {
	if projection == nil {
		return ContextWindowProjection{}, false, nil
	}
	if projection.ContextWindowProjectionInvalid {
		return ContextWindowProjection{}, false, errors.New(contextWindowProjectionOverflowReason)
	}
	state := projection.contextWindowState(agentKind, false)
	if state == nil || state.LatestRewind == nil {
		return ContextWindowProjection{}, false, nil
	}
	if err := projection.validateContextWindowState(*state); err != nil {
		return ContextWindowProjection{}, false, err
	}
	rewind, err := cloneValidatedContextOperation(state.LatestRewind.Operation)
	if err != nil {
		return ContextWindowProjection{}, false, fmt.Errorf("context rewind %q has an invalid durable boundary: %w", state.LatestRewind.Operation.CheckpointID, err)
	}
	return ContextWindowProjection{
		Checkpoint: checkpointOperationFromRewind(rewind), Rewind: rewind,
		RewindAfterIndex: state.LatestRewind.MessageIndex,
		ContextRevision:  state.LatestRewind.ContextRevision,
	}, true, nil
}

func cloneValidatedContextOperation(operation ContextOperation) (ContextOperation, error) {
	boundary, err := CloneContextCheckpointBoundary(operation.Boundary)
	if err != nil {
		return ContextOperation{}, err
	}
	clone := operation
	clone.Boundary = boundary
	clone.MutationReceipts = append([]ContextMutationReceipt(nil), operation.MutationReceipts...)
	return clone, nil
}

func (projection *sessionJournalProjection) validateContextWindows() error {
	if projection.ContextWindowProjectionInvalid {
		return errors.New(contextWindowProjectionOverflowReason)
	}
	if len(projection.ContextWindows) > maxContextWindowProjectionAgents {
		return fmt.Errorf("context window projection has %d Agent states", len(projection.ContextWindows))
	}
	seen := make(map[string]struct{}, len(projection.ContextWindows))
	for _, state := range projection.ContextWindows {
		if _, exists := seen[state.AgentKind]; exists {
			return fmt.Errorf("context window projection repeats Agent kind %q", state.AgentKind)
		}
		seen[state.AgentKind] = struct{}{}
		if err := projection.validateContextWindowState(state); err != nil {
			return fmt.Errorf("Agent %q context window projection: %w", state.AgentKind, err)
		}
	}
	return nil
}

func (projection *sessionJournalProjection) validateContextWindowState(state agentContextWindowProjection) error {
	if state.AgentKind == "" || state.AgentKind != strings.TrimSpace(state.AgentKind) || len(state.AgentKind) > maxContextLabelBytes {
		return fmt.Errorf("Agent kind is invalid")
	}
	if len(state.ActiveCheckpoints) > maxProjectedCheckpointsPerAgent {
		return fmt.Errorf("active checkpoint count %d exceeds limit", len(state.ActiveCheckpoints))
	}
	seen := make(map[string]struct{}, len(state.ActiveCheckpoints))
	for _, located := range state.ActiveCheckpoints {
		if err := projection.validateContextOperationLocator(state.AgentKind, ContextOperationCheckpoint, located); err != nil {
			return err
		}
		if _, exists := seen[located.Operation.CheckpointID]; exists {
			return fmt.Errorf("active checkpoint %q is duplicated", located.Operation.CheckpointID)
		}
		seen[located.Operation.CheckpointID] = struct{}{}
	}
	if state.LatestRewind == nil {
		return nil
	}
	rewind := state.LatestRewind
	if err := projection.validateContextOperationLocator(state.AgentKind, ContextOperationRewind, *rewind); err != nil {
		return err
	}
	return nil
}

func (projection *sessionJournalProjection) validateContextOperationLocator(
	agentKind, kind string,
	located contextOperationLocator,
) error {
	return projection.validateLocatedContextOperation(
		agentKind, kind,
		located.Cursor, located.RecordIndex, located.OperationIndex, located.MessageIndex, located.ContextRevision, located.Operation,
	)
}

func (projection *sessionJournalProjection) validateLocatedContextOperation(
	agentKind, kind string,
	cursor conversationjournal.Cursor,
	recordIndex, operationIndex, messageIndex int,
	contextRevision uint64,
	operation ContextOperation,
) error {
	if cursor == 0 || cursor > projection.lastCursor || (projection.ClearCursor > 0 && cursor <= projection.ClearCursor) ||
		recordIndex < 0 || operationIndex < 0 || operationIndex >= maxContextOperationsPerMessage {
		return fmt.Errorf("context %s %q locator is invalid", kind, operation.CheckpointID)
	}
	if messageIndex < projection.ClearAfter || messageIndex >= projection.MessageCount {
		return fmt.Errorf("context %s %q message index %d is invalid", kind, operation.CheckpointID, messageIndex)
	}
	if contextRevision > projection.ContextRevision {
		return fmt.Errorf("context %s %q revision is invalid", kind, operation.CheckpointID)
	}
	if operation.Kind != kind || operation.AgentKind != agentKind || operation.CheckpointID == "" ||
		operation.CheckpointID != strings.TrimSpace(operation.CheckpointID) || len(operation.CheckpointID) > maxContextLabelBytes {
		return fmt.Errorf("context %s operation identity is invalid", kind)
	}
	boundary, err := CloneContextCheckpointBoundary(operation.Boundary)
	if err != nil {
		return fmt.Errorf("context %s %q has an invalid durable boundary: %w", kind, operation.CheckpointID, err)
	}
	if operation.MessageCount != boundary.Cursor.MessageCount || operation.MessageCount < projection.ClearAfter || operation.MessageCount > messageIndex {
		return fmt.Errorf("context %s %q boundary message index is invalid", kind, operation.CheckpointID)
	}
	if boundary.Cursor.ClearAfterIndex != projection.ClearAfter {
		return fmt.Errorf("context %s %q boundary clear index is invalid", kind, operation.CheckpointID)
	}
	return nil
}
