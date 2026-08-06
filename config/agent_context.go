package config

const (
	// Context maintenance policy is intentionally internal to the backend. The
	// user controls when compaction starts and whether recoverable tool results
	// may leave the rich context; the runtime owns the coordinated watermarks.
	DefaultContextCompactionRetainedTurns          = 1
	MaxContextCompactionRetainedTurns              = 30
	DefaultContextCompactionTargetMinRatio         = 0.05
	DefaultContextCompactionTargetMaxRatio         = 0.20
	AgentContextCompactionStrategySummaryAgent     = "summary_agent"
	DefaultToolResultCleanupThreshold              = 0.70
	DefaultToolResultCleanupTarget                 = 0.60
	DefaultToolResultCleanupMinTokens              = 20_000
	DefaultToolResultKeepRecent                    = 3
	DefaultToolResultKeepRecentTokens              = 16_000
	DefaultToolResultWarmSuffixTokens              = 8_000
	DefaultToolResultEagerMinTokens                = 32_000
	DefaultContextCompactionThreshold              = 0.85
	DefaultContextCompactionRecoveryBand           = 0.80
	DefaultContextCompactionMaxConsecutiveFailures = 3

	// Context assembly defaults are deliberately generous for creative work.
	// They are injection limits, not transcript limits: persisted conversation
	// history is governed by compaction and is never silently rewritten here.
	DefaultAgentContextMaxFragmentBytes      = 256 * 1024
	DefaultAgentContextMaxTotalInjectedBytes = 4 * 1024 * 1024
	DefaultAgentContextMaxFragments          = 256
	DefaultAgentContextMaxMetadataFieldBytes = 4 * 1024
	// Provider input is a non-disableable safety boundary over the complete
	// serialized prompt (history, tools, and injected context), not a semantic
	// compaction preference.
	DefaultAgentContextMaxProviderInputBytes = 4 * 1024 * 1024

	MaxAgentContextFragmentBytes      = 16 * 1024 * 1024
	MaxAgentContextTotalInjectedBytes = 64 * 1024 * 1024
	MaxAgentContextFragments          = 4096
	MaxAgentContextMetadataFieldBytes = 64 * 1024
	MaxAgentContextProviderInputBytes = 64 * 1024 * 1024
)

// AgentContextSettings stores per-agent context maintenance intent. Detailed
// cleanup and compaction mechanics are derived by the backend so every client
// and Agent kind observes one coherent policy.
type AgentContextSettings struct {
	Default             AgentContextOverride `toml:"default,omitempty" json:"default,omitempty"`
	General             AgentContextOverride `toml:"general,omitempty" json:"general,omitempty"`
	IDE                 AgentContextOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory    AgentContextOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager       AgentContextOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveDirector AgentContextOverride `toml:"interactive_director,omitempty" json:"interactive_director,omitempty"`
	VersionSummary      AgentContextOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent           AgentContextOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Image               AgentContextOverride `toml:"image,omitempty" json:"image,omitempty"`
	Automation          AgentContextOverride `toml:"automation,omitempty" json:"automation,omitempty"`
	ContextCompaction   AgentContextOverride `toml:"context_compaction,omitempty" json:"context_compaction,omitempty"`
}

type AgentContextOverride struct {
	CompactionEnabled        *bool    `toml:"compaction_enabled,omitempty" json:"compaction_enabled,omitempty"`
	CompactionThreshold      *float64 `toml:"compaction_threshold,omitempty" json:"compaction_threshold,omitempty"`
	ToolResultContextEnabled *bool    `toml:"tool_result_context_enabled,omitempty" json:"tool_result_context_enabled,omitempty"`
	MaxFragmentBytes         *int     `toml:"max_fragment_bytes,omitempty" json:"max_fragment_bytes,omitempty"`
	MaxTotalInjectedBytes    *int     `toml:"max_total_injected_bytes,omitempty" json:"max_total_injected_bytes,omitempty"`
	MaxFragments             *int     `toml:"max_fragments,omitempty" json:"max_fragments,omitempty"`
	MaxMetadataFieldBytes    *int     `toml:"max_metadata_field_bytes,omitempty" json:"max_metadata_field_bytes,omitempty"`
	MaxProviderInputBytes    *int     `toml:"max_provider_input_bytes,omitempty" json:"max_provider_input_bytes,omitempty"`
}

// ResolvedAgentContextSettings is the canonical backend-normalized policy
// returned to clients and consumed by the runtime.
type ResolvedAgentContextSettings struct {
	CompactionEnabled        bool    `json:"compaction_enabled"`
	CompactionThreshold      float64 `json:"compaction_threshold"`
	ToolResultContextEnabled bool    `json:"tool_result_context_enabled"`
	MaxFragmentBytes         int     `json:"max_fragment_bytes"`
	MaxTotalInjectedBytes    int     `json:"max_total_injected_bytes"`
	MaxFragments             int     `json:"max_fragments"`
	MaxMetadataFieldBytes    int     `json:"max_metadata_field_bytes"`
	MaxProviderInputBytes    int     `json:"max_provider_input_bytes"`
}

func DefaultAgentContextSettings() AgentContextSettings {
	return AgentContextSettings{
		Default: AgentContextOverride{
			CompactionEnabled:     boolPtr(true),
			CompactionThreshold:   floatPtr(DefaultContextCompactionThreshold),
			MaxFragmentBytes:      intPtr(DefaultAgentContextMaxFragmentBytes),
			MaxTotalInjectedBytes: intPtr(DefaultAgentContextMaxTotalInjectedBytes),
			MaxFragments:          intPtr(DefaultAgentContextMaxFragments),
			MaxMetadataFieldBytes: intPtr(DefaultAgentContextMaxMetadataFieldBytes),
			MaxProviderInputBytes: intPtr(DefaultAgentContextMaxProviderInputBytes),
		},
	}
}

func MergeAgentContextSettings(parent, child AgentContextSettings) AgentContextSettings {
	return AgentContextSettings{
		Default:             mergeAgentContextOverride(parent.Default, child.Default),
		General:             mergeAgentContextOverride(parent.General, child.General),
		IDE:                 mergeAgentContextOverride(parent.IDE, child.IDE),
		InteractiveStory:    mergeAgentContextOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:       mergeAgentContextOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveDirector: mergeAgentContextOverride(parent.InteractiveDirector, child.InteractiveDirector),
		VersionSummary:      mergeAgentContextOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:           mergeAgentContextOverride(parent.ToolAgent, child.ToolAgent),
		Image:               mergeAgentContextOverride(parent.Image, child.Image),
		Automation:          mergeAgentContextOverride(parent.Automation, child.Automation),
		ContextCompaction:   mergeAgentContextOverride(parent.ContextCompaction, child.ContextCompaction),
	}
}

func ResolveAgentContext(cfg *Config, agentKind string) ResolvedAgentContextSettings {
	settings := DefaultAgentContextSettings()
	if cfg != nil {
		settings = MergeAgentContextSettings(settings, cfg.AgentContexts)
	}
	override := mergeAgentContextOverride(settings.Default, agentContextOverrideFor(settings, agentKind))

	compactionEnabled := true
	if override.CompactionEnabled != nil {
		compactionEnabled = *override.CompactionEnabled
	}
	compactionThreshold := normalizedCompactionThreshold(override.CompactionThreshold)
	toolResultContextEnabled := defaultToolResultContextEnabled(agentKind)
	if override.ToolResultContextEnabled != nil {
		toolResultContextEnabled = *override.ToolResultContextEnabled
	}

	return ResolvedAgentContextSettings{
		CompactionEnabled:        compactionEnabled,
		CompactionThreshold:      compactionThreshold,
		ToolResultContextEnabled: toolResultContextEnabled,
		MaxFragmentBytes:         resolvedPositiveLimit(override.MaxFragmentBytes, DefaultAgentContextMaxFragmentBytes, MaxAgentContextFragmentBytes),
		MaxTotalInjectedBytes:    resolvedPositiveLimit(override.MaxTotalInjectedBytes, DefaultAgentContextMaxTotalInjectedBytes, MaxAgentContextTotalInjectedBytes),
		MaxFragments:             resolvedPositiveLimit(override.MaxFragments, DefaultAgentContextMaxFragments, MaxAgentContextFragments),
		MaxMetadataFieldBytes:    resolvedPositiveLimit(override.MaxMetadataFieldBytes, DefaultAgentContextMaxMetadataFieldBytes, MaxAgentContextMetadataFieldBytes),
		MaxProviderInputBytes:    resolvedPositiveLimit(override.MaxProviderInputBytes, DefaultAgentContextMaxProviderInputBytes, MaxAgentContextProviderInputBytes),
	}
}

// ResolveAgentContexts returns the authoritative effective policy for every
// registered Agent kind. UI clients consume this instead of reimplementing
// inheritance, defaults, or normalization.
func ResolveAgentContexts(cfg *Config) map[string]ResolvedAgentContextSettings {
	resolved := make(map[string]ResolvedAgentContextSettings, len(AgentKindDefinitions()))
	for _, definition := range AgentKindDefinitions() {
		resolved[definition.Kind] = ResolveAgentContext(cfg, definition.Kind)
	}
	return resolved
}

func mergeAgentContextOverride(parent, child AgentContextOverride) AgentContextOverride {
	out := parent
	if child.CompactionEnabled != nil {
		out.CompactionEnabled = child.CompactionEnabled
	}
	if child.CompactionThreshold != nil {
		out.CompactionThreshold = child.CompactionThreshold
	}
	if child.ToolResultContextEnabled != nil {
		out.ToolResultContextEnabled = child.ToolResultContextEnabled
	}
	if child.MaxFragmentBytes != nil {
		out.MaxFragmentBytes = child.MaxFragmentBytes
	}
	if child.MaxTotalInjectedBytes != nil {
		out.MaxTotalInjectedBytes = child.MaxTotalInjectedBytes
	}
	if child.MaxFragments != nil {
		out.MaxFragments = child.MaxFragments
	}
	if child.MaxMetadataFieldBytes != nil {
		out.MaxMetadataFieldBytes = child.MaxMetadataFieldBytes
	}
	if child.MaxProviderInputBytes != nil {
		out.MaxProviderInputBytes = child.MaxProviderInputBytes
	}
	return out
}

func agentContextOverrideFor(settings AgentContextSettings, agentKind string) AgentContextOverride {
	if definition, ok := LookupAgentKind(agentKind); ok && definition.ContextOverride != nil {
		return definition.ContextOverride(settings)
	}
	return AgentContextOverride{}
}

func sanitizeAgentContextSettings(settings AgentContextSettings) AgentContextSettings {
	settings.Default = sanitizeAgentContextOverride(settings.Default)
	settings.General = sanitizeAgentContextOverride(settings.General)
	settings.IDE = sanitizeAgentContextOverride(settings.IDE)
	settings.InteractiveStory = sanitizeAgentContextOverride(settings.InteractiveStory)
	settings.ConfigManager = sanitizeAgentContextOverride(settings.ConfigManager)
	settings.InteractiveDirector = sanitizeAgentContextOverride(settings.InteractiveDirector)
	settings.VersionSummary = sanitizeAgentContextOverride(settings.VersionSummary)
	settings.ToolAgent = sanitizeAgentContextOverride(settings.ToolAgent)
	settings.Image = sanitizeAgentContextOverride(settings.Image)
	settings.Automation = sanitizeAgentContextOverride(settings.Automation)
	settings.ContextCompaction = sanitizeAgentContextOverride(settings.ContextCompaction)
	return settings
}

func sanitizeAgentContextOverride(override AgentContextOverride) AgentContextOverride {
	if override.CompactionThreshold != nil {
		*override.CompactionThreshold = normalizedCompactionThreshold(override.CompactionThreshold)
	}
	sanitizePositiveLimit(override.MaxFragmentBytes, DefaultAgentContextMaxFragmentBytes, MaxAgentContextFragmentBytes)
	sanitizePositiveLimit(override.MaxTotalInjectedBytes, DefaultAgentContextMaxTotalInjectedBytes, MaxAgentContextTotalInjectedBytes)
	sanitizePositiveLimit(override.MaxFragments, DefaultAgentContextMaxFragments, MaxAgentContextFragments)
	sanitizePositiveLimit(override.MaxMetadataFieldBytes, DefaultAgentContextMaxMetadataFieldBytes, MaxAgentContextMetadataFieldBytes)
	sanitizePositiveLimit(override.MaxProviderInputBytes, DefaultAgentContextMaxProviderInputBytes, MaxAgentContextProviderInputBytes)
	return override
}

func normalizedCompactionThreshold(value *float64) float64 {
	if value == nil {
		return DefaultContextCompactionThreshold
	}
	if *value < 0.50 {
		return 0.50
	}
	if *value > 0.98 {
		return 0.98
	}
	return *value
}

func resolvedPositiveLimit(value *int, fallback, maximum int) int {
	if value == nil || *value <= 0 {
		return fallback
	}
	if *value > maximum {
		return maximum
	}
	return *value
}

func sanitizePositiveLimit(value *int, fallback, maximum int) {
	if value != nil {
		*value = resolvedPositiveLimit(value, fallback, maximum)
	}
}

func defaultToolResultContextEnabled(agentKind string) bool {
	switch agentKind {
	case AgentKindGeneral, AgentKindIDE, AgentKindInteractiveStory:
		return true
	default:
		return false
	}
}
