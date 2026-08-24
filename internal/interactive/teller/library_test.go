package teller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTellerLibraryMaterializesBuiltinsAndListsThem(t *testing.T) {
	novaDir := t.TempDir()
	library := NewLibrary(novaDir)

	tellers, err := library.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tellers) != len(builtinTellers) {
		t.Fatalf("expected built-in tellers, got %#v", tellers)
	}
	if tellers[0].ID == "" || tellers[0].Name == "" {
		t.Fatalf("teller metadata should be parsed: %#v", tellers[0])
	}
	if tellers[0].ID != "rhythm" {
		t.Fatalf("default narrative style should be listed first, got %q", tellers[0].ID)
	}

	classicPath := filepath.Join(novaDir, "story-tellers", "classic.json")
	data, err := os.ReadFile(classicPath)
	if err != nil {
		t.Fatalf("classic teller should be materialized: %v", err)
	}
	assertContains(t, string(data), `"id": "classic"`)

	classic, err := library.Get("classic")
	if err != nil {
		t.Fatalf("Get classic failed: %v", err)
	}
	if classic.ID != "classic" || len(classic.Slots) == 0 || classic.PromptForTargets("system") == "" {
		t.Fatalf("unexpected classic teller: %#v", classic)
	}

	wantNames := map[string]string{
		"rhythm":         "节奏叙事",
		"classic":        "稳健叙事",
		"screenwriter":   "编剧风格",
		"grimdark":       "暗黑压抑",
		"direct-erotica": "直白情色",
	}
	if len(tellers) != len(wantNames) {
		t.Fatalf("built-in narrative style count = %d, want %d", len(tellers), len(wantNames))
	}
	for id, name := range wantNames {
		teller, err := library.Get(id)
		if err != nil {
			t.Fatalf("Get %s failed: %v", id, err)
		}
		if teller.ID != id || teller.Name != name || teller.PromptForTargets("system") == "" || teller.PromptForTargets("turn_context") == "" {
			t.Fatalf("unexpected builtin teller %s: %#v", id, teller)
		}
	}
}

func TestBuiltInNarrativeStylePromptContracts(t *testing.T) {
	if !strings.Contains(screenwriterSystemPrompt, "直接交付标准剧本") || !strings.Contains(screenwriterTurnContext, "不为了影视效果另造线索") {
		t.Fatalf("screenwriter prompt must request standard screenplay form without changing the plot")
	}
	if strings.Contains(screenwriterSystemPrompt, "人物不总是直说真实意图") {
		t.Fatalf("screenwriter prompt must not force hidden intent")
	}
	if !strings.Contains(rhythmSystemPrompt, "铺垫的兑现") || !strings.Contains(steadySystemPrompt, "从容不等于平淡") || !strings.Contains(bleakSystemPrompt, "压力不夺走人物的能动性") {
		t.Fatalf("narrative style intent is incomplete")
	}
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(directEroticaSystemPrompt+"\x00"+directEroticaTurnContext)))
	const unchangedDirectEroticaHash = "30f2cd3843fb3adebe1c77920051a86c4b7449a723050175dd47f5c6de083f64"
	if got != unchangedDirectEroticaHash {
		t.Fatalf("direct-erotica prompt changed: hash=%s", got)
	}
	directErotica := builtinTellers["direct-erotica"]
	if directErotica.Name != "直白情色" || directErotica.Description != "以事件驱动故事，自然导向情色场景，文风直白粗俗" {
		t.Fatalf("direct-erotica metadata changed: name=%q description=%q", directErotica.Name, directErotica.Description)
	}
}

func TestTellerLibraryRefreshesOldBuiltinVersion(t *testing.T) {
	novaDir := t.TempDir()
	tellerDir := filepath.Join(novaDir, "story-tellers")
	if err := os.MkdirAll(tellerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldClassic := `{
  "version": 2,
  "id": "classic",
  "name": "旧导演",
  "description": "旧版本",
  "random_event_rate": 0.15,
  "tags": ["旧"],
  "context_policy": {
    "creator": "always",
    "lore": "relevant",
    "runtime_state": "always"
  },
  "slots": [
    {
      "id": "identity",
      "name": "系统提示",
      "target": "system",
      "enabled": true,
      "content": "旧规则"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(tellerDir, "classic.json"), []byte(oldClassic), 0o644); err != nil {
		t.Fatal(err)
	}

	library := NewLibrary(novaDir)
	classic, err := library.Get("classic")
	if err != nil {
		t.Fatalf("Get classic failed: %v", err)
	}
	if classic.Version != tellerVersion || classic.Name != builtinTellers["classic"].Name || !containsTellerSlot(classic, "turn_context") {
		t.Fatalf("classic builtin should be refreshed to current version: %#v", classic)
	}
}

func TestTellerLibraryRefreshesVersionNineEnglishBuiltin(t *testing.T) {
	novaDir := t.TempDir()
	tellerDir := filepath.Join(novaDir, "story-tellers")
	if err := os.MkdirAll(tellerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	englishClassic := builtinTellers["classic"]
	englishClassic.Version = 9
	englishClassic.Slots = append([]PromptSlot(nil), englishClassic.Slots...)
	for i := range englishClassic.Slots {
		switch englishClassic.Slots[i].Target {
		case "system":
			englishClassic.Slots[i].Content = "Use steady narration that values credibility, continuity, and cumulative worth in a long story."
		case "turn_context":
			englishClassic.Slots[i].Content = "Continue from established facts, character state, and unfinished material."
		}
	}
	data, err := json.Marshal(englishClassic)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tellerDir, "classic.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	classic, err := NewLibrary(novaDir).Get("classic")
	if err != nil {
		t.Fatalf("Get classic failed: %v", err)
	}
	if classic.Version != tellerVersion || !strings.Contains(classic.PromptForTargets("system"), "你采用稳健叙事") || strings.Contains(classic.PromptForTargets("system"), "Use steady narration") {
		t.Fatalf("version 9 English builtin should be refreshed to the Chinese default: %#v", classic)
	}
}

func TestTellerLibraryOverridesAndRestoresBuiltinInUserSpace(t *testing.T) {
	novaDir := t.TempDir()
	library := NewLibrary(novaDir)

	classic, err := library.Get("classic")
	if err != nil {
		t.Fatalf("Get classic failed: %v", err)
	}
	classic.Name = "我的经典叙事"
	classic.Slots[0].Content = "用户覆盖规则"

	overridden, err := library.Update("classic", classic, classic.Revision)
	if err != nil {
		t.Fatalf("Update builtin teller should create user override: %v", err)
	}
	if overridden.ID != "classic" || overridden.Custom || !overridden.BuiltinOverridden {
		t.Fatalf("builtin override ownership mismatch: %#v", overridden)
	}
	if overridden.Name != "我的经典叙事" || overridden.Slots[0].Content != "用户覆盖规则" {
		t.Fatalf("builtin override should keep edited content: %#v", overridden)
	}

	listed, err := library.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	found := false
	for _, teller := range listed {
		if teller.ID == "classic" {
			found = true
			if teller.Custom || !teller.BuiltinOverridden || teller.Name != "我的经典叙事" {
				t.Fatalf("list should expose builtin override state: %#v", teller)
			}
		}
	}
	if !found {
		t.Fatalf("classic teller missing from list: %#v", listed)
	}

	path := filepath.Join(novaDir, "story-tellers", "classic.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read overridden classic: %v", err)
	}
	assertContains(t, string(data), `"builtin_overridden": true`)

	if err := library.Delete("classic"); err != nil {
		t.Fatalf("Delete builtin override should restore builtin: %v", err)
	}
	restored, err := library.Get("classic")
	if err != nil {
		t.Fatalf("Get restored classic failed: %v", err)
	}
	if restored.Custom || restored.BuiltinOverridden || restored.Name != builtinTellers["classic"].Name || restored.Slots[0].Content != builtinTellers["classic"].Slots[0].Content {
		t.Fatalf("classic should be restored to builtin: %#v", restored)
	}
}

func TestTellerLibraryUpdateRejectsStaleRevision(t *testing.T) {
	library := NewLibrary(t.TempDir())
	created, err := library.Create(Definition{
		ID:   "custom",
		Name: "旧叙事",
		Slots: []PromptSlot{{
			ID:      "identity",
			Name:    "系统提示",
			Target:  "system",
			Enabled: true,
			Content: "旧规则",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := library.Update(created.ID, Definition{
		Name: "Agent 叙事",
		Slots: []PromptSlot{{
			ID:      "identity",
			Name:    "系统提示",
			Target:  "system",
			Enabled: true,
			Content: "Agent 规则",
		}},
	}, created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Update(created.ID, Definition{
		Name: "前端旧叙事",
		Slots: []PromptSlot{{
			ID:      "identity",
			Name:    "系统提示",
			Target:  "system",
			Enabled: true,
			Content: "前端旧规则",
		}},
	}, created.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected teller revision conflict, got %v", err)
	}
	got, err := library.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != agent.Name {
		t.Fatalf("stale save should not overwrite Agent teller: %#v", got)
	}
}

func TestNormalizeStyleRulesStoresRefsAndLegacyContents(t *testing.T) {
	longContent := strings.Repeat("风", 8020)
	teller := Normalize(Definition{
		StyleRefs: []string{" default.md ", ".denova/styles/default.md", "../bad.md"},
		StyleRules: []StyleRule{
			{Scene: " 激烈打斗 ", StyleRefs: []string{" style.md ", ".denova/styles/style.md", "../bad.md"}, StyleContents: []string{" 短句留白 ", "短句留白", longContent}},
			{Scene: "", StyleContents: []string{"无效"}},
			{Scene: "空内容", StyleContents: []string{"", " "}},
		},
	})
	rules := teller.StyleRules

	if len(teller.StyleRefs) != 2 || teller.StyleRefs[0] != ".denova/styles/default.md" || teller.StyleRefs[1] != ".denova/styles/bad.md" {
		t.Fatalf("global style refs = %#v, want normalized deduped refs", teller.StyleRefs)
	}

	if len(rules) != 1 {
		t.Fatalf("style rules = %#v, want one valid rule", rules)
	}
	rule := rules[0]
	if rule.Scene != "激烈打斗" {
		t.Fatalf("scene = %q", rule.Scene)
	}
	if len(rule.StyleRefs) != 2 || rule.StyleRefs[0] != ".denova/styles/style.md" || rule.StyleRefs[1] != ".denova/styles/bad.md" {
		t.Fatalf("style refs = %#v, want normalized deduped refs", rule.StyleRefs)
	}
	if len(rule.StyleContents) != 2 {
		t.Fatalf("style contents = %#v, want deduped contents", rule.StyleContents)
	}
	if rule.StyleContents[0] != "短句留白" {
		t.Fatalf("first content = %q", rule.StyleContents[0])
	}
	if got := len([]rune(rule.StyleContents[1])); got != len([]rune(longContent)) {
		t.Fatalf("long content chars = %d, want %d", got, len([]rune(longContent)))
	}
}

func TestNormalizeStyleRulesKeepsAllRefsPerRule(t *testing.T) {
	refs := make([]string, 0, 27)
	for i := 0; i < 27; i++ {
		refs = append(refs, fmt.Sprintf("style-%02d.md", i))
	}
	rules := NormalizeStyleRules([]StyleRule{{Scene: "日常", StyleRefs: refs}})
	if len(rules) != 1 {
		t.Fatalf("rules = %#v", rules)
	}
	if len(rules[0].StyleRefs) != len(refs) {
		t.Fatalf("style refs = %d, want %d", len(rules[0].StyleRefs), len(refs))
	}
}

func TestTellerLibraryIgnoresLegacyStylePathField(t *testing.T) {
	novaDir := t.TempDir()
	tellerDir := filepath.Join(novaDir, "story-tellers")
	if err := os.MkdirAll(tellerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "version": 4,
  "id": "custom",
  "name": "旧风格",
  "description": "旧路径字段",
  "random_event_rate": 0.1,
  "style_rules": [{"scene": "战斗", "styles": ["古龙.md"]}],
  "tags": [],
  "context_policy": {"creator": "always", "lore": "relevant", "runtime_state": "always"},
  "slots": [{"id": "identity", "name": "系统提示", "target": "system", "enabled": true, "content": "规则"}]
}`
	if err := os.WriteFile(filepath.Join(tellerDir, "custom.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	library := NewLibrary(novaDir)
	teller, err := library.Get("custom")
	if err != nil {
		t.Fatalf("Get custom failed: %v", err)
	}
	if len(teller.StyleRules) != 0 {
		t.Fatalf("legacy styles field should be ignored: %#v", teller.StyleRules)
	}
}

func containsTellerSlot(teller Definition, target string) bool {
	for _, slot := range teller.Slots {
		if slot.Enabled && slot.Target == target && slot.Content != "" {
			return true
		}
	}
	return false
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}
