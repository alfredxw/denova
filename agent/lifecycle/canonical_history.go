package lifecycle

import (
	"context"
	"errors"

	agentengine "github.com/alfredxw/denova/agent/engine"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

// LoadCanonicalHistory reuses an aligned active checkpoint or reconstructs it
// from the canonical source. Call it before admission and structural operations.
func (session *Session) LoadCanonicalHistory(ctx context.Context, source agentcanonical.CanonicalHistorySource) error {
	if err := session.usable(); err != nil {
		return err
	}
	if source == nil {
		return errors.New("canonical history source is required")
	}
	head, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return err
	}
	if head.Identity == "" || head.Revision == "" {
		return errors.New("canonical history head is incomplete")
	}
	session.mu.Lock()
	if session.active != nil && !session.active.isSuspended() || session.maintenance {
		session.mu.Unlock()
		return agentschema.ErrSessionBusy
	}
	session.canonicalSource = source
	checkpoint := session.messageCheckpoint
	revision := session.revision
	state, aligned, err := agentengine.AlignedCanonicalState(session.engineState, head)
	if err != nil {
		session.mu.Unlock()
		return err
	}
	if aligned {
		session.engineState = state
		session.mu.Unlock()
		return nil
	}
	session.mu.Unlock()
	if len(state) == 0 {
		restored, matched, err := agentengine.RestoreCanonicalCheckpoint(ctx, checkpoint, head, source)
		if err != nil {
			return err
		}
		if matched {
			after, err := source.CanonicalHistoryHead(ctx)
			if err != nil {
				return err
			}
			session.mu.Lock()
			defer session.mu.Unlock()
			if after != head || session.revision != revision || session.active != nil && !session.active.isSuspended() || session.maintenance {
				return agentschema.ErrSessionBusy
			}
			session.engineState = restored
			return nil
		}
	}
	messages, err := source.CanonicalMessages(ctx)
	if err != nil {
		return err
	}
	after, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return err
	}
	if after != head {
		return agentschema.ErrSessionBusy
	}
	return session.loadCanonicalMessages(ctx, messages, head)
}
