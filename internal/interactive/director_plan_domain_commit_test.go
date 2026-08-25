package interactive

import (
	"denova/internal/book/lore"
	"denova/internal/interactive/director"
	"errors"
	"testing"
)

type directorPatchTestRun struct {
	store        *Store
	story        StorySummary
	turn         TurnEvent
	token        DirectorPlanRunToken
	plan         DirectorPlan
	loreRevision string
	draft        *DirectorPlanUpdateDraft
}

func startDirectorPatchTestRun(t *testing.T, workspace, title string) directorPatchTestRun {
	t.Helper()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "前进", Narrative: "抵达门前。"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, turn.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	loreRevision, err := lore.NewStore(workspace).Revision()
	if err != nil {
		t.Fatal(err)
	}
	return directorPatchTestRun{
		store: store, story: story, turn: turn, token: token, plan: plan, loreRevision: loreRevision,
		draft: NewDirectorPlanUpdateDraft(plan.Docs, token),
	}
}

func TestCommitDirectorPlanRunReplaysCanonicalReceiptAfterStoreRestart(t *testing.T) {
	workspace := t.TempDir()
	run := startDirectorPatchTestRun(t, workspace, "导演提交重放")
	intent, err := NewDirectorPlanDomainCommitIntent(
		DirectorPlanDomainCommitIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1},
		"agent-output-hash-1",
		run.token,
		run.turn.ID,
		`{"mode":"keep","reason":"当前规划仍然有效"}`,
		run.plan.Docs,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := run.store.CommitDirectorPlanRun(intent)
	if err != nil {
		t.Fatal(err)
	}

	// Re-open the store to simulate a retry after the process lost its
	// in-memory participant receipt.
	replayed, err := NewStore(workspace).CommitDirectorPlanRun(intent)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Identity != first.Identity || replayed.Hash != first.Hash || replayed.Revision != first.Revision {
		t.Fatalf("replayed receipt = %#v, want %#v", replayed, first)
	}
	if replayed.Plan.Metadata.LastRun == nil || replayed.Plan.Metadata.LastRun.DomainCommit == nil || replayed.Plan.Metadata.LastRun.DomainCommit.Hash != intent.Hash {
		t.Fatalf("canonical Director metadata is missing its durable receipt: %#v", replayed.Plan.Metadata.LastRun)
	}
	if err := NewStore(workspace).MarkDirectorPlanRunFailed(run.story.ID, "main", run.turn.ID, errors.New("late runtime error")); err != nil {
		t.Fatal(err)
	}
	afterLateFailure, err := NewStore(workspace).DirectorPlan(run.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if afterLateFailure.Metadata.LastRun == nil || afterLateFailure.Metadata.LastRun.Status != director.PlanStatusReady || afterLateFailure.Metadata.LastRun.DomainCommit == nil {
		t.Fatalf("late runtime failure replaced an authorized canonical commit: %#v", afterLateFailure.Metadata.LastRun)
	}

	conflict, err := NewDirectorPlanDomainCommitIntent(
		intent.Identity,
		"agent-output-hash-1",
		run.token,
		run.turn.ID,
		`{"mode":"keep","reason":"同一命令却换了语义"}`,
		run.plan.Docs,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(workspace).CommitDirectorPlanRun(conflict); !errors.Is(err, ErrDirectorPlanDomainCommitConflict) {
		t.Fatalf("semantic identity reuse error = %v, want %v", err, ErrDirectorPlanDomainCommitConflict)
	}
}
