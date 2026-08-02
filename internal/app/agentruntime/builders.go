// Package agentruntime owns application-level Agent construction shared by
// writing, game, automation, image, and configuration modes.
package agentruntime

import (
	"context"
	"fmt"

	"denova/config"
	agents "denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	agentinteractive "denova/internal/agents/interactive"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"
)

// BuildConversation creates either a writing or a general-purpose project
// runner. Agent kind is explicit because it determines the prompt and tools.
func BuildConversation(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.IDEStoryTeller,
	agentKind string,
) (*agents.Runner, prompts.SystemPromptComposition, error) {
	switch agentKind {
	case agentrun.AgentKindGeneral:
		built, composition, err := agents.BuildGeneralAgentWithCompositionForHost(
			ctx, cfg, agents.AgentHostCapabilities{Interactive: true},
		)
		if err != nil {
			return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build General Agent: %w", err)
		}
		return agentchat.NewRunnerWithOptions(ctx, built, runnerOptions(cfg, agentKind)), composition, nil
	case agentrun.AgentKindIDE:
		built, composition, err := agents.BuildWithCompositionForHost(
			ctx, cfg, state, teller, agents.AgentHostCapabilities{Interactive: true},
		)
		if err != nil {
			return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build Writing Agent: %w", err)
		}
		return agentchat.NewRunnerWithOptions(ctx, built, runnerOptions(cfg, agentKind)), composition, nil
	default:
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("unsupported conversation Agent kind %q", agentKind)
	}
}

// BuildInteractive creates the game-story runner and exact prompt composition.
func BuildInteractive(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.InteractiveStorySystemInstructionInput,
	toolContexts ...agentinteractive.InteractiveStoryToolContext,
) (*agents.Runner, prompts.SystemPromptComposition, error) {
	built, composition, err := agents.BuildInteractiveStoryWithComposition(ctx, cfg, state, teller, toolContexts...)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build Interactive Story Agent: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, built, runnerOptions(cfg, agentrun.AgentKindInteractiveStory)), composition, nil
}

// BuildConfigManager creates the resource-configuration runner and exact
// prompt composition.
func BuildConfigManager(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	resourceSkills ...prompts.ConfigManagerResourceSkill,
) (*agents.Runner, prompts.SystemPromptComposition, error) {
	built, composition, err := agents.BuildConfigManagerAgentWithCompositionForHost(
		ctx, cfg, state, agents.AgentHostCapabilities{Interactive: true}, resourceSkills...,
	)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build Config Manager Agent: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, built, runnerOptions(cfg, agentrun.AgentKindConfigManager)), composition, nil
}

// BuildAutomation creates the runner and exact system-prompt composition
// persisted with the durable run receipt.
func BuildAutomation(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	task prompts.AutomationTaskInstruction,
) (*agents.Runner, prompts.SystemPromptComposition, error) {
	built, composition, err := agents.BuildAutomationAgentWithComposition(ctx, cfg, state, task)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build Automation Agent: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, built, runnerOptions(cfg, agentrun.AgentKindAutomation)), composition, nil
}

// BuildImage creates the image-generation runner and exact prompt composition.
func BuildImage(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	systemPrompt string,
) (*agents.Runner, prompts.SystemPromptComposition, error) {
	built, composition, err := agents.BuildImageAgentWithComposition(ctx, cfg, state, systemPrompt)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build Image Agent: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, built, runnerOptions(cfg, agentrun.AgentKindImage)), composition, nil
}

func runnerOptions(cfg *config.Config, agentKind string) agentrun.Options {
	return agentrun.Options{
		AgentKind: agentKind,
		ProjectID: cfg.ProjectID,
		StateRoot: cfg.ProjectStateDir,
		Workspace: cfg.Workspace,
	}
}
