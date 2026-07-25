package session

import (
	"context"
	"errors"
	"fmt"

	"denova/internal/conversationjournal"
)

// withCanonicalMutation refreshes the bounded materialization and then runs a
// domain mutation. Append performs the authoritative file lease + CAS; if an
// independent handle wins the race, the new tail is materialized and the pure
// prepare/append callback is retried against current state.
func (s *Session) withCanonicalMutation(ctx context.Context, operation string, mutate func() error) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		if err := s.refreshCanonicalTailLocked(); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("refresh session before %s: %w", operation, err)
		}
		err := mutate()
		if err == nil {
			s.trimMaterializedWindowLocked()
		}
		s.mu.Unlock()
		if !errors.Is(err, conversationjournal.ErrConflict) {
			return err
		}
	}
}
