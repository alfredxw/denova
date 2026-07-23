package app

import (
	"encoding/json"

	"denova/internal/agent"
)

const taskDisplayCheckpointVersion = 2

func (t *Task) displayCheckpointLocked() TaskDisplayCheckpoint {
	events := make([]agent.Event, len(t.checkpointEvents))
	copy(events, t.checkpointEvents)
	return TaskDisplayCheckpoint{
		Version:                 taskDisplayCheckpointVersion,
		TaskID:                  t.id,
		Cursor:                  t.checkpointCursor,
		Complete:                t.checkpointComplete,
		Settled:                 t.finished,
		Status:                  t.status,
		TerminalReason:          t.terminalReason,
		TerminalReasonTruncated: t.terminalReasonTruncated,
		PersistenceRequired:     t.gameTurnPersistenceRequired,
		Events:                  events,
	}
}

// displayReplayBytes returns the memory charged to reconnectable display
// history. Registries use this value to retain command identity without
// retaining an unbounded number of settled Task buffers.
func (t *Task) displayReplayBytes() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.retainedBytes + t.checkpointSize
}

// displayReplayRegistryCharge reserves the maximum raw-suffix plus checkpoint
// capacity while a Task is active. Without this reservation, a small new Task
// could coexist with a full settled cache and exceed the registry budget as it
// streams, before another registry access gets a chance to prune.
func (t *Task) displayReplayRegistryCharge() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return t.retainedBytes + t.checkpointSize
	}
	byteLimit := t.retainedByteLimit
	if byteLimit <= 0 {
		byteLimit = defaultTaskRetainedByteLimit
	}
	return 2 * byteLimit
}

// releaseDisplayReplay drops only settled, display-only history. Durable
// receipts, canonical Writing/Game history, and Task settlement remain intact.
// A stale holder can still inspect status, but reconnect now receives an
// explicit incomplete checkpoint and must rehydrate canonically.
func (t *Task) releaseDisplayReplay() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.finished {
		return 0
	}
	released := t.retainedBytes + t.checkpointSize
	t.events = nil
	t.eventBytes = nil
	t.retainedBytes = 0
	t.eventBaseCursor = t.nextCursor
	t.checkpointEvents = nil
	t.checkpointBytes = nil
	t.checkpointSize = 0
	t.checkpointCursor = t.nextCursor
	t.checkpointComplete = false
	return released
}

func (t *Task) projectDisplayCheckpointLocked(item TaskEvent) {
	event := cloneTaskDisplayEvent(item.Event)
	switch event.Type {
	case "agent_cycle_started":
		t.gameTurnPersistenceRequired = true
	case "interactive_turn_persisted":
		t.gameTurnPersistenceRequired = false
	}
	if t.checkpointCursor == 0 {
		t.checkpointComplete = true
	}
	if event.Type == "agent_cycle_started" {
		// Earlier cycles already belong to canonical Writing/Game history. The
		// display checkpoint owns only the currently live cycle.
		t.checkpointEvents = nil
		t.checkpointBytes = nil
		t.checkpointSize = 0
		t.checkpointComplete = true
	}
	if !t.mergeDisplayCheckpointEventLocked(event) {
		t.checkpointEvents = append(t.checkpointEvents, event)
		size := taskEventSize(TaskEvent{Event: event})
		t.checkpointBytes = append(t.checkpointBytes, size)
		t.checkpointSize += size
	}
	t.checkpointCursor = item.Cursor
	t.boundDisplayCheckpointLocked()
}

func (t *Task) mergeDisplayCheckpointEventLocked(event agent.Event) bool {
	switch event.Type {
	case "chunk", "thinking":
		return t.mergeDisplayCheckpointTextLocked(event, "content")
	case "tool_args_delta":
		if t.mergeDisplayCheckpointToolArgsLocked(event) {
			return true
		}
		return t.mergeDisplayCheckpointTextLocked(event, "delta")
	case "tool_call", "tool_result", "done", "error", "aborted", "agent_cycle_started":
		return false
	}

	key := taskDisplayReplacementKey(event)
	if key == "" {
		return false
	}
	for index := len(t.checkpointEvents) - 1; index >= 0; index-- {
		if taskDisplayReplacementKey(t.checkpointEvents[index]) != key {
			continue
		}
		t.replaceDisplayCheckpointEventLocked(index, event)
		return true
	}
	return false
}

func (t *Task) mergeDisplayCheckpointTextLocked(event agent.Event, field string) bool {
	if len(t.checkpointEvents) == 0 {
		return false
	}
	next, ok := taskDisplayDataMap(event.Data)
	if !ok {
		return false
	}
	delta, ok := next[field].(string)
	if !ok {
		return false
	}
	index := len(t.checkpointEvents) - 1
	previous := t.checkpointEvents[index]
	if previous.Type != event.Type {
		return false
	}
	current, ok := taskDisplayDataMap(previous.Data)
	if !ok || taskDisplayMergeIdentity(current, field) != taskDisplayMergeIdentity(next, field) {
		return false
	}
	content, ok := current[field].(string)
	if !ok {
		return false
	}
	current[field] = content + delta
	previous.Data = current
	t.replaceDisplayCheckpointEventLocked(index, previous)
	return true
}

func (t *Task) mergeDisplayCheckpointToolArgsLocked(event agent.Event) bool {
	deltaData, ok := taskDisplayDataMap(event.Data)
	if !ok {
		return false
	}
	delta, ok := deltaData["delta"].(string)
	if !ok || delta == "" {
		return false
	}
	key := taskDisplayToolKey(deltaData)
	if key == "" {
		return false
	}
	for index := len(t.checkpointEvents) - 1; index >= 0; index-- {
		candidate := t.checkpointEvents[index]
		if candidate.Type != "tool_call" {
			continue
		}
		callData, ok := taskDisplayDataMap(candidate.Data)
		if !ok || taskDisplayToolKey(callData) != key {
			continue
		}
		args, _ := callData["args"].(string)
		callData["args"] = args + delta
		candidate.Data = callData
		t.replaceDisplayCheckpointEventLocked(index, candidate)
		return true
	}
	return false
}

func (t *Task) replaceDisplayCheckpointEventLocked(index int, event agent.Event) {
	oldSize := t.checkpointBytes[index]
	newSize := taskEventSize(TaskEvent{Event: event})
	t.checkpointEvents[index] = event
	t.checkpointBytes[index] = newSize
	t.checkpointSize += newSize - oldSize
}

func (t *Task) boundDisplayCheckpointLocked() {
	eventLimit := t.retainedEventLimit
	if eventLimit <= 0 {
		eventLimit = defaultTaskRetainedEventLimit
	}
	byteLimit := t.retainedByteLimit
	if byteLimit <= 0 {
		byteLimit = defaultTaskRetainedByteLimit
	}
	for len(t.checkpointEvents) > 0 && (len(t.checkpointEvents) > eventLimit || t.checkpointSize > byteLimit) {
		// Keep the current cycle anchor when possible. If the projection itself
		// exceeds its hard budget, omit whole semantic events and make recovery
		// explicitly incomplete. Thinking/tool text is never sliced mid-event.
		drop := 0
		if t.checkpointEvents[0].Type == "agent_cycle_started" && len(t.checkpointEvents) > 1 {
			drop = 1
		}
		t.checkpointSize -= t.checkpointBytes[drop]
		copy(t.checkpointEvents[drop:], t.checkpointEvents[drop+1:])
		copy(t.checkpointBytes[drop:], t.checkpointBytes[drop+1:])
		t.checkpointEvents[len(t.checkpointEvents)-1] = agent.Event{}
		t.checkpointBytes[len(t.checkpointBytes)-1] = 0
		t.checkpointEvents = t.checkpointEvents[:len(t.checkpointEvents)-1]
		t.checkpointBytes = t.checkpointBytes[:len(t.checkpointBytes)-1]
		t.checkpointComplete = false
	}
}

func cloneTaskDisplayEvent(event agent.Event) agent.Event {
	if data, ok := taskDisplayDataMap(event.Data); ok {
		event.Data = data
	}
	return event
}

func taskDisplayDataMap(value any) (map[string]any, bool) {
	if value == nil {
		return map[string]any{}, true
	}
	if data, ok := value.(map[string]any); ok {
		clone := make(map[string]any, len(data))
		for key, item := range data {
			clone[key] = item
		}
		return clone, true
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, false
	}
	return data, true
}

func taskDisplayMergeIdentity(data map[string]any, contentField string) string {
	identity := make(map[string]any, len(data))
	for key, value := range data {
		if key != contentField {
			identity[key] = value
		}
	}
	raw, _ := json.Marshal(identity)
	return string(raw)
}

func taskDisplayToolKey(data map[string]any) string {
	identity := map[string]any{}
	for _, key := range []string{"run_id", "agent_name", "subagent_session_id", "run_path"} {
		if value, ok := data[key]; ok {
			identity[key] = value
		}
	}
	// Providers do not always repeat name/index on argument deltas. Prefer the
	// strongest available identity so otherwise matching call and delta frames
	// still fold into one reconstructable tool invocation.
	for _, key := range []string{"id", "index", "name"} {
		if value, ok := data[key]; ok {
			identity[key] = value
			raw, _ := json.Marshal(identity)
			return string(raw)
		}
	}
	return ""
}

func taskDisplayReplacementKey(event agent.Event) string {
	data, ok := taskDisplayDataMap(event.Data)
	if !ok {
		return ""
	}
	identity := map[string]any{}
	for _, key := range []string{"id", "operation_id", "command_id", "change_set_id", "resolution_id", "compaction_id"} {
		if value, exists := data[key]; exists {
			identity[key] = value
		}
	}
	if len(identity) == 0 {
		return ""
	}
	for _, key := range []string{"run_id", "agent_name", "subagent_session_id", "run_path"} {
		if value, exists := data[key]; exists {
			identity[key] = value
		}
	}
	raw, _ := json.Marshal(identity)
	return event.Type + ":" + string(raw)
}
