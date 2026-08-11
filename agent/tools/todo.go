package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type TodoStatus = agent.TodoStatus
type TodoItem = agent.TodoItem

const (
	TodoPending    = agent.TodoPending
	TodoInProgress = agent.TodoInProgress
	TodoCompleted  = agent.TodoCompleted
)

type TodoMutation struct {
	ID     string      `json:"id"`
	Text   *string     `json:"text,omitempty"`
	Status *TodoStatus `json:"status,omitempty"`
	Delete bool        `json:"delete,omitempty"`
}

type TodoStore interface {
	Identity() agent.CapabilityIdentity
	Load(context.Context) ([]TodoItem, uint64, error)
	Apply(context.Context, uint64, []TodoMutation) (TodoApplyResult, error)
}

type TodoMutationResult struct {
	Index int    `json:"index"`
	ID    string `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

type TodoApplyResult struct {
	Items    []TodoItem           `json:"items"`
	Revision uint64               `json:"revision"`
	Results  []TodoMutationResult `json:"results"`
}

type todoToolInput struct {
	Action           string         `json:"action" jsonschema:"enum=read,enum=update"`
	ExpectedRevision uint64         `json:"expected_revision,omitempty"`
	Mutations        []TodoMutation `json:"mutations,omitempty" jsonschema:"maxItems=256"`
}

// Todo exposes revisioned read/update semantics. With no Store it uses the
// current Session's durable Agent state; callers may inject a product Store.
func Todo(stores ...TodoStore) (agent.Toolset, error) {
	if len(stores) > 1 {
		return nil, errors.New("todo Toolset accepts at most one TodoStore")
	}
	var store TodoStore
	if len(stores) == 1 {
		store = stores[0]
		if store == nil {
			return nil, errors.New("todo Toolset received a nil TodoStore")
		}
	}
	tool, err := agent.InferTool("todo", "Read or atomically update the current task list using an explicit revision.\n\n读取或使用显式版本号原子更新当前任务列表。", func(ctx context.Context, input todoToolInput) (agent.ToolResult, error) {
		var result TodoApplyResult
		var err error
		switch input.Action {
		case "read":
			if store == nil {
				result.Items, result.Revision, err = loadSessionTodo(ctx)
			} else {
				result.Items, result.Revision, err = store.Load(ctx)
			}
		case "update":
			if len(input.Mutations) == 0 {
				return agent.ToolResult{}, errors.New("todo update requires at least one mutation")
			}
			if store == nil {
				result, err = applySessionTodo(ctx, input.ExpectedRevision, input.Mutations)
			} else {
				result, err = store.Apply(ctx, input.ExpectedRevision, input.Mutations)
			}
		default:
			return agent.ToolResult{}, errors.New("todo action must be read or update")
		}
		if err != nil {
			return agent.ToolResult{}, err
		}
		return JSONResult(result)
	})
	if err != nil {
		return nil, err
	}
	descriptor := writeDescriptor()
	descriptor.MutationScope = agent.ToolMutationSession
	descriptor.Execution = agent.ToolExecutionSessionExclusive
	descriptor.PostCheck = agent.ToolPostCheckSessionState
	descriptor.Recovery = agent.ToolRecoveryIdempotent
	identity := agent.CapabilityIdentity{Kind: "tools.todo.session", Version: 1}
	if store != nil {
		identity = toolsetIdentity("tools.todo.custom", store.Identity())
	}
	return agent.StaticToolsIdentified(identity, agent.ToolDefinition{Tool: tool, Descriptor: descriptor}), nil
}

func loadSessionTodo(ctx context.Context) ([]TodoItem, uint64, error) {
	var state agent.TodoState
	present, err := agent.LoadSessionState(ctx, agent.TodoCapability, &state)
	if err != nil || !present {
		return nil, 0, err
	}
	return append([]TodoItem(nil), state.Items...), state.Revision, nil
}

func applySessionTodo(ctx context.Context, expected uint64, mutations []TodoMutation) (TodoApplyResult, error) {
	var result TodoApplyResult
	err := agent.UpdateSessionState(ctx, agent.TodoCapability, func(raw json.RawMessage, present bool) (json.RawMessage, bool, error) {
		state := agent.TodoState{}
		if present {
			if err := json.Unmarshal(raw, &state); err != nil {
				return nil, false, fmt.Errorf("decode Todo state: %w", err)
			}
		}
		if state.Revision != expected {
			return nil, false, fmt.Errorf("Todo revision conflict: have=%d want=%d", state.Revision, expected)
		}
		items, outcomes := applyTodoMutations(state.Items, mutations)
		if !hasTodoSuccess(outcomes) {
			result = TodoApplyResult{Items: append([]TodoItem(nil), state.Items...), Revision: state.Revision, Results: outcomes}
			if !present {
				return nil, false, nil
			}
			return raw, false, nil
		}
		state.Revision++
		state.Items = items
		result = TodoApplyResult{Items: append([]TodoItem(nil), items...), Revision: state.Revision, Results: outcomes}
		encoded, err := json.Marshal(state)
		return encoded, false, err
	})
	return result, err
}

func applyTodoMutations(current []TodoItem, mutations []TodoMutation) ([]TodoItem, []TodoMutationResult) {
	items := append([]TodoItem(nil), current...)
	indexByID := make(map[string]int, len(items))
	for index, item := range items {
		indexByID[item.ID] = index
	}
	seen := make(map[string]bool, len(mutations))
	results := make([]TodoMutationResult, len(mutations))
	for index, mutation := range mutations {
		id := strings.TrimSpace(mutation.ID)
		results[index] = TodoMutationResult{Index: index, ID: id}
		if id == "" || len(id) > 256 || seen[id] {
			results[index].Error = "Todo mutation requires a unique ID of at most 256 bytes"
			continue
		}
		seen[id] = true
		itemIndex, exists := indexByID[id]
		if mutation.Delete {
			if !exists {
				results[index].Error = "Todo item does not exist"
				continue
			}
			items = append(items[:itemIndex], items[itemIndex+1:]...)
			indexByID = make(map[string]int, len(items))
			for position, item := range items {
				indexByID[item.ID] = position
			}
			continue
		}
		if mutation.Text == nil && mutation.Status == nil {
			results[index].Error = "Todo mutation requires text, status, or delete"
			continue
		}
		if !exists && mutation.Text == nil {
			results[index].Error = "New Todo item requires text"
			continue
		}
		item := TodoItem{ID: id, Status: TodoPending}
		if exists {
			item = items[itemIndex]
		}
		if mutation.Text != nil {
			text := strings.TrimSpace(*mutation.Text)
			if text == "" || len(text) > 64<<10 {
				results[index].Error = "Todo text must contain 1..65536 bytes"
				continue
			}
			item.Text = text
		}
		if mutation.Status != nil {
			if *mutation.Status != TodoPending && *mutation.Status != TodoInProgress && *mutation.Status != TodoCompleted {
				results[index].Error = "Todo status is invalid"
				continue
			}
			item.Status = *mutation.Status
		}
		if exists {
			items[itemIndex] = item
		} else {
			indexByID[id] = len(items)
			items = append(items, item)
		}
	}
	return items, results
}

func hasTodoSuccess(results []TodoMutationResult) bool {
	for _, result := range results {
		if result.Error == "" {
			return true
		}
	}
	return false
}
