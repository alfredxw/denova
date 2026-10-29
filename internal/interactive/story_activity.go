package interactive

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"
)

// StoryActivity describes retained Game activity, not lifetime generation
// attempts. Turns shared by branches count once; abandoned versions and rewound
// tails do not count. The canonical journal remains the only source of truth.
type StoryActivity struct {
	Story       StorySummary
	TurnTimes   []time.Time
	NewBranches int
}

// ReadActivity reads one stable story revision without changing its selection.
// It snapshots indexed metadata under the Story lease, then walks the union of
// branch ancestry outside the lease without reading historical prose.
func (s *Store) ReadActivity(ctx context.Context, storyID string, from, until time.Time) (StoryActivity, error) {
	if err := ctx.Err(); err != nil {
		return StoryActivity{}, err
	}
	var result StoryActivity
	var branches []BranchMeta
	var heads []string
	var nodes map[string]storyModelNode
	if err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		release, err := s.acquireStoryReadLeaseLocked(storyID)
		if err != nil {
			return err
		}
		defer release()
		handle, err := s.refreshStoryJournalLocked(storyID, false)
		if err != nil {
			return err
		}
		result.Story = storySummaryFromProjection(handle.projection)
		if result.Story.Preview {
			return nil
		}
		for _, branch := range handle.projection.Meta.Branches {
			branches = append(branches, branch)
		}
		for _, branch := range handle.projection.Branches {
			heads = append(heads, branch.Head)
		}
		// Traversal reads only scalar node fields. Copy the map so a concurrent
		// commit through this Store cannot change the captured ancestry.
		nodes = maps.Clone(handle.projection.ModelNodes)
		return nil
	}(); err != nil {
		return StoryActivity{}, err
	}
	if result.Story.Preview {
		return result, nil
	}
	for _, branch := range branches {
		if branch.From == "" {
			continue // The initial branch is not a newly explored alternative.
		}
		at, err := time.Parse(time.RFC3339Nano, branch.CreatedAt)
		if err != nil {
			return StoryActivity{}, fmt.Errorf("invalid branch activity timestamp: %w", err)
		}
		if !at.Before(from) && at.Before(until) {
			result.NewBranches++
		}
	}
	visited := make(map[string]uint8)
	for _, head := range heads {
		var path []string
		for id := head; id != ""; {
			if err := ctx.Err(); err != nil {
				return StoryActivity{}, err
			}
			if visited[id] == 2 {
				break
			}
			node, found := nodes[id]
			if !found || visited[id] == 1 {
				return StoryActivity{}, fmt.Errorf("Game activity ancestry is incomplete at %s", id)
			}
			visited[id] = 1
			path = append(path, id)
			if node.Count > 0 {
				at, err := time.Parse(time.RFC3339Nano, node.TurnAt)
				if err != nil {
					return StoryActivity{}, fmt.Errorf("invalid turn activity timestamp: %w", err)
				}
				if !at.Before(from) && at.Before(until) {
					result.TurnTimes = append(result.TurnTimes, at)
				}
			}
			id = node.Parent
		}
		for _, id := range path {
			visited[id] = 2
		}
	}
	slices.SortFunc(result.TurnTimes, time.Time.Compare)
	return result, nil
}
