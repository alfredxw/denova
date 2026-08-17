package builtin

import (
	"context"
	"errors"
	"sync"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

type modelDefinition struct {
	config    providers.ModelConfig
	configErr error

	once     sync.Once
	model    agent.ToolCallingChatModel
	identity agent.CapabilityIdentity
	err      error
}

// Model declares a model from the built-in provider catalog. Agent resolves
// the protocol adapter and stable credential-free identity in agent.New.
func Model(config providers.ModelConfig) agent.BaseChatModel {
	cloned, err := config.Clone()
	return &modelDefinition{config: cloned, configErr: err}
}

func (definition *modelDefinition) InitializeDefinition(ctx context.Context) error {
	if definition == nil {
		return errors.New("built-in Model Definition is nil")
	}
	definition.once.Do(func() {
		if definition.configErr != nil {
			definition.err = definition.configErr
			return
		}
		registry, err := NewRegistry()
		if err != nil {
			definition.err = err
			return
		}
		definition.model, definition.config, definition.err = registry.NewChatModelWithResolvedConfig(ctx, definition.config)
		if definition.err != nil {
			return
		}
		definition.identity, definition.err = providers.ModelIdentity(definition.config)
	})
	return definition.err
}

func (definition *modelDefinition) ModelIdentity() agent.CapabilityIdentity {
	if err := definition.InitializeDefinition(context.Background()); err != nil {
		return agent.CapabilityIdentity{}
	}
	return definition.identity
}

func (definition *modelDefinition) Generate(
	ctx context.Context,
	input []*agent.Message,
	options ...agent.ModelOption,
) (*agent.Message, error) {
	if err := definition.InitializeDefinition(ctx); err != nil {
		return nil, err
	}
	return definition.model.Generate(ctx, input, options...)
}

func (definition *modelDefinition) Stream(
	ctx context.Context,
	input []*agent.Message,
	options ...agent.ModelOption,
) (*agent.StreamReader[*agent.Message], error) {
	if err := definition.InitializeDefinition(ctx); err != nil {
		return nil, err
	}
	return definition.model.Stream(ctx, input, options...)
}

func (definition *modelDefinition) WithTools(tools []*agent.ToolInfo) (agent.ToolCallingChatModel, error) {
	if err := definition.InitializeDefinition(context.Background()); err != nil {
		return nil, err
	}
	return definition.model.WithTools(tools)
}

var _ agent.DefinitionInitializer = (*modelDefinition)(nil)
var _ agent.DefinitionModel = (*modelDefinition)(nil)
var _ agent.ToolCallingChatModel = (*modelDefinition)(nil)
