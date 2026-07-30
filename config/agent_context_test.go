package config

import "testing"

func TestResolveAgentContextCompactionDefaultsAndCaps(t *testing.T) {
	resolved := ResolveAgentContext(&Config{}, AgentKindIDE)
	if !resolved.CompactionEnabled {
		t.Fatal("context compaction should be enabled by default")
	}
	if resolved.CompactionThreshold != DefaultContextCompactionThreshold {
		t.Fatalf("default compaction threshold = %v, want %v", resolved.CompactionThreshold, DefaultContextCompactionThreshold)
	}
	if resolved.CompactionRecentTurns != DefaultContextCompactionRetainedTurns {
		t.Fatalf("default compaction recent turns = %d, want %d", resolved.CompactionRecentTurns, DefaultContextCompactionRetainedTurns)
	}
	if resolved.CompactionStrategy != AgentContextCompactionStrategySummaryAgent {
		t.Fatalf("default compaction strategy = %q, want %q", resolved.CompactionStrategy, AgentContextCompactionStrategySummaryAgent)
	}
	if resolved.CompactionTargetMin != 0.05 {
		t.Fatalf("default compaction target min = %v, want 0.05", resolved.CompactionTargetMin)
	}
	if resolved.CompactionTargetMax != 0.20 {
		t.Fatalf("default compaction target max = %v, want 0.20", resolved.CompactionTargetMax)
	}
	if !resolved.ToolResultRetentionEnabled {
		t.Fatal("IDE tool result retention should be enabled by default")
	}
	if ResolveAgentContext(&Config{}, AgentKindInteractiveStory).ToolResultRetentionEnabled != true {
		t.Fatal("interactive story tool result retention should be enabled by default")
	}
	if ResolveAgentContext(&Config{}, AgentKindAutomation).ToolResultRetentionEnabled {
		t.Fatal("automation tool result retention should be disabled by default")
	}

	disabled := false
	lowThreshold := 0.30
	lowRecentTurns := 0
	lowTargetMin := 0.001
	highTargetMax := 0.95
	cfg := &Config{AgentContexts: AgentContextSettings{
		IDE: AgentContextOverride{
			CompactionEnabled:     &disabled,
			CompactionThreshold:   &lowThreshold,
			CompactionRecentTurns: &lowRecentTurns,
			CompactionTargetMin:   &lowTargetMin,
			CompactionTargetMax:   &highTargetMax,
		},
	}}
	resolved = ResolveAgentContext(cfg, AgentKindIDE)
	if resolved.CompactionEnabled {
		t.Fatal("per-agent compaction enabled override should be respected")
	}
	if resolved.CompactionThreshold != 0.50 {
		t.Fatalf("low threshold should be capped to 0.50, got %v", resolved.CompactionThreshold)
	}
	if resolved.CompactionRecentTurns != DefaultContextCompactionRetainedTurns {
		t.Fatalf("low recent turns should fall back to %d, got %d", DefaultContextCompactionRetainedTurns, resolved.CompactionRecentTurns)
	}
	if resolved.CompactionTargetMin != 0.01 {
		t.Fatalf("target min should be capped to 0.01, got %v", resolved.CompactionTargetMin)
	}
	if resolved.CompactionTargetMax != 0.80 {
		t.Fatalf("target max should be capped to 0.80, got %v", resolved.CompactionTargetMax)
	}

	highRecentTurns := MaxContextCompactionRetainedTurns + 20
	unknownStrategy := "parent_prefix"
	cfg = &Config{AgentContexts: AgentContextSettings{
		IDE: AgentContextOverride{
			CompactionRecentTurns: &highRecentTurns,
			CompactionStrategy:    &unknownStrategy,
		},
	}}
	resolved = ResolveAgentContext(cfg, AgentKindIDE)
	if got := resolved.CompactionRecentTurns; got != MaxContextCompactionRetainedTurns {
		t.Fatalf("high recent turns should be capped to %d, got %d", MaxContextCompactionRetainedTurns, got)
	}
	if resolved.CompactionStrategy != AgentContextCompactionStrategySummaryAgent {
		t.Fatalf("unknown compaction strategy should fall back to %q, got %q", AgentContextCompactionStrategySummaryAgent, resolved.CompactionStrategy)
	}
}

func TestResolveAgentContextPressurePolicyDefaultsAndCaps(t *testing.T) {
	resolved := ResolveAgentContext(&Config{}, AgentKindIDE)
	if resolved.ContextPressureScope != AgentContextPressureScopeBodyAfterPrefix ||
		resolved.ToolResultCleanupThreshold != 0.70 || resolved.ToolResultCleanupTarget != 0.60 ||
		resolved.ToolResultCleanupMinTokens != 20_000 || resolved.ToolResultKeepRecent != 3 ||
		resolved.ToolResultKeepRecentTokens != 16_000 || resolved.ToolResultWarmSuffixTokens != 8_000 ||
		resolved.ToolResultEagerMinTokens != 32_000 || resolved.CompactionRecoveryBand != 0.80 ||
		resolved.CompactionMaxConsecutiveFailures != 3 {
		t.Fatalf("unexpected context pressure defaults: %#v", resolved)
	}

	unknownScope := "mutable_everything"
	lowCleanupThreshold := 0.001
	highCleanupTarget := 2.0
	negativeTokens := -1
	highRecent := MaxToolResultKeepRecent + 1
	highTokens := MaxAgentContextPolicyTokens + 1
	lowRecoveryBand := 0.01
	highFailures := MaxContextCompactionConsecutiveFailures + 1
	cfg := &Config{AgentContexts: AgentContextSettings{IDE: AgentContextOverride{
		ContextPressureScope:             &unknownScope,
		ToolResultCleanupThreshold:       &lowCleanupThreshold,
		ToolResultCleanupTarget:          &highCleanupTarget,
		ToolResultCleanupMinTokens:       &negativeTokens,
		ToolResultKeepRecent:             &highRecent,
		ToolResultKeepRecentTokens:       &highTokens,
		ToolResultWarmSuffixTokens:       &highTokens,
		ToolResultEagerMinTokens:         &highTokens,
		CompactionRecoveryBand:           &lowRecoveryBand,
		CompactionMaxConsecutiveFailures: &highFailures,
	}}}
	resolved = ResolveAgentContext(cfg, AgentKindIDE)
	if resolved.ContextPressureScope != DefaultAgentContextPressureScope {
		t.Fatalf("unknown pressure scope = %q, want %q", resolved.ContextPressureScope, DefaultAgentContextPressureScope)
	}
	if resolved.ToolResultCleanupThreshold != 0.01 || resolved.ToolResultCleanupTarget != 0.0085 {
		t.Fatalf("cleanup ratios were not capped coherently: threshold=%v target=%v", resolved.ToolResultCleanupThreshold, resolved.ToolResultCleanupTarget)
	}
	if resolved.ToolResultCleanupMinTokens != DefaultToolResultCleanupMinTokens {
		t.Fatalf("negative cleanup minimum = %d, want default %d", resolved.ToolResultCleanupMinTokens, DefaultToolResultCleanupMinTokens)
	}
	if resolved.ToolResultKeepRecent != MaxToolResultKeepRecent ||
		resolved.ToolResultKeepRecentTokens != MaxAgentContextPolicyTokens ||
		resolved.ToolResultWarmSuffixTokens != MaxAgentContextPolicyTokens ||
		resolved.ToolResultEagerMinTokens != MaxAgentContextPolicyTokens {
		t.Fatalf("pressure policy caps were not applied: %#v", resolved)
	}
	if resolved.CompactionRecoveryBand != 0.10 || resolved.CompactionMaxConsecutiveFailures != MaxContextCompactionConsecutiveFailures {
		t.Fatalf("compaction recovery caps were not applied: band=%v failures=%d", resolved.CompactionRecoveryBand, resolved.CompactionMaxConsecutiveFailures)
	}
}

func TestResolveAgentContextPressurePolicyUsesLayeredOverridesAndKeepsLegacySwitch(t *testing.T) {
	totalScope := AgentContextPressureScopeTotal
	defaultTarget := 0.55
	storyTarget := 0.45
	legacyRetention := false
	zeroProtectedGroups := 0
	cfg := &Config{AgentContexts: AgentContextSettings{
		Default: AgentContextOverride{
			ContextPressureScope:       &totalScope,
			ToolResultCleanupTarget:    &defaultTarget,
			ToolResultRetentionEnabled: &legacyRetention,
		},
		InteractiveStory: AgentContextOverride{
			ToolResultCleanupTarget: &storyTarget,
			ToolResultKeepRecent:    &zeroProtectedGroups,
		},
	}}

	ide := ResolveAgentContext(cfg, AgentKindIDE)
	story := ResolveAgentContext(cfg, AgentKindInteractiveStory)
	if ide.ContextPressureScope != AgentContextPressureScopeTotal || ide.ToolResultCleanupTarget != defaultTarget {
		t.Fatalf("default pressure policy was not inherited: %#v", ide)
	}
	if story.ContextPressureScope != AgentContextPressureScopeTotal || story.ToolResultCleanupTarget != storyTarget || story.ToolResultKeepRecent != 0 {
		t.Fatalf("story pressure policy override was not layered: %#v", story)
	}
	if ide.ToolResultRetentionEnabled || story.ToolResultRetentionEnabled {
		t.Fatal("legacy tool_result_retention_enabled should remain readable")
	}
}

func TestSanitizeAgentContextSettingsKeepsCrossLayerPressureOrder(t *testing.T) {
	defaultCompaction := 0.75
	defaultCleanup := 0.65
	defaultTarget := 0.55
	storyCompaction := 0.60
	storyCleanup := 0.90
	storyTarget := 0.80

	sanitized := sanitizeAgentContextSettings(AgentContextSettings{
		Default: AgentContextOverride{
			CompactionThreshold: &defaultCompaction, ToolResultCleanupThreshold: &defaultCleanup,
			ToolResultCleanupTarget: &defaultTarget,
		},
		InteractiveStory: AgentContextOverride{
			CompactionThreshold: &storyCompaction, ToolResultCleanupThreshold: &storyCleanup,
			ToolResultCleanupTarget: &storyTarget,
		},
	})
	story := sanitized.InteractiveStory
	wantCleanup := storyCompaction * contextPressureOrderingFallbackRatio
	wantTarget := wantCleanup * contextPressureOrderingFallbackRatio
	if story.ToolResultCleanupThreshold == nil || *story.ToolResultCleanupThreshold != wantCleanup ||
		story.ToolResultCleanupTarget == nil || *story.ToolResultCleanupTarget != wantTarget {
		t.Fatalf("cross-layer pressure order was not sanitized: %#v", story)
	}
	cfg := &Config{AgentContexts: sanitized}
	resolved := ResolveAgentContext(cfg, AgentKindInteractiveStory)
	if !(resolved.ToolResultCleanupTarget < resolved.ToolResultCleanupThreshold && resolved.ToolResultCleanupThreshold < resolved.CompactionThreshold) {
		t.Fatalf("resolved pressure order is inconsistent: %#v", resolved)
	}
}

func TestResolveAgentContextUsesPerAgentOverride(t *testing.T) {
	defaultThreshold := 0.80
	directorThreshold := 0.70
	defaultRecentTurns := 4
	directorRecentTurns := 2
	cfg := &Config{AgentContexts: AgentContextSettings{
		Default:             AgentContextOverride{CompactionThreshold: &defaultThreshold, CompactionRecentTurns: &defaultRecentTurns},
		InteractiveDirector: AgentContextOverride{CompactionThreshold: &directorThreshold, CompactionRecentTurns: &directorRecentTurns},
	}}
	if got := ResolveAgentContext(cfg, AgentKindIDE).CompactionThreshold; got != 0.80 {
		t.Fatalf("default inherited threshold = %v, want 0.80", got)
	}
	if got := ResolveAgentContext(cfg, AgentKindIDE).CompactionRecentTurns; got != 4 {
		t.Fatalf("default inherited recent turns = %v, want 4", got)
	}
	if got := ResolveAgentContext(cfg, AgentKindInteractiveDirector).CompactionThreshold; got != 0.70 {
		t.Fatalf("per-agent threshold = %v, want 0.70", got)
	}
	if got := ResolveAgentContext(cfg, AgentKindInteractiveDirector).CompactionRecentTurns; got != 2 {
		t.Fatalf("per-agent recent turns = %v, want 2", got)
	}
}

func TestResolveAgentContextAssemblyBudgetDefaultsAndPerAgentOverride(t *testing.T) {
	resolved := ResolveAgentContext(&Config{}, AgentKindIDE)
	if resolved.MaxFragmentBytes <= 128*1024 || resolved.MaxTotalInjectedBytes <= 128*1024 || resolved.MaxProviderInputBytes <= 128*1024 {
		t.Fatalf("default assembly budget is too small: %#v", resolved)
	}
	if resolved.MaxFragments <= 0 || resolved.MaxMetadataFieldBytes <= 0 {
		t.Fatalf("default assembly budget is incomplete: %#v", resolved)
	}

	fragmentBytes := 384 * 1024
	totalBytes := 2 * 1024 * 1024
	fragments := 64
	metadataBytes := 8 * 1024
	providerBytes := 8 * 1024 * 1024
	cfg := &Config{AgentContexts: AgentContextSettings{
		IDE: AgentContextOverride{
			MaxFragmentBytes:      &fragmentBytes,
			MaxTotalInjectedBytes: &totalBytes,
			MaxFragments:          &fragments,
			MaxMetadataFieldBytes: &metadataBytes,
			MaxProviderInputBytes: &providerBytes,
		},
	}}
	resolved = ResolveAgentContext(cfg, AgentKindIDE)
	if resolved.MaxFragmentBytes != fragmentBytes || resolved.MaxTotalInjectedBytes != totalBytes || resolved.MaxFragments != fragments || resolved.MaxMetadataFieldBytes != metadataBytes || resolved.MaxProviderInputBytes != providerBytes {
		t.Fatalf("resolved assembly budget = %#v", resolved)
	}
}
