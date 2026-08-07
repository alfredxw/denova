package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
)

const (
	DefaultContextWindowTokens = 400000
	MaxContextWindowTokens     = 2000000
)

type ModelProfileSettings struct {
	ID       string `toml:"id,omitempty" json:"id,omitempty"`
	Name     string `toml:"name,omitempty" json:"name,omitempty"`
	Provider string `toml:"provider,omitempty" json:"provider,omitempty"`
	Protocol string `toml:"protocol,omitempty" json:"protocol,omitempty"`
	APIKey   string `toml:"api_key,omitempty" json:"api_key,omitempty"`
	BaseURL  string `toml:"base_url,omitempty" json:"base_url,omitempty"`
	Model    string `toml:"model,omitempty" json:"model,omitempty"`
	// LegacyOpenAI* are decode-only aliases for profiles persisted before the
	// provider/protocol split. Sanitization moves them into the generic fields
	// and clears them, so every subsequent write uses the canonical schema.
	LegacyOpenAIAPIKey  string                       `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	LegacyOpenAIBaseURL string                       `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	LegacyOpenAIModel   string                       `toml:"openai_model,omitempty" json:"openai_model,omitempty"`
	Headers             map[string]string            `toml:"headers,omitempty" json:"headers,omitempty"`
	ProtocolOptions     map[string]any               `toml:"protocol_options,omitempty" json:"protocol_options,omitempty"`
	SessionKeyMapping   *providers.SessionKeyMapping `toml:"session_key_mapping,omitempty" json:"session_key_mapping,omitempty"`
	Temperature         *float64                     `toml:"temperature,omitempty" json:"temperature,omitempty"`
	ContextWindowTokens *int                         `toml:"context_window_tokens,omitempty" json:"context_window_tokens,omitempty"`
	MaxOutputTokens     *int                         `toml:"max_output_tokens,omitempty" json:"max_output_tokens,omitempty"`
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
	APIKey              string
	BaseURL             string
	Model               string
	Headers             map[string]string
	ProtocolOptions     map[string]any
	SessionKeyMapping   *providers.SessionKeyMapping
	Temperature         *float64
	ContextWindowTokens int
	MaxOutputTokens     *int
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
	legacyProfile := legacyModelProfile(cfg)
	profiles := map[string]ModelProfileSettings{
		"default": legacyProfile,
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
	if defaultProfile.BaseURL == "" {
		if defaultProfile.Provider == "" {
			defaultProfile.BaseURL = cfg.OpenAIBaseURL
		}
	}
	if defaultProfile.APIKey == "" && sameModelCredentialScope(defaultProfile, legacyProfile) {
		defaultProfile.APIKey = legacyProfile.APIKey
	}
	if defaultProfile.Model == "" {
		defaultProfile.Model = cfg.OpenAIModel
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
		if profile.Provider == "" || profile.Provider == defaultProfile.Provider {
			profile.Protocol = defaultProfile.Protocol
		}
	}
	if profile.BaseURL == "" {
		if profile.Provider == "" || profile.Provider == defaultProfile.Provider {
			profile.BaseURL = defaultProfile.BaseURL
		}
	}
	if profile.APIKey == "" && sameModelCredentialScope(profile, defaultProfile) {
		profile.APIKey = defaultProfile.APIKey
	}
	if profile.Model == "" {
		profile.Model = defaultProfile.Model
	}
	if profile.ContextWindowTokens == nil {
		profile.ContextWindowTokens = defaultProfile.ContextWindowTokens
	}
	if profile.MaxOutputTokens == nil {
		profile.MaxOutputTokens = defaultProfile.MaxOutputTokens
	}
	if profile.MaxOutputTokens == nil {
		if limits, ok := providers.LookupModelLimits(providers.ProviderID(profile.Provider), profile.Model); ok && limits.MaxOutputTokens > 0 {
			profile.MaxOutputTokens = intPtr(limits.MaxOutputTokens)
		}
	}
	temperature := profile.Temperature
	if agentOverride.Temperature != nil {
		temperature = agentOverride.Temperature
	}
	return ResolvedModelSettings{
		ProfileID:           profileID,
		Provider:            profile.Provider,
		Protocol:            profile.Protocol,
		APIKey:              profile.APIKey,
		BaseURL:             profile.BaseURL,
		Model:               profile.Model,
		Headers:             cloneModelProfileHeaders(profile.Headers),
		ProtocolOptions:     cloneModelProfileOptions(profile.ProtocolOptions),
		SessionKeyMapping:   cloneModelProfileSessionKeyMapping(profile.SessionKeyMapping),
		Temperature:         temperature,
		ContextWindowTokens: *profile.ContextWindowTokens,
		MaxOutputTokens:     cloneIntPointer(profile.MaxOutputTokens),
		ThinkingLevel:       resolvedThinkingLevel(agentOverride.ThinkingLevel),
	}
}

// ResolveModelProfile resolves an inline profile using the same inheritance as
// an Agent selection. It is intended for validating unsaved settings drafts:
// omitted secrets inherit from the stored profile/default, while routing and
// model fields in the draft take precedence.
func ResolveModelProfile(cfg *Config, draft ModelProfileSettings) (ResolvedModelSettings, error) {
	if cfg == nil {
		return ResolvedModelSettings{}, fmt.Errorf("config is nil")
	}
	originalID := modelProfileID(draft)
	if originalID == "" {
		return ResolvedModelSettings{}, fmt.Errorf("model profile requires an id or model")
	}
	base := ModelProfileSettings{}
	for _, profile := range cfg.ModelProfiles {
		if modelProfileID(profile) == originalID {
			base = profile
			break
		}
	}
	draft = mergeModelProfile(base, draft)
	const validationProfileID = "__model_validation__"
	draft.ID = validationProfileID
	temporary := *cfg
	temporary.ModelProfiles = append(append([]ModelProfileSettings(nil), cfg.ModelProfiles...), draft)
	temporary.AgentModels = AgentModelSettings{
		Default: AgentModelOverride{ProfileID: validationProfileID},
	}
	resolved := ResolveAgentModel(&temporary, "")
	resolved.ProfileID = originalID
	return resolved, nil
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
		profile = migrateLegacyModelProfile(profile)
		profile = normalizeModelProfileRouting(profile)
		profile.Model = strings.TrimSpace(profile.Model)
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
			profile.BaseURL = strings.TrimSpace(profile.BaseURL)
			profile.ContextWindowTokens = normalizeModelProfileContextWindow(profile.ContextWindowTokens)
			profile.MaxOutputTokens = normalizeModelProfileMaxOutput(profile.MaxOutputTokens)
			out = append(out, profile)
			continue
		}
		if profile.Model == "" && profile.ID != "default" {
			profile.Model = profile.ID
		}
		profile.Name = strings.TrimSpace(profile.Name)
		profile.ContextWindowTokens = normalizeModelProfileContextWindow(profile.ContextWindowTokens)
		profile.MaxOutputTokens = normalizeModelProfileMaxOutput(profile.MaxOutputTokens)
		out = append(out, profile)
	}
	return out
}

// migrateLegacyModelProfile accepts the former OpenAI-prefixed profile fields
// without letting them override an explicitly configured canonical value.
func migrateLegacyModelProfile(profile ModelProfileSettings) ModelProfileSettings {
	if profile.APIKey == "" {
		profile.APIKey = profile.LegacyOpenAIAPIKey
	}
	if strings.TrimSpace(profile.BaseURL) == "" {
		profile.BaseURL = profile.LegacyOpenAIBaseURL
	}
	if strings.TrimSpace(profile.Model) == "" {
		profile.Model = profile.LegacyOpenAIModel
	}
	profile.LegacyOpenAIAPIKey = ""
	profile.LegacyOpenAIBaseURL = ""
	profile.LegacyOpenAIModel = ""
	return profile
}

func hasModelProfileDraftFields(profile ModelProfileSettings) bool {
	return strings.TrimSpace(profile.Name) != "" ||
		strings.TrimSpace(profile.Provider) != "" ||
		strings.TrimSpace(profile.Protocol) != "" ||
		profile.APIKey != "" ||
		strings.TrimSpace(profile.BaseURL) != "" ||
		len(profile.Headers) != 0 ||
		len(profile.ProtocolOptions) != 0 ||
		profile.SessionKeyMapping != nil ||
		profile.Temperature != nil ||
		profile.ContextWindowTokens != nil ||
		profile.MaxOutputTokens != nil
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

func normalizeModelProfileMaxOutput(tokens *int) *int {
	if tokens == nil || *tokens <= 0 {
		return nil
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
	previousProvider := strings.TrimSpace(out.Provider)
	previousProtocol := strings.TrimSpace(out.Protocol)
	previousScope := modelCredentialScope(out)
	if id := modelProfileID(child); id != "" {
		out.ID = id
	}
	out.Name = strings.TrimSpace(child.Name)
	if child.Provider != "" {
		inheritedProvider := strings.TrimSpace(out.Provider)
		if inheritedProvider == "" && strings.TrimSpace(out.BaseURL) != "" {
			inheritedProvider = inferModelProvider(out.BaseURL)
		}
		providerChanged := inheritedProvider != "" && strings.TrimSpace(child.Provider) != inheritedProvider
		if providerChanged && child.BaseURL == "" {
			out.BaseURL = ""
		}
		if providerChanged && child.Protocol == "" {
			out.Protocol = ""
		}
		out.Provider = strings.TrimSpace(child.Provider)
	}
	if child.Protocol != "" {
		out.Protocol = strings.TrimSpace(child.Protocol)
	}
	if child.BaseURL != "" {
		out.BaseURL = child.BaseURL
	}
	// Credentials and compatibility settings may be inherited across settings
	// layers, but never silently cross an endpoint origin. Protocol options are
	// additionally scoped to their wire protocol. Explicit child values below
	// can establish the new route after inherited values have been discarded.
	if previousScope != modelCredentialScope(out) {
		out.APIKey = ""
		out.Headers = nil
		out.ProtocolOptions = nil
		out.SessionKeyMapping = nil
	} else if previousProtocol != strings.TrimSpace(out.Protocol) {
		out.ProtocolOptions = nil
		out.SessionKeyMapping = nil
	}
	if previousProvider != strings.TrimSpace(out.Provider) {
		out.SessionKeyMapping = nil
	}
	if child.APIKey != "" {
		out.APIKey = child.APIKey
	}
	if child.Model != "" {
		out.Model = strings.TrimSpace(child.Model)
	}
	if child.Headers != nil {
		out.Headers = mergeModelProfileHeaders(out.Headers, child.Headers)
	}
	if child.ProtocolOptions != nil {
		out.ProtocolOptions = mergeModelProfileOptions(out.ProtocolOptions, child.ProtocolOptions)
	}
	if child.SessionKeyMapping != nil {
		out.SessionKeyMapping = cloneModelProfileSessionKeyMapping(child.SessionKeyMapping)
	}
	if child.Temperature != nil {
		out.Temperature = child.Temperature
	}
	if child.ContextWindowTokens != nil {
		out.ContextWindowTokens = child.ContextWindowTokens
	}
	if child.MaxOutputTokens != nil {
		out.MaxOutputTokens = child.MaxOutputTokens
	}
	return out
}

func sameModelCredentialScope(left, right ModelProfileSettings) bool {
	return modelCredentialScope(left) == modelCredentialScope(right)
}

func modelCredentialScope(profile ModelProfileSettings) string {
	baseURL := strings.TrimSpace(profile.BaseURL)
	if baseURL != "" {
		if parsed, err := url.Parse(baseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			// API credentials are generally valid across protocol paths on one
			// origin (for example DeepSeek's root and /anthropic endpoints), but
			// must not follow a draft to another network destination.
			return "origin:" + strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
		}
		return "endpoint:" + strings.ToLower(strings.TrimRight(baseURL, "/"))
	}
	return "provider:" + strings.ToLower(strings.TrimSpace(profile.Provider))
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
		APIKey:              cfg.OpenAIAPIKey,
		BaseURL:             cfg.OpenAIBaseURL,
		Model:               cfg.OpenAIModel,
		ContextWindowTokens: intPtr(contextWindowTokens),
	})
}

func normalizeModelProfileRouting(profile ModelProfileSettings) ModelProfileSettings {
	profile = migrateLegacyModelProfile(profile)
	profile.Provider = strings.TrimSpace(profile.Provider)
	profile.Protocol = strings.TrimSpace(profile.Protocol)
	if profile.SessionKeyMapping != nil {
		profile.SessionKeyMapping = cloneModelProfileSessionKeyMapping(profile.SessionKeyMapping)
		profile.SessionKeyMapping.Location = providers.SessionKeyLocation(strings.ToLower(strings.TrimSpace(string(profile.SessionKeyMapping.Location))))
		profile.SessionKeyMapping.Name = strings.TrimSpace(profile.SessionKeyMapping.Name)
	}
	explicitProvider := profile.Provider != ""
	if profile.Provider == "" && strings.TrimSpace(profile.BaseURL) != "" {
		profile.Provider = inferModelProvider(profile.BaseURL)
	}
	if profile.Protocol == "" && profile.Provider != "" && !explicitProvider {
		// Profiles written before provider/protocol existed always used Chat
		// Completions. Preserve that behavior only when provider was inferred;
		// explicit providers let the central registry select their preset default.
		profile.Protocol = string(providers.ProtocolOpenAIChatCompletions)
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
	case strings.Contains(normalized, "ark.cn-beijing.volces.com"):
		return string(providers.ProviderVolcengine)
	case strings.Contains(normalized, "generativelanguage.googleapis.com"):
		return string(providers.ProviderGoogle)
	default:
		return string(providers.ProviderOpenAICompatible)
	}
}

func normalizeModelProfileID(id string) string {
	return strings.TrimSpace(id)
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneModelProfileHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	clone := make(map[string]string, len(headers))
	for key, value := range headers {
		clone[key] = value
	}
	return clone
}

func cloneModelProfileSessionKeyMapping(mapping *providers.SessionKeyMapping) *providers.SessionKeyMapping {
	if mapping == nil {
		return nil
	}
	clone := *mapping
	return &clone
}

func mergeModelProfileHeaders(parent, child map[string]string) map[string]string {
	result := cloneModelProfileHeaders(parent)
	if result == nil {
		result = make(map[string]string, len(child))
	}
	for key, value := range child {
		result[key] = value
	}
	return result
}

func cloneModelProfileOptions(options map[string]any) map[string]any {
	if options == nil {
		return nil
	}
	return mergeModelProfileOptions(nil, options)
}

func mergeModelProfileOptions(parent, child map[string]any) map[string]any {
	result := make(map[string]any, len(parent)+len(child))
	for key, value := range parent {
		if nested, ok := value.(map[string]any); ok {
			result[key] = mergeModelProfileOptions(nil, nested)
		} else {
			result[key] = value
		}
	}
	for key, value := range child {
		if nested, ok := value.(map[string]any); ok {
			if base, ok := result[key].(map[string]any); ok {
				result[key] = mergeModelProfileOptions(base, nested)
			} else {
				result[key] = mergeModelProfileOptions(nil, nested)
			}
		} else {
			result[key] = value
		}
	}
	return result
}

func modelProfileID(profile ModelProfileSettings) string {
	if id := normalizeModelProfileID(profile.ID); id != "" {
		return id
	}
	return strings.TrimSpace(profile.Model)
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
