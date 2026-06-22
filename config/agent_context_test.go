package config

import "testing"

func TestResolveAgentContextCompactionDefaultsAndCaps(t *testing.T) {
	resolved := ResolveAgentContext(&Config{}, AgentKindIDE)
	if !resolved.CompactionEnabled {
		t.Fatal("context compaction should be enabled by default")
	}
	if resolved.CompactionThreshold != 0.90 {
		t.Fatalf("default compaction threshold = %v, want 0.90", resolved.CompactionThreshold)
	}
	if resolved.CompactionRecentTurns != DefaultContextCompactionRetainedTurns {
		t.Fatalf("default compaction recent turns = %d, want %d", resolved.CompactionRecentTurns, DefaultContextCompactionRetainedTurns)
	}
	if resolved.CompactionTargetMin != 0.05 {
		t.Fatalf("default compaction target min = %v, want 0.05", resolved.CompactionTargetMin)
	}
	if resolved.CompactionTargetMax != 0.20 {
		t.Fatalf("default compaction target max = %v, want 0.20", resolved.CompactionTargetMax)
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
	cfg = &Config{AgentContexts: AgentContextSettings{
		IDE: AgentContextOverride{CompactionRecentTurns: &highRecentTurns},
	}}
	if got := ResolveAgentContext(cfg, AgentKindIDE).CompactionRecentTurns; got != MaxContextCompactionRetainedTurns {
		t.Fatalf("high recent turns should be capped to %d, got %d", MaxContextCompactionRetainedTurns, got)
	}
}

func TestResolveAgentContextUsesPerAgentOverride(t *testing.T) {
	defaultThreshold := 0.80
	hotChoicesThreshold := 0.70
	defaultRecentTurns := 4
	hotChoicesRecentTurns := 2
	cfg := &Config{AgentContexts: AgentContextSettings{
		Default:               AgentContextOverride{CompactionThreshold: &defaultThreshold, CompactionRecentTurns: &defaultRecentTurns},
		InteractiveHotChoices: AgentContextOverride{CompactionThreshold: &hotChoicesThreshold, CompactionRecentTurns: &hotChoicesRecentTurns},
	}}
	if got := ResolveAgentContext(cfg, AgentKindIDE).CompactionThreshold; got != 0.80 {
		t.Fatalf("default inherited threshold = %v, want 0.80", got)
	}
	if got := ResolveAgentContext(cfg, AgentKindIDE).CompactionRecentTurns; got != 4 {
		t.Fatalf("default inherited recent turns = %v, want 4", got)
	}
	if got := ResolveAgentContext(cfg, AgentKindInteractiveHotChoices).CompactionThreshold; got != 0.70 {
		t.Fatalf("per-agent threshold = %v, want 0.70", got)
	}
	if got := ResolveAgentContext(cfg, AgentKindInteractiveHotChoices).CompactionRecentTurns; got != 2 {
		t.Fatalf("per-agent recent turns = %v, want 2", got)
	}
}
