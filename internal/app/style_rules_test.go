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

	got := convertTellerStyleRules("", nil, rules, []string{"日常对话"})
	if len(got) != 1 || got[0].Scene != "日常对话" || got[0].StyleContents[0] != "温吞对白" {
		t.Fatalf("filtered style rules mismatch: %#v", got)
	}
}

func TestConvertTellerStyleRulesUsesAllScenesWhenUnspecified(t *testing.T) {
	rules := []interactive.StyleRule{
		{Scene: "激烈打斗", StyleContents: []string{"短句留白"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
	}

	got := convertTellerStyleRules("", nil, rules, nil)
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
	got := convertTellerStyleRules(novaDir, nil, []interactive.StyleRule{{
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

func TestConvertTellerStyleRulesKeepsGlobalRefsWhenSceneFiltered(t *testing.T) {
	novaDir := t.TempDir()
	ref, err := styleref.NewLibrary(novaDir).Write(styleref.WriteRequest{
		Name:        "全局克制",
		Description: "所有正文默认参考",
		Filename:    "global-restraint.md",
		Content:     "# 全局克制\n\n所有正文默认参考。\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := convertTellerStyleRules(novaDir, []string{ref.DisplayPath}, []interactive.StyleRule{
		{Scene: "激烈打斗", StyleContents: []string{"短句留白"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
	}, []string{"日常对话"})

	if len(got) != 2 {
		t.Fatalf("style rules = %#v, want global plus selected scene", got)
	}
	if !got[0].Global || len(got[0].StyleReferences) != 1 {
		t.Fatalf("global style ref missing: %#v", got)
	}
	if got[1].Scene != "日常对话" {
		t.Fatalf("scene rule mismatch: %#v", got)
	}
}

func TestConvertTellerStyleRulesTreatsGlobalSceneAsDefault(t *testing.T) {
	got := convertTellerStyleRules("", nil, []interactive.StyleRule{
		{Scene: "全局", StyleContents: []string{"默认文风"}},
		{Scene: "日常对话", StyleContents: []string{"温吞对白"}},
		{Scene: "激烈打斗", StyleContents: []string{"短句留白"}},
	}, []string{"日常对话"})

	if len(got) != 2 {
		t.Fatalf("style rules = %#v, want global scene plus selected scene", got)
	}
	if !got[0].Global || got[0].StyleContents[0] != "默认文风" {
		t.Fatalf("global scene was not converted to default rule: %#v", got)
	}
	if got[1].Scene != "日常对话" {
		t.Fatalf("scene rule mismatch: %#v", got)
	}
}
