package config

import "strings"

const (
	AgentKindIDE                   = "ide"
	AgentKindInteractiveStory      = "interactive_story"
	AgentKindLoreEditor            = "lore_editor"
	AgentKindTellerEditor          = "teller_editor"
	AgentKindInteractiveState      = "interactive_state"
	AgentKindInteractiveHotChoices = "interactive_hot_choices"
	AgentKindVersionSummary        = "version_summary"
)

type ModelProfileSettings struct {
	ID            string   `toml:"id,omitempty" json:"id,omitempty"`
	Name          string   `toml:"name,omitempty" json:"name,omitempty"`
	OpenAIAPIKey  string   `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	OpenAIBaseURL string   `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	OpenAIModel   string   `toml:"openai_model,omitempty" json:"openai_model,omitempty"`
	Temperature   *float64 `toml:"temperature,omitempty" json:"temperature,omitempty"`
}

type AgentModelSettings struct {
	Default               AgentModelOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentModelOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentModelOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	LoreEditor            AgentModelOverride `toml:"lore_editor,omitempty" json:"lore_editor,omitempty"`
	TellerEditor          AgentModelOverride `toml:"teller_editor,omitempty" json:"teller_editor,omitempty"`
	InteractiveState      AgentModelOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentModelOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentModelOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
}

type AgentModelOverride struct {
	ProfileID       string   `toml:"profile_id,omitempty" json:"profile_id,omitempty"`
	Temperature     *float64 `toml:"temperature,omitempty" json:"temperature,omitempty"`
	EnableThinking  *bool    `toml:"enable_thinking,omitempty" json:"enable_thinking,omitempty"`
	ReasoningEffort string   `toml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
}

type ResolvedModelSettings struct {
	ProfileID       string
	OpenAIAPIKey    string
	OpenAIBaseURL   string
	OpenAIModel     string
	Temperature     *float64
	EnableThinking  *bool
	ReasoningEffort string
}

func MergeAgentModelSettings(parent, child AgentModelSettings) AgentModelSettings {
	return AgentModelSettings{
		Default:               mergeAgentModelOverride(parent.Default, child.Default),
		IDE:                   mergeAgentModelOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentModelOverride(parent.InteractiveStory, child.InteractiveStory),
		LoreEditor:            mergeAgentModelOverride(parent.LoreEditor, child.LoreEditor),
		TellerEditor:          mergeAgentModelOverride(parent.TellerEditor, child.TellerEditor),
		InteractiveState:      mergeAgentModelOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentModelOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentModelOverride(parent.VersionSummary, child.VersionSummary),
	}
}

func ResolveAgentModel(cfg *Config, agentKind string) ResolvedModelSettings {
	if cfg == nil {
		return ResolvedModelSettings{}
	}
	profiles := map[string]ModelProfileSettings{
		"default": legacyModelProfile(cfg),
	}
	for _, profile := range cfg.ModelProfiles {
		id := normalizeModelProfileID(profile.ID)
		if id == "" {
			continue
		}
		base := profiles[id]
		profile.ID = id
		profiles[id] = mergeModelProfile(base, profile)
	}

	defaultOverride := cfg.AgentModels.Default
	agentOverride := mergeAgentModelOverride(defaultOverride, agentModelOverrideFor(cfg.AgentModels, agentKind))
	profileID := normalizeModelProfileID(agentOverride.ProfileID)
	if profileID == "" {
		profileID = "default"
	}
	profile, ok := profiles[profileID]
	if !ok {
		profileID = "default"
		profile = profiles[profileID]
	}
	if profile.OpenAIAPIKey == "" {
		profile.OpenAIAPIKey = cfg.OpenAIAPIKey
	}
	if profile.OpenAIBaseURL == "" {
		profile.OpenAIBaseURL = cfg.OpenAIBaseURL
	}
	if profile.OpenAIModel == "" {
		profile.OpenAIModel = cfg.OpenAIModel
	}
	temperature := profile.Temperature
	if agentOverride.Temperature != nil {
		temperature = agentOverride.Temperature
	}
	return ResolvedModelSettings{
		ProfileID:       profileID,
		OpenAIAPIKey:    profile.OpenAIAPIKey,
		OpenAIBaseURL:   profile.OpenAIBaseURL,
		OpenAIModel:     profile.OpenAIModel,
		Temperature:     temperature,
		EnableThinking:  agentOverride.EnableThinking,
		ReasoningEffort: normalizeReasoningEffort(agentOverride.ReasoningEffort),
	}
}

func mergeModelProfiles(parent, child []ModelProfileSettings) []ModelProfileSettings {
	if len(child) == 0 {
		return parent
	}
	out := make([]ModelProfileSettings, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, profile := range parent {
		id := normalizeModelProfileID(profile.ID)
		if id == "" {
			continue
		}
		profile.ID = id
		index[id] = len(out)
		out = append(out, profile)
	}
	for _, profile := range child {
		id := normalizeModelProfileID(profile.ID)
		if id == "" {
			continue
		}
		profile.ID = id
		if i, ok := index[id]; ok {
			out[i] = mergeModelProfile(out[i], profile)
		} else {
			index[id] = len(out)
			out = append(out, profile)
		}
	}
	return out
}

func mergeModelProfile(parent, child ModelProfileSettings) ModelProfileSettings {
	out := parent
	if child.ID != "" {
		out.ID = normalizeModelProfileID(child.ID)
	}
	if child.Name != "" {
		out.Name = child.Name
	}
	if child.OpenAIAPIKey != "" {
		out.OpenAIAPIKey = child.OpenAIAPIKey
	}
	if child.OpenAIBaseURL != "" {
		out.OpenAIBaseURL = child.OpenAIBaseURL
	}
	if child.OpenAIModel != "" {
		out.OpenAIModel = child.OpenAIModel
	}
	if child.Temperature != nil {
		out.Temperature = child.Temperature
	}
	return out
}

func mergeAgentModelOverride(parent, child AgentModelOverride) AgentModelOverride {
	out := parent
	if child.ProfileID != "" {
		out.ProfileID = normalizeModelProfileID(child.ProfileID)
	}
	if child.Temperature != nil {
		out.Temperature = child.Temperature
	}
	if child.EnableThinking != nil {
		out.EnableThinking = child.EnableThinking
	}
	if child.ReasoningEffort != "" {
		out.ReasoningEffort = normalizeReasoningEffort(child.ReasoningEffort)
	}
	return out
}

func agentModelOverrideFor(settings AgentModelSettings, agentKind string) AgentModelOverride {
	switch agentKind {
	case AgentKindIDE:
		return settings.IDE
	case AgentKindInteractiveStory:
		return settings.InteractiveStory
	case AgentKindLoreEditor:
		return settings.LoreEditor
	case AgentKindTellerEditor:
		return settings.TellerEditor
	case AgentKindInteractiveState:
		return settings.InteractiveState
	case AgentKindInteractiveHotChoices:
		return settings.InteractiveHotChoices
	case AgentKindVersionSummary:
		return settings.VersionSummary
	default:
		return AgentModelOverride{}
	}
}

func legacyModelProfile(cfg *Config) ModelProfileSettings {
	return ModelProfileSettings{
		ID:            "default",
		Name:          "默认模型",
		OpenAIAPIKey:  cfg.OpenAIAPIKey,
		OpenAIBaseURL: cfg.OpenAIBaseURL,
		OpenAIModel:   cfg.OpenAIModel,
	}
}

func normalizeModelProfileID(id string) string {
	return strings.TrimSpace(id)
}

func normalizeReasoningEffort(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
