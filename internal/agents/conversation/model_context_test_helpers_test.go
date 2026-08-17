package conversation

import (
	"context"
	"fmt"
	"sync/atomic"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
)

var modelContextTestCycle atomic.Uint64

func assembleAndCommitModelContextForTest(conversation any, originalMessage, userMessage string, references ...agentcontext.UserReference) ([]*agent.Message, error) {
	if sessionConversation, ok := conversation.(*SessionConversation); ok {
		identity := sessionConversation.agentCycleIdentitySnapshot()
		_, alreadyCommitted := sessionConversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitInput)
		if !agentrun.ValidCycleIdentity(identity) || alreadyCommitted {
			cycle := modelContextTestCycle.Add(1)
			sessionConversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
				CommandID:   agentrun.CommandID(fmt.Sprintf("test-command-%d", cycle)),
				OperationID: agentrun.OperationID(fmt.Sprintf("test-operation-%d", cycle)),
				Cycle:       1,
			})
		}
	}
	result, err := agentcontext.AssembleModelContext(context.Background(), conversation, originalMessage, agentcontext.ModelContextInput{
		UserMessage:    userMessage,
		UserReferences: references,
		Budget:         agentcontext.ModelContextBudgetFor(conversation),
	})
	if err == nil {
		err = agentcontext.CommitModelInput(context.Background(), conversation, originalMessage, result)
	}
	return result.Messages, err
}
