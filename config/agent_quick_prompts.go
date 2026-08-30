package config

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	AgentQuickPromptBehaviorFill = "fill"
	AgentQuickPromptBehaviorSend = "send"

	maxAgentQuickPromptIdentifierRunes = 128
	maxAgentQuickPromptNameRunes       = 128
	maxAgentQuickPromptBytes           = 64 * 1024
)

var ErrInvalidAgentQuickPrompt = errors.New("invalid Agent quick prompt configuration")

// AgentQuickPromptSettings is one user-owned prompt starter. Registry keys
// identify the page surface; array order is the visible shortcut order.
type AgentQuickPromptSettings struct {
	ID       string `toml:"id" json:"id"`
	Name     string `toml:"name" json:"name"`
	Prompt   string `toml:"prompt" json:"prompt"`
	Behavior string `toml:"behavior" json:"behavior"`
	Enabled  bool   `toml:"enabled" json:"enabled"`
}

// AgentQuickPromptRegistry contains only user-customized page groups. Missing
// groups continue to use the frontend's localized built-in defaults.
type AgentQuickPromptRegistry map[string][]AgentQuickPromptSettings

func cloneAgentQuickPrompts(registry AgentQuickPromptRegistry) AgentQuickPromptRegistry {
	if registry == nil {
		return nil
	}
	result := make(AgentQuickPromptRegistry, len(registry))
	for scope, prompts := range registry {
		cloned := make([]AgentQuickPromptSettings, len(prompts))
		copy(cloned, prompts)
		result[scope] = cloned
	}
	return result
}

func normalizeAgentQuickPrompts(registry AgentQuickPromptRegistry) AgentQuickPromptRegistry {
	if registry == nil {
		return nil
	}
	result := make(AgentQuickPromptRegistry, len(registry))
	for rawScope, prompts := range registry {
		scope := strings.TrimSpace(rawScope)
		normalized := make([]AgentQuickPromptSettings, len(prompts))
		for index, prompt := range prompts {
			prompt.ID = strings.TrimSpace(prompt.ID)
			prompt.Name = strings.TrimSpace(prompt.Name)
			prompt.Prompt = strings.TrimSpace(prompt.Prompt)
			prompt.Behavior = strings.TrimSpace(prompt.Behavior)
			if prompt.Behavior == "" {
				prompt.Behavior = AgentQuickPromptBehaviorFill
			}
			normalized[index] = prompt
		}
		result[scope] = normalized
	}
	return result
}

func validateAgentQuickPrompts(registry AgentQuickPromptRegistry) error {
	for scope, prompts := range normalizeAgentQuickPrompts(registry) {
		if err := validateAgentQuickPromptIdentifier(scope); err != nil {
			return fmt.Errorf("%w: agent_quick_prompts scope %q: %v", ErrInvalidAgentQuickPrompt, scope, err)
		}
		seen := make(map[string]struct{}, len(prompts))
		for index, prompt := range prompts {
			path := fmt.Sprintf("agent_quick_prompts[%q][%d]", scope, index)
			if err := validateAgentQuickPromptIdentifier(prompt.ID); err != nil {
				return fmt.Errorf("%w: %s.id: %v", ErrInvalidAgentQuickPrompt, path, err)
			}
			if _, duplicate := seen[prompt.ID]; duplicate {
				return fmt.Errorf("%w: %s.id %q is duplicated", ErrInvalidAgentQuickPrompt, path, prompt.ID)
			}
			seen[prompt.ID] = struct{}{}
			if prompt.Name == "" {
				return fmt.Errorf("%w: %s.name is required", ErrInvalidAgentQuickPrompt, path)
			}
			if utf8.RuneCountInString(prompt.Name) > maxAgentQuickPromptNameRunes {
				return fmt.Errorf("%w: %s.name exceeds %d characters", ErrInvalidAgentQuickPrompt, path, maxAgentQuickPromptNameRunes)
			}
			if prompt.Prompt == "" {
				return fmt.Errorf("%w: %s.prompt is required", ErrInvalidAgentQuickPrompt, path)
			}
			if len(prompt.Prompt) > maxAgentQuickPromptBytes {
				return fmt.Errorf("%w: %s.prompt exceeds %d bytes", ErrInvalidAgentQuickPrompt, path, maxAgentQuickPromptBytes)
			}
			if prompt.Behavior != AgentQuickPromptBehaviorFill && prompt.Behavior != AgentQuickPromptBehaviorSend {
				return fmt.Errorf("%w: %s.behavior must be %q or %q", ErrInvalidAgentQuickPrompt, path, AgentQuickPromptBehaviorFill, AgentQuickPromptBehaviorSend)
			}
		}
	}
	return nil
}

func validateAgentQuickPromptIdentifier(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("is required")
	}
	if utf8.RuneCountInString(value) > maxAgentQuickPromptIdentifierRunes {
		return fmt.Errorf("exceeds %d characters", maxAgentQuickPromptIdentifierRunes)
	}
	for index, character := range value {
		isASCIILetter := character >= 'a' && character <= 'z'
		isASCIIDigit := character >= '0' && character <= '9'
		if isASCIILetter || isASCIIDigit || (index > 0 && (character == '-' || character == '_' || character == '.')) {
			continue
		}
		return errors.New("must use lowercase letters, numbers, '.', '_' or '-'")
	}
	return nil
}
