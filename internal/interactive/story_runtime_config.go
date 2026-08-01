package interactive

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"denova/config"
	"denova/internal/conversationconfig"
)

// BranchRuntimeConfig reads the complete durable selection without mutating a
// legacy branch. Product adapters call EnsureBranchRuntimeConfig with their
// Settings/recent-session seed when initialization is needed.
func (s *Store) BranchRuntimeConfig(storyID, branchID string) (conversationconfig.Snapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, 1)
	if err != nil {
		return conversationconfig.Snapshot{}, false, err
	}
	branch := meta.Branches[snapshot.BranchID]
	return branchRuntimeConfigSnapshot(branch)
}

// EnsureBranchRuntimeConfig initializes a legacy branch exactly once.
func (s *Store) EnsureBranchRuntimeConfig(storyID, branchID string, seed conversationconfig.Config) (conversationconfig.Snapshot, error) {
	if err := conversationconfig.ValidateShape(seed, config.AgentKindInteractiveStory); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	defer releaseStory()

	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, 1)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	branchID = snapshot.BranchID
	branch := meta.Branches[branchID]
	if existing, ok, err := branchRuntimeConfigSnapshot(branch); err != nil || ok {
		return existing, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	value := seed
	branch.RuntimeConfig = &value
	branch.RuntimeConfigRevision = 1
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	event := StoryConfigUpdatedEvent{
		V: schemaVersion, Type: StoryEventTypeStoryConfigUpdated, ID: newID("scu"),
		ParentID: branch.Head, BranchID: branchID, Ts: now, Fields: []string{"runtime_config"},
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return conversationconfig.Snapshot{Config: value, Revision: 1}, nil
}

// SetBranchRuntimeConfig replaces one branch snapshot with CAS protection.
func (s *Store) SetBranchRuntimeConfig(storyID, branchID string, next conversationconfig.Config, expectedRevision uint64) (conversationconfig.Snapshot, error) {
	if err := conversationconfig.ValidateShape(next, config.AgentKindInteractiveStory); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	defer releaseStory()

	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, 1)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	branchID = snapshot.BranchID
	branch := meta.Branches[branchID]
	if branch.RuntimeConfig == nil || branch.RuntimeConfigRevision == 0 {
		return conversationconfig.Snapshot{}, conversationconfig.ErrNotInitialized
	}
	if expectedRevision == 0 || branch.RuntimeConfigRevision != expectedRevision {
		return conversationconfig.Snapshot{}, fmt.Errorf("%w: have=%d want=%d", conversationconfig.ErrRevisionConflict, branch.RuntimeConfigRevision, expectedRevision)
	}
	revision := branch.RuntimeConfigRevision + 1
	now := time.Now().UTC().Format(time.RFC3339Nano)
	value := next
	branch.RuntimeConfig = &value
	branch.RuntimeConfigRevision = revision
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	event := StoryConfigUpdatedEvent{
		V: schemaVersion, Type: StoryEventTypeStoryConfigUpdated, ID: newID("scu"),
		ParentID: branch.Head, BranchID: branchID, Ts: now, Fields: []string{"runtime_config"},
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return conversationconfig.Snapshot{Config: value, Revision: revision}, nil
}

// RecentRuntimeConfig returns the current branch snapshot of the most recently
// updated story, without crossing the writing/game storage boundary.
func (s *Store) RecentRuntimeConfig(excludeStoryID string) (conversationconfig.Config, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.readIndexLocked()
	if err != nil {
		return conversationconfig.Config{}, false, err
	}
	stories := append([]StorySummary(nil), index.Stories...)
	sort.SliceStable(stories, func(i, j int) bool { return stories[i].UpdatedAt > stories[j].UpdatedAt })
	for _, story := range stories {
		if strings.TrimSpace(story.ID) == strings.TrimSpace(excludeStoryID) {
			continue
		}
		meta, snapshot, readErr := s.boundedStorySnapshotWithLimitLocked(story.ID, "", 1)
		if readErr != nil {
			return conversationconfig.Config{}, false, readErr
		}
		branch := meta.Branches[snapshot.BranchID]
		if branch.RuntimeConfig != nil && branch.RuntimeConfigRevision > 0 {
			return *branch.RuntimeConfig, true, nil
		}
	}
	return conversationconfig.Config{}, false, nil
}

func branchRuntimeConfigSnapshot(branch BranchMeta) (conversationconfig.Snapshot, bool, error) {
	if branch.RuntimeConfig == nil {
		if branch.RuntimeConfigRevision != 0 {
			return conversationconfig.Snapshot{}, false, fmt.Errorf("branch runtime config revision exists without a config")
		}
		return conversationconfig.Snapshot{}, false, nil
	}
	if branch.RuntimeConfigRevision == 0 {
		return conversationconfig.Snapshot{}, false, fmt.Errorf("branch runtime config revision is missing")
	}
	return conversationconfig.Snapshot{Config: *branch.RuntimeConfig, Revision: branch.RuntimeConfigRevision}, true, nil
}
