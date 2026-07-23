package interactive

import (
	"context"
	"fmt"
	"log"
	"strings"

	"denova/internal/filelease"
)

// acquireStoryMutationLeaseLocked linearizes the read/CAS/write section across
// every Store instance (and process) that targets the same canonical story.
// Public Store APIs do not expose a context, so acquisition intentionally waits
// without an elapsed-time timeout; process shutdown/crash still releases the
// underlying OS lease.
func (s *Store) acquireStoryMutationLeaseLocked(storyID string) (func(), error) {
	storyID = strings.TrimSpace(storyID)
	if storyID == "" {
		return nil, fmt.Errorf("story id is required for mutation lease")
	}
	if s.heldStoryLeases != nil && s.heldStoryLeases[storyID] > 0 {
		s.heldStoryLeases[storyID]++
		return func() { s.heldStoryLeases[storyID]-- }, nil
	}
	release, err := filelease.Acquire(context.Background(), s.storyPath(storyID)+".mutation.lock")
	if err != nil {
		return nil, fmt.Errorf("acquire story mutation lease: %w", err)
	}
	if s.heldStoryLeases == nil {
		s.heldStoryLeases = make(map[string]int)
	}
	s.heldStoryLeases[storyID] = 1
	return func() {
		delete(s.heldStoryLeases, storyID)
		if err := release(); err != nil {
			log.Printf("[interactive-story] release mutation lease failed story_id=%s error=%v", storyID, err)
		}
	}, nil
}

// acquireStoryReadLeaseLocked protects readers from observing and repairing a
// partial append written by another Store. A mutation that already owns the
// exact story lease reuses it without attempting a non-reentrant lock.
func (s *Store) acquireStoryReadLeaseLocked(storyID string) (func(), error) {
	storyID = strings.TrimSpace(storyID)
	if s.heldStoryLeases != nil && s.heldStoryLeases[storyID] > 0 {
		return func() {}, nil
	}
	return s.acquireStoryMutationLeaseLocked(storyID)
}
