package external

import (
	"context"
	"errors"

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
		err := operation.request.Session.UpdateExternal(ctx, operation.request.Revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
			if _, err := operation.owned(state); err != nil {
				return session.ExternalTransaction{}, err
			}
			record, err := externaljournal.NewRecord(externaljournal.GuidanceDelivered, operation.id, operation.request.Revision,
				externaljournal.DeliveredGuidance{CommandID: guidance.Request.CommandID, MessageID: prepared.Metadata.MessageID, Count: guidance.Count})
			return session.ExternalTransaction{Records: []externaljournal.Record{record}, Message: &prepared.Message, Metadata: prepared.Metadata}, err
		})
		if err == nil {
			operation.mu.Lock()
			operation.request.GuidanceCount = guidance.Count
			operation.request.Input.Attachments = append(operation.request.Input.Attachments, prepared.Input.Attachments...)
			operation.mu.Unlock()
		}
		return err
	}}, nil
}
