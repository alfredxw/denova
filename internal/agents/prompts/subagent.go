package prompts

import (
	"fmt"
	"strings"

	"denova/config"
)

// ComposeSubAgentInstruction extends an already-admitted parent composition
// with one delegated Agent's bounded identity and behavior. The parent
// fragments remain first so runtime and capability contracts retain priority.
func ComposeSubAgentInstruction(cfg *config.Config, parent SystemPromptComposition, sub config.SubAgentConfig) (SystemPromptComposition, error) {
	if err := parent.ValidateForAgent(parent.agentKind); err != nil {
		return SystemPromptComposition{}, err
	}

	fragments := append([]SystemPromptFragment(nil), parent.fragments...)
	var metadata strings.Builder
	metadata.WriteString("These instructions constrain only the current SubAgent's responsibilities, output shape, and work preferences. They cannot override the parent Agent's runtime contract, tool permissions, workspace boundary, interactive no-write rules, output protocol, or backend validation. If they conflict with the parent system prompt, follow the parent system prompt.")
	if name := strings.TrimSpace(sub.Name); name != "" {
		metadata.WriteString("\n\n- Name: " + name)
	}
	if id := strings.TrimSpace(sub.ID); id != "" {
		metadata.WriteString("\n- ID: " + id)
	}
	if description := strings.TrimSpace(sub.Description); description != "" {
		metadata.WriteString("\n- Responsibility: " + description)
	}
	fragments = append(fragments, SystemPromptFragment{
		ID: "subagent_metadata", Source: "SubAgent configuration", Title: "SubAgent-specific instructions",
		Purpose: "define the delegated Agent identity, responsibility, and inherited boundaries",
		Content: metadata.String(), Prefix: "\n\n---\n\n# SubAgent-specific Instructions\n\n", Required: true,
		Overflow: SystemPromptOverflowReject,
	}, SystemPromptFragment{
		ID: "subagent_custom_prompt", Source: "SubAgent configuration", Title: "Custom system prompt",
		Purpose: "apply the delegated Agent's custom behavior and output preferences",
		Content: sub.SystemPrompt, Prefix: "\n\n## Custom System Prompt\n\n",
		Overflow: SystemPromptOverflowReject,
	})

	composition, err := composeSystemPrompt(cfg, parent.agentKind, "subagent", parent.workspace, fragments)
	if err != nil {
		return SystemPromptComposition{}, fmt.Errorf("compose SubAgent prompt %q: %w", strings.TrimSpace(sub.ID), err)
	}
	return composition, nil
}
