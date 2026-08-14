package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agent "github.com/alfredxw/denova/agent"
)

// definitionToolset delays construction until Agent accepts its Definition.
// This keeps a complete capability composition declarative while preserving
// ordinary errors instead of panicking from convenience constructors.
type definitionToolset struct {
	build func(context.Context) (agent.Toolset, error)

	once    sync.Once
	toolset agent.Toolset
	err     error
}

func defineToolset(build func(context.Context) (agent.Toolset, error)) agent.Toolset {
	return &definitionToolset{build: build}
}

func (definition *definitionToolset) InitializeDefinition(ctx context.Context) error {
	if definition == nil {
		return errors.New("toolset Definition is nil")
	}
	definition.once.Do(func() {
		if definition.build == nil {
			definition.err = errors.New("toolset Definition builder is nil")
			return
		}
		definition.toolset, definition.err = definition.build(ctx)
		if definition.err == nil && definition.toolset == nil {
			definition.err = errors.New("toolset Definition returned nil")
		}
	})
	return definition.err
}

func (definition *definitionToolset) Identity() agent.CapabilityIdentity {
	if err := definition.InitializeDefinition(context.Background()); err != nil {
		return agent.CapabilityIdentity{}
	}
	return definition.toolset.Identity()
}

func (definition *definitionToolset) PrepareTools(
	ctx context.Context,
	request agent.ToolRequest,
) ([]agent.ToolDefinition, error) {
	if err := definition.InitializeDefinition(ctx); err != nil {
		return nil, err
	}
	return definition.toolset.PrepareTools(ctx, request)
}

func initializeToolset(ctx context.Context, index int, toolset agent.Toolset) error {
	if toolset == nil {
		return nil
	}
	if initializer, ok := toolset.(agent.DefinitionInitializer); ok {
		if err := initializer.InitializeDefinition(ctx); err != nil {
			return fmt.Errorf("Toolset[%d]: %w", index, err)
		}
	}
	return nil
}

var _ agent.DefinitionInitializer = (*definitionToolset)(nil)
var _ agent.Toolset = (*definitionToolset)(nil)
