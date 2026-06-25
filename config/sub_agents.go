package config

import "strings"

// SubAgentConfig describes one user-defined delegated subagent.
// ID is the stable agent name used by the delegation tool, so it is normalized
// to a lowercase identifier that can safely cross config, UI and tool calls.
type SubAgentConfig struct {
	ID           string             `toml:"id,omitempty" json:"id,omitempty"`
	Name         string             `toml:"name,omitempty" json:"name,omitempty"`
	Description  string             `toml:"description,omitempty" json:"description,omitempty"`
	SystemPrompt string             `toml:"system_prompt,omitempty" json:"system_prompt,omitempty"`
	Enabled      *bool              `toml:"enabled,omitempty" json:"enabled,omitempty"`
	Parents      []string           `toml:"parents,omitempty" json:"parents,omitempty"`
	Model        AgentModelOverride `toml:"model,omitempty" json:"model,omitempty"`
	Tools        AgentToolOverride  `toml:"tools,omitempty" json:"tools,omitempty"`
}

// AgentGeneralSubAgentSettings stores the built-in General SubAgent switch per
// parent agent. Nil means inherit from default; default fallback is enabled.
type AgentGeneralSubAgentSettings struct {
	Default          *bool `toml:"default,omitempty" json:"default,omitempty"`
	IDE              *bool `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory *bool `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager    *bool `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	Automation       *bool `toml:"automation,omitempty" json:"automation,omitempty"`
}

var deepAgentParentKinds = []string{
	AgentKindIDE,
	AgentKindInteractiveStory,
	AgentKindConfigManager,
	AgentKindAutomation,
}

// DeepAgentParentKinds returns the parent agents that expose Eino task delegation.
func DeepAgentParentKinds() []string {
	out := make([]string, len(deepAgentParentKinds))
	copy(out, deepAgentParentKinds)
	return out
}

func IsDeepAgentParentKind(kind string) bool {
	kind = strings.TrimSpace(kind)
	for _, parent := range deepAgentParentKinds {
		if kind == parent {
			return true
		}
	}
	return false
}

func DefaultAgentGeneralSubAgentSettings() AgentGeneralSubAgentSettings {
	return AgentGeneralSubAgentSettings{Default: boolPtr(true)}
}

func MergeAgentGeneralSubAgentSettings(parent, child AgentGeneralSubAgentSettings) AgentGeneralSubAgentSettings {
	return AgentGeneralSubAgentSettings{
		Default:          mergeBoolOverride(parent.Default, child.Default),
		IDE:              mergeBoolOverride(parent.IDE, child.IDE),
		InteractiveStory: mergeBoolOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:    mergeBoolOverride(parent.ConfigManager, child.ConfigManager),
		Automation:       mergeBoolOverride(parent.Automation, child.Automation),
	}
}

func GeneralSubAgentEnabled(cfg *Config, parentKind string) bool {
	settings := DefaultAgentGeneralSubAgentSettings()
	if cfg != nil {
		settings = MergeAgentGeneralSubAgentSettings(settings, cfg.GeneralSubAgents)
	}
	enabled := boolValue(settings.Default, true)
	if override := generalSubAgentOverrideFor(settings, parentKind); override != nil {
		enabled = *override
	}
	return enabled
}

func generalSubAgentOverrideFor(settings AgentGeneralSubAgentSettings, parentKind string) *bool {
	switch parentKind {
	case AgentKindIDE:
		return settings.IDE
	case AgentKindInteractiveStory:
		return settings.InteractiveStory
	case AgentKindConfigManager:
		return settings.ConfigManager
	case AgentKindAutomation:
		return settings.Automation
	default:
		return nil
	}
}

func mergeBoolOverride(parent, child *bool) *bool {
	if child != nil {
		return child
	}
	return parent
}

func MergeSubAgents(parent, child []SubAgentConfig) []SubAgentConfig {
	if len(child) == 0 {
		return parent
	}
	out := make([]SubAgentConfig, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, sub := range SanitizeSubAgents(parent) {
		index[sub.ID] = len(out)
		out = append(out, sub)
	}
	for _, sub := range SanitizeSubAgents(child) {
		if i, ok := index[sub.ID]; ok {
			out[i] = mergeSubAgent(out[i], sub)
			continue
		}
		index[sub.ID] = len(out)
		out = append(out, sub)
	}
	return out
}

func SanitizeSubAgents(subAgents []SubAgentConfig) []SubAgentConfig {
	if len(subAgents) == 0 {
		return subAgents
	}
	out := make([]SubAgentConfig, 0, len(subAgents))
	seen := map[string]bool{}
	for _, sub := range subAgents {
		sub.ID = NormalizeSubAgentID(sub.ID)
		if sub.ID == "" || seen[sub.ID] {
			continue
		}
		sub.Name = strings.TrimSpace(sub.Name)
		sub.Description = strings.TrimSpace(sub.Description)
		sub.SystemPrompt = strings.TrimSpace(sub.SystemPrompt)
		if sub.Description == "" || sub.SystemPrompt == "" {
			continue
		}
		sub.Parents = sanitizeSubAgentParents(sub.Parents)
		sub.Model.ProfileID = normalizeModelProfileID(sub.Model.ProfileID)
		sub.Model.ReasoningEffort = normalizeReasoningEffort(sub.Model.ReasoningEffort)
		if sub.Name == "" {
			sub.Name = sub.ID
		}
		seen[sub.ID] = true
		out = append(out, sub)
	}
	return out
}

func NormalizeSubAgentID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	var b strings.Builder
	lastSeparator := false
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSeparator = false
		case r == '-' || r == '_':
			if !lastSeparator && b.Len() > 0 {
				b.WriteRune(r)
				lastSeparator = true
			}
		default:
			if !lastSeparator && b.Len() > 0 {
				b.WriteByte('-')
				lastSeparator = true
			}
		}
	}
	return strings.Trim(b.String(), "-_")
}

func SubAgentEnabled(sub SubAgentConfig) bool {
	return boolValue(sub.Enabled, true)
}

func SubAgentAllowedForParent(sub SubAgentConfig, parentKind string) bool {
	if !SubAgentEnabled(sub) || !IsDeepAgentParentKind(parentKind) {
		return false
	}
	if len(sub.Parents) == 0 {
		return true
	}
	for _, parent := range sub.Parents {
		if parent == parentKind {
			return true
		}
	}
	return false
}

func ResolveSubAgentModel(cfg *Config, parentKind string, sub SubAgentConfig) ResolvedModelSettings {
	resolved := ResolveAgentModel(cfg, parentKind)
	override := sub.Model
	profileOverride := AgentModelSettings{Default: AgentModelOverride{
		ProfileID:       resolved.ProfileID,
		Temperature:     resolved.Temperature,
		EnableThinking:  resolved.EnableThinking,
		ReasoningEffort: resolved.ReasoningEffort,
	}}
	if cfg != nil {
		tmp := *cfg
		tmp.AgentModels = MergeAgentModelSettings(profileOverride, AgentModelSettings{Default: override})
		resolved = ResolveAgentModel(&tmp, "")
	}
	return resolved
}

func ResolveSubAgentTools(parent ResolvedAgentToolSettings, override AgentToolOverride) ResolvedAgentToolSettings {
	return ResolvedAgentToolSettings{
		FileRead:         parent.FileRead && boolValue(override.FileRead, parent.FileRead),
		FileWrite:        parent.FileWrite && boolValue(override.FileWrite, parent.FileWrite),
		ShellExecute:     parent.ShellExecute && boolValue(override.ShellExecute, parent.ShellExecute),
		Skills:           parent.Skills && boolValue(override.Skills, parent.Skills),
		LoreRead:         parent.LoreRead && boolValue(override.LoreRead, parent.LoreRead),
		LoreWrite:        parent.LoreWrite && boolValue(override.LoreWrite, parent.LoreWrite),
		Todo:             parent.Todo && boolValue(override.Todo, parent.Todo),
		WebSearch:        parent.WebSearch && boolValue(override.WebSearch, parent.WebSearch),
		AgentConfigRead:  parent.AgentConfigRead && boolValue(override.AgentConfigRead, parent.AgentConfigRead),
		AgentConfigWrite: parent.AgentConfigWrite && boolValue(override.AgentConfigWrite, parent.AgentConfigWrite),
	}
}

func mergeSubAgent(parent, child SubAgentConfig) SubAgentConfig {
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
	if child.SystemPrompt != "" {
		out.SystemPrompt = child.SystemPrompt
	}
	if child.Enabled != nil {
		out.Enabled = child.Enabled
	}
	if child.Parents != nil {
		out.Parents = child.Parents
	}
	out.Model = mergeAgentModelOverride(out.Model, child.Model)
	out.Tools = mergeAgentToolOverride(out.Tools, child.Tools)
	return out
}

func sanitizeSubAgentParents(parents []string) []string {
	if len(parents) == 0 {
		return nil
	}
	out := make([]string, 0, len(parents))
	seen := map[string]bool{}
	for _, parent := range parents {
		parent = strings.TrimSpace(parent)
		if !IsDeepAgentParentKind(parent) || seen[parent] {
			continue
		}
		seen[parent] = true
		out = append(out, parent)
	}
	return out
}
