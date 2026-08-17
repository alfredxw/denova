// Package skillassembly projects one effective Skill catalog into the model
// prompt, callable tool, and reference-read surfaces used by an Agent.
package skillassembly

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/prompts"
	novaskills "denova/internal/agents/skills"
	agenttoolruntime "denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
)

// Assembly contains the three runtime surfaces derived from one effective
// catalog. SystemPrompt remains unchanged when Skills are unavailable.
type Assembly struct {
	SystemPrompt prompts.SystemPromptComposition
	Tools        []agent.ToolDefinition
	ReadAdapters []producttools.ReadAdapterBinding
}

// Build resolves one Agent's effective catalog for prompt assembly and wires
// the callable tool and skill:// reference router to the same filtered
// backend. This keeps Agent-specific visibility and overrides consistent
// across all three surfaces.
func Build(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	enabled bool,
	settings config.ResolvedAgentToolSettings,
	systemPrompt prompts.SystemPromptComposition,
) (Assembly, error) {
	assembly := Assembly{SystemPrompt: systemPrompt}
	if !enabled || !settings.Allows(config.AgentToolSkills) || cfg == nil {
		return assembly, nil
	}
	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		agentKind,
		config.ResolveAgentSkillOverrides(cfg, agentKind),
	)
	available, err := backend.List(ctx)
	if err != nil {
		return Assembly{}, fmt.Errorf("list available Skills for Agent %s: %w", agentKind, err)
	}
	if len(available) == 0 {
		return assembly, nil
	}

	entries := make([]prompts.SkillCatalogEntry, 0, len(available))
	for _, item := range available {
		entries = append(entries, prompts.SkillCatalogEntry{Name: item.Name, Description: item.Description})
	}
	assembly.SystemPrompt, err = prompts.AppendSkillsCatalogPrompt(cfg, systemPrompt, entries)
	if err != nil {
		return Assembly{}, fmt.Errorf("append available Skills catalog for Agent %s: %w", agentKind, err)
	}

	catalog := agenttoolruntime.NewCatalog(cfg)
	skillTool, err := catalog.Skill(ctx, backend, config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes)
	if err != nil {
		return Assembly{}, fmt.Errorf("create Skill tool for Agent %s: %w", agentKind, err)
	}
	if skillTool.Tool == nil {
		return Assembly{}, fmt.Errorf("create Skill tool for Agent %s: tool is unavailable for a non-empty catalog", agentKind)
	}
	referenceAdapter, err := catalog.SkillReference(backend)
	if err != nil {
		return Assembly{}, fmt.Errorf("create Skill reference adapter for Agent %s: %w", agentKind, err)
	}
	assembly.Tools = []agent.ToolDefinition{skillTool}
	assembly.ReadAdapters = []producttools.ReadAdapterBinding{referenceAdapter}
	return assembly, nil
}
