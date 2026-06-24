package config

import "strings"

const (
	DefaultContextWindowTokens = 400000
	MaxContextWindowTokens     = 2000000
)

type ModelProfileSettings struct {
	ID                  string   `toml:"id,omitempty" json:"id,omitempty"`
	Name                string   `toml:"name,omitempty" json:"name,omitempty"`
	OpenAIAPIKey        string   `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	OpenAIBaseURL       string   `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	OpenAIModel         string   `toml:"openai_model,omitempty" json:"openai_model,omitempty"`
	Temperature         *float64 `toml:"temperature,omitempty" json:"temperature,omitempty"`
	ContextWindowTokens *int     `toml:"context_window_tokens,omitempty" json:"context_window_tokens,omitempty"`
}

type AgentModelSettings struct {
	Default               AgentModelOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentModelOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentModelOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager         AgentModelOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveState      AgentModelOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentModelOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentModelOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentModelOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Automation            AgentModelOverride `toml:"automation,omitempty" json:"automation,omitempty"`
	ContextCompaction     AgentModelOverride `toml:"context_compaction,omitempty" json:"context_compaction,omitempty"`
}

type AgentModelOverride struct {
	ProfileID       string   `toml:"profile_id,omitempty" json:"profile_id,omitempty"`
	Temperature     *float64 `toml:"temperature,omitempty" json:"temperature,omitempty"`
	EnableThinking  *bool    `toml:"enable_thinking,omitempty" json:"enable_thinking,omitempty"`
	ReasoningEffort string   `toml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
}

type ResolvedModelSettings struct {
	ProfileID           string
	OpenAIAPIKey        string
	OpenAIBaseURL       string
	OpenAIModel         string
	Temperature         *float64
	ContextWindowTokens int
	EnableThinking      *bool
	ReasoningEffort     string
}

func MergeAgentModelSettings(parent, child AgentModelSettings) AgentModelSettings {
	return AgentModelSettings{
		Default:               mergeAgentModelOverride(parent.Default, child.Default),
		IDE:                   mergeAgentModelOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentModelOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:         mergeAgentModelOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveState:      mergeAgentModelOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentModelOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentModelOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentModelOverride(parent.ToolAgent, child.ToolAgent),
		Automation:            mergeAgentModelOverride(parent.Automation, child.Automation),
		ContextCompaction:     mergeAgentModelOverride(parent.ContextCompaction, child.ContextCompaction),
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
		id := modelProfileID(profile)
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
	if profile.ContextWindowTokens == nil {
		contextWindowTokens := cfg.OpenAIContextWindowTokens
		if contextWindowTokens <= 0 {
			contextWindowTokens = DefaultContextWindowTokens
		}
		profile.ContextWindowTokens = intPtr(contextWindowTokens)
	}
	temperature := profile.Temperature
	if agentOverride.Temperature != nil {
		temperature = agentOverride.Temperature
	}
	return ResolvedModelSettings{
		ProfileID:           profileID,
		OpenAIAPIKey:        profile.OpenAIAPIKey,
		OpenAIBaseURL:       profile.OpenAIBaseURL,
		OpenAIModel:         profile.OpenAIModel,
		Temperature:         temperature,
		ContextWindowTokens: *profile.ContextWindowTokens,
		EnableThinking:      agentOverride.EnableThinking,
		ReasoningEffort:     normalizeReasoningEffort(agentOverride.ReasoningEffort),
	}
}

func mergeModelProfiles(parent, child []ModelProfileSettings) []ModelProfileSettings {
	if len(child) == 0 {
		return parent
	}
	out := make([]ModelProfileSettings, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, profile := range parent {
		id := modelProfileID(profile)
		if id == "" {
			continue
		}
		profile.ID = id
		index[id] = len(out)
		out = append(out, profile)
	}
	for _, profile := range child {
		id := modelProfileID(profile)
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

func sanitizeModelProfiles(profiles []ModelProfileSettings) []ModelProfileSettings {
	if len(profiles) == 0 {
		return profiles
	}
	out := make([]ModelProfileSettings, 0, len(profiles))
	for _, profile := range profiles {
		profile.OpenAIModel = strings.TrimSpace(profile.OpenAIModel)
		profile.ID = modelProfileID(profile)
		if profile.ID == "" {
			continue
		}
		if profile.OpenAIModel == "" {
			profile.OpenAIModel = profile.ID
		}
		profile.Name = strings.TrimSpace(profile.Name)
		if profile.ContextWindowTokens != nil {
			if *profile.ContextWindowTokens <= 0 {
				profile.ContextWindowTokens = nil
			} else if *profile.ContextWindowTokens > MaxContextWindowTokens {
				*profile.ContextWindowTokens = MaxContextWindowTokens
			}
		}
		out = append(out, profile)
	}
	return out
}

func mergeModelProfile(parent, child ModelProfileSettings) ModelProfileSettings {
	out := parent
	if id := modelProfileID(child); id != "" {
		out.ID = id
	}
	if child.Name != "" {
		out.Name = strings.TrimSpace(child.Name)
	}
	if child.OpenAIAPIKey != "" {
		out.OpenAIAPIKey = child.OpenAIAPIKey
	}
	if child.OpenAIBaseURL != "" {
		out.OpenAIBaseURL = child.OpenAIBaseURL
	}
	if child.OpenAIModel != "" {
		out.OpenAIModel = strings.TrimSpace(child.OpenAIModel)
	}
	if child.Temperature != nil {
		out.Temperature = child.Temperature
	}
	if child.ContextWindowTokens != nil {
		out.ContextWindowTokens = child.ContextWindowTokens
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
	if definition, ok := LookupAgentKind(agentKind); ok && definition.ModelOverride != nil {
		return definition.ModelOverride(settings)
	}
	return AgentModelOverride{}
}

func legacyModelProfile(cfg *Config) ModelProfileSettings {
	contextWindowTokens := cfg.OpenAIContextWindowTokens
	if contextWindowTokens <= 0 {
		contextWindowTokens = DefaultContextWindowTokens
	}
	return ModelProfileSettings{
		ID:                  "default",
		Name:                "默认模型",
		OpenAIAPIKey:        cfg.OpenAIAPIKey,
		OpenAIBaseURL:       cfg.OpenAIBaseURL,
		OpenAIModel:         cfg.OpenAIModel,
		ContextWindowTokens: intPtr(contextWindowTokens),
	}
}

func normalizeModelProfileID(id string) string {
	return strings.TrimSpace(id)
}

func modelProfileID(profile ModelProfileSettings) string {
	if id := normalizeModelProfileID(profile.ID); id != "" {
		return id
	}
	return strings.TrimSpace(profile.OpenAIModel)
}

func normalizeReasoningEffort(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
