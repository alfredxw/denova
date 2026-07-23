package session

import (
	"context"
	"errors"
	"fmt"

	"denova/internal/filelease"
)

// withCanonicalMutation is the only lock-ordering seam for an existing
// session journal mutation. It serializes independent Session instances on the
// canonical file identity, then refreshes the caller before any lookup, CAS,
// or append. Callbacks run with s.mu held and must use the *Locked helpers;
// appendJournalRecordLocked intentionally does not reacquire the file lease.
func (s *Session) withCanonicalMutation(ctx context.Context, operation string, mutate func() error) (resultErr error) {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := filelease.Acquire(ctx, s.filePath+".domain.lock")
	if err != nil {
		return fmt.Errorf("acquire session journal lease for %s: %w", operation, err)
	}
	defer func() { resultErr = errors.Join(resultErr, release()) }()

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return fmt.Errorf("refresh session before %s: %w", operation, err)
	}
	return mutate()
}
