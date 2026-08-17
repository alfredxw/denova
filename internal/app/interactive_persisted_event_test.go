package app

import (
	"context"
	"encoding/json"
	"testing"

	agentrun "denova/internal/agents/run"
	interactivestate "denova/internal/interactive/state"

	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive"
	"denova/internal/interactive/director"

	agent "github.com/alfredxw/denova/agent"
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
	conversation := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "继续前进", 800, nil)
	submitTestTurnResult(t, store, story.ID, "main", conversation, "走出门外", "确认雾中环境")
	if err := commitInteractiveAssistantForTest(t, store, story.ID, "main", "继续前进", conversation, "雾气在门外散开。", "先确认场景。"); err != nil {
		t.Fatal(err)
	}
	turn, _, ok := conversation.LastTurnForState()
	if !ok {
		t.Fatal("expected last turn")
	}
	if _, err := store.AppendStateDelta(story.ID, interactive.AppendStateDeltaRequest{
		ParentID: turn.ID,
		BranchID: turn.BranchID,
		Ops: []interactivestate.Op{
			{Op: "merge", Path: "scene", Value: map[string]any{"location": "旧门外"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	projection, err := conversation.PrepareAgentCompaction(context.Background(), agent.CompactionCompactRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := conversation.BindAgentCompaction(&agent.CompactionState{
		ID: "agent-checkpoint", Revision: 2, Summary: "bounded current story", ContextData: projection.ContextData,
	}); err != nil {
		t.Fatal(err)
	}

	var events []agentrun.Event
	emitInteractiveTurnPersisted(store, story.ID, conversation, func(event agentrun.Event) {
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
	if payload.TurnCount != 1 {
		t.Fatalf("payload turn count = %d, want 1", payload.TurnCount)
	}
	if payload.Turn.User != "继续前进" || payload.Turn.Narrative != "雾气在门外散开。" || payload.Turn.Thinking != "先确认场景。" {
		t.Fatalf("payload turn mismatch: %#v", payload.Turn)
	}
	if payload.DirectorPlanStatus == nil || payload.DirectorPlanStatus.Status == "" {
		t.Fatalf("payload director status should come from current snapshot: %#v", payload.DirectorPlanStatus)
	}
	if payload.DirectorPlanStatus.Status != director.PlanStatusWaitingOpening {
		t.Fatalf("payload director status mismatch: %#v", payload.DirectorPlanStatus)
	}
	if payload.ContextCompaction == nil || payload.ContextCompaction.ID != "agent-checkpoint" ||
		payload.ContextCompaction.Summary != "bounded current story" {
		t.Fatalf("payload Compaction must come from Agent Session projection: %#v", payload.ContextCompaction)
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
	conversation := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "继续前进", 800, nil)

	var events []agentrun.Event
	emitInteractiveTurnPersisted(store, story.ID, conversation, func(event agentrun.Event) {
		events = append(events, event)
	})

	if len(events) != 0 {
		t.Fatalf("event count = %d, want 0", len(events))
	}
}
