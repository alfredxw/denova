package app

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	agentcontext "denova/internal/agents/context"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	agentsession "denova/internal/agents/session"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive"
	interactivestate "denova/internal/interactive/state"
)

var interactiveAgentTestCycle atomic.Uint64

func commitInteractiveAssistantForTest(t testing.TB, store *interactive.Store, storyID, branchID, user string, conversation *interactiveapp.Conversation, content, thinking string) error {
	t.Helper()
	cycle := interactiveAgentTestCycle.Add(1)
	identity := fmt.Sprintf("test-cycle:%d", cycle)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
		CommandID: agentrun.CommandID(identity), OperationID: agentrun.OperationID(identity), Cycle: 1,
	})
	materializeInteractiveInputForTest(t, store, storyID, branchID, user, conversation.AgentCycleIdentitySnapshot())
	if err := conversation.AppendAssistantWithThinking(content, thinking); err != nil {
		return err
	}
	return conversation.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted})
}

func materializeInteractiveInputForTest(t testing.TB, store *interactive.Store, storyID, branchID, user string, identity agentrun.CycleIdentity) {
	t.Helper()
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, storyContext.Snapshot.BranchID, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(storyID, intent); err != nil {
		t.Fatal(err)
	}
}

func submitTestTurnResult(t *testing.T, store *interactive.Store, storyID, branchID string, conversation *interactiveapp.Conversation, intent, goal string) {
	t.Helper()
	updates := []interactivestate.Update{}
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := storyContext.Snapshot.State["actors"].(map[string]any)
	_, hasStoryContext := actors[interactive.DefaultStoryContextActorID]
	actorState := conversation.StoryDirectorForMeta(storyContext.Meta).ActorState
	if storyContext.Meta.ActorStateSchema != nil {
		actorState = storyContext.Meta.ActorStateSchema.System
	}
	if !hasStoryContext {
		for _, actor := range actorState.InitialActors {
			if actor.ID == interactive.DefaultStoryContextActorID && actor.TemplateID == interactive.ActorStateStoryContextTemplateID {
				hasStoryContext = true
				break
			}
		}
	}
	if hasStoryContext {
		event := strings.TrimSpace(goal)
		if event == "" {
			event = strings.TrimSpace(intent)
		}
		updates = append(updates,
			interactivestate.Update{Op: "replace", Path: "/story/当前详细地点", Value: "测试场景"},
			interactivestate.Update{Op: "replace", Path: "/story/当前事件", Value: event},
		)
	}
	choices := []string{"继续当前行动", "观察周围变化", "询问在场人物", "检查自身状态", "暂时等待"}
	input := interactive.TurnSubmissionInput{StateUpdates: &updates, Choices: &choices}
	if storyContext.Meta.PlanningMode == interactive.StoryPlanningModeEnabled && storyContext.Snapshot.BranchPlan == nil {
		plan := "Keep the current branch coherent while responding to the player's choices."
		input.PlanUpdate = &plan
	}
	receipt, err := conversation.SubmitTurnResult(context.Background(), input)
	if err != nil || !receipt.Ready {
		t.Fatalf("SubmitTurnResult failed: receipt=%#v err=%v", receipt, err)
	}
}

type interactiveReplayConversation struct {
	store    *interactive.Store
	storyID  string
	branchID string
	message  string
}

func (*interactiveReplayConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}

func (*interactiveReplayConversation) AppendAssistant(string) error                    { return nil }
func (*interactiveReplayConversation) MarkInterrupted(string, string, string) error    { return nil }
func (*interactiveReplayConversation) PendingInterruption() *agentsession.Interruption { return nil }
func (*interactiveReplayConversation) ResolveInterruption(string) error                { return nil }

var _ agentlifecycle.ConversationCommitter = interactiveReplayConversationCommitter{}
