package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Registry owns a stable, insertion-ordered set of uniquely named tools.
type Registry struct {
	mu      sync.RWMutex
	tools   map[string]BaseTool
	infos   map[string]*ToolInfo
	ordered []string
}

// NewRegistry validates and registers tools in source order.
func NewRegistry(ctx context.Context, tools ...BaseTool) (*Registry, error) {
	registry := &Registry{}
	for _, current := range tools {
		if err := registry.Register(ctx, current); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register adds one tool and rejects empty or duplicate names.
func (registry *Registry) Register(ctx context.Context, current BaseTool) error {
	if current == nil {
		return errors.New("register tool: nil tool")
	}
	info, err := current.Info(ctx)
	if err != nil {
		return fmt.Errorf("register tool: get info: %w", err)
	}
	if info == nil {
		return errors.New("register tool: nil info")
	}
	name := strings.TrimSpace(info.Name)
	if name == "" {
		return errors.New("register tool: empty name")
	}
	if name != info.Name {
		return fmt.Errorf("register tool %q: leading or trailing whitespace is not allowed", info.Name)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureMaps()
	if _, exists := registry.tools[name]; exists {
		return fmt.Errorf("register tool %q: duplicate name", name)
	}
	registry.tools[name] = current
	registry.infos[name] = cloneToolInfo(info)
	registry.ordered = append(registry.ordered, name)
	return nil
}

// Lookup returns a named tool.
func (registry *Registry) Lookup(name string) (BaseTool, bool) {
	if registry == nil {
		return nil, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	current, exists := registry.tools[name]
	return current, exists
}

// Info returns the immutable description captured when the tool was
// registered. Middleware can use Extra metadata without asking the concrete
// tool to rebuild its schema for every invocation.
func (registry *Registry) Info(name string) (*ToolInfo, bool) {
	if registry == nil {
		return nil, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	info, exists := registry.infos[name]
	return cloneToolInfo(info), exists
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
		result = append(result, cloneToolInfo(registry.infos[name]))
	}
	return result
}

// Tools returns tools in registration order.
func (registry *Registry) Tools() []BaseTool {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]BaseTool, 0, len(registry.ordered))
	for _, name := range registry.ordered {
		result = append(result, registry.tools[name])
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
	if registry.tools == nil {
		registry.tools = make(map[string]BaseTool)
	}
	if registry.infos == nil {
		registry.infos = make(map[string]*ToolInfo)
	}
}
