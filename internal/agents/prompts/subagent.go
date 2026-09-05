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
	metadata.WriteString("Only the root Agent may interact with the user. Never ask the user or wait for user input. Make safe assumptions when possible; otherwise return the blocker to the parent Agent.")
	name := strings.TrimSpace(sub.Name)
	if name == "" {
		name = strings.TrimSpace(sub.ID)
	}
	if name != "" {
		metadata.WriteString("\n")
		metadata.WriteString("Name: " + name)
	}
	if description := strings.TrimSpace(sub.Description); description != "" {
		if metadata.Len() > 0 {
			metadata.WriteString("\n")
		}
		metadata.WriteString("Responsibility: " + description)
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
