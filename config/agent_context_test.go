package config

import "testing"

func TestResolveAgentContextDefaultsAndCapsRecentTurns(t *testing.T) {
	if got := ResolveAgentContext(&Config{}, AgentKindInteractiveStory).RecentTurns; got != 30 {
		t.Fatalf("default recent turns = %d, want 30", got)
	}
	recentTurns := 45
	cfg := &Config{AgentContexts: AgentContextSettings{
		InteractiveStory: AgentContextOverride{RecentTurns: &recentTurns},
	}}
	if got := ResolveAgentContext(cfg, AgentKindInteractiveStory).RecentTurns; got != 30 {
		t.Fatalf("capped recent turns = %d, want 30", got)
	}
}

func TestResolveAgentContextCompactionDefaultsAndCaps(t *testing.T) {
	resolved := ResolveAgentContext(&Config{}, AgentKindIDE)
	if !resolved.CompactionEnabled {
		t.Fatal("context compaction should be enabled by default")
	}
	if resolved.CompactionThreshold != 0.90 {
		t.Fatalf("default compaction threshold = %v, want 0.90", resolved.CompactionThreshold)
	}
	if resolved.CompactionRecentTurns != 8 {
		t.Fatalf("default compaction recent turns = %d, want 8", resolved.CompactionRecentTurns)
	}
	if resolved.CompactionTargetMin != 0.05 {
		t.Fatalf("default compaction target min = %v, want 0.05", resolved.CompactionTargetMin)
	}
	if resolved.CompactionTargetMax != 0.20 {
		t.Fatalf("default compaction target max = %v, want 0.20", resolved.CompactionTargetMax)
	}

	disabled := false
	lowThreshold := 0.30
	highRecent := 50
	lowTargetMin := 0.001
	highTargetMax := 0.95
	cfg := &Config{AgentContexts: AgentContextSettings{
		IDE: AgentContextOverride{
			CompactionEnabled:     &disabled,
			CompactionThreshold:   &lowThreshold,
			CompactionRecentTurns: &highRecent,
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
	if resolved.CompactionRecentTurns != 30 {
		t.Fatalf("retained recent turns should be capped to 30, got %d", resolved.CompactionRecentTurns)
	}
	if resolved.CompactionTargetMin != 0.01 {
		t.Fatalf("target min should be capped to 0.01, got %v", resolved.CompactionTargetMin)
	}
	if resolved.CompactionTargetMax != 0.80 {
		t.Fatalf("target max should be capped to 0.80, got %v", resolved.CompactionTargetMax)
	}
}

func TestResolveAgentContextUsesPerAgentOverride(t *testing.T) {
	defaultTurns := 20
	hotChoicesTurns := 12
	cfg := &Config{AgentContexts: AgentContextSettings{
		Default:               AgentContextOverride{RecentTurns: &defaultTurns},
		InteractiveHotChoices: AgentContextOverride{RecentTurns: &hotChoicesTurns},
	}}
	if got := ResolveAgentContext(cfg, AgentKindIDE).RecentTurns; got != 20 {
		t.Fatalf("default inherited recent turns = %d, want 20", got)
	}
	if got := ResolveAgentContext(cfg, AgentKindInteractiveHotChoices).RecentTurns; got != 12 {
		t.Fatalf("per-agent recent turns = %d, want 12", got)
	}
}
