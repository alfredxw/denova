package app

import (
	"testing"

	"denova/config"
	"denova/internal/interactive"
)

func TestAppendInteractiveMemoryGoesThroughStoreAndAligns(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "手工注入", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "开始",
		Narrative: "一切开始。",
	})
	if err != nil {
		t.Fatal(err)
	}

	app := &App{cfg: &config.Config{}, interactive: store}

	event, err := app.AppendInteractiveMemory(story.ID, "main", interactive.NarrativeMemoryEvent{
		SourceTurnID: turn.ID,
		Records: []interactive.NarrativeMemoryRecord{
			{
				ID:        "lore_1",
				Kind:      interactive.MemoryKindBeat,
				Subject:   "新 人", // 故意带空格,验证 service 同样走写入路径的对齐层
				Text:      "新人登场。",
				Evidence:  "登场",
				ValidFrom: turn.ID,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := event.Records[0].Subject; got != "新 人" {
		t.Fatalf("first injection has no roster yet, spelling must pass through: got %q", got)
	}

	// 第二次注入同样写成"新人",名册里现在有"新 人",必须被对齐到它。
	event, err = app.AppendInteractiveMemory(story.ID, "main", interactive.NarrativeMemoryEvent{
		SourceTurnID: turn.ID,
		Records: []interactive.NarrativeMemoryRecord{
			{ID: "lore_2", Kind: interactive.MemoryKindBeat, Subject: "新人", Text: "新人开口。", Evidence: "开口", ValidFrom: turn.ID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := event.Records[0].Subject; got != "新 人" {
		t.Fatalf("manual injection should be aligned to the roster, got %q", got)
	}
	if event.Trace == nil || len(event.Trace.AlignedEntities) != 1 {
		t.Fatalf("alignment must be traced even on manual injection: %#v", event.Trace)
	}
}

func TestAppendInteractiveMemoryRequiresStore(t *testing.T) {
	// 没有 store 时 service 必须显式拒绝,而不是把 nil 静默传给 store。
	app := &App{cfg: &config.Config{}}
	if _, err := app.AppendInteractiveMemory("story", "main", interactive.NarrativeMemoryEvent{}); err == nil {
		t.Fatal("app without workspace should refuse manual injection")
	}
}

func TestAppendInteractiveMemoryRejectsBadSource(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "bad source", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{cfg: &config.Config{}, interactive: store}
	_, err = app.AppendInteractiveMemory(story.ID, "main", interactive.NarrativeMemoryEvent{
		SourceTurnID: "不存在的回合",
		Records: []interactive.NarrativeMemoryRecord{
			{ID: "r", Kind: interactive.MemoryKindBeat, Subject: "甲", Text: "x", Evidence: "y"},
		},
	})
	if err == nil {
		t.Fatal("invalid source_turn_id must be rejected by the store, not silently accepted")
	}
}