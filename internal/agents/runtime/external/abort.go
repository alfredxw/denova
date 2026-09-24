package external

import (
	"context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

// AbortQuestions closes known read-only waits after an explicit task abort.
// Unknown business mutations are left for receipt-based reconciliation.
func (service *Service) AbortQuestions(ctx context.Context, projectID string, sess *session.Session) error {
	questions, err := sess.PendingExternalAsks(ctx)
	if err != nil {
		return err
	}
	for _, question := range questions {
		if _, _, err := service.ResolveAsk(ctx, projectID, sess, question.ID, session.AskCancelled, nil, agentrun.AbortReasonUserRequested); err != nil {
			return err
		}
	}
	return nil
}
