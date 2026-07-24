package tools

import (
	"context"
	"fmt"
	"sync"

	agent "github.com/alfredxw/denova/agent"
)

// Registry owns validated Definitions in stable insertion order.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
	tools       map[string]agent.BaseTool
	infos       map[string]*agent.ToolInfo
	ordered     []string
}

// Build validates and registers definitions in source order.
func Build(ctx context.Context, definitions ...Definition) (*Registry, error) {
	registry := &Registry{}
	for _, definition := range definitions {
		if err := registry.Register(ctx, definition); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Validate checks a complete definition slice, including duplicate names.
func Validate(ctx context.Context, definitions ...Definition) error {
	_, err := Build(ctx, definitions...)
	return err
}

// Register adds one definition. The only name is read from Tool.Info.
func (registry *Registry) Register(ctx context.Context, definition Definition) error {
	if err := definition.Validate(ctx); err != nil {
		return fmt.Errorf("register tool definition: %w", err)
	}
	tool := described(definition)
	info, err := tool.Info(ctx)
	if err != nil {
		return fmt.Errorf("register tool definition: %w", err)
	}
	name := info.Name
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureMaps()
	if _, exists := registry.definitions[name]; exists {
		return fmt.Errorf("register tool %q: duplicate name", name)
	}
	registry.definitions[name] = definition
	registry.tools[name] = tool
	registry.infos[name] = info
	registry.ordered = append(registry.ordered, name)
	return nil
}

// Lookup returns a complete named definition.
func (registry *Registry) Lookup(name string) (Definition, bool) {
	if registry == nil {
		return Definition{}, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	definition, ok := registry.definitions[name]
	return definition, ok
}

// Schemas returns provider-visible tool descriptions in registration order.
func (registry *Registry) Schemas() []*agent.ToolInfo {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]*agent.ToolInfo, 0, len(registry.ordered))
	for _, name := range registry.ordered {
		info := *registry.infos[name]
		info.Extra = cloneExtra(info.Extra)
		result = append(result, &info)
	}
	return result
}

// Tools returns descriptor-carrying tools ready for AgentConfig.
func (registry *Registry) Tools() []agent.BaseTool {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]agent.BaseTool, 0, len(registry.ordered))
	for _, name := range registry.ordered {
		result = append(result, registry.tools[name])
	}
	return result
}

// Definitions returns registration units in source order.
func (registry *Registry) Definitions() []Definition {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]Definition, 0, len(registry.ordered))
	for _, name := range registry.ordered {
		result = append(result, registry.definitions[name])
	}
	return result
}

func (registry *Registry) ensureMaps() {
	if registry.definitions == nil {
		registry.definitions = make(map[string]Definition)
	}
	if registry.tools == nil {
		registry.tools = make(map[string]agent.BaseTool)
	}
	if registry.infos == nil {
		registry.infos = make(map[string]*agent.ToolInfo)
	}
}
