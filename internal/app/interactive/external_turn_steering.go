package interactiveapp

import (
	"context"
	"errors"

	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/runtime/external"
	"denova/internal/interactive"
)

func (turn *ExternalTurn) PrepareSteer(ctx context.Context, guidance external.Guidance) (external.PreparedSteer, error) {
	turn.mu.Lock()
	defer turn.mu.Unlock()
	if turn.closed {
		return external.PreparedSteer{}, external.ErrSteerUnavailable
	}
	input, err := turn.projectMedia(ctx, external.Input{Text: guidance.Request.Message, Attachments: guidance.Request.AttachedFiles})
	if err != nil {
		return external.PreparedSteer{}, err
	}
	return external.PreparedSteer{Input: input, Commit: func(context.Context) error {
		turn.mu.Lock()
		defer turn.mu.Unlock()
		if guidance.Count != len(turn.config.Guidance)+1 {
			return errors.New("Game guidance does not extend the accepted input prefix")
		}
		if err := turn.commitGuidance(guidance.Request); err != nil {
			return err
		}
		turn.config.Guidance = append(turn.config.Guidance, guidance.Request)
		return nil
	}}, nil
}

// commitGuidance reuses the Game draft journal for both resumed and live input.
// The caller serializes it with tool batches; no provider transcript is trusted.
func (turn *ExternalTurn) commitGuidance(guidance agentchat.ChatRequest) error {
	for _, message := range turn.restored {
		if interactive.UserGuidanceCommand(message) == guidance.CommandID {
			return nil
		}
	}
	c := turn.config.Conversation
	message := interactive.UserGuidanceMessage(guidance.CommandID, guidance.Message, guidance.AttachedFiles)
	intent, err := interactive.NewAgentContextBatchIntent(interactive.DomainCommitIdentity{CommandID: string(turn.identity.CommandID), OperationID: string(turn.identity.OperationID), Cycle: turn.identity.Cycle}, c.branchID, c.modelContextBatchSequence, []interactive.ModelContextMessage{interactive.ModelContextMessageFromAgent(message, nil)})
	if err != nil {
		return err
	}
	receipt, err := c.store.AppendModelContextBatch(c.storyID, intent)
	if err != nil {
		return err
	}
	c.modelContextBatchSequence = receipt.Event.Sequence + 1
	turn.restored = append(turn.restored, message)
	return nil
}
