package app

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive"
	interactivestate "denova/internal/interactive/state"
)

var interactiveAgentTestCycle atomic.Uint64

// commitInteractiveAssistantForTest crosses the same durable output barrier
// as production. Tests that exercise parsing or validation failures should
// call AppendAssistant directly and assert the pre-commit error instead.
func commitInteractiveAssistantForTest(
	t testing.TB,
	store *interactive.Store,
	storyID, branchID, user string,
	conversation *interactiveapp.Conversation,
	content, thinking string,
) error {
	t.Helper()
	cycle := interactiveAgentTestCycle.Add(1)
	identity := fmt.Sprintf("test-cycle:%d", cycle)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
		CommandID:   agentrun.CommandID(identity),
		OperationID: agentrun.OperationID(identity),
		Cycle:       1,
	})
	materializeInteractiveInputForTest(t, store, storyID, branchID, user, conversation.AgentCycleIdentitySnapshot())
	if err := conversation.AppendAssistantWithThinking(content, thinking); err != nil {
		return err
	}
	return conversation.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted})
}

func materializeInteractiveInputForTest(
	t testing.TB,
	store *interactive.Store,
	storyID, branchID, user string,
	identity agentrun.CycleIdentity,
) {
	t.Helper()
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	branchID = storyContext.Snapshot.BranchID
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, branchID, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(storyID, intent); err != nil {
		t.Fatal(err)
	}
}

func submitTestTurnResult(
	t *testing.T,
	store *interactive.Store,
	storyID, branchID string,
	conversation *interactiveapp.Conversation,
	intent, goal string,
) {
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
	receipt, err := conversation.SubmitTurnResult(context.Background(), interactive.TurnSubmissionInput{
		StateUpdates: &updates,
		Choices:      &choices,
	})
	if err != nil || !receipt.Ready {
		t.Fatalf("SubmitTurnResult failed: receipt=%#v err=%v", receipt, err)
	}
}

func assembleAndCommitInteractiveContextForTest(
	conversation *interactiveapp.Conversation,
	originalMessage, userMessage string,
) ([]*agents.Message, error) {
	result, err := conversation.AssembleModelContext(context.Background(), originalMessage, agentcontext.ModelContextInput{
		UserMessage: userMessage,
		Budget:      conversation.ModelContextBudget(),
	})
	if err == nil {
		err = conversation.CommitModelInput(context.Background(), originalMessage, result)
	}
	return result.Messages, err
}
