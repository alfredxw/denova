package config

import (
	"errors"
	"fmt"
	"strings"
)

var ErrCustomAgentNotFound = errors.New("custom Agent is unavailable")

// CustomAgentConfig is a user-owned Agent instance backed by one immutable
// runtime kind. Every behavioral field is a sparse override; protected runtime
// protocols and the base kind's tool ceiling remain owned by Denova.
type CustomAgentConfig struct {
	ID                string               `toml:"id,omitempty" json:"id,omitempty"`
	Name              string               `toml:"name,omitempty" json:"name,omitempty"`
	Description       string               `toml:"description,omitempty" json:"description,omitempty"`
	BaseKind          string               `toml:"base_kind,omitempty" json:"base_kind,omitempty"`
	Enabled           *bool                `toml:"enabled,omitempty" json:"enabled,omitempty"`
	Model             AgentModelOverride   `toml:"model,omitempty" json:"model,omitempty"`
	Tools             AgentToolOverride    `toml:"tools,omitempty" json:"tools,omitempty"`
	Prompt            AgentPromptOverride  `toml:"prompt,omitempty" json:"prompt,omitempty"`
	Skills            AgentSkillOverride   `toml:"skills,omitempty" json:"skills,omitempty"`
	Context           AgentContextOverride `toml:"context,omitempty" json:"context,omitempty"`
	ImageAPIProfileID string               `toml:"image_api_profile_id,omitempty" json:"image_api_profile_id,omitempty"`
}

var customizableAgentKinds = []string{
	AgentKindGeneral,
	AgentKindIDE,
	AgentKindInteractiveStory,
	AgentKindImage,
}

// CustomizableAgentKinds returns the fixed runtime kinds that may back a
// user-defined Agent instance.
func CustomizableAgentKinds() []string {
	out := make([]string, len(customizableAgentKinds))
	copy(out, customizableAgentKinds)
	return out
}

func IsCustomizableAgentKind(kind string) bool {
	kind = strings.TrimSpace(kind)
	for _, candidate := range customizableAgentKinds {
		if kind == candidate {
			return true
		}
	}
	return false
}

func NormalizeCustomAgentID(id string) string {
	return NormalizeSubAgentID(id)
}

func CustomAgentEnabled(agent CustomAgentConfig) bool {
	return boolValue(agent.Enabled, true)
}

// MergeCustomAgents merges stable Agent identities while preserving sparse
// user/workspace overrides. BaseKind is immutable once inherited.
func MergeCustomAgents(parent, child []CustomAgentConfig) []CustomAgentConfig {
	if len(child) == 0 {
		return parent
	}
	out := make([]CustomAgentConfig, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, agent := range SanitizeCustomAgents(parent) {
		index[agent.ID] = len(out)
		out = append(out, agent)
	}
	for _, agent := range SanitizeCustomAgents(child) {
		if position, ok := index[agent.ID]; ok {
			out[position] = mergeCustomAgent(out[position], agent)
			continue
		}
		index[agent.ID] = len(out)
		out = append(out, agent)
	}
	return out
}

func SanitizeCustomAgents(agents []CustomAgentConfig) []CustomAgentConfig {
	if len(agents) == 0 {
		return agents
	}
	out := make([]CustomAgentConfig, 0, len(agents))
	seen := make(map[string]bool, len(agents))
	for _, agent := range agents {
		agent.ID = NormalizeCustomAgentID(agent.ID)
		if agent.ID == "" || seen[agent.ID] {
			continue
		}
		agent.Name = strings.TrimSpace(agent.Name)
		agent.Description = strings.TrimSpace(agent.Description)
		agent.BaseKind = strings.TrimSpace(agent.BaseKind)
		if agent.BaseKind != "" && !IsCustomizableAgentKind(agent.BaseKind) {
			continue
		}
		agent.Model.ProfileID = normalizeModelProfileID(agent.Model.ProfileID)
		if agent.Model.ThinkingLevel != "" {
			agent.Model.ThinkingLevel = normalizeThinkingLevel(agent.Model.ThinkingLevel)
		}
		agent.Prompt = sanitizeAgentPromptOverride(agent.Prompt)
		agent.Skills = mergeAgentSkillOverride(nil, agent.Skills)
		agent.Context = sanitizeAgentContextOverride(agent.Context)
		agent.ImageAPIProfileID = strings.TrimSpace(agent.ImageAPIProfileID)
		seen[agent.ID] = true
		out = append(out, agent)
	}
	return out
}

// FindCustomAgent returns an enabled, complete Agent definition from the
// effective runtime catalog.
func FindCustomAgent(cfg *Config, id string) (CustomAgentConfig, bool) {
	return findCustomAgent(cfg, id, false)
}

func findCustomAgent(cfg *Config, id string, includeDisabled bool) (CustomAgentConfig, bool) {
	id = NormalizeCustomAgentID(id)
	if cfg == nil || id == "" {
		return CustomAgentConfig{}, false
	}
	for _, agent := range cfg.CustomAgents {
		if agent.ID != id || (!includeDisabled && !CustomAgentEnabled(agent)) {
			continue
		}
		if strings.TrimSpace(agent.Name) == "" || !IsCustomizableAgentKind(agent.BaseKind) {
			return CustomAgentConfig{}, false
		}
		return agent, true
	}
	return CustomAgentConfig{}, false
}

// ApplyCustomAgent projects one instance onto a request-local runtime config.
// The runtime kind remains unchanged, so all builder protocols and capability
// ceilings continue to be selected from the fixed registry.
func ApplyCustomAgent(cfg *Config, baseKind, id string) error {
	return applyCustomAgent(cfg, baseKind, id, false)
}

// ApplyPersistedCustomAgent keeps an archived Agent runnable for conversations
// that already persist its identity. New selections continue to use
// ApplyCustomAgent, which excludes archived instances.
func ApplyPersistedCustomAgent(cfg *Config, baseKind, id string) error {
	return applyCustomAgent(cfg, baseKind, id, true)
}

func applyCustomAgent(cfg *Config, baseKind, id string, includeDisabled bool) error {
	id = NormalizeCustomAgentID(id)
	if id == "" {
		if cfg != nil {
			cfg.ActiveCustomAgentID = ""
			cfg.ActiveCustomAgentName = ""
		}
		return nil
	}
	if cfg == nil {
		return errors.New("runtime config is nil")
	}
	agent, ok := findCustomAgent(cfg, id, includeDisabled)
	if !ok {
		return fmt.Errorf("%w: %s", ErrCustomAgentNotFound, id)
	}
	baseKind = strings.TrimSpace(baseKind)
	if agent.BaseKind != baseKind {
		return fmt.Errorf("custom Agent %q is based on %q, not %q", id, agent.BaseKind, baseKind)
	}
	definition, ok := LookupAgentKind(baseKind)
	if !ok || definition.SetModelOverride == nil || definition.SetToolOverride == nil ||
		definition.SetPromptOverride == nil || definition.SetSkillOverride == nil || definition.SetContextOverride == nil {
		return fmt.Errorf("custom Agent base kind %q is unsupported", baseKind)
	}
	definition.SetModelOverride(&cfg.AgentModels, mergeAgentModelOverride(definition.ModelOverride(cfg.AgentModels), agent.Model))
	definition.SetToolOverride(&cfg.AgentTools, mergeAgentToolOverride(definition.ToolOverride(cfg.AgentTools), agent.Tools))
	definition.SetPromptOverride(&cfg.AgentPrompts, mergeAgentPromptOverride(definition.PromptOverride(cfg.AgentPrompts), agent.Prompt))
	definition.SetSkillOverride(&cfg.AgentSkills, mergeAgentSkillOverride(definition.SkillOverride(cfg.AgentSkills), agent.Skills))
	definition.SetContextOverride(&cfg.AgentContexts, mergeAgentContextOverride(definition.ContextOverride(cfg.AgentContexts), agent.Context))
	if baseKind == AgentKindImage && agent.ImageAPIProfileID != "" {
		cfg.DefaultImageAPIProfileID = agent.ImageAPIProfileID
	}
	cfg.ActiveCustomAgentID = agent.ID
	cfg.ActiveCustomAgentName = agent.Name
	return nil
}

func mergeCustomAgent(parent, child CustomAgentConfig) CustomAgentConfig {
	out := parent
	if child.ID != "" {
		out.ID = child.ID
	}
	if child.Name != "" {
		out.Name = child.Name
	}
	if child.Description != "" {
		out.Description = child.Description
	}
	if out.BaseKind == "" && child.BaseKind != "" {
		out.BaseKind = child.BaseKind
	}
	if child.Enabled != nil {
		out.Enabled = child.Enabled
	}
	out.Model = mergeAgentModelOverride(out.Model, child.Model)
	out.Tools = mergeAgentToolOverride(out.Tools, child.Tools)
	out.Prompt = mergeAgentPromptOverride(out.Prompt, child.Prompt)
	out.Skills = mergeAgentSkillOverride(out.Skills, child.Skills)
	out.Context = mergeAgentContextOverride(out.Context, child.Context)
	if child.ImageAPIProfileID != "" {
		out.ImageAPIProfileID = child.ImageAPIProfileID
	}
	return out
}
