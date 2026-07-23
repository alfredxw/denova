package app

import (
	"context"

	"github.com/cloudwego/eino/schema"

	"denova/internal/agent"
)

func assembleAndCommitInteractiveContextForTest(conversation *interactiveConversation, originalMessage, userMessage string) ([]*schema.Message, error) {
	result, err := conversation.AssembleModelContext(context.Background(), originalMessage, agent.ModelContextInput{
		UserMessage: userMessage,
		Budget:      conversation.ModelContextBudget(),
	})
	if err == nil {
		err = conversation.CommitModelInput(context.Background(), originalMessage, result)
	}
	return result.Messages, err
}
