package statistics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/internal/app/projectbook"
	"denova/internal/interactive"
	"denova/internal/project"
)

func TestGameStatisticsProjectScopePreviewAndPartialSources(t *testing.T) {
	root := t.TempDir()
	registry := project.NewRegistry(root)
	var projects []project.Record
	for _, name := range []string{"First", "Second"} {
		content := filepath.Join(root, "projects", name)
		if err := os.MkdirAll(content, 0700); err != nil {
			t.Fatal(err)
		}
		record, err := registry.Add(content, project.TypeGeneral, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.EnsureStore(record); err != nil {
			t.Fatal(err)
		}
		projects = append(projects, record)
		store := interactive.NewStore(content)
		for _, preview := range []bool{false, true} {
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Same title", Preview: preview})
			if err != nil {
				t.Fatal(err)
			}
			turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", Narrative: "Opening."})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "Alternative"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	service := New(registry, projectbook.NewService(registry))
	defer service.Close()
	location := time.FixedZone("Asia/Shanghai", 480*60)
	read := func(id string, wantStories, wantTurns, wantIssues int) {
		t.Helper()
		report, err := service.Query(context.Background(), id, 7, location, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		turns := 0
		for _, day := range report.Game.Days {
			turns += day.Turns
			if day.Turns > 0 && day.Date != time.Now().In(location).Format(time.DateOnly) {
				t.Fatalf("activity bucket = %+v", day)
			}
		}
		if len(report.Game.Stories) != wantStories || turns != wantTurns || report.Game.NewBranches != wantStories || report.Issues != wantIssues {
			t.Fatalf("game report = %+v, issues=%d", report.Game, report.Issues)
		}
		for _, story := range report.Game.Stories {
			if id != "" && story.ProjectID != id {
				t.Fatalf("cross-project story = %+v", story)
			}
		}
	}
	read("", 2, 2, 0)
	read(projects[0].ID, 1, 1, 0)
	// A broken source must not erase valid activity or appear as zero activity.
	layout, err := registry.Layout(projects[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ContentRoot, "interactive", "story", "index.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	read("", 1, 1, 1)
}
