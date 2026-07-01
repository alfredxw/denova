package app

import (
	"testing"

	"denova/internal/interactive"
)

func TestConvertTellerStyleRulesFiltersSelectedScenes(t *testing.T) {
	rules := []interactive.StyleRule{
		{Scene: "激烈打斗", StyleContents: []string{"短句留白"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
	}

	got := convertTellerStyleRules(rules, []string{"日常对话"})
	if len(got) != 1 || got[0].Scene != "日常对话" || got[0].StyleContents[0] != "温吞对白" {
		t.Fatalf("filtered style rules mismatch: %#v", got)
	}
}

func TestConvertTellerStyleRulesUsesAllScenesWhenUnspecified(t *testing.T) {
	rules := []interactive.StyleRule{
		{Scene: "激烈打斗", StyleContents: []string{"短句留白"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
	}

	got := convertTellerStyleRules(rules, nil)
	if len(got) != 2 {
		t.Fatalf("style rules = %#v, want all scenes", got)
	}
}
