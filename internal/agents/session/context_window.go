package session

import (
	"fmt"
	"strings"
)

// ActiveContextCheckpoints returns durable checkpoints after the latest clear
// marker for one Agent kind. The newest checkpoint with a repeated ID wins;
// any active or applied corrupt boundary fails closed.
func (s *Session) ActiveContextCheckpoints(agentKind string) ([]ContextOperation, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	agentKind = strings.TrimSpace(agentKind)
	if s.projection != nil {
		operations, err := s.projection.activeContextCheckpoints(agentKind)
		if err != nil {
			return nil, err
		}
		return s.resolveContextOperationsLocked(operations)
	}
	byID := map[string]ContextOperation{}
	order := make([]string, 0)
	messageIndex := s.messageBaseIndex
	for _, record := range s.records {
		if record.message == nil {
			continue
		}
		currentIndex := messageIndex
		messageIndex++
		if currentIndex < s.clearAfterIndex || record.kind != historyTypeMessage {
			continue
		}
		for _, operation := range record.messageMetadata.ContextOperations {
			if operation.AgentKind != agentKind || operation.MessageCount < s.clearAfterIndex {
				continue
			}
			switch operation.Kind {
			case ContextOperationCheckpoint:
				if _, exists := byID[operation.CheckpointID]; !exists {
					order = append(order, operation.CheckpointID)
				}
				byID[operation.CheckpointID] = operation
			case ContextOperationRewind:
				// A rewind starts a new context branch. Checkpoints created on the
				// discarded branch must not remain addressable after restart.
				for index, id := range order {
					if id != operation.CheckpointID {
						continue
					}
					for _, droppedID := range order[index:] {
						delete(byID, droppedID)
					}
					order = order[:index]
					break
				}
			}
		}
	}
	result := make([]ContextOperation, 0, len(order))
	for _, id := range order {
		result = append(result, byID[id])
	}
	return s.resolveContextOperationsLocked(result)
}

func (s *Session) latestContextWindowProjectionLocked(agentKind string) (ContextWindowProjection, bool, error) {
	agentKind = strings.TrimSpace(agentKind)
	if agentKind == "" {
		return ContextWindowProjection{}, false, nil
	}
	if s.projection != nil {
		projection, found, err := s.projection.latestContextWindowProjection(agentKind)
		if err != nil || !found {
			return projection, found, err
		}
		boundary, err := s.resolveContextOperationLocked(projection.Checkpoint)
		if err != nil {
			return ContextWindowProjection{}, false, fmt.Errorf("context rewind %q has an invalid durable boundary: %w", projection.Rewind.CheckpointID, err)
		}
		projection.Checkpoint.ResolvedBoundary = boundary
		projection.Rewind.ResolvedBoundary = boundary
		return projection, true, nil
	}
	messageIndex := s.messageBaseIndex
	var latest ContextWindowProjection
	found := false
	for _, record := range s.records {
		if record.message == nil {
			continue
		}
		currentIndex := messageIndex
		messageIndex++
		if currentIndex < s.clearAfterIndex || record.kind != historyTypeMessage {
			continue
		}
		for _, operation := range record.messageMetadata.ContextOperations {
			if operation.Kind != ContextOperationRewind || operation.AgentKind != agentKind || operation.MessageCount < s.clearAfterIndex {
				continue
			}
			checkpoint := ContextOperation{
				Kind: ContextOperationCheckpoint, AgentKind: operation.AgentKind,
				CheckpointID: operation.CheckpointID, Purpose: operation.Purpose, MessageCount: operation.MessageCount,
				BoundaryID: operation.BoundaryID, BoundaryLocator: operation.BoundaryLocator,
			}
			latest = ContextWindowProjection{
				Checkpoint: checkpoint, Rewind: operation, RewindAfterIndex: currentIndex,
				ContextRevision: record.messageMetadata.ContextRevision,
			}
			found = true
		}
	}
	if found {
		boundary, err := s.resolveContextOperationLocked(latest.Checkpoint)
		if err != nil {
			return ContextWindowProjection{}, false, fmt.Errorf("context rewind %q has an invalid durable boundary: %w", latest.Rewind.CheckpointID, err)
		}
		latest.Checkpoint.ResolvedBoundary = boundary
		latest.Rewind.ResolvedBoundary = boundary
	}
	return latest, found, nil
}

func (s *Session) resolveContextOperationsLocked(operations []ContextOperation) ([]ContextOperation, error) {
	result := make([]ContextOperation, 0, len(operations))
	for _, operation := range operations {
		boundary, err := s.resolveContextOperationLocked(operation)
		if err != nil {
			return nil, fmt.Errorf("active context checkpoint %q has an invalid durable boundary: %w", operation.CheckpointID, err)
		}
		operation = copyContextOperation(operation)
		operation.ResolvedBoundary = boundary
		result = append(result, operation)
	}
	return result, nil
}

func (s *Session) resolveContextOperationLocked(operation ContextOperation) (*ContextBoundarySnapshot, error) {
	boundary, err := s.loadContextBoundaryLocked(operation.BoundaryID, operation.BoundaryLocator)
	if err != nil {
		return nil, err
	}
	if operation.MessageCount != boundary.Cursor.MessageCount {
		return nil, fmt.Errorf("boundary message count is invalid")
	}
	if boundary.Cursor.ClearAfterIndex != s.clearAfterIndex {
		return nil, fmt.Errorf("boundary clear index is invalid")
	}
	return boundary, nil
}
