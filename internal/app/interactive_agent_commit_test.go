package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

var interactiveAgentTestCycle atomic.Uint64

// commitInteractiveAssistantForTest crosses the same durable output barrier
// as production. Tests that exercise parsing or validation failures should
// call AppendAssistant directly and assert the pre-commit error instead.
func commitInteractiveAssistantForTest(t testing.TB, conversation *interactiveConversation, content, thinking string) error {
	t.Helper()
	cycle := interactiveAgentTestCycle.Add(1)
	identity := fmt.Sprintf("test-cycle:%d", cycle)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
		CommandID:   agentrun.CommandID(identity),
		OperationID: agentrun.OperationID(identity),
		Cycle:       1,
	})
	materializeInteractiveInputForTest(t, conversation, conversation.agentCycleIdentitySnapshot())
	if err := conversation.AppendAssistantWithThinking(content, thinking); err != nil {
		return err
	}
	return conversation.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted})
}

func materializeInteractiveInputForTest(t testing.TB, conversation *interactiveConversation, identity agentrun.CycleIdentity) {
	t.Helper()
	storyContext, err := conversation.store.StoryContext(conversation.storyID, conversation.branchID)
	if err != nil {
		t.Fatal(err)
	}
	branchID := storyContext.Snapshot.BranchID
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, branchID, conversation.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conversation.store.CommitPlayerInput(conversation.storyID, intent); err != nil {
		t.Fatal(err)
	}
}
