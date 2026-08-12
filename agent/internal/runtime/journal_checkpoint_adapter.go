package runtime

import (
	"encoding/json"
	"fmt"
)

// journalCheckpointState keeps the checkpoint storage contract independent of
// harnessState while preserving all reducer invariants inside this package.
type journalCheckpointState struct {
	state *harnessState
}

// NewJournalCheckpointState returns a fully retained reducer target for a
// checkpoint-aware Journal's standalone Load or Replay implementation.
func NewJournalCheckpointState(binding BindingRef) JournalCheckpointState {
	state := newHarnessState(binding)
	return journalCheckpointState{state: &state}
}

// NewJournalProjectionState returns a reducer target that retains only the
// state required to validate cursors and durable command receipts.
func NewJournalProjectionState(binding BindingRef) JournalCheckpointState {
	state := newProjectionState(binding)
	return journalCheckpointState{state: &state}
}

func (s journalCheckpointState) Cursor() Cursor {
	if s.state == nil {
		return 0
	}
	return s.state.cursor
}

func (s journalCheckpointState) CheckpointSafe() bool {
	return s.state != nil && s.state.checkpointSafe()
}

func (s journalCheckpointState) Fresh() JournalCheckpointState {
	if s.state == nil {
		return journalCheckpointState{}
	}
	template := s.state
	state := newHarnessState(template.binding)
	state.retainTimeline = template.retainTimeline
	state.retainCommandIndex = template.retainCommandIndex
	state.maxRetainedEvents = template.maxRetainedEvents
	state.maxRetainedMessages = template.maxRetainedMessages
	state.maxRetainedCommands = template.maxRetainedCommands
	state.memoryLimits = template.memoryLimits.normalized()
	return journalCheckpointState{state: &state}
}

func (s journalCheckpointState) Reduce(event Event) error {
	if s.state == nil {
		return fmt.Errorf("journal checkpoint reducer is nil")
	}
	return s.state.reduce(event)
}

func (s journalCheckpointState) MarshalCheckpoint() (json.RawMessage, error) {
	if s.state == nil {
		return nil, fmt.Errorf("journal checkpoint reducer is nil")
	}
	checkpoint, err := s.state.checkpoint()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("encode runtime checkpoint: %w", err)
	}
	return encoded, nil
}

func (s journalCheckpointState) RestoreCheckpoint(encoded json.RawMessage) error {
	if s.state == nil {
		return fmt.Errorf("journal checkpoint reducer is nil")
	}
	var checkpoint harnessCheckpoint
	if err := json.Unmarshal(encoded, &checkpoint); err != nil {
		return fmt.Errorf("decode runtime checkpoint: %w", err)
	}
	return restoreHarnessCheckpoint(s.state, checkpoint)
}

func (s journalCheckpointState) PublishFrom(source JournalCheckpointState) error {
	if s.state == nil {
		return fmt.Errorf("journal checkpoint publish target is nil")
	}
	other, ok := source.(journalCheckpointState)
	if !ok || other.state == nil {
		return fmt.Errorf("journal checkpoint state implementation %T is incompatible", source)
	}
	*s.state = *other.state
	return nil
}

func (s journalCheckpointState) RetainedEvents() []Event {
	if s.state == nil {
		return nil
	}
	return cloneEvents(s.state.events)
}
