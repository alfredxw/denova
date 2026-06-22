package config

const (
	// DefaultContextCompactionRetainedTurns is the raw-history tail kept next to
	// a compaction summary when the user has not configured a value.
	DefaultContextCompactionRetainedTurns = 1
	MaxContextCompactionRetainedTurns     = 30
)

// AgentContextSettings stores per-agent context compaction settings.
type AgentContextSettings struct {
	Default               AgentContextOverride `toml:"default,omitempty" json:"default,omitempty"`
	IDE                   AgentContextOverride `toml:"ide,omitempty" json:"ide,omitempty"`
	InteractiveStory      AgentContextOverride `toml:"interactive_story,omitempty" json:"interactive_story,omitempty"`
	ConfigManager         AgentContextOverride `toml:"config_manager,omitempty" json:"config_manager,omitempty"`
	InteractiveState      AgentContextOverride `toml:"interactive_state,omitempty" json:"interactive_state,omitempty"`
	InteractiveHotChoices AgentContextOverride `toml:"interactive_hot_choices,omitempty" json:"interactive_hot_choices,omitempty"`
	VersionSummary        AgentContextOverride `toml:"version_summary,omitempty" json:"version_summary,omitempty"`
	ToolAgent             AgentContextOverride `toml:"tool_agent,omitempty" json:"tool_agent,omitempty"`
	Automation            AgentContextOverride `toml:"automation,omitempty" json:"automation,omitempty"`
	ContextCompaction     AgentContextOverride `toml:"context_compaction,omitempty" json:"context_compaction,omitempty"`
}

type AgentContextOverride struct {
	CompactionEnabled     *bool    `toml:"compaction_enabled,omitempty" json:"compaction_enabled,omitempty"`
	CompactionThreshold   *float64 `toml:"compaction_threshold,omitempty" json:"compaction_threshold,omitempty"`
	CompactionRecentTurns *int     `toml:"compaction_recent_turns,omitempty" json:"compaction_recent_turns,omitempty"`
	CompactionTargetMin   *float64 `toml:"compaction_target_min_ratio,omitempty" json:"compaction_target_min_ratio,omitempty"`
	CompactionTargetMax   *float64 `toml:"compaction_target_max_ratio,omitempty" json:"compaction_target_max_ratio,omitempty"`
}

type ResolvedAgentContextSettings struct {
	CompactionEnabled     bool    `json:"compaction_enabled"`
	CompactionThreshold   float64 `json:"compaction_threshold"`
	CompactionRecentTurns int     `json:"compaction_recent_turns"`
	CompactionTargetMin   float64 `json:"compaction_target_min_ratio"`
	CompactionTargetMax   float64 `json:"compaction_target_max_ratio"`
}

func DefaultAgentContextSettings() AgentContextSettings {
	return AgentContextSettings{
		Default: AgentContextOverride{
			CompactionEnabled:     boolPtr(true),
			CompactionThreshold:   floatPtr(0.90),
			CompactionRecentTurns: intPtr(DefaultContextCompactionRetainedTurns),
			CompactionTargetMin:   floatPtr(0.05),
			CompactionTargetMax:   floatPtr(0.20),
		},
	}
}

func MergeAgentContextSettings(parent, child AgentContextSettings) AgentContextSettings {
	return AgentContextSettings{
		Default:               mergeAgentContextOverride(parent.Default, child.Default),
		IDE:                   mergeAgentContextOverride(parent.IDE, child.IDE),
		InteractiveStory:      mergeAgentContextOverride(parent.InteractiveStory, child.InteractiveStory),
		ConfigManager:         mergeAgentContextOverride(parent.ConfigManager, child.ConfigManager),
		InteractiveState:      mergeAgentContextOverride(parent.InteractiveState, child.InteractiveState),
		InteractiveHotChoices: mergeAgentContextOverride(parent.InteractiveHotChoices, child.InteractiveHotChoices),
		VersionSummary:        mergeAgentContextOverride(parent.VersionSummary, child.VersionSummary),
		ToolAgent:             mergeAgentContextOverride(parent.ToolAgent, child.ToolAgent),
		Automation:            mergeAgentContextOverride(parent.Automation, child.Automation),
		ContextCompaction:     mergeAgentContextOverride(parent.ContextCompaction, child.ContextCompaction),
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
	compactionThreshold := 0.90
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
	return ResolvedAgentContextSettings{
		CompactionEnabled:     compactionEnabled,
		CompactionThreshold:   compactionThreshold,
		CompactionRecentTurns: compactionRecentTurns,
		CompactionTargetMin:   compactionTargetMin,
		CompactionTargetMax:   compactionTargetMax,
	}
}

func mergeAgentContextOverride(parent, child AgentContextOverride) AgentContextOverride {
	out := parent
	if child.CompactionEnabled != nil {
		out.CompactionEnabled = child.CompactionEnabled
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
	settings.InteractiveState = sanitizeAgentContextOverride(settings.InteractiveState)
	settings.InteractiveHotChoices = sanitizeAgentContextOverride(settings.InteractiveHotChoices)
	settings.VersionSummary = sanitizeAgentContextOverride(settings.VersionSummary)
	settings.ToolAgent = sanitizeAgentContextOverride(settings.ToolAgent)
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
	return override
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
