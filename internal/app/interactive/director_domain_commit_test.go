package interactiveapp

import (
	"context"
	"strings"
	"sync"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/book/lore"
	"denova/internal/interactive"
	"denova/internal/interactive/director"
)

func TestInteractiveDirectorCustomCycleIdentityIsFixedLength(t *testing.T) {
	token := interactive.DirectorPlanRunToken{
		StoryID: strings.Repeat("story", 1000), BranchID: strings.Repeat("branch", 1000),
		Revision: strings.Repeat("revision", 1000), Hashes: map[string]string{"plan": strings.Repeat("hash", 1000)},
	}
	first, err := interactiveDirectorCustomCycleIdentity(token, strings.Repeat("turn", 1000))
	if err != nil {
		t.Fatal(err)
	}
	second, err := interactiveDirectorCustomCycleIdentity(token, strings.Repeat("turn", 1000))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first.CommandID) > 128 || len(first.OperationID) > 128 {
		t.Fatalf("custom Director identity is unstable or unbounded: first=%#v second=%#v", first, second)
	}
}

func TestInteractiveDirectorPlanCommitPublishesOnlyAfterOutputAuthorization(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "导演提交屏障"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "继续", Narrative: "故事继续。"})
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
	draft := interactive.NewDirectorPlanUpdateDraft(plan.Docs, token)
	loreRevision, err := lore.NewStore(workspace).Revision()
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, draft, interactive.DirectorPlanUpdateSubmission{
		Decision: director.Decision{Mode: director.DecisionKeep, Reason: "当前计划仍然有效"},
		Finalize: true, SourceLoreRevision: loreRevision,
	}); err != nil || !receipt.Finalized {
		t.Fatalf("finalize draft: receipt=%#v err=%v", receipt, err)
	}

	var draftMu sync.Mutex
	participant := newInteractiveDirectorPlanCommit(store, story.ID, "main", turn.ID, token, draft, &draftMu)
	identity := agentrun.CycleIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1}
	participant.BindAgentCycleIdentity(identity)
	participant.agentOutputHash = "agent-output-hash-1"
	intent, pending, err := participant.PendingAgentCycleCommit(agentrun.DomainCommitOutput)
	if err != nil || !pending || intent.Identity != identity || intent.Hash == "" {
		t.Fatalf("pending output intent = %#v pending=%v err=%v", intent, pending, err)
	}
	before, err := store.DirectorPlanStatus(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != director.PlanStatusRunning {
		t.Fatalf("plan escaped before output commit authorization: %#v", before)
	}

	if err := participant.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		t.Fatal(err)
	}
	receipt, ok := participant.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput)
	if !ok || receipt.Identity != identity || receipt.Hash != intent.Hash || receipt.Revision == "" {
		t.Fatalf("canonical output receipt = %#v ok=%v", receipt, ok)
	}
	after, err := store.DirectorPlanStatus(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != director.PlanStatusReady || after.Decision == nil || after.Decision.Reason != "当前计划仍然有效" {
		t.Fatalf("authorized Director plan was not published: %#v", after)
	}
	canonical, summary, found, err := interactiveDirectorCanonicalResult(store, story.ID, "main", string(identity.CommandID))
	if err != nil || !found || canonical.Metadata.LastRun == nil || summary != "当前计划仍然有效" {
		t.Fatalf("canonical Director replay result = plan=%#v summary=%q found=%v err=%v", canonical, summary, found, err)
	}
}

func TestInteractiveDirectorPlanCommitCancellationDiscardsStagedOutput(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "导演取消提交"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "继续", Narrative: "故事继续。"})
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
	draft := interactive.NewDirectorPlanUpdateDraft(plan.Docs, token)
	loreRevision, err := lore.NewStore(workspace).Revision()
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, draft, interactive.DirectorPlanUpdateSubmission{
		Decision: director.Decision{Mode: director.DecisionKeep, Reason: "不应提交"},
		Finalize: true, SourceLoreRevision: loreRevision,
	}); err != nil || !receipt.Finalized {
		t.Fatalf("finalize draft: receipt=%#v err=%v", receipt, err)
	}

	participant := newInteractiveDirectorPlanCommit(store, story.ID, "main", turn.ID, token, draft, &sync.Mutex{})
	participant.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "command-cancel", OperationID: "operation-cancel", Cycle: 1})
	participant.agentOutputHash = "agent-output-hash-cancel"
	if _, pending, err := participant.PendingAgentCycleCommit(agentrun.DomainCommitOutput); err != nil || !pending {
		t.Fatalf("prepare canceled output: pending=%v err=%v", pending, err)
	}
	if err := participant.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeAborted}); err != nil {
		t.Fatal(err)
	}
	if receipt, ok := participant.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput); ok {
		t.Fatalf("canceled output produced canonical receipt: %#v", receipt)
	}
	after, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Metadata.LastRun == nil || after.Metadata.LastRun.Status != director.PlanStatusRunning || after.Metadata.LastRun.DomainCommit != nil {
		t.Fatalf("canceled staged output escaped into canonical plan: %#v", after.Metadata.LastRun)
	}
}
