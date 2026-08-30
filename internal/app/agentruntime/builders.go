// Package agentruntime owns application-level Agent construction shared by
// writing, game, automation, image, and configuration modes.
package agentruntime

import (
	"context"
	"fmt"

	"denova/config"
	agents "denova/internal/agents"
	agentinteractive "denova/internal/agents/interactive"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"
)

type BuiltAgent struct {
	Definition  agents.Definition
	Composition prompts.SystemPromptComposition
}

func BuildConversationAgent(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.IDEStoryTeller,
	agentKind string,
	host agents.AgentHostCapabilities,
) (BuiltAgent, error) {
	host.Interactive = true
	var definition agents.Definition
	var composition prompts.SystemPromptComposition
	var err error
	switch agentKind {
	case agentrun.AgentKindGeneral:
		definition, composition, err = agents.BuildGeneralDefinitionWithCompositionForHost(ctx, cfg, state, host)
		if err != nil {
			return BuiltAgent{}, fmt.Errorf("build General Agent Definition: %w", err)
		}
	case agentrun.AgentKindHarness:
		definition, composition, err = agents.BuildHarnessDefinitionWithCompositionForHost(ctx, cfg, host)
		if err != nil {
			return BuiltAgent{}, fmt.Errorf("build Harness Agent Definition: %w", err)
		}
	case agentrun.AgentKindIDE:
		definition, composition, err = agents.BuildDefinitionWithCompositionForHost(ctx, cfg, state, teller, host)
		if err != nil {
			return BuiltAgent{}, fmt.Errorf("build Writing Agent Definition: %w", err)
		}
	default:
		return BuiltAgent{}, fmt.Errorf("unsupported conversation Agent kind %q", agentKind)
	}
	return BuiltAgent{Definition: definition, Composition: composition}, nil
}

func BuildInteractiveAgent(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.InteractiveStorySystemInstructionInput,
	host agents.AgentHostCapabilities,
	toolContexts ...agentinteractive.InteractiveStoryToolContext,
) (BuiltAgent, error) {
	host.Interactive = true
	definition, composition, err := agents.BuildInteractiveStoryDefinitionWithCompositionForHost(
		ctx, cfg, state, teller, host, toolContexts...,
	)
	if err != nil {
		return BuiltAgent{}, fmt.Errorf("build Interactive Story Agent Definition: %w", err)
	}
	return BuiltAgent{Definition: definition, Composition: composition}, nil
}

func BuildImageAgent(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (BuiltAgent, error) {
	definition, composition, err := BuildImageDefinition(ctx, cfg, state, systemPrompt)
	if err != nil {
		return BuiltAgent{}, err
	}
	return BuiltAgent{Definition: definition, Composition: composition}, nil
}

func BuildImageDefinition(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	systemPrompt string,
) (agents.Definition, prompts.SystemPromptComposition, error) {
	definition, composition, err := agents.BuildImageDefinitionWithComposition(ctx, cfg, state, systemPrompt)
	if err != nil {
		return agents.Definition{}, prompts.SystemPromptComposition{}, fmt.Errorf("build Image Agent Definition: %w", err)
	}
	return definition, composition, nil
}
