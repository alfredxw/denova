package interactiveapp

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
	interactivestate "denova/internal/interactive/state"
)

var testCycleSequence atomic.Uint64

func assembleAndCommitInteractiveContextForTest(conversation *Conversation, originalMessage, userMessage string) ([]*agents.Message, error) {
	result, err := conversation.AssembleModelContext(context.Background(), originalMessage, agentcontext.ModelContextInput{
		UserMessage: userMessage,
		Budget:      conversation.ModelContextBudget(),
	})
	if err == nil {
		err = conversation.CommitModelInput(context.Background(), originalMessage, result)
	}
	return result.Messages, err
}

func commitInteractiveAssistantForTest(t testing.TB, conversation *Conversation, content, thinking string) error {
	t.Helper()
	cycle := testCycleSequence.Add(1)
	identity := fmt.Sprintf("test-cycle:%d", cycle)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
		CommandID: agentrun.CommandID(identity), OperationID: agentrun.OperationID(identity), Cycle: 1,
	})
	materializeInteractiveInputForTest(t, conversation, conversation.AgentCycleIdentitySnapshot())
	if err := conversation.AppendAssistantWithThinking(content, thinking); err != nil {
		return err
	}
	return conversation.CommitAgentCycleStage(
		context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted},
	)
}

func materializeInteractiveInputForTest(t testing.TB, conversation *Conversation, identity agentrun.CycleIdentity) {
	t.Helper()
	storyContext, err := conversation.store.StoryContext(conversation.storyID, conversation.branchID)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, storyContext.Snapshot.BranchID, conversation.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.store.CommitPlayerInput(conversation.storyID, intent); err != nil {
		t.Fatal(err)
	}
}

func submitTestTurnResult(t *testing.T, conversation *Conversation, intent, goal string) {
	t.Helper()
	updates := []interactivestate.Update{}
	storyContext, err := conversation.store.StoryContext(conversation.storyID, conversation.branchID)
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
	input := testTurnSubmissionInput(updates, true)
	if storyContext.Meta.PlanningMode == interactive.StoryPlanningModeEnabled && storyContext.Snapshot.BranchPlan == nil {
		plan := "Keep the current branch coherent while responding to the player's choices."
		input.PlanUpdate = &plan
	}
	receipt, err := conversation.SubmitTurnResult(context.Background(), input)
	if err != nil || !receipt.Ready {
		t.Fatalf("SubmitTurnResult failed: receipt=%#v err=%v", receipt, err)
	}
}

func testTurnSubmissionInput(updates []interactivestate.Update, includeChoices bool) interactive.TurnSubmissionInput {
	input := interactive.TurnSubmissionInput{StateUpdates: &updates}
	if includeChoices {
		choices := []string{"继续当前行动", "观察周围变化", "询问在场人物", "检查自身状态", "暂时等待"}
		input.Choices = &choices
	}
	return input
}
