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
