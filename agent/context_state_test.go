package agent

import (
	"strings"
	"testing"
)

func TestContextStateEmitsOnlyChangesAndRemoval(t *testing.T) {
	fragment := testContextStateFragment("revision-1", "workspace v1")
	raw := []*Message{UserMessage("earlier"), AssistantMessage("answer", nil)}

	first, snapshot, err := advanceContextState(raw, []ContextFragment{fragment}, contextStateSnapshot{}, CompactionState{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || !IsContextStateMessage(first[0]) || !strings.Contains(first[0].Content, "workspace v1") {
		t.Fatalf("initial Context State update = %#v", first)
	}
	if section := snapshot.Sections[fragment.StateID]; section.MessageIndex != len(raw) {
		t.Fatalf("initial Context State index = %d, want %d", section.MessageIndex, len(raw))
	}
	raw = append(raw, first...)

	unchanged, stable, err := advanceContextState(raw, []ContextFragment{fragment}, snapshot, CompactionState{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged) != 0 || stable.Generation != snapshot.Generation {
		t.Fatalf("unchanged Context State emitted a delta: messages=%#v state=%#v", unchanged, stable)
	}

	fragment.Revision = "revision-2"
	fragment.Content = "workspace v2"
	updated, snapshot, err := advanceContextState(raw, []ContextFragment{fragment}, stable, CompactionState{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || !strings.Contains(updated[0].Content, "workspace v2") {
		t.Fatalf("updated Context State = %#v", updated)
	}
	raw = append(raw, updated...)

	removed, snapshot, err := advanceContextState(raw, nil, snapshot, CompactionState{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || !strings.Contains(removed[0].Content, "Operation: remove") || !snapshot.Sections[fragment.StateID].Removed {
		t.Fatalf("Context State removal = messages=%#v state=%#v", removed, snapshot)
	}
	raw = append(raw, removed...)
	removalHidden := CompactionState{ReplacementFrom: 0, ReplacementTo: len(raw)}
	restoredRemoval, restoredSnapshot, err := advanceContextState(raw, nil, snapshot, removalHidden, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredRemoval) != 1 || !strings.Contains(restoredRemoval[0].Content, "Operation: remove") ||
		!restoredSnapshot.Sections[fragment.StateID].Removed {
		t.Fatalf("rehydrated Context State removal = messages=%#v state=%#v", restoredRemoval, restoredSnapshot)
	}
}

func TestContextStateRehydratesOnlyWhenLatestUpdateIsCompacted(t *testing.T) {
	fragment := testContextStateFragment("revision-1", "current workspace")
	initial, snapshot, err := advanceContextState(nil, []ContextFragment{fragment}, contextStateSnapshot{}, CompactionState{}, false)
	if err != nil {
		t.Fatal(err)
	}
	raw := append(initial, UserMessage("request"), AssistantMessage("answer", nil))

	hidden := CompactionState{ReplacementFrom: 0, ReplacementTo: 2}
	rehydrated, next, err := advanceContextState(raw, []ContextFragment{fragment}, snapshot, hidden, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rehydrated) != 1 || next.Sections[fragment.StateID].MessageIndex != len(raw) {
		t.Fatalf("rehydrated Context State = messages=%#v state=%#v", rehydrated, next)
	}
	raw = append(raw, rehydrated...)
	again, _, err := advanceContextState(raw, []ContextFragment{fragment}, next, hidden, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("visible Context State was duplicated: %#v", again)
	}
}

func TestContextStateValidationRejectsAmbiguousIdentity(t *testing.T) {
	fragment := testContextStateFragment("revision-1", "state")
	duplicate := fragment
	duplicate.Resource = "another-resource"
	if err := validateContextFragments([]ContextFragment{fragment, duplicate}); err == nil || !strings.Contains(err.Error(), "reuse StateID") {
		t.Fatalf("duplicate StateID error = %v", err)
	}
	fragment.StateID = ""
	if err := validateContextFragments([]ContextFragment{fragment}); err == nil || !strings.Contains(err.Error(), "requires state_message placement and StateID") {
		t.Fatalf("missing StateID error = %v", err)
	}
}

func testContextStateFragment(revision, content string) ContextFragment {
	return ContextFragment{
		Source: "test.workspace", Purpose: "provide current workspace state", Resource: "workspace",
		Revision: revision, StateID: "workspace", Stability: ContextSessionState, Placement: ContextStateMessage,
		Rendering: ContextRenderVerbatim, Content: content, HardLimit: 64 << 10,
	}
}
