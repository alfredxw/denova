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
		return s.projection.activeContextCheckpoints(agentKind)
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
				if _, err := CloneContextCheckpointBoundary(operation.Boundary); err != nil {
					return nil, fmt.Errorf("context rewind %q has an invalid durable boundary: %w", operation.CheckpointID, err)
				}
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
		operation := byID[id]
		boundary, err := CloneContextCheckpointBoundary(operation.Boundary)
		if err != nil {
			return nil, fmt.Errorf("active context checkpoint %q has an invalid durable boundary: %w", operation.CheckpointID, err)
		}
		operation.Boundary = boundary
		result = append(result, operation)
	}
	return result, nil
}

func (s *Session) latestContextWindowProjectionLocked(agentKind string) (ContextWindowProjection, bool, error) {
	agentKind = strings.TrimSpace(agentKind)
	if agentKind == "" {
		return ContextWindowProjection{}, false, nil
	}
	if s.projection != nil {
		return s.projection.latestContextWindowProjection(agentKind)
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
			boundary, err := CloneContextCheckpointBoundary(operation.Boundary)
			if err != nil {
				return ContextWindowProjection{}, false, fmt.Errorf("context rewind %q has an invalid durable boundary: %w", operation.CheckpointID, err)
			}
			operation.Boundary = boundary
			checkpoint := ContextOperation{
				Kind: ContextOperationCheckpoint, AgentKind: operation.AgentKind,
				CheckpointID: operation.CheckpointID, Purpose: operation.Purpose, MessageCount: operation.MessageCount,
				Boundary: operation.Boundary,
			}
			latest = ContextWindowProjection{
				Checkpoint: checkpoint, Rewind: operation, RewindAfterIndex: currentIndex,
				ContextRevision: record.messageMetadata.ContextRevision,
			}
			found = true
		}
	}
	return latest, found, nil
}
