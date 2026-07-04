package app

import (
	"testing"

	"denova/internal/interactive"
	"denova/internal/styleref"
)

func TestConvertTellerStyleRulesFiltersSelectedScenes(t *testing.T) {
	rules := []interactive.StyleRule{
		{Scene: "激烈打斗", StyleContents: []string{"短句留白"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
	}

	got := convertTellerStyleRules("", rules, []string{"日常对话"})
	if len(got) != 1 || got[0].Scene != "日常对话" || got[0].StyleContents[0] != "温吞对白" {
		t.Fatalf("filtered style rules mismatch: %#v", got)
	}
}

func TestConvertTellerStyleRulesUsesAllScenesWhenUnspecified(t *testing.T) {
	rules := []interactive.StyleRule{
		{Scene: "激烈打斗", StyleContents: []string{"短句留白"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
	}

	got := convertTellerStyleRules("", rules, nil)
	if len(got) != 2 {
		t.Fatalf("style rules = %#v, want all scenes", got)
	}
}

func TestConvertTellerStyleRulesResolvesSharedStyleRefs(t *testing.T) {
	novaDir := t.TempDir()
	ref, err := styleref.NewLibrary(novaDir).Write(styleref.WriteRequest{
		Name:        "克制细腻",
		Description: "动作和停顿承载情绪",
		Filename:    "restraint.md",
		Content:     "# 克制细腻\n\n动作和停顿承载情绪。\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := convertTellerStyleRules(novaDir, []interactive.StyleRule{{
		Scene:     "日常对话",
		StyleRefs: []string{ref.DisplayPath},
	}}, nil)
	if len(got) != 1 || len(got[0].StyleReferences) != 1 {
		t.Fatalf("style refs not resolved: %#v", got)
	}
	if got[0].StyleReferences[0].Path == "" || got[0].StyleReferences[0].DisplayPath != ".denova/styles/restraint.md" {
		t.Fatalf("resolved ref mismatch: %#v", got[0].StyleReferences[0])
	}
}
