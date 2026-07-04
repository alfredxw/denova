package app

import (
	"encoding/json"
	"testing"

	"denova/internal/agent"
	"denova/internal/interactive"
)

func TestEmitInteractiveTurnPersistedUsesCurrentSnapshot(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "收尾事件",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "继续前进", 800, nil)
	if err := conversation.AppendAssistantWithThinking("雾气在门外散开。", "先确认场景。"); err != nil {
		t.Fatal(err)
	}
	turn, _, ok := conversation.LastTurnForState()
	if !ok {
		t.Fatal("expected last turn")
	}
	if _, err := store.AppendStateDelta(story.ID, interactive.AppendStateDeltaRequest{
		ParentID: turn.ID,
		BranchID: turn.BranchID,
		Ops: []interactive.StateOp{
			{Op: "merge", Path: "scene", Value: map[string]any{"location": "旧门外"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var events []agent.Event
	emitInteractiveTurnPersisted(store, story.ID, conversation, func(event agent.Event) {
		events = append(events, event)
	})

	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].Type != "interactive_turn_persisted" {
		t.Fatalf("event type = %q, want interactive_turn_persisted", events[0].Type)
	}
	payload, ok := events[0].Data.(InteractiveTurnPersistedEvent)
	if !ok {
		t.Fatalf("event payload type = %T, want InteractiveTurnPersistedEvent", events[0].Data)
	}
	if payload.StoryID != story.ID || payload.BranchID != "main" {
		t.Fatalf("payload story/branch mismatch: %#v", payload)
	}
	if payload.Turn.User != "继续前进" || payload.Turn.Narrative != "雾气在门外散开。" || payload.Turn.Thinking != "先确认场景。" {
		t.Fatalf("payload turn mismatch: %#v", payload.Turn)
	}
	if payload.DirectorPlanStatus == nil || payload.DirectorPlanStatus.Status == "" {
		t.Fatalf("payload director status should come from current snapshot: %#v", payload.DirectorPlanStatus)
	}
	if payload.DirectorPlanStatus.Status != interactive.DirectorPlanStatusWaitingOpening {
		t.Fatalf("payload director status mismatch: %#v", payload.DirectorPlanStatus)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["director_plan"]; ok {
		t.Fatalf("persisted turn payload should not expose director plan docs: %s", string(encoded))
	}
	if _, ok := raw["visible_docs"]; ok {
		t.Fatalf("persisted turn payload should not expose director visible docs: %s", string(encoded))
	}
	scene := payload.State["scene"].(map[string]any)
	if scene["location"] != "旧门外" {
		t.Fatalf("payload state should come from current snapshot: %#v", payload.State)
	}
	if len(payload.Branches) != 1 || payload.Branches[0].ID != "main" || payload.Branches[0].Head != payload.Turn.ID {
		t.Fatalf("payload branches should come from current graph: %#v", payload.Branches)
	}
}

func TestEmitInteractiveTurnPersistedSkipsWhenNoTurnWasPersisted(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "无回合",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "继续前进", 800, nil)

	var events []agent.Event
	emitInteractiveTurnPersisted(store, story.ID, conversation, func(event agent.Event) {
		events = append(events, event)
	})

	if len(events) != 0 {
		t.Fatalf("event count = %d, want 0", len(events))
	}
}
