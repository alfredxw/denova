package agent

import (
	"context"
	"fmt"
	"sync"
)

// Registry owns a stable, insertion-ordered set of complete tool definitions.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]ToolDefinition
	snapshots   map[string]ToolDefinitionSnapshot
	ordered     []string
}

// NewRegistry validates and registers definitions in source order.
func NewRegistry(ctx context.Context, definitions ...ToolDefinition) (*Registry, error) {
	registry := &Registry{}
	for _, definition := range definitions {
		if err := registry.Register(ctx, definition); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register adds one definition and rejects invalid or duplicate names.
func (registry *Registry) Register(ctx context.Context, definition ToolDefinition) error {
	snapshot, err := definition.snapshot(ctx)
	if err != nil {
		return fmt.Errorf("register tool definition: %w", err)
	}
	name := snapshot.Info.Name

	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureMaps()
	if _, exists := registry.definitions[name]; exists {
		return fmt.Errorf("register tool %q: duplicate name", name)
	}
	registry.definitions[name] = definition
	registry.snapshots[name] = snapshot
	registry.ordered = append(registry.ordered, name)
	return nil
}

// Lookup returns a named definition.
func (registry *Registry) Lookup(name string) (ToolDefinition, bool) {
	if registry == nil {
		return ToolDefinition{}, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	definition, exists := registry.definitions[name]
	return definition, exists
}

// Snapshot returns immutable schema and descriptor metadata captured during
// registration.
func (registry *Registry) Snapshot(name string) (ToolDefinitionSnapshot, bool) {
	if registry == nil {
		return ToolDefinitionSnapshot{}, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	snapshot, exists := registry.snapshots[name]
	snapshot.Info = cloneToolInfo(snapshot.Info)
	return snapshot, exists
}

// Schemas returns tool descriptions in registration order.
func (registry *Registry) Schemas() []*ToolInfo {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]*ToolInfo, 0, len(registry.ordered))
	for _, name := range registry.ordered {
		result = append(result, cloneToolInfo(registry.snapshots[name].Info))
	}
	return result
}

// Definitions returns complete definitions in registration order.
func (registry *Registry) Definitions() []ToolDefinition {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]ToolDefinition, 0, len(registry.ordered))
	for _, name := range registry.ordered {
		result = append(result, registry.definitions[name])
	}
	return result
}

// Snapshots returns immutable schema/descriptor pairs in registration order.
func (registry *Registry) Snapshots() []ToolDefinitionSnapshot {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]ToolDefinitionSnapshot, 0, len(registry.ordered))
	for _, name := range registry.ordered {
		snapshot := registry.snapshots[name]
		snapshot.Info = cloneToolInfo(snapshot.Info)
		result = append(result, snapshot)
	}
	return result
}

// Len returns the number of registered tools.
func (registry *Registry) Len() int {
	if registry == nil {
		return 0
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return len(registry.ordered)
}

func (registry *Registry) ensureMaps() {
	if registry.definitions == nil {
		registry.definitions = make(map[string]ToolDefinition)
	}
	if registry.snapshots == nil {
		registry.snapshots = make(map[string]ToolDefinitionSnapshot)
	}
}
