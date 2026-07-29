package config

const (
	// DefaultContextCompactionRetainedTurns is the raw-history tail kept next to
	// a compaction summary when the user has not configured a value.
	DefaultContextCompactionRetainedTurns = 1
	MaxContextCompactionRetainedTurns     = 30

	AgentContextCompactionStrategySummaryAgent = "summary_agent"

	// Context assembly defaults are deliberately generous for creative work.
	// They are injection limits, not transcript limits: persisted conversation
	// history is governed by compaction and is never silently rewritten here.
	DefaultAgentContextMaxFragmentBytes      = 256 * 1024
	DefaultAgentContextMaxTotalInjectedBytes = 1024 * 1024
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

// AgentContextSettings stores per-agent context compaction settings.
type AgentContextSettings struct {
	Default             AgentContextOverride `toml:"default,omitempty" json:"default,omitempty"`
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
	CompactionEnabled          *bool    `toml:"compaction_enabled,omitempty" json:"compaction_enabled,omitempty"`
	CompactionStrategy         *string  `toml:"compaction_strategy,omitempty" json:"compaction_strategy,omitempty"`
	CompactionThreshold        *float64 `toml:"compaction_threshold,omitempty" json:"compaction_threshold,omitempty"`
	CompactionRecentTurns      *int     `toml:"compaction_recent_turns,omitempty" json:"compaction_recent_turns,omitempty"`
	CompactionTargetMin        *float64 `toml:"compaction_target_min_ratio,omitempty" json:"compaction_target_min_ratio,omitempty"`
	CompactionTargetMax        *float64 `toml:"compaction_target_max_ratio,omitempty" json:"compaction_target_max_ratio,omitempty"`
	ToolResultRetentionEnabled *bool    `toml:"tool_result_retention_enabled,omitempty" json:"tool_result_retention_enabled,omitempty"`
	MaxFragmentBytes           *int     `toml:"max_fragment_bytes,omitempty" json:"max_fragment_bytes,omitempty"`
	MaxTotalInjectedBytes      *int     `toml:"max_total_injected_bytes,omitempty" json:"max_total_injected_bytes,omitempty"`
	MaxFragments               *int     `toml:"max_fragments,omitempty" json:"max_fragments,omitempty"`
	MaxMetadataFieldBytes      *int     `toml:"max_metadata_field_bytes,omitempty" json:"max_metadata_field_bytes,omitempty"`
	MaxProviderInputBytes      *int     `toml:"max_provider_input_bytes,omitempty" json:"max_provider_input_bytes,omitempty"`
}

type ResolvedAgentContextSettings struct {
	CompactionEnabled          bool    `json:"compaction_enabled"`
	CompactionStrategy         string  `json:"compaction_strategy"`
	CompactionThreshold        float64 `json:"compaction_threshold"`
	CompactionRecentTurns      int     `json:"compaction_recent_turns"`
	CompactionTargetMin        float64 `json:"compaction_target_min_ratio"`
	CompactionTargetMax        float64 `json:"compaction_target_max_ratio"`
	ToolResultRetentionEnabled bool    `json:"tool_result_retention_enabled"`
	MaxFragmentBytes           int     `json:"max_fragment_bytes"`
	MaxTotalInjectedBytes      int     `json:"max_total_injected_bytes"`
	MaxFragments               int     `json:"max_fragments"`
	MaxMetadataFieldBytes      int     `json:"max_metadata_field_bytes"`
	MaxProviderInputBytes      int     `json:"max_provider_input_bytes"`
}

func DefaultAgentContextSettings() AgentContextSettings {
	return AgentContextSettings{
		Default: AgentContextOverride{
			CompactionEnabled:     boolPtr(true),
			CompactionStrategy:    stringPtr(AgentContextCompactionStrategySummaryAgent),
			CompactionThreshold:   floatPtr(0.80),
			CompactionRecentTurns: intPtr(DefaultContextCompactionRetainedTurns),
			CompactionTargetMin:   floatPtr(0.05),
			CompactionTargetMax:   floatPtr(0.20),
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
	compactionStrategy := AgentContextCompactionStrategySummaryAgent
	if override.CompactionStrategy != nil {
		compactionStrategy = normalizeCompactionStrategy(*override.CompactionStrategy)
	}
	compactionThreshold := 0.80
	if override.CompactionThreshold != nil {
		compactionThreshold = *override.CompactionThreshold
	}
	if compactionThreshold < 0.50 {
		compactionThreshold = 0.50
	}
	if compactionThreshold > 0.98 {
		compactionThreshold = 0.98
	}
	compactionRecentTurns := DefaultContextCompactionRetainedTurns
	if override.CompactionRecentTurns != nil {
		compactionRecentTurns = normalizeCompactionRetainedTurns(*override.CompactionRecentTurns)
	}
	compactionTargetMin := 0.05
	if override.CompactionTargetMin != nil {
		compactionTargetMin = *override.CompactionTargetMin
	}
	compactionTargetMin = clampCompactionTargetRatio(compactionTargetMin, 0.05)
	compactionTargetMax := 0.20
	if override.CompactionTargetMax != nil {
		compactionTargetMax = *override.CompactionTargetMax
	}
	compactionTargetMax = clampCompactionTargetRatio(compactionTargetMax, 0.20)
	if compactionTargetMax < compactionTargetMin {
		compactionTargetMax = compactionTargetMin
	}
	toolResultRetentionEnabled := defaultToolResultRetentionEnabled(agentKind)
	if override.ToolResultRetentionEnabled != nil {
		toolResultRetentionEnabled = *override.ToolResultRetentionEnabled
	}
	maxFragmentBytes := resolvedPositiveLimit(override.MaxFragmentBytes, DefaultAgentContextMaxFragmentBytes, MaxAgentContextFragmentBytes)
	maxTotalInjectedBytes := resolvedPositiveLimit(override.MaxTotalInjectedBytes, DefaultAgentContextMaxTotalInjectedBytes, MaxAgentContextTotalInjectedBytes)
	maxFragments := resolvedPositiveLimit(override.MaxFragments, DefaultAgentContextMaxFragments, MaxAgentContextFragments)
	maxMetadataFieldBytes := resolvedPositiveLimit(override.MaxMetadataFieldBytes, DefaultAgentContextMaxMetadataFieldBytes, MaxAgentContextMetadataFieldBytes)
	maxProviderInputBytes := resolvedPositiveLimit(override.MaxProviderInputBytes, DefaultAgentContextMaxProviderInputBytes, MaxAgentContextProviderInputBytes)
	return ResolvedAgentContextSettings{
		CompactionEnabled:          compactionEnabled,
		CompactionStrategy:         compactionStrategy,
		CompactionThreshold:        compactionThreshold,
		CompactionRecentTurns:      compactionRecentTurns,
		CompactionTargetMin:        compactionTargetMin,
		CompactionTargetMax:        compactionTargetMax,
		ToolResultRetentionEnabled: toolResultRetentionEnabled,
		MaxFragmentBytes:           maxFragmentBytes,
		MaxTotalInjectedBytes:      maxTotalInjectedBytes,
		MaxFragments:               maxFragments,
		MaxMetadataFieldBytes:      maxMetadataFieldBytes,
		MaxProviderInputBytes:      maxProviderInputBytes,
	}
}

func mergeAgentContextOverride(parent, child AgentContextOverride) AgentContextOverride {
	out := parent
	if child.CompactionEnabled != nil {
		out.CompactionEnabled = child.CompactionEnabled
	}
	if child.CompactionStrategy != nil {
		out.CompactionStrategy = child.CompactionStrategy
	}
	if child.CompactionThreshold != nil {
		out.CompactionThreshold = child.CompactionThreshold
	}
	if child.CompactionRecentTurns != nil {
		out.CompactionRecentTurns = child.CompactionRecentTurns
	}
	if child.CompactionTargetMin != nil {
		out.CompactionTargetMin = child.CompactionTargetMin
	}
	if child.CompactionTargetMax != nil {
		out.CompactionTargetMax = child.CompactionTargetMax
	}
	if child.ToolResultRetentionEnabled != nil {
		out.ToolResultRetentionEnabled = child.ToolResultRetentionEnabled
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
		if *override.CompactionThreshold < 0.50 {
			*override.CompactionThreshold = 0.50
		}
		if *override.CompactionThreshold > 0.98 {
			*override.CompactionThreshold = 0.98
		}
	}
	if override.CompactionStrategy != nil {
		*override.CompactionStrategy = normalizeCompactionStrategy(*override.CompactionStrategy)
	}
	if override.CompactionRecentTurns != nil {
		*override.CompactionRecentTurns = normalizeCompactionRetainedTurns(*override.CompactionRecentTurns)
	}
	if override.CompactionTargetMin != nil {
		*override.CompactionTargetMin = clampCompactionTargetRatio(*override.CompactionTargetMin, 0.05)
	}
	if override.CompactionTargetMax != nil {
		*override.CompactionTargetMax = clampCompactionTargetRatio(*override.CompactionTargetMax, 0.20)
	}
	if override.CompactionTargetMin != nil && override.CompactionTargetMax != nil && *override.CompactionTargetMax < *override.CompactionTargetMin {
		*override.CompactionTargetMax = *override.CompactionTargetMin
	}
	sanitizePositiveLimit(override.MaxFragmentBytes, DefaultAgentContextMaxFragmentBytes, MaxAgentContextFragmentBytes)
	sanitizePositiveLimit(override.MaxTotalInjectedBytes, DefaultAgentContextMaxTotalInjectedBytes, MaxAgentContextTotalInjectedBytes)
	sanitizePositiveLimit(override.MaxFragments, DefaultAgentContextMaxFragments, MaxAgentContextFragments)
	sanitizePositiveLimit(override.MaxMetadataFieldBytes, DefaultAgentContextMaxMetadataFieldBytes, MaxAgentContextMetadataFieldBytes)
	sanitizePositiveLimit(override.MaxProviderInputBytes, DefaultAgentContextMaxProviderInputBytes, MaxAgentContextProviderInputBytes)
	return override
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
	if value == nil {
		return
	}
	*value = resolvedPositiveLimit(value, fallback, maximum)
}

func normalizeCompactionStrategy(value string) string {
	switch value {
	case AgentContextCompactionStrategySummaryAgent:
		return value
	default:
		return AgentContextCompactionStrategySummaryAgent
	}
}

func normalizeCompactionRetainedTurns(value int) int {
	if value <= 0 {
		return DefaultContextCompactionRetainedTurns
	}
	if value > MaxContextCompactionRetainedTurns {
		return MaxContextCompactionRetainedTurns
	}
	return value
}

func clampCompactionTargetRatio(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	if value < 0.01 {
		return 0.01
	}
	if value > 0.80 {
		return 0.80
	}
	return value
}

func defaultToolResultRetentionEnabled(agentKind string) bool {
	switch agentKind {
	case AgentKindIDE, AgentKindInteractiveStory:
		return true
	default:
		return false
	}
}
