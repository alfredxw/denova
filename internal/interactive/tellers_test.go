package interactive

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/narrativestyle"
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
		if !teller.SupportsMode(narrativestyle.ModeWriting) || !teller.SupportsMode(narrativestyle.ModeGame) {
			t.Fatalf("built-in narrative style %s should support writing and game: %#v", id, teller.Modes)
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

func TestTellerModesNormalizeLegacyAndScopedStyles(t *testing.T) {
	library := NewTellerLibrary(t.TempDir())
	legacy, err := library.Create(Teller{ID: "legacy-shared", Name: "Legacy", Slots: []TellerPromptSlot{{ID: "system", Target: "system", Enabled: true, Content: "shared"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.SupportsMode(narrativestyle.ModeWriting) || !legacy.SupportsMode(narrativestyle.ModeGame) {
		t.Fatalf("legacy style should remain shared: %#v", legacy.Modes)
	}
	writingOnly, err := library.Create(Teller{ID: "writing-only", Name: "Writing", Modes: []string{narrativestyle.ModeWriting}, Slots: []TellerPromptSlot{{ID: "system", Target: "system", Enabled: true, Content: "writing"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !writingOnly.SupportsMode(narrativestyle.ModeWriting) || writingOnly.SupportsMode(narrativestyle.ModeGame) {
		t.Fatalf("writing-only style mode mismatch: %#v", writingOnly.Modes)
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
	if classic.Version != tellerVersion || classic.Name != builtinTellers["classic"].Name || !containsTellerSlot(classic, "turn_context") {
		t.Fatalf("classic builtin should be refreshed to current version: %#v", classic)
	}
}

func TestTellerLibraryOverridesAndRestoresBuiltinInUserSpace(t *testing.T) {
	novaDir := t.TempDir()
	library := NewTellerLibrary(novaDir)

	classic, err := library.Get("classic")
	if err != nil {
		t.Fatalf("Get classic failed: %v", err)
	}
	classic.Name = "我的经典叙事"
	classic.Slots[0].Content = "用户覆盖规则"

	overridden, err := library.Update("classic", classic, classic.UpdatedAt)
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
	library := NewTellerLibrary(t.TempDir())
	created, err := library.Create(Teller{
		ID:   "custom",
		Name: "旧叙事",
		Slots: []TellerPromptSlot{{
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
	agent, err := library.Update(created.ID, Teller{
		Name: "Agent 叙事",
		Slots: []TellerPromptSlot{{
			ID:      "identity",
			Name:    "系统提示",
			Target:  "system",
			Enabled: true,
			Content: "Agent 规则",
		}},
	}, created.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Update(created.ID, Teller{
		Name: "前端旧叙事",
		Slots: []TellerPromptSlot{{
			ID:      "identity",
			Name:    "系统提示",
			Target:  "system",
			Enabled: true,
			Content: "前端旧规则",
		}},
	}, created.UpdatedAt); !errors.Is(err, ErrTellerRevisionConflict) {
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
	teller := normalizeTeller(Teller{
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
	rules := normalizeStyleRules([]StyleRule{{Scene: "日常", StyleRefs: refs}})
	if len(rules) != 1 {
		t.Fatalf("rules = %#v", rules)
	}
	if len(rules[0].StyleRefs) != len(refs) {
		t.Fatalf("style refs = %d, want %d", len(rules[0].StyleRefs), len(refs))
	}
}

func TestDefaultWebnovelEventCardsUseDifferentiatedPresets(t *testing.T) {
	cards := defaultTellerEventCards()
	if len(cards) < 2 {
		t.Fatalf("default event package should include multiple cards: %#v", cards)
	}

	bodies := map[string]string{}
	for _, card := range cards {
		markdown := strings.TrimSpace(card.DescriptionMarkdown)
		if !strings.Contains(markdown, "## 背景融合方式") {
			t.Fatalf("event card should use structured markdown: %#v", card)
		}
		body := markdown[strings.Index(markdown, "## 背景融合方式"):]
		if previousID, ok := bodies[body]; ok {
			t.Fatalf("event cards %s and %s should not share the same markdown body:\n%s", previousID, card.ID, body)
		}
		bodies[body] = card.ID
	}
	if !strings.Contains(cards[0].DescriptionMarkdown, "公开轻视") || !strings.Contains(cards[1].DescriptionMarkdown, "长期隐藏实力") {
		t.Fatalf("default webnovel cards should carry event-specific preset details: %#v", cards[:2])
	}
}

func TestEventPackageCardsNormalizeAndBuildDirectorCatalog(t *testing.T) {
	longDescription := strings.Repeat("伏笔", MaxEventCardDescriptionChars+20)
	events := normalizeTellerEventCards([]TellerEventCard{
		{
			ID:                  "academy_trial",
			TypeName:            "外门考核打脸",
			DescriptionMarkdown: "## 触发场景\n主角在外门考核被执事和同门轻视。\n\n## 背景融合方式\n绑定外门名额、执事偏见和残卷线索。",
			Enabled:             true,
			Category:            "学院",
			Tags:                []string{"外门", "考核", "外门"},
			Intensity:           "high",
		},
		{
			ID:                  "academy_trial",
			TypeName:            "重复事件",
			DescriptionMarkdown: "应被去重",
			Enabled:             true,
		},
		{
			ID:                  "disabled_card",
			TypeName:            "停用事件",
			DescriptionMarkdown: "## 触发场景\n暂不启用。",
			Enabled:             false,
		},
		{
			ID:                  "long_card",
			TypeName:            "长事件",
			DescriptionMarkdown: longDescription,
			Enabled:             true,
		},
	}, "academy-pack")
	if len(events) != 3 {
		t.Fatalf("event cards should be normalized and deduped: %#v", events)
	}
	if got := len([]rune(events[2].DescriptionMarkdown)); got != len([]rune(longDescription)) {
		t.Fatalf("event card normalization silently changed description: got=%d want=%d", got, len([]rune(longDescription)))
	}
	if len(events[0].Tags) != 2 {
		t.Fatalf("event card tags should be deduped: %#v", events[0].Tags)
	}

	catalog := DirectorEventCatalogFromStoryDirector(StoryDirector{
		ID:            "event-cards",
		ModuleRefs:    StoryDirectorModuleRefs{EventPackageIDs: []string{"academy-pack"}},
		EventPackages: []TellerEventPackage{{ID: "academy-pack", Name: "学院包", Enabled: true, Events: events}},
	})
	if !directorEventQueued(catalog, "academy-pack/academy_trial") || !directorEventQueued(catalog, "academy-pack/long_card") || directorEventQueued(catalog, "academy-pack/disabled_card") {
		t.Fatalf("director catalog should contain enabled event cards only: %#v", catalog)
	}
	event := directorEventByID(catalog, "academy-pack/academy_trial")
	if event.Name != "外门考核打脸" || event.Category != "学院" || event.Template == "" || event.Intensity != "high" {
		t.Fatalf("event card should map to director event: %#v", event)
	}
	if !strings.Contains(event.Summary, "主角在外门考核") || !strings.Contains(event.Template, "背景融合方式") {
		t.Fatalf("event card markdown should produce summary and template: %#v", event)
	}
}

func TestDirectorEventCatalogIncludesEventCardMarkdown(t *testing.T) {
	director := normalizeStoryDirector(StoryDirector{
		ID:         "catalog-card",
		ModuleRefs: StoryDirectorModuleRefs{EventPackageIDs: []string{"conflict-pack"}},
		EventPackages: []TellerEventPackage{{
			ID:      "conflict-pack",
			Enabled: true,
			Events: []TellerEventCard{{
				ID:                  "faction_conflict",
				TypeName:            "宗门冲突",
				DescriptionMarkdown: "## 触发场景\n宗门长老逼迫主角交出线索。\n\n## 事件回收 / 后果\n后续以宗门戒律和人情债回收。",
				Enabled:             true,
				Category:            "冲突",
			}},
		}},
	})
	catalog := DirectorEventCatalogFromStoryDirector(director)
	card := directorEventByID(catalog, "conflict-pack/faction_conflict")
	if card.Template == "" || !strings.Contains(card.Template, "宗门长老") || card.Category != "冲突" {
		t.Fatalf("catalog should include event card markdown: %#v", card)
	}
}

func directorEventQueued(events []DirectorEvent, id string) bool {
	return directorEventByID(events, id).ID != ""
}

func directorEventByID(events []DirectorEvent, id string) DirectorEvent {
	for _, event := range events {
		if event.ID == id {
			return event
		}
	}
	return DirectorEvent{}
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
