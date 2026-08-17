package config

import "testing"

func TestResolveAgentContextDefaultsAndCaps(t *testing.T) {
	resolved := ResolveAgentContext(&Config{}, AgentKindIDE)
	if !resolved.CompactionEnabled {
		t.Fatal("context compaction should be enabled by default")
	}
	if resolved.CompactionThreshold != DefaultContextCompactionThreshold {
		t.Fatalf("default compaction threshold = %v, want %v", resolved.CompactionThreshold, DefaultContextCompactionThreshold)
	}
	if !resolved.ToolResultContextEnabled {
		t.Fatal("IDE tool result context should be enabled by default")
	}
	if !ResolveAgentContext(&Config{}, AgentKindInteractiveStory).ToolResultContextEnabled {
		t.Fatal("interactive story tool result context should be enabled by default")
	}
	if ResolveAgentContext(&Config{}, AgentKindAutomation).ToolResultContextEnabled {
		t.Fatal("automation tool result context should be disabled by default")
	}

	disabled := false
	lowThreshold := 0.30
	cfg := &Config{AgentContexts: AgentContextSettings{
		IDE: AgentContextOverride{
			CompactionEnabled:        &disabled,
			CompactionThreshold:      &lowThreshold,
			ToolResultContextEnabled: &disabled,
		},
	}}
	resolved = ResolveAgentContext(cfg, AgentKindIDE)
	if resolved.CompactionEnabled || resolved.ToolResultContextEnabled {
		t.Fatalf("per-agent switches were not respected: %#v", resolved)
	}
	if resolved.CompactionThreshold != 0.50 {
		t.Fatalf("low threshold should be capped to 0.50, got %v", resolved.CompactionThreshold)
	}
}

func TestResolveAgentContextUsesLayeredOverrides(t *testing.T) {
	defaultThreshold := 0.80
	directorThreshold := 0.70
	disableToolContext := false
	enableToolContext := true
	cfg := &Config{AgentContexts: AgentContextSettings{
		Default: AgentContextOverride{
			CompactionThreshold:      &defaultThreshold,
			ToolResultContextEnabled: &disableToolContext,
		},
		InteractiveDirector: AgentContextOverride{
			CompactionThreshold:      &directorThreshold,
			ToolResultContextEnabled: &enableToolContext,
		},
	}}

	ide := ResolveAgentContext(cfg, AgentKindIDE)
	if ide.CompactionThreshold != defaultThreshold || ide.ToolResultContextEnabled {
		t.Fatalf("default context intent was not inherited: %#v", ide)
	}
	director := ResolveAgentContext(cfg, AgentKindInteractiveDirector)
	if director.CompactionThreshold != directorThreshold || !director.ToolResultContextEnabled {
		t.Fatalf("per-agent context intent was not applied: %#v", director)
	}
}

func TestSanitizeAgentContextSettingsNormalizesEditableValues(t *testing.T) {
	lowThreshold := 0.20
	highThreshold := 1.20
	invalidLimit := -1
	overLimit := MaxAgentContextFragmentBytes + 1

	sanitized := sanitizeAgentContextSettings(AgentContextSettings{
		Default: AgentContextOverride{
			CompactionThreshold: &lowThreshold,
			MaxFragments:        &invalidLimit,
		},
		InteractiveStory: AgentContextOverride{
			CompactionThreshold: &highThreshold,
			MaxFragmentBytes:    &overLimit,
		},
	})
	if got := *sanitized.Default.CompactionThreshold; got != 0.50 {
		t.Fatalf("low threshold = %v, want 0.50", got)
	}
	if got := *sanitized.InteractiveStory.CompactionThreshold; got != 0.98 {
		t.Fatalf("high threshold = %v, want 0.98", got)
	}
	if got := *sanitized.Default.MaxFragments; got != DefaultAgentContextMaxFragments {
		t.Fatalf("invalid fragment count = %d, want %d", got, DefaultAgentContextMaxFragments)
	}
	if got := *sanitized.InteractiveStory.MaxFragmentBytes; got != MaxAgentContextFragmentBytes {
		t.Fatalf("fragment limit = %d, want %d", got, MaxAgentContextFragmentBytes)
	}
}

func TestResolveAgentContextsCoversRegistry(t *testing.T) {
	resolved := ResolveAgentContexts(&Config{})
	for _, definition := range AgentKindDefinitions() {
		if _, ok := resolved[definition.Kind]; !ok {
			t.Fatalf("resolved context is missing agent kind %q", definition.Kind)
		}
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
