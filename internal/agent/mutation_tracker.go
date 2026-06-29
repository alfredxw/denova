package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type ToolMutation struct {
	ToolName           string     `json:"tool_name"`
	ToolCallID         string     `json:"tool_call_id,omitempty"`
	Target             string     `json:"target,omitempty"`
	Source             ToolSource `json:"source"`
	RequiresPostCheck  bool       `json:"requires_post_check"`
	IdempotencyKey     string     `json:"idempotency_key,omitempty"`
	LoreItemIDs        []string   `json:"lore_item_ids,omitempty"`
	DeletedLoreItemIDs []string   `json:"deleted_lore_item_ids,omitempty"`
}

type mutationTracker struct {
	mu    sync.Mutex
	calls map[string]*trackedToolCall
	order []string
}

type trackedToolCall struct {
	id       string
	name     string
	args     strings.Builder
	target   string
	itemIDs  []string
	deleteID []string
}

func newMutationTracker() *mutationTracker {
	return &mutationTracker{calls: map[string]*trackedToolCall{}}
}

func (t *mutationTracker) Observe(ev Event) {
	if t == nil {
		return
	}
	switch ev.Type {
	case "tool_call":
		t.observeToolCall(ev.Data)
	case "tool_args_delta":
		t.observeToolArgsDelta(ev.Data)
	case "tool_target":
		t.observeToolTarget(ev.Data)
	case "tool_result":
		t.observeToolResult(ev.Data)
	}
}

func (t *mutationTracker) Mutations() []ToolMutation {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]ToolMutation, 0, len(t.order))
	for _, id := range t.order {
		call := t.calls[id]
		if call == nil {
			continue
		}
		manifest := ManifestForTool(call.name)
		if !manifest.MutatesWorkspace {
			continue
		}
		args := call.args.String()
		target := call.target
		if target == "" {
			target = toolPathFromArgs(args)
		}
		result = append(result, ToolMutation{
			ToolName:           manifest.Name,
			ToolCallID:         call.id,
			Target:             filepath.ToSlash(strings.TrimSpace(target)),
			Source:             manifest.Source,
			RequiresPostCheck:  manifest.RequiresPostCheck,
			IdempotencyKey:     toolIdempotencyKey(manifest.Name, args),
			LoreItemIDs:        uniqueStrings(call.itemIDs),
			DeletedLoreItemIDs: uniqueStrings(call.deleteID),
		})
	}
	return result
}

func (t *mutationTracker) observeToolCall(data any) {
	id := eventDataString(data, "id")
	name := eventDataString(data, "name")
	if id == "" {
		id = fmt.Sprintf("%s:%d", name, len(t.order)+1)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	call := t.ensureCallLocked(id, name)
	if args := eventDataString(data, "args"); args != "" {
		call.args.WriteString(args)
	}
	if target := eventDataString(data, "target"); target != "" {
		call.target = target
	}
}

func (t *mutationTracker) observeToolArgsDelta(data any) {
	id := eventDataString(data, "id")
	name := eventDataString(data, "name")
	if id == "" {
		id = fmt.Sprintf("%s:%d", name, len(t.order)+1)
	}
	delta := eventDataString(data, "delta")
	t.mu.Lock()
	defer t.mu.Unlock()
	call := t.ensureCallLocked(id, name)
	call.args.WriteString(delta)
	if path := toolPathFromArgs(call.args.String()); path != "" {
		call.target = path
	}
}

func (t *mutationTracker) observeToolTarget(data any) {
	id := eventDataString(data, "id")
	name := eventDataString(data, "name")
	target := eventDataString(data, "target")
	if id == "" || target == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	call := t.ensureCallLocked(id, name)
	call.target = target
}

func (t *mutationTracker) observeToolResult(data any) {
	id := eventDataString(data, "id")
	name := eventDataString(data, "name")
	if id == "" && name == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	call := t.ensureCallLocked(id, name)
	if target := eventDataString(data, "target"); target != "" {
		call.target = target
	}
	call.itemIDs = append(call.itemIDs, eventDataStringSlice(data, "item_ids")...)
	call.deleteID = append(call.deleteID, eventDataStringSlice(data, "deleted_ids")...)
}

func (t *mutationTracker) ensureCallLocked(id, name string) *trackedToolCall {
	call := t.calls[id]
	if call == nil {
		call = &trackedToolCall{id: id}
		t.calls[id] = call
		t.order = append(t.order, id)
	}
	if strings.TrimSpace(name) != "" {
		call.name = name
	}
	return call
}

func eventDataStringSlice(data any, key string) []string {
	switch typed := data.(type) {
	case map[string]interface{}:
		value, ok := typed[key]
		if !ok {
			return nil
		}
		return anyToStringSlice(value)
	case map[string][]string:
		return typed[key]
	default:
		return nil
	}
}

func anyToStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
