package interactive

import (
	"os"
	"strings"
	"testing"

	"denova/internal/agents/conversationjournal"
)

func TestTurnDraftRebuildRetainsPartialModulesRulesAndFullNarrative(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Draft recovery"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "draft-input", OperationID: "draft-run", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "Continue the journey")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	system, state := turnSubmissionTestState()
	validation := TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5, PlanningMode: StoryPlanningModeEnabled,
		CurrentPlan: &BranchPlan{Markdown: "## Direction\n\nOld direction.\n\n## Next\n\nOld next."}}
	prepared, receipt := PrepareTurnSubmission(validation, nil, DecodeInteractiveTurnSubmissionInput(`{"state_changes":[],"plan_update":{"mode":"replace_sections","sections":[{"heading":"Direction","markdown":"Accepted direction."},{"heading":"Next","markdown":"## Invalid heading\n\nRejected"}]}}`))
	if receipt.Ready || receipt.ModuleStatus.StateChanges != TurnSubmissionModuleAccepted || receipt.ModuleStatus.PlanUpdate != TurnSubmissionModuleRejected {
		t.Fatalf("initial receipt=%#v", receipt)
	}
	draft := TurnDraft{Identity: identity, Narrative: strings.Repeat("Complete accepted prose. ", 1500), NarrativeSource: "complete_model_response",
		Submission: prepared.Progress(), RuleResolution: &RuleResolution{ID: "locked-roll"}}
	if err := store.SaveTurnDraft(story.ID, "main", draft, nil); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(store.storyPath(story.ID) + ".pre-resilience-v1.bak")
	if err != nil || strings.Contains(string(backup), `"type":"turn_draft"`) {
		t.Fatalf("pre-upgrade backup invalid: %v", err)
	}
	// Drop every derived locator; the canonical log alone must retain progress.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(conversationjournal.SidecarPath(store.storyPath(story.ID))); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	store = NewStore(workspace)
	restored, found, err := store.LoadTurnDraft(story.ID, "main", identity)
	if err != nil || !found || !sameCanonicalJSON(restored, draft) {
		t.Fatalf("restored draft identity=%#v submission=%#v full_narrative=%t found=%t error=%v", restored.Identity, restored.Submission, restored.Narrative == draft.Narrative, found, err)
	}
	final, receipt := PrepareTurnSubmission(validation, restored.Submission.Prepared(), DecodeInteractiveTurnSubmissionInput(`{"choices":["One","Two","Three","Four","Five"],"plan_update":{"mode":"replace_sections","sections":[{"heading":"Next","markdown":"Repaired next."}]}}`))
	if !receipt.Ready || final.TurnResult().PlanUpdate == nil || !strings.Contains(*final.TurnResult().PlanUpdate, "Accepted direction.") {
		t.Fatalf("partial plan was lost: %#v", receipt)
	}
	if restored.RuleResolution.ID != "locked-roll" || len(restored.Narrative) <= 16<<10 {
		t.Fatal("draft was reduced to feedback text")
	}
}

func TestFinalTurnEndsDraftInTheSameTransaction(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Draft commit"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "input", OperationID: "run", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "Continue")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTurnDraft(story.ID, "main", TurnDraft{Identity: identity, Narrative: "Accepted prose.", NarrativeSource: "complete_model_response"}, nil); err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{BranchID: "main", User: "Continue", Narrative: "Accepted prose.",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle}
	first, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil || first.ID != second.ID {
		t.Fatalf("duplicate output=%s error=%v", second.ID, err)
	}
	if _, found, err := store.LoadTurnDraft(story.ID, "main", identity); err != nil || found {
		t.Fatalf("committed draft remained active: found=%t error=%v", found, err)
	}
}
