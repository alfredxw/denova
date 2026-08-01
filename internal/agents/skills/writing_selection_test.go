package skills

import (
	"testing"

	"denova/config"
)

func TestResolveWritingSkillNameDefaultsAndSelection(t *testing.T) {
	if got := ResolveWritingSkillName(&config.Config{WritingSkillDefault: "slow-burn"}, ""); got != "slow-burn" {
		t.Fatalf("default writing skill = %s, want slow-burn", got)
	}
	if got := ResolveWritingSkillName(&config.Config{WritingSkillDefault: "slow-burn"}, "scene-first"); got != "scene-first" {
		t.Fatalf("selected writing skill = %s, want scene-first", got)
	}
	if got := ResolveWritingSkillName(&config.Config{}, ""); got != config.DefaultWritingSkillName {
		t.Fatalf("fallback writing skill = %s, want %s", got, config.DefaultWritingSkillName)
	}
}
