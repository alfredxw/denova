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
	"denova/internal/agents/session"
	agenttools "denova/internal/agents/tools"
	"denova/internal/book"

	agent "github.com/alfredxw/denova/agent"
)

type BuiltAgent struct {
	Definition  agents.Definition
	Composition prompts.SystemPromptComposition
}

// GoalTools returns the root-only tool surface for one conversation. The tool
// remains registered while no goal is active so model tool schemas stay stable
// across turns; its captured revision fails closed if the goal changes.
func GoalTools(ctx context.Context, sess *session.Session) ([]agents.ToolDefinition, error) {
	if sess == nil {
		return nil, fmt.Errorf("build goal tools: session is nil")
	}
	current, _, err := sess.Goal(ctx)
	if err != nil {
		return nil, fmt.Errorf("read conversation goal: %w", err)
	}
	definition, err := agenttools.NewGoalFinish(sess, current)
	if err != nil {
		return nil, fmt.Errorf("build goal_finish tool: %w", err)
	}
	return []agents.ToolDefinition{definition}, nil
}

func BuildConversationAgent(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.IDEStoryTeller,
	agentKind string,
	rootTools ...agents.ToolDefinition,
) (BuiltAgent, error) {
	definition, composition, err := BuildConversationDefinition(ctx, cfg, state, teller, agentKind, rootTools...)
	if err != nil {
		return BuiltAgent{}, err
	}
	return BuiltAgent{Definition: definition, Composition: composition}, nil
}

// BuildConversationDefinition creates the writing/general Definition and its
// exact prompt composition for the public Agent lifecycle.
func BuildConversationDefinition(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.IDEStoryTeller,
	agentKind string,
	rootTools ...agents.ToolDefinition,
) (agents.Definition, prompts.SystemPromptComposition, error) {
	host := agents.AgentHostCapabilities{Interactive: true, RootTools: rootTools}
	switch agentKind {
	case agentrun.AgentKindGeneral:
		definition, composition, err := agents.BuildGeneralDefinitionWithCompositionForHost(ctx, cfg, host)
		if err != nil {
			return agents.Definition{}, prompts.SystemPromptComposition{}, fmt.Errorf("build General Agent Definition: %w", err)
		}
		return definition, composition, nil
	case agentrun.AgentKindIDE:
		definition, composition, err := agents.BuildDefinitionWithCompositionForHost(ctx, cfg, state, teller, host)
		if err != nil {
			return agents.Definition{}, prompts.SystemPromptComposition{}, fmt.Errorf("build Writing Agent Definition: %w", err)
		}
		return definition, composition, nil
	default:
		return agents.Definition{}, prompts.SystemPromptComposition{}, fmt.Errorf("unsupported conversation Agent kind %q", agentKind)
	}
}

func BuildHarnessOptimizerAgent(
	ctx context.Context,
	cfg *config.Config,
	readAdapters []agenttools.ReadAdapterBinding,
	completionGuard func(context.Context, *agent.RetryContext) *agent.RetryDecision,
	rootTools ...agents.ToolDefinition,
) (BuiltAgent, error) {
	definition, composition, err := agents.BuildHarnessOptimizerDefinitionWithCompositionForHost(
		ctx, cfg, agents.AgentHostCapabilities{
			Interactive: true, RootTools: rootTools, ReadAdapters: readAdapters, CompletionGuard: completionGuard,
		},
	)
	if err != nil {
		return BuiltAgent{}, fmt.Errorf("build Harness Optimizer Definition: %w", err)
	}
	return BuiltAgent{Definition: definition, Composition: composition}, nil
}

func BuildInteractiveAgent(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agentinteractive.InteractiveStoryToolContext) (BuiltAgent, error) {
	definition, composition, err := BuildInteractiveDefinition(ctx, cfg, state, teller, toolContexts...)
	if err != nil {
		return BuiltAgent{}, err
	}
	return BuiltAgent{Definition: definition, Composition: composition}, nil
}

func BuildInteractiveDefinition(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.InteractiveStorySystemInstructionInput,
	toolContexts ...agentinteractive.InteractiveStoryToolContext,
) (agents.Definition, prompts.SystemPromptComposition, error) {
	definition, composition, err := agents.BuildInteractiveStoryDefinitionWithComposition(ctx, cfg, state, teller, toolContexts...)
	if err != nil {
		return agents.Definition{}, prompts.SystemPromptComposition{}, fmt.Errorf("build Interactive Story Agent Definition: %w", err)
	}
	return definition, composition, nil
}

func BuildConfigManagerAgent(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...prompts.ConfigManagerResourceSkill) (BuiltAgent, error) {
	definition, composition, err := BuildConfigManagerDefinition(ctx, cfg, state, resourceSkills...)
	if err != nil {
		return BuiltAgent{}, err
	}
	return BuiltAgent{Definition: definition, Composition: composition}, nil
}

func BuildConfigManagerDefinition(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	resourceSkills ...prompts.ConfigManagerResourceSkill,
) (agents.Definition, prompts.SystemPromptComposition, error) {
	definition, composition, err := agents.BuildConfigManagerDefinitionWithCompositionForHost(
		ctx, cfg, state, agents.AgentHostCapabilities{Interactive: true}, resourceSkills...,
	)
	if err != nil {
		return agents.Definition{}, prompts.SystemPromptComposition{}, fmt.Errorf("build Config Manager Agent Definition: %w", err)
	}
	return definition, composition, nil
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
