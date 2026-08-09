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
	"denova/internal/agents/session"
	agenttools "denova/internal/agents/tools"
	"denova/internal/book"
)

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

// BuildConversation creates either a writing or a general-purpose project
// runner. Agent kind is explicit because it determines the prompt and tools.
func BuildConversation(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.IDEStoryTeller,
	agentKind string,
	rootTools ...agents.ToolDefinition,
) (*agents.Runner, prompts.SystemPromptComposition, error) {
	switch agentKind {
	case agentrun.AgentKindGeneral:
		built, composition, err := agents.BuildGeneralAgentWithCompositionForHost(
			ctx, cfg, agents.AgentHostCapabilities{Interactive: true, RootTools: rootTools},
		)
		if err != nil {
			return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build General Agent: %w", err)
		}
		return agentchat.NewRunnerWithOptions(ctx, built, runnerOptions(cfg, agentKind)), composition, nil
	case agentrun.AgentKindIDE:
		built, composition, err := agents.BuildWithCompositionForHost(
			ctx, cfg, state, teller, agents.AgentHostCapabilities{Interactive: true, RootTools: rootTools},
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
