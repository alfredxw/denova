package app

import (
	"context"
	agentinteractive "denova/internal/agents/interactive"
	"errors"
	"testing"

	"denova/config"
	agentrun "denova/internal/agents/run"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestInteractiveConversationPublishesOnlyAuthorizedOutputStage(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "游戏提交屏障", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	aborted := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "先观察", 800, nil)
	aborted.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: agentrun.CommandID("command-abort"), OperationID: agentrun.OperationID("operation-abort"), Cycle: 1})
	materializeInteractiveInputForTest(t, store, story.ID, "main", "先观察", aborted.AgentCycleIdentitySnapshot())
	submitTestTurnResult(t, store, story.ID, "main", aborted, "观察", "确认环境")
	if err := aborted.AppendAssistant("尚未授权的叙事"); err != nil {
		t.Fatal(err)
	}
	if snapshot, err := store.Snapshot(story.ID, "main"); err != nil || len(snapshot.Turns) != 0 {
		t.Fatalf("staged output advanced story before authorization: turns=%+v err=%v", snapshot.Turns, err)
	}
	if _, ok, err := aborted.PendingAgentCycleCommit(agentrun.DomainCommitOutput); err != nil || !ok {
		t.Fatalf("pending game intent missing: ok=%t err=%v", ok, err)
	}
	if err := aborted.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeAborted}); err != nil {
		t.Fatal(err)
	}

	completed := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "再观察", 800, nil)
	completed.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: agentrun.CommandID("command-complete"), OperationID: agentrun.OperationID("operation-complete"), Cycle: 1})
	materializeInteractiveInputForTest(t, store, story.ID, "main", "再观察", completed.AgentCycleIdentitySnapshot())
	submitTestTurnResult(t, store, story.ID, "main", completed, "观察", "发现线索")
	if err := completed.AppendAssistant("授权后写入的叙事"); err != nil {
		t.Fatal(err)
	}
	if err := completed.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].Narrative != "授权后写入的叙事" {
		t.Fatalf("authorized game output was not canonical: %+v", snapshot.Turns)
	}
	if receipt, ok := completed.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput); !ok || receipt.Revision != snapshot.Turns[0].ID {
		t.Fatalf("game commit receipt = %+v ok=%t", receipt, ok)
	}
}

func TestInteractiveAgentCycleAcceptsMaintenanceOnlyCompletionWithoutTurn(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "maintenance checkpoint", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	conversation := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "pending input", 800, nil)
	cycle := &interactiveAgentCycle{store: store, storyID: story.ID, branchID: "main", conversation: conversation}
	cycle.bindCommit(func(agentrun.Event) {})

	if err := conversation.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{
		Status: agentrun.OutcomeCompleted, MaintenanceOnly: true,
	}); err != nil {
		t.Fatalf("maintenance-only completion was rejected: %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 0 {
		t.Fatalf("maintenance-only completion persisted a narrative turn: %#v", snapshot.Turns)
	}
	if err := conversation.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{
		Status: agentrun.OutcomeCompleted,
	}); err == nil {
		t.Fatal("ordinary completed cycle without a turn must still fail closed")
	}
}

func TestInteractiveConversationRejectsOutputWithoutDurableCycleIdentity(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "缺少运行身份", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	conversation := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "观察", 800, nil)
	submitTestTurnResult(t, store, story.ID, "main", conversation, "观察", "确认环境")
	if err := conversation.AppendAssistant("不应直接写入的叙事"); err == nil {
		t.Fatal("assistant output without a durable cycle identity must fail closed")
	}
	if snapshot, err := store.Snapshot(story.ID, "main"); err != nil || len(snapshot.Turns) != 0 {
		t.Fatalf("identity-less output advanced story: turns=%+v err=%v", snapshot.Turns, err)
	}
}

func TestInteractiveConversationPinsBranchHeadWhenQueuedCycleStarts(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "队列回合", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	initialContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	initialHead := initialContext.Meta.Branches["main"].Head
	queued := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "跟进动作", 800, nil).WithBaseParentID(initialHead).WithExecutionParentPinning()

	first := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "初始动作", 800, nil).WithBaseParentID(initialHead)
	submitTestTurnResult(t, store, story.ID, "main", first, "先观察", "完成初始动作")
	if err := commitInteractiveAssistantForTest(t, store, story.ID, "main", "初始动作", first, "第一回合完成。", ""); err != nil {
		t.Fatal(err)
	}
	firstTurn, _, ok := first.LastTurnForState()
	if !ok {
		t.Fatal("initial turn was not persisted")
	}

	if _, err := assembleAndCommitInteractiveContextForTest(queued, "跟进动作", "跟进动作"); err != nil {
		t.Fatalf("prepare queued cycle: %v", err)
	}
	submitTestTurnResult(t, store, story.ID, "main", queued, "继续观察", "完成跟进动作")
	if err := commitInteractiveAssistantForTest(t, store, story.ID, "main", "跟进动作", queued, "第二回合完成。", ""); err != nil {
		t.Fatalf("persist queued cycle against latest head: %v", err)
	}
	secondTurn, _, ok := queued.LastTurnForState()
	if !ok {
		t.Fatal("queued turn was not persisted")
	}
	if secondTurn.ParentID != firstTurn.ID {
		t.Fatalf("queued turn parent = %q, want preceding cycle %q", secondTurn.ParentID, firstTurn.ID)
	}
}

func TestInteractiveAgentCycleKeepsFailedDirectorProjectionPending(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "派生屏障", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "前进一步", Narrative: "事实已经提交。",
		AgentCommandID: "command-1", AgentOperationID: "operation-1", AgentCycle: 1,
	}
	playerInput, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: request.AgentCommandID, OperationID: request.AgentOperationID, Cycle: request.AgentCycle,
	}, request.BranchID, request.User)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, playerInput); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	storyContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	generated := 0
	generator := func(context.Context, *config.Config, *book.State, agentinteractive.InteractiveStoryToolContext, string) (string, error) {
		generated++
		return "", errors.New("simulated Director failure")
	}
	tasks := interactiveapp.NewDirectorTaskGroup()
	defer tasks.Close()
	cfg := config.Config{Workspace: workspace}
	conversation := interactiveapp.NewConversation(store, "", workspace, story.ID, "main", "下一步", story.ReplyTargetChars, &cfg).BindDirectorRuntime(tasks, generator)
	cycle := &interactiveAgentCycle{
		store: store, state: book.NewState(workspace), runtimeCfg: cfg,
		workspace: workspace, storyID: story.ID, branchID: "main",
		storyContext: storyContext, conversation: conversation,
	}

	if err := cycle.reconcilePreviousAgentCommit(context.Background()); err == nil {
		t.Fatal("failed Director projection unexpectedly acknowledged the canonical outbox item")
	}
	if generated != 1 {
		t.Fatalf("Director generator calls = %d, want one attempt", generated)
	}
	updated, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Snapshot.DirectorPlan != nil && updated.Snapshot.DirectorPlan.Metadata.DerivedThroughTurnID == turn.ID {
		t.Fatalf("failed projection persisted a false derived receipt: %#v", updated.Snapshot.DirectorPlan)
	}
}
