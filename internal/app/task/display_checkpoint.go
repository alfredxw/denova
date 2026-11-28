package task

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"denova/internal/agents/run"
)

const taskDisplayCheckpointVersion = 2

type checkpointEntry struct {
	event      agentrun.Event
	data       map[string]any
	textField  string
	textBuf    *strings.Builder
	mergeKey   string
	toolKey    string
	replaceKey string
	segmentID  string
	size       int
}

func (e *checkpointEntry) materialize() agentrun.Event {
	ev := e.event
	if e.data != nil {
		cloned := make(map[string]any, len(e.data))
		for key, value := range e.data {
			cloned[key] = value
		}
		if e.textBuf != nil && e.textField != "" {
			cloned[e.textField] = e.textBuf.String()
		}
		ev.Data = cloned
	}
	return ev
}

func (t *Task) displayCheckpointLocked() DisplayCheckpoint {
	events := make([]agentrun.Event, len(t.checkpointEntries))
	for i := range t.checkpointEntries {
		events[i] = t.checkpointEntries[i].materialize()
	}
	return DisplayCheckpoint{
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

// DisplayReplayBytes returns the memory charged to reconnectable display
// history. Registries use this value to retain command identity without
// retaining an unbounded number of settled Task buffers.
func (t *Task) DisplayReplayBytes() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.retainedBytes + t.checkpointSize
}

// DisplayReplayCharge reserves the maximum raw-suffix plus checkpoint
// capacity while a Task is active. Without this reservation, a small new Task
// could coexist with a full settled cache and exceed the registry budget as it
// streams, before another registry access gets a chance to prune.
func (t *Task) DisplayReplayCharge() int {
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
		byteLimit = DefaultRetainedByteLimit
	}
	return 2 * byteLimit
}

// ReleaseDisplayReplay drops only settled, display-only history. Durable
// receipts, canonical Writing/Game history, and Task settlement remain intact.
// A stale holder can still inspect status, but reconnect now receives an
// explicit incomplete checkpoint and must rehydrate canonically.
func (t *Task) ReleaseDisplayReplay() int {
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
	t.checkpointEntries = nil
	t.checkpointToolIndex = nil
	t.checkpointSize = 0
	t.checkpointCursor = t.nextCursor
	t.checkpointComplete = false
	return released
}

func (t *Task) projectDisplayCheckpointLocked(item Event) {
	ev := item.Event
	switch ev.Type {
	case "agent_cycle_started":
		t.gameTurnPersistenceRequired = true
	case "interactive_turn_persisted":
		t.gameTurnPersistenceRequired = false
	}
	if t.checkpointCursor == 0 {
		t.checkpointComplete = true
	}
	if ev.Type == "agent_cycle_started" {
		// Earlier cycles already belong to canonical Writing/Game history. The
		// display checkpoint owns only the currently live cycle.
		t.checkpointEntries = nil
		t.checkpointToolIndex = nil
		t.checkpointSize = 0
		t.checkpointComplete = true
	}
	if ev.Type == "model_retry" {
		t.retractDisplayCheckpointRetryLocked(ev.Data)
	}
	if !t.mergeDisplayCheckpointEventLocked(ev) {
		entry := buildCheckpointEntry(ev)
		t.checkpointEntries = append(t.checkpointEntries, entry)
		if entry.toolKey != "" {
			if t.checkpointToolIndex == nil {
				t.checkpointToolIndex = make(map[string]int)
			}
			t.checkpointToolIndex[entry.toolKey] = len(t.checkpointEntries) - 1
		}
		t.checkpointSize += entry.size
	}
	t.checkpointCursor = item.Cursor
	t.boundDisplayCheckpointLocked()
}

func (t *Task) retractDisplayCheckpointRetryLocked(raw any) {
	data, ok := readTaskDisplayDataMap(raw)
	if !ok {
		return
	}
	discard := readStringSet(data["discard_ids"])
	if len(discard) == 0 || len(t.checkpointEntries) == 0 {
		return
	}
	removed := false
	kept := t.checkpointEntries[:0]
	for _, entry := range t.checkpointEntries {
		if entry.segmentID != "" && discard[entry.segmentID] {
			t.checkpointSize -= entry.size
			removed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !removed {
		return
	}
	for i := len(kept); i < len(t.checkpointEntries); i++ {
		t.checkpointEntries[i] = checkpointEntry{}
	}
	t.checkpointEntries = kept
	t.rebuildCheckpointToolIndexLocked()
}

func (t *Task) mergeDisplayCheckpointEventLocked(ev agentrun.Event) bool {
	switch ev.Type {
	case "chunk", "thinking":
		return t.mergeDisplayCheckpointTextLocked(ev, "content")
	case "tool_args_delta":
		if t.mergeDisplayCheckpointToolArgsLocked(ev) {
			return true
		}
		return t.mergeDisplayCheckpointTextLocked(ev, "delta")
	case "tool_call", "tool_result", "done", "error", "aborted", "suspended", "agent_cycle_started":
		return false
	}

	data, ok := readTaskDisplayDataMap(ev.Data)
	if !ok {
		return false
	}
	key := taskDisplayReplacementKeyFromMap(ev.Type, data)
	if key == "" {
		return false
	}
	for index := len(t.checkpointEntries) - 1; index >= 0; index-- {
		if t.checkpointEntries[index].replaceKey != key {
			continue
		}
		replacement := buildCheckpointEntry(ev)
		oldSize := t.checkpointEntries[index].size
		t.checkpointEntries[index] = replacement
		t.checkpointSize += replacement.size - oldSize
		return true
	}
	return false
}

func (t *Task) mergeDisplayCheckpointTextLocked(ev agentrun.Event, field string) bool {
	if len(t.checkpointEntries) == 0 {
		return false
	}
	last := &t.checkpointEntries[len(t.checkpointEntries)-1]
	if last.event.Type != ev.Type || last.textBuf == nil || last.textField != field {
		return false
	}
	next, ok := readTaskDisplayDataMap(ev.Data)
	if !ok {
		return false
	}
	delta, ok := next[field].(string)
	if !ok {
		return false
	}
	if taskDisplayMergeKey(next, field) != last.mergeKey {
		return false
	}
	last.textBuf.WriteString(delta)
	last.size += len(delta)
	t.checkpointSize += len(delta)
	return true
}

func (t *Task) mergeDisplayCheckpointToolArgsLocked(ev agentrun.Event) bool {
	if len(t.checkpointToolIndex) == 0 {
		return false
	}
	deltaData, ok := readTaskDisplayDataMap(ev.Data)
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
	index, exists := t.checkpointToolIndex[key]
	if !exists || index < 0 || index >= len(t.checkpointEntries) {
		return false
	}
	entry := &t.checkpointEntries[index]
	if entry.event.Type != "tool_call" || entry.textBuf == nil {
		return false
	}
	entry.textBuf.WriteString(delta)
	entry.size += len(delta)
	t.checkpointSize += len(delta)
	return true
}

func buildCheckpointEntry(ev agentrun.Event) checkpointEntry {
	entry := checkpointEntry{
		event: ev,
		size:  taskEventSize(Event{Event: ev}),
	}
	data, ok := taskDisplayDataMap(ev.Data)
	if !ok {
		return entry
	}
	entry.data = data
	entry.event.Data = data

	switch ev.Type {
	case "chunk", "thinking":
		entry.segmentID, _ = data["display_segment_id"].(string)
		if content, ok := data["content"].(string); ok {
			var buf strings.Builder
			buf.WriteString(content)
			entry.textField = "content"
			entry.textBuf = &buf
			entry.mergeKey = taskDisplayMergeKey(data, "content")
			delete(data, "content")
		}
	case "tool_args_delta":
		if delta, ok := data["delta"].(string); ok {
			var buf strings.Builder
			buf.WriteString(delta)
			entry.textField = "delta"
			entry.textBuf = &buf
			entry.mergeKey = taskDisplayMergeKey(data, "delta")
			delete(data, "delta")
		}
	case "tool_call":
		entry.segmentID, _ = data["id"].(string)
		entry.toolKey = taskDisplayToolKey(data)
		var buf strings.Builder
		if args, ok := data["args"].(string); ok {
			buf.WriteString(args)
		}
		entry.textField = "args"
		entry.textBuf = &buf
		delete(data, "args")
	default:
		entry.replaceKey = taskDisplayReplacementKeyFromMap(ev.Type, data)
	}
	return entry
}

func (t *Task) boundDisplayCheckpointLocked() {
	eventLimit := t.retainedEventLimit
	if eventLimit <= 0 {
		eventLimit = DefaultRetainedEventLimit
	}
	byteLimit := t.retainedByteLimit
	if byteLimit <= 0 {
		byteLimit = DefaultRetainedByteLimit
	}
	evicted := false
	for len(t.checkpointEntries) > 0 && (len(t.checkpointEntries) > eventLimit || t.checkpointSize > byteLimit) {
		// Keep the current cycle anchor when possible. If the projection itself
		// exceeds its hard budget, omit whole semantic events and make recovery
		// explicitly incomplete. Thinking/tool text is never sliced mid-event.
		drop := 0
		if t.checkpointEntries[0].event.Type == "agent_cycle_started" && len(t.checkpointEntries) > 1 {
			drop = 1
		}
		t.checkpointSize -= t.checkpointEntries[drop].size
		copy(t.checkpointEntries[drop:], t.checkpointEntries[drop+1:])
		t.checkpointEntries[len(t.checkpointEntries)-1] = checkpointEntry{}
		t.checkpointEntries = t.checkpointEntries[:len(t.checkpointEntries)-1]
		t.checkpointComplete = false
		evicted = true
	}
	if evicted {
		t.rebuildCheckpointToolIndexLocked()
	}
}

func (t *Task) rebuildCheckpointToolIndexLocked() {
	if len(t.checkpointEntries) == 0 {
		t.checkpointToolIndex = nil
		return
	}
	if t.checkpointToolIndex != nil {
		clear(t.checkpointToolIndex)
	}
	for index := range t.checkpointEntries {
		key := t.checkpointEntries[index].toolKey
		if key == "" {
			continue
		}
		if t.checkpointToolIndex == nil {
			t.checkpointToolIndex = make(map[string]int)
		}
		t.checkpointToolIndex[key] = index
	}
}

// readTaskDisplayDataMap returns a read-only view of event data without cloning
// maps on the hot streaming path.
func readTaskDisplayDataMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if data, ok := value.(map[string]any); ok {
		return data, true
	}
	return taskDisplayDataMap(value)
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
	if data, ok := value.(map[string]string); ok {
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

func taskDisplayMergeKey(data map[string]any, skipField string) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		if key != skipField {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		writeCheckpointValue(&b, data[key])
		b.WriteByte('\x1f')
	}
	return b.String()
}

func taskDisplayToolKey(data map[string]any) string {
	var b strings.Builder
	writeCheckpointScope(&b, data)
	// Providers do not always repeat name/index on argument deltas. Prefer the
	// strongest available identity so otherwise matching call and delta frames
	// still fold into one reconstructable tool invocation.
	for _, key := range []string{"id", "index", "name"} {
		if value, ok := data[key]; ok {
			b.WriteString(key)
			b.WriteByte('=')
			writeCheckpointValue(&b, value)
			return b.String()
		}
	}
	return ""
}

func taskDisplayReplacementKeyFromMap(eventType string, data map[string]any) string {
	var b strings.Builder
	b.WriteString(eventType)
	b.WriteByte(':')
	found := false
	for _, key := range []string{"id", "operation_id", "command_id", "change_set_id", "resolution_id", "compaction_id"} {
		if value, exists := data[key]; exists {
			found = true
			b.WriteString(key)
			b.WriteByte('=')
			writeCheckpointValue(&b, value)
			b.WriteByte('\x1f')
		}
	}
	if !found {
		return ""
	}
	writeCheckpointScope(&b, data)
	return b.String()
}

func writeCheckpointScope(b *strings.Builder, data map[string]any) {
	for _, key := range []string{"run_id", "agent_name", "subagent_session_id", "run_path"} {
		if value, exists := data[key]; exists {
			b.WriteString(key)
			b.WriteByte('=')
			writeCheckpointValue(b, value)
			b.WriteByte('\x1f')
		}
	}
}

func writeCheckpointValue(b *strings.Builder, value any) {
	switch typed := value.(type) {
	case nil:
		b.WriteString("null")
	case string:
		b.WriteByte('s')
		b.WriteString(strconv.Itoa(len(typed)))
		b.WriteByte(':')
		b.WriteString(typed)
	case bool:
		if typed {
			b.WriteString("b:1")
		} else {
			b.WriteString("b:0")
		}
	case int:
		b.WriteString("i:")
		b.WriteString(strconv.Itoa(typed))
	case int64:
		b.WriteString("i:")
		b.WriteString(strconv.FormatInt(typed, 10))
	case float64:
		b.WriteString("f:")
		b.WriteString(strconv.FormatFloat(typed, 'g', -1, 64))
	case []string:
		b.WriteString("ss[")
		for i, item := range typed {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Itoa(len(item)))
			b.WriteByte(':')
			b.WriteString(item)
		}
		b.WriteByte(']')
	default:
		if raw, err := json.Marshal(typed); err == nil {
			b.WriteString("j:")
			b.Write(raw)
		} else {
			b.WriteString("v:")
			b.WriteString(fmt.Sprint(typed))
		}
	}
}

func readStringSet(raw any) map[string]bool {
	switch typed := raw.(type) {
	case []string:
		if len(typed) == 0 {
			return nil
		}
		out := make(map[string]bool, len(typed))
		for _, item := range typed {
			if item != "" {
				out[item] = true
			}
		}
		return out
	case []any:
		if len(typed) == 0 {
			return nil
		}
		out := make(map[string]bool, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out[s] = true
			}
		}
		return out
	default:
		return nil
	}
}
