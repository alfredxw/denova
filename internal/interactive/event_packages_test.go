package interactive

import (
	"strings"
	"testing"
)

func TestDefaultWebnovelEventCardsUseDifferentiatedPresets(t *testing.T) {
	cards := defaultEventCards()
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
	events := normalizeEventCards([]EventCard{
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
		EventPackages: []EventPackage{{ID: "academy-pack", Name: "学院包", Enabled: true, Events: events}},
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
		EventPackages: []EventPackage{{
			ID:      "conflict-pack",
			Enabled: true,
			Events: []EventCard{{
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
