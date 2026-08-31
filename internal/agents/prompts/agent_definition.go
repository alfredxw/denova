package prompts

import (
	"strings"

	"denova/config"
)

// applyAgentPromptDefinition keeps the Engine and Experience contracts owned
// by Denova while replacing the user-owned workflow fragment. Built-in Agents
// still support sparse flow/custom overrides; custom Agents own the workflow
// outright, including an intentionally empty workflow.
func applyAgentPromptDefinition(cfg *config.Config, agentKind string, fragments []SystemPromptFragment) []SystemPromptFragment {
	resolved := config.ResolveAgentPrompt(cfg, agentKind)
	flow := strings.TrimSpace(resolved.FlowPrompt)
	custom := strings.TrimSpace(resolved.SystemPrompt)
	explicitFlow := flow != ""
	source := "Agent configuration"
	title := "Configured Agent workflow"
	purpose := "apply the configured Agent workflow"
	if definition, ok := config.FindActiveCustomAgent(cfg); ok && config.CustomAgentRuntimeKind(definition) == strings.TrimSpace(agentKind) {
		flow = strings.TrimSpace(definition.Instructions)
		custom = ""
		explicitFlow = true
		source = "custom Agent definition"
		title = "Custom Agent instructions"
		purpose = "apply the complete user-owned Agent behavior"
	}

	result := append([]SystemPromptFragment(nil), fragments...)
	if explicitFlow {
		for index := range result {
			if result[index].ID != "builtin_base" {
				continue
			}
			result[index].Source = source
			result[index].Title = title
			result[index].Purpose = purpose
			result[index].Content = flow
			result[index].Required = flow != ""
			break
		}
	}
	if custom != "" {
		result = append(result, SystemPromptFragment{
			ID: "agent_custom_rules", Source: "Agent configuration", Title: "Custom Agent rules",
			Purpose: "apply additional user-authored Agent behavior", Content: custom,
			Prefix: "\n\n## Agent Custom Rules\n\n", Overflow: SystemPromptOverflowTruncate,
		})
	}
	return result
}
