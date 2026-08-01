package config

import (
	"fmt"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
)

const (
	DefaultContextWindowTokens = 400000
	MaxContextWindowTokens     = 2000000
)

type ModelProfileSettings struct {
	ID                  string   `toml:"id,omitempty" json:"id,omitempty"`
	Name                string   `toml:"name,omitempty" json:"name,omitempty"`
	Provider            string   `toml:"provider,omitempty" json:"provider,omitempty"`
	Protocol            string   `toml:"protocol,omitempty" json:"protocol,omitempty"`
	OpenAIAPIKey        string   `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	OpenAIBaseURL       string   `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	OpenAIModel         string   `toml:"openai_model,omitempty" json:"openai_model,omitempty"`
	Temperature         *float64 `toml:"temperature,omitempty" json:"temperature,omitempty"`
	ContextWindowTokens *int     `toml:"context_window_tokens,omitempty" json:"context_window_tokens,omitempty"`
}

type AgentModelSettings struct {
	Default             AgentModelOverride `toml:"default,omitempty" json:"default,omitempty"`
	General             AgentModelOverride `toml:"general,omitempty" json:"general,omitempty"`
	IDE                 AgentModelOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory    AgentModelOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager       AgentModelOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveDirector AgentModelOverride `toml:"interactive_director,omitempty" json:"interactive_director,omitempty"`
	VersionSummary      AgentModelOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent           AgentModelOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Image               AgentModelOverride `toml:"image,omitempty" json:"image,omitempty"`
	Automation          AgentModelOverride `toml:"automation,omitempty" json:"automation,omitempty"`
	ContextCompaction   AgentModelOverride `toml:"context_compaction,omitempty" json:"context_compaction,omitempty"`
}

type AgentModelOverride struct {
	ProfileID     string   `toml:"profile_id,omitempty" json:"profile_id,omitempty"`
	Temperature   *float64 `toml:"temperature,omitempty" json:"temperature,omitempty"`
	ThinkingLevel string   `toml:"thinking_level,omitempty" json:"thinking_level,omitempty"`
}

type ResolvedModelSettings struct {
	ProfileID           string
	Provider            string
	Protocol            string
	OpenAIAPIKey        string
	OpenAIBaseURL       string
	OpenAIModel         string
	Temperature         *float64
	ContextWindowTokens int
	ThinkingLevel       string
}

func MergeAgentModelSettings(parent, child AgentModelSettings) AgentModelSettings {
	return AgentModelSettings{
		Default:             mergeAgentModelOverride(parent.Default, child.Default),
		General:             mergeAgentModelOverride(parent.General, child.General),
		IDE:                 mergeAgentModelOverride(parent.IDE, child.IDE),
		InteractiveStory:    mergeAgentModelOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:       mergeAgentModelOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveDirector: mergeAgentModelOverride(parent.InteractiveDirector, child.InteractiveDirector),
		VersionSummary:      mergeAgentModelOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:           mergeAgentModelOverride(parent.ToolAgent, child.ToolAgent),
		Image:               mergeAgentModelOverride(parent.Image, child.Image),
		Automation:          mergeAgentModelOverride(parent.Automation, child.Automation),
		ContextCompaction:   mergeAgentModelOverride(parent.ContextCompaction, child.ContextCompaction),
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
		profile = normalizeModelProfileRouting(profile)
		profile.ID = id
		profiles[id] = mergeModelProfile(base, profile)
	}
	defaultProfile := profiles["default"]
	if defaultProfile.OpenAIAPIKey == "" {
		defaultProfile.OpenAIAPIKey = cfg.OpenAIAPIKey
	}
	if defaultProfile.OpenAIBaseURL == "" {
		defaultProfile.OpenAIBaseURL = defaultModelProviderBaseURL(defaultProfile.Provider)
		if defaultProfile.OpenAIBaseURL == "" && defaultProfile.Provider == "" {
			defaultProfile.OpenAIBaseURL = cfg.OpenAIBaseURL
		}
	}
	if defaultProfile.OpenAIModel == "" {
		defaultProfile.OpenAIModel = cfg.OpenAIModel
	}
	if defaultProfile.ContextWindowTokens == nil {
		contextWindowTokens := cfg.OpenAIContextWindowTokens
		if contextWindowTokens <= 0 {
			contextWindowTokens = DefaultContextWindowTokens
		}
		defaultProfile.ContextWindowTokens = intPtr(contextWindowTokens)
	}
	profiles["default"] = defaultProfile

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
	if profile.Provider == "" {
		profile.Provider = defaultProfile.Provider
	}
	if profile.Protocol == "" {
		profile.Protocol = defaultProfile.Protocol
	}
	if profile.OpenAIAPIKey == "" {
		profile.OpenAIAPIKey = defaultProfile.OpenAIAPIKey
	}
	if profile.OpenAIBaseURL == "" {
		if profile.Provider != "" && profile.Provider != defaultProfile.Provider {
			profile.OpenAIBaseURL = defaultModelProviderBaseURL(profile.Provider)
		} else {
			profile.OpenAIBaseURL = defaultProfile.OpenAIBaseURL
		}
	}
	if profile.OpenAIModel == "" {
		profile.OpenAIModel = defaultProfile.OpenAIModel
	}
	if profile.ContextWindowTokens == nil {
		profile.ContextWindowTokens = defaultProfile.ContextWindowTokens
	}
	temperature := profile.Temperature
	if agentOverride.Temperature != nil {
		temperature = agentOverride.Temperature
	}
	return ResolvedModelSettings{
		ProfileID:           profileID,
		Provider:            profile.Provider,
		Protocol:            profile.Protocol,
		OpenAIAPIKey:        profile.OpenAIAPIKey,
		OpenAIBaseURL:       profile.OpenAIBaseURL,
		OpenAIModel:         profile.OpenAIModel,
		Temperature:         temperature,
		ContextWindowTokens: *profile.ContextWindowTokens,
		ThinkingLevel:       resolvedThinkingLevel(agentOverride.ThinkingLevel),
	}
}

// ApplyAgentModelSelection overrides only the model identity and reasoning
// effort for one Agent kind. Other per-Agent policy, such as temperature,
// remains owned by Settings.
func ApplyAgentModelSelection(cfg *Config, agentKind, profileID, thinkingLevel string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	definition, ok := LookupAgentKind(strings.TrimSpace(agentKind))
	if !ok || definition.ModelOverride == nil || definition.SetModelOverride == nil {
		return fmt.Errorf("unsupported Agent kind %q", agentKind)
	}
	profileID = normalizeModelProfileID(profileID)
	if !ModelProfileExists(cfg, profileID) {
		return fmt.Errorf("model profile %q does not exist", profileID)
	}
	level, err := providers.ParseThinkingLevel(thinkingLevel)
	if err != nil {
		return err
	}
	override := definition.ModelOverride(cfg.AgentModels)
	override.ProfileID = profileID
	override.ThinkingLevel = string(level)
	definition.SetModelOverride(&cfg.AgentModels, override)
	return nil
}

// ModelProfileExists reports whether a stable selectable profile ID exists.
// The legacy/default profile is always present even when no explicit profiles
// have been configured.
func ModelProfileExists(cfg *Config, profileID string) bool {
	profileID = normalizeModelProfileID(profileID)
	if profileID == "default" {
		return true
	}
	if cfg == nil || profileID == "" {
		return false
	}
	for _, profile := range cfg.ModelProfiles {
		if modelProfileID(profile) == profileID {
			return true
		}
	}
	return false
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
		profile = normalizeModelProfileRouting(profile)
		profile.OpenAIModel = strings.TrimSpace(profile.OpenAIModel)
		profile.ID = modelProfileID(profile)
		if profile.ID == "" {
			// Settings autosave persists a newly added profile before the user has
			// supplied its model identifier. Keep any meaningful partial draft so a
			// successful save cannot make that active form disappear. Resolution and
			// inheritance still ignore id-less profiles, so drafts cannot be selected
			// by an Agent until the user fills in a model name or explicit ID.
			if !hasModelProfileDraftFields(profile) {
				continue
			}
			profile.Name = strings.TrimSpace(profile.Name)
			profile.OpenAIBaseURL = strings.TrimSpace(profile.OpenAIBaseURL)
			profile.ContextWindowTokens = normalizeModelProfileContextWindow(profile.ContextWindowTokens)
			out = append(out, profile)
			continue
		}
		if profile.OpenAIModel == "" && profile.ID != "default" {
			profile.OpenAIModel = profile.ID
		}
		profile.Name = strings.TrimSpace(profile.Name)
		profile.ContextWindowTokens = normalizeModelProfileContextWindow(profile.ContextWindowTokens)
		out = append(out, profile)
	}
	return out
}

func hasModelProfileDraftFields(profile ModelProfileSettings) bool {
	return strings.TrimSpace(profile.Name) != "" ||
		strings.TrimSpace(profile.Provider) != "" ||
		strings.TrimSpace(profile.Protocol) != "" ||
		profile.OpenAIAPIKey != "" ||
		strings.TrimSpace(profile.OpenAIBaseURL) != "" ||
		profile.Temperature != nil ||
		profile.ContextWindowTokens != nil
}

func normalizeModelProfileContextWindow(tokens *int) *int {
	if tokens == nil {
		return nil
	}
	if *tokens <= 0 {
		return nil
	}
	if *tokens > MaxContextWindowTokens {
		*tokens = MaxContextWindowTokens
	}
	return tokens
}

func defaultModelProfile(profiles []ModelProfileSettings) (ModelProfileSettings, bool) {
	for _, profile := range profiles {
		if modelProfileID(profile) == "default" {
			return profile, true
		}
	}
	return ModelProfileSettings{}, false
}

func mergeModelProfile(parent, child ModelProfileSettings) ModelProfileSettings {
	out := parent
	if id := modelProfileID(child); id != "" {
		out.ID = id
	}
	out.Name = strings.TrimSpace(child.Name)
	if child.Provider != "" {
		inheritedProvider := strings.TrimSpace(out.Provider)
		if inheritedProvider == "" && strings.TrimSpace(out.OpenAIBaseURL) != "" {
			inheritedProvider = inferModelProvider(out.OpenAIBaseURL)
		}
		providerChanged := inheritedProvider != "" && strings.TrimSpace(child.Provider) != inheritedProvider
		if providerChanged && child.OpenAIBaseURL == "" {
			out.OpenAIBaseURL = ""
		}
		if providerChanged && child.Protocol == "" {
			out.Protocol = ""
		}
		out.Provider = strings.TrimSpace(child.Provider)
	}
	if child.Protocol != "" {
		out.Protocol = strings.TrimSpace(child.Protocol)
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
	if child.ThinkingLevel != "" {
		out.ThinkingLevel = normalizeThinkingLevel(child.ThinkingLevel)
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
	return normalizeModelProfileRouting(ModelProfileSettings{
		ID:                  "default",
		Name:                "默认模型",
		OpenAIAPIKey:        cfg.OpenAIAPIKey,
		OpenAIBaseURL:       cfg.OpenAIBaseURL,
		OpenAIModel:         cfg.OpenAIModel,
		ContextWindowTokens: intPtr(contextWindowTokens),
	})
}

func normalizeModelProfileRouting(profile ModelProfileSettings) ModelProfileSettings {
	profile.Provider = strings.TrimSpace(profile.Provider)
	profile.Protocol = strings.TrimSpace(profile.Protocol)
	explicitProvider := profile.Provider != ""
	if profile.Provider == "" && strings.TrimSpace(profile.OpenAIBaseURL) != "" {
		profile.Provider = inferModelProvider(profile.OpenAIBaseURL)
	}
	if profile.Protocol == "" && profile.Provider != "" {
		// Profiles written before provider/protocol existed always used Chat
		// Completions. Preserve that behavior when provider was inferred. A new,
		// explicit OpenAI provider adopts OpenAI's Responses default.
		if explicitProvider && profile.Provider == string(providers.ProviderOpenAI) {
			profile.Protocol = string(providers.ProtocolOpenAIResponses)
		} else {
			profile.Protocol = string(providers.ProtocolOpenAIChatCompletions)
		}
	}
	return profile
}

func inferModelProvider(baseURL string) string {
	normalized := strings.ToLower(strings.TrimSpace(baseURL))
	switch {
	case strings.Contains(normalized, "api.deepseek.com"):
		return string(providers.ProviderDeepSeek)
	case strings.Contains(normalized, "api.openai.com"):
		return string(providers.ProviderOpenAI)
	default:
		return string(providers.ProviderOpenAICompatible)
	}
}

func defaultModelProviderBaseURL(provider string) string {
	switch strings.TrimSpace(provider) {
	case string(providers.ProviderOpenAI):
		return "https://api.openai.com/v1"
	case string(providers.ProviderDeepSeek):
		return "https://api.deepseek.com"
	default:
		return ""
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

func normalizeThinkingLevel(value string) string {
	return string(providers.NormalizeThinkingLevel(value))
}

func resolvedThinkingLevel(value string) string {
	if strings.TrimSpace(value) == "" {
		return string(providers.ThinkingLevelDefault)
	}
	return normalizeThinkingLevel(value)
}
