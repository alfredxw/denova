package interactive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTellerLibraryMaterializesBuiltinsAndListsThem(t *testing.T) {
	novaDir := t.TempDir()
	library := NewTellerLibrary(novaDir)

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

	for _, id := range []string{"direct-erotica", "screenwriter"} {
		teller, err := library.Get(id)
		if err != nil {
			t.Fatalf("Get %s failed: %v", id, err)
		}
		if teller.ID != id || teller.Name == "" || teller.PromptForTargets("system") == "" || teller.PromptForTargets("turn_context") == "" || teller.PromptForTargets("state_memory") == "" {
			t.Fatalf("unexpected builtin teller %s: %#v", id, teller)
		}
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

	library := NewTellerLibrary(novaDir)
	classic, err := library.Get("classic")
	if err != nil {
		t.Fatalf("Get classic failed: %v", err)
	}
	if classic.Version != tellerVersion || classic.Name != builtinTellers["classic"].Name || !containsTellerSlot(classic, "turn_context") || !containsTellerSlot(classic, "state_memory") {
		t.Fatalf("classic builtin should be refreshed to current version: %#v", classic)
	}
}

func TestNormalizeStyleRulesStoresContentsOnly(t *testing.T) {
	longContent := strings.Repeat("风", MaxStyleContentChars+20)
	rules := normalizeStyleRules([]StyleRule{
		{Scene: " 激烈打斗 ", StyleContents: []string{" 短句留白 ", "短句留白", longContent}},
		{Scene: "", StyleContents: []string{"无效"}},
		{Scene: "空内容", StyleContents: []string{"", " "}},
	})

	if len(rules) != 1 {
		t.Fatalf("style rules = %#v, want one valid rule", rules)
	}
	rule := rules[0]
	if rule.Scene != "激烈打斗" {
		t.Fatalf("scene = %q", rule.Scene)
	}
	if len(rule.StyleContents) != 2 {
		t.Fatalf("style contents = %#v, want deduped contents", rule.StyleContents)
	}
	if rule.StyleContents[0] != "短句留白" {
		t.Fatalf("first content = %q", rule.StyleContents[0])
	}
	if got := len([]rune(rule.StyleContents[1])); got != MaxStyleContentChars {
		t.Fatalf("long content chars = %d, want %d", got, MaxStyleContentChars)
	}
}

func TestTellerImagePromptNormalizesAndRoundTrips(t *testing.T) {
	novaDir := t.TempDir()
	library := NewTellerLibrary(novaDir)
	longPrompt := "  " + strings.Repeat("图", MaxImagePromptChars+20) + "  "
	created, err := library.Create(Teller{
		ID:              "visual",
		Name:            "视觉叙事",
		Description:     "带互动图像提示",
		ImagePrompt:     longPrompt,
		RandomEventRate: 0.1,
		Tags:            []string{"自定义"},
		ContextPolicy:   TellerContextPolicy{Creator: "always", Lore: "relevant", RuntimeState: "always"},
		Slots:           []TellerPromptSlot{{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "规则"}},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if got := len([]rune(created.ImagePrompt)); got != MaxImagePromptChars {
		t.Fatalf("created image_prompt chars = %d, want %d", got, MaxImagePromptChars)
	}
	loaded, err := library.Get("visual")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if loaded.ImagePrompt != created.ImagePrompt {
		t.Fatalf("image_prompt should round trip, got %q want %q", loaded.ImagePrompt, created.ImagePrompt)
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

	library := NewTellerLibrary(novaDir)
	teller, err := library.Get("custom")
	if err != nil {
		t.Fatalf("Get custom failed: %v", err)
	}
	if len(teller.StyleRules) != 0 {
		t.Fatalf("legacy styles field should be ignored: %#v", teller.StyleRules)
	}
}

func containsTellerSlot(teller Teller, target string) bool {
	for _, slot := range teller.Slots {
		if slot.Enabled && slot.Target == target && slot.Content != "" {
			return true
		}
	}
	return false
}
