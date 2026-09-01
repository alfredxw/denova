package interactive

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateBranchPlanPersistsCreatorRevisionWithoutChangingTurnModules(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "Editable plan", PlanningMode: StoryPlanningModeEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	initialMarkdown := "## Long-term direction\n\nReach the sealed city.\n\n## Near-term beats\n\nFind a safe route."
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "Continue", Narrative: "The road divides beneath the rain.",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &initialMarkdown},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.BranchPlan == nil || before.BranchPlan.Revision == "" {
		t.Fatalf("initial branch plan is missing its revision: %#v", before.BranchPlan)
	}
	stateBefore := cloneStoryState(before.State)
	choicesBefore := append([]string(nil), before.CurrentTurn.TurnResult.Choices...)

	updatedMarkdown := "## Long-term direction\n\nReach the sealed city before winter.\n\n## Near-term beats\n\nMeet the ferryman, then choose a crossing."
	result, err := store.UpdateBranchPlan(story.ID, UpdateBranchPlanRequest{
		BranchID: "main", Markdown: updatedMarkdown, BaseRevision: before.BranchPlan.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BranchPlan.Markdown != updatedMarkdown || result.BranchPlan.Revision == before.BranchPlan.Revision {
		t.Fatalf("updated plan = %#v", result.BranchPlan)
	}
	if result.ContextRevision != before.ContextRevision+1 {
		t.Fatalf("context revision = %d, want %d", result.ContextRevision, before.ContextRevision+1)
	}

	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.BranchPlan == nil || *after.BranchPlan != result.BranchPlan {
		t.Fatalf("snapshot plan = %#v, want %#v", after.BranchPlan, result.BranchPlan)
	}
	if after.CurrentTurn == nil || after.CurrentTurn.ID != turn.ID || len(after.Turns) != len(before.Turns) {
		t.Fatalf("plan edit changed story turns: before=%d after=%d current=%#v", len(before.Turns), len(after.Turns), after.CurrentTurn)
	}
	if !reflect.DeepEqual(after.State, stateBefore) || !reflect.DeepEqual(after.CurrentTurn.TurnResult.Choices, choicesBefore) {
		t.Fatalf("plan edit changed state or choices: state=%#v choices=%#v", after.State, after.CurrentTurn.TurnResult.Choices)
	}
	if after.ContextRevision != result.ContextRevision {
		t.Fatalf("snapshot context revision = %d, want %d", after.ContextRevision, result.ContextRevision)
	}

	reloaded, err := NewStore(workspace).Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.BranchPlan == nil || *reloaded.BranchPlan != result.BranchPlan || reloaded.ContextRevision != result.ContextRevision {
		t.Fatalf("reloaded plan projection = %#v revision=%d", reloaded.BranchPlan, reloaded.ContextRevision)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "Alternate route"})
	if err != nil {
		t.Fatal(err)
	}
	branched, err := store.Snapshot(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branched.BranchPlan == nil || branched.BranchPlan.Markdown != updatedMarkdown || branched.BranchPlan.Revision != result.BranchPlan.Revision {
		t.Fatalf("branch did not inherit the current creator revision: %#v", branched.BranchPlan)
	}
}

func TestUpdateBranchPlanRejectsStaleAndUnstructuredDocuments(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Plan conflicts", PlanningMode: StoryPlanningModeEnabled})
	if err != nil {
		t.Fatal(err)
	}
	initialMarkdown := "## Direction\n\nOpen the gate."
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "Continue", Narrative: "The gate waits.",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &initialMarkdown},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	baseRevision := snapshot.BranchPlan.Revision
	first, err := store.UpdateBranchPlan(story.ID, UpdateBranchPlanRequest{
		BranchID: "main", Markdown: "## Direction\n\nOpen the gate through diplomacy.", BaseRevision: baseRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateBranchPlan(story.ID, UpdateBranchPlanRequest{
		BranchID: "main", Markdown: "## Direction\n\nForce the gate.", BaseRevision: baseRevision,
	}); !errors.Is(err, ErrBranchPlanRevisionConflict) {
		t.Fatalf("stale update error = %v, want %v", err, ErrBranchPlanRevisionConflict)
	}
	if _, err := store.UpdateBranchPlan(story.ID, UpdateBranchPlanRequest{
		BranchID: "main", Markdown: "A flat summary without modules.", BaseRevision: first.BranchPlan.Revision,
	}); err == nil || !strings.Contains(err.Error(), "H2") {
		t.Fatalf("unstructured update error = %v", err)
	}
	unchanged, err := store.UpdateBranchPlan(story.ID, UpdateBranchPlanRequest{
		BranchID: "main", Markdown: first.BranchPlan.Markdown, BaseRevision: first.BranchPlan.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.BranchPlan.Revision != first.BranchPlan.Revision || unchanged.ContextRevision != first.ContextRevision {
		t.Fatalf("no-op update changed revisions: before=%#v after=%#v", first, unchanged)
	}
}

func TestUpdateBranchPlanRevisionRemainsOnTheNextTurnPath(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Continue after plan edit", PlanningMode: StoryPlanningModeEnabled})
	if err != nil {
		t.Fatal(err)
	}
	initialMarkdown := "## Direction\n\nReach the observatory."
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "Continue", Narrative: "The observatory appears above the fog.",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &initialMarkdown},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	updatedMarkdown := "## Direction\n\nReach the observatory through the flooded archive."
	updated, err := store.UpdateBranchPlan(story.ID, UpdateBranchPlanRequest{
		BranchID: "main", Markdown: updatedMarkdown, BaseRevision: before.BranchPlan.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "Enter the archive", Narrative: "Water closes over the first stair.",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatalf("append after creator plan revision: %v", err)
	}
	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.BranchPlan == nil || after.BranchPlan.Markdown != updatedMarkdown || after.BranchPlan.Revision != updated.BranchPlan.Revision {
		t.Fatalf("next turn did not preserve creator plan revision: %#v", after.BranchPlan)
	}
}
