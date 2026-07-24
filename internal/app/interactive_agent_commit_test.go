package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	agents "denova/internal/agents"
	"denova/internal/interactive"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

var interactiveAgentTestCycle atomic.Uint64

// commitInteractiveAssistantForTest crosses the same durable output barrier
// as production. Tests that exercise parsing or validation failures should
// call AppendAssistant directly and assert the pre-commit error instead.
func commitInteractiveAssistantForTest(t testing.TB, conversation *interactiveConversation, content, thinking string) error {
	t.Helper()
	cycle := interactiveAgentTestCycle.Add(1)
	identity := fmt.Sprintf("test-cycle:%d", cycle)
	conversation.BindAgentCycleIdentity(agents.HarnessCycleIdentity{
		CommandID:   runstate.CommandID(identity),
		OperationID: runstate.OperationID(identity),
		Cycle:       1,
	})
	materializeInteractiveInputForTest(t, conversation, conversation.agentCycleIdentitySnapshot())
	if err := conversation.AppendAssistantWithThinking(content, thinking); err != nil {
		return err
	}
	return conversation.CommitAgentCycleStage(context.Background(), agents.HarnessDomainCommitOutput, agents.RunOutcome{Status: agents.RunOutcomeCompleted})
}

func materializeInteractiveInputForTest(t testing.TB, conversation *interactiveConversation, identity agents.HarnessCycleIdentity) {
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
