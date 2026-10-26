package statistics

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"time"

	"denova/internal/interactive"
	"denova/internal/project"
)

type GameStory struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Turns     int    `json:"turns"`
}

type GameDay struct {
	Date  string `json:"date"`
	Turns int    `json:"turns"`
}

type GameReport struct {
	Stories     []GameStory `json:"stories"`
	Days        []GameDay   `json:"days"`
	NewBranches int         `json:"new_branches"`
}

func (report *Report) readGame(ctx context.Context, record project.Record, layout project.Layout, from, until time.Time, dayIndex map[string]int) {
	store := interactive.NewStore(layout.ContentRoot)
	defer func() {
		if err := store.Close(); err != nil {
			slog.WarnContext(ctx, "Close Game statistics store failed", "project_id", record.ID, "error", err)
		}
	}()
	index, err := store.Index()
	if err != nil {
		report.Issues++
		slog.WarnContext(ctx, "Read Game statistics catalog failed", "project_id", record.ID, "error", err)
		return
	}
	for _, story := range index.Stories {
		if ctx.Err() != nil {
			return
		}
		if story.Preview {
			continue
		}
		activity, err := store.ReadActivity(ctx, story.ID, from, until)
		// Background aggregation must not retain every story's model index.
		err = errors.Join(err, store.Close())
		if err != nil {
			report.Issues++
			slog.WarnContext(ctx, "Read Game statistics failed", "project_id", record.ID, "story_id", story.ID, "error", err)
			continue
		}
		if activity.Story.Preview {
			continue
		}
		report.Game.Stories = append(report.Game.Stories, GameStory{ID: activity.Story.ID, ProjectID: record.ID, Name: activity.Story.Title, Turns: len(activity.TurnTimes)})
		report.Game.NewBranches += activity.NewBranches
		for _, at := range activity.TurnTimes {
			report.Game.Days[dayIndex[at.In(from.Location()).Format(time.DateOnly)]].Turns++
		}
	}
}

func (report *GameReport) sortStories() {
	sort.Slice(report.Stories, func(i, j int) bool {
		left, right := report.Stories[i], report.Stories[j]
		if left.Turns != right.Turns {
			return left.Turns > right.Turns
		}
		if left.ProjectID != right.ProjectID {
			return left.ProjectID < right.ProjectID
		}
		return left.ID < right.ID
	})
}
