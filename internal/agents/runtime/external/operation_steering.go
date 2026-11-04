package external

import (
	"context"
	"errors"
	"fmt"

	agentattachment "denova/internal/agents/attachment"
	agentrun "denova/internal/agents/run"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/session"
)

func (operation *Operation) PrepareSteer(ctx context.Context, guidance Guidance) (PreparedSteer, error) {
	if operation.request.PrepareGuidance == nil {
		return PreparedSteer{}, errors.New("external operation has no product input preparer")
	}
	prepared, err := operation.request.PrepareGuidance(ctx, guidance.Request)
	if err != nil {
		return PreparedSteer{}, err
	}
	input, err := operation.projectMedia(ctx, Input{Text: prepared.Input.Text, Attachments: prepared.Input.Attachments})
	if err != nil {
		return PreparedSteer{}, err
	}
	return PreparedSteer{Input: input, Commit: func(ctx context.Context) error {
		var output string
		var outputEnd int
		if guidance.StartsNewReply {
			operation.mu.Lock()
			outputEnd = operation.output.Len()
			output = operation.output.String()[operation.committedOutput:outputEnd]
			operation.mu.Unlock()
		}
		err := operation.request.Session.UpdateExternal(ctx, operation.request.Revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
			if _, err := operation.owned(state); err != nil {
				return session.ExternalTransaction{}, err
			}
			record, err := externaljournal.NewRecord(externaljournal.GuidanceDelivered, operation.id, operation.request.Revision,
				externaljournal.DeliveredGuidance{CommandID: guidance.Request.CommandID, MessageID: prepared.Metadata.MessageID, Count: guidance.Count})
			change := session.ExternalTransaction{Records: []externaljournal.Record{record}, Message: &prepared.Message, Metadata: prepared.Metadata}
			if output != "" {
				change.PriorOutput = &session.ExternalPriorOutput{
					Text: output, Metadata: session.MessageMetadata{
						MessageID: fmt.Sprintf("%s-output-before-%d", operation.id, guidance.Count),
						RunID:     operation.id, AgentOperationID: operation.id, AgentCommandID: operation.request.CommandID,
						AgentCycle: 1, AgentKind: state.Config.AgentKind,
					},
				}
			}
			return change, err
		})
		if err == nil {
			operation.mu.Lock()
			operation.request.GuidanceCount = guidance.Count
			if guidance.StartsNewReply {
				operation.committedOutput = outputEnd
			}
			operation.request.Input.Attachments = append(operation.request.Input.Attachments, prepared.Input.Attachments...)
			operation.mu.Unlock()
			if guidance.StartsNewReply && !prepared.Metadata.ContextOnly {
				message := prepared.Metadata.DisplayContent
				if message == "" {
					message = prepared.Message.Content
				}
				operation.send(agentrun.Event{Type: "agent_guidance_delivered", Data: map[string]any{
					"id": prepared.Metadata.MessageID, "message": message,
					"attachments": agentattachment.Descriptors(prepared.Message.Attachments), "user_references": prepared.Metadata.UserReferences,
				}})
			}
		}
		return err
	}}, nil
}
