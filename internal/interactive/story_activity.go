package interactive

import (
	"context"
	"fmt"
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
// It walks the union of branch ancestry once, using indexed Turn timestamps
// without reading historical narrative bodies.
func (s *Store) ReadActivity(ctx context.Context, storyID string, from, until time.Time) (StoryActivity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryActivity{}, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, false)
	if err != nil {
		return StoryActivity{}, err
	}
	result := StoryActivity{Story: storySummaryFromProjection(handle.projection)}
	if result.Story.Preview {
		return result, nil
	}
	for _, branch := range handle.projection.Meta.Branches {
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
	for _, branch := range handle.projection.Branches {
		var path []string
		for id := branch.Head; id != ""; {
			if err := ctx.Err(); err != nil {
				return StoryActivity{}, err
			}
			if visited[id] == 2 {
				break
			}
			node, found := handle.projection.ModelNodes[id]
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
	return result, nil
}
