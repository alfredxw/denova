// Package skillassembly projects one effective Skill catalog into the model
// prompt, callable tool, and reference-read surfaces used by an Agent.
package skillassembly

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"

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
	mode, overrides, pinnedNames := config.ResolveActiveAgentSkillPolicy(cfg, agentKind)
	backend := novaskills.NewAgentBackendWithPolicy(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		agentKind,
		overrides,
		mode == config.AgentSkillPolicyExplicit,
	)
	available, err := backend.List(ctx)
	if err != nil {
		return Assembly{}, fmt.Errorf("list available Skills for Agent %s: %w", agentKind, err)
	}
	if len(available) == 0 {
		return assembly, nil
	}

	pinnedSet := make(map[string]bool, len(pinnedNames))
	for _, name := range pinnedNames {
		pinnedSet[name] = true
	}
	entries := make([]prompts.SkillCatalogEntry, 0, len(pinnedNames))
	for _, item := range available {
		if pinnedSet[item.Name] {
			entries = append(entries, prompts.SkillCatalogEntry{Name: item.Name, Description: item.Description})
		}
	}
	assembly.SystemPrompt, err = prompts.AppendSkillsDiscoveryPrompt(cfg, systemPrompt, entries)
	if err != nil {
		return Assembly{}, fmt.Errorf("append Skill discovery policy for Agent %s: %w", agentKind, err)
	}

	catalog := agenttoolruntime.NewCatalog(cfg)
	maxBytes := config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes
	canonical, err := catalog.Skill(ctx, backend, maxBytes)
	if err != nil {
		return Assembly{}, fmt.Errorf("create canonical Skill descriptor for Agent %s: %w", agentKind, err)
	}
	toolset := publictools.Skills(newSkillSource(backend, maxBytes, struct {
		AgentKind string
		Mode      string
		Overrides map[string]bool
	}{agentKind, mode, overrides}))
	skillTools, err := toolset.PrepareTools(ctx, agent.ToolRequest{})
	if err != nil {
		return Assembly{}, fmt.Errorf("create Skill list/read tool for Agent %s: %w", agentKind, err)
	}
	if len(skillTools) != 1 || canonical.Tool == nil {
		return Assembly{}, fmt.Errorf("create Skill list/read tool for Agent %s: expected exactly one canonical tool", agentKind)
	}
	skillTools[0].Descriptor = canonical.Descriptor
	referenceAdapter, err := catalog.SkillReference(backend)
	if err != nil {
		return Assembly{}, fmt.Errorf("create Skill reference adapter for Agent %s: %w", agentKind, err)
	}
	assembly.Tools = skillTools
	assembly.ReadAdapters = []producttools.ReadAdapterBinding{referenceAdapter}
	return assembly, nil
}
