package statistics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/internal/app/projectbook"
	"denova/internal/project"
	stats "denova/internal/statistics"
)

func TestObservedSavedChangesDoNotInventRestartOrResyncOutput(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	content := filepath.Join(root, "projects", "Book")
	if err := os.MkdirAll(filepath.Join(content, "chapters"), 0700); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(root)
	record, err := registry.Add(content, project.TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(record)
	if err != nil {
		t.Fatal(err)
	}
	books := projectbook.NewService(registry)
	service := New(registry, books)
	t.Cleanup(service.Close)
	save := func(words int) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(content, "chapters", "chapter1.md"), []byte(strings.Repeat("文", words)), 0600); err != nil {
			t.Fatal(err)
		}
		books.InvalidateSummary(record.ID, nil, true)
	}
	observe := func(mode observationMode) {
		t.Helper()
		if _, err := service.observe(ctx, record, layout, mode); err != nil {
			t.Fatal(err)
		}
	}
	save(100)
	observe(resyncBaseline)
	save(130)
	observe(snapshotOnly) // A dashboard read must not consume the pending change.
	observe(savedChange)
	save(80)
	observe(savedChange)
	save(150)
	observe(resyncBaseline) // Unobserved/relinked content is a new baseline.
	service.Close()
	service = New(registry, books)
	t.Cleanup(service.Close)
	save(180)
	observe(resyncBaseline) // Restart does not count the offline difference.
	save(190)
	observe(savedChange)
	report, err := service.Query(ctx, record.ID, 1, time.UTC, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Writing.Projects) != 1 || report.Writing.Projects[0].Words != 190 {
		t.Fatalf("current manuscript = %+v", report.Writing.Projects)
	}
	if delta := report.Writing.Days[0].Delta; delta == nil || *delta != -10 {
		t.Fatalf("observed delta = %v, want -10", delta)
	}
}

func TestProjectScopeAndViewerMidnight(t *testing.T) {
	root := t.TempDir()
	registry := project.NewRegistry(root)
	var records []project.Record
	for _, name := range []string{"A", "B"} {
		content := filepath.Join(root, "projects", name)
		if err := os.MkdirAll(content, 0700); err != nil {
			t.Fatal(err)
		}
		record, err := registry.Add(content, project.TypeGeneral, name)
		if err != nil {
			t.Fatal(err)
		}
		layout, err := registry.EnsureStore(record)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
		for _, at := range []time.Time{time.Date(2026, 10, 25, 15, 59, 0, 0, time.UTC), time.Date(2026, 10, 25, 16, 1, 0, 0, time.UTC)} {
			if err := stats.AppendCall(layout.StoreRoot, stats.Call{ID: name, At: at, Kind: stats.ModelCall, Model: name, Runtime: "native", Usage: &stats.Usage{Total: 10}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	service := New(registry, projectbook.NewService(registry))
	defer service.Close()
	now := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	report, err := service.Query(context.Background(), records[0].ID, 1, time.FixedZone("Asia/Shanghai", 480*60), now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Usage.Totals.Requests != 1 || report.Usage.Totals.Tokens != 10 || report.Usage.Days[0].Date != "2026-10-26" || len(report.Usage.Models) != 1 {
		t.Fatalf("scoped midnight report = %+v", report.Usage)
	}
	report, err = service.Query(context.Background(), "", 1, time.FixedZone("Asia/Shanghai", 480*60), now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Usage.Totals.Requests != 2 || report.Usage.Totals.Tokens != 20 {
		t.Fatalf("global usage = %+v", report.Usage.Totals)
	}
	// The fall-back day has 25 hours. Using its current UTC offset for the
	// whole date would incorrectly drop the first hour's observations.
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.Layout(records[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{
		time.Date(2026, 11, 1, 3, 59, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 4, 1, 0, 0, time.UTC),
		time.Date(2026, 11, 2, 4, 59, 0, 0, time.UTC),
	} {
		if err := stats.AppendCall(layout.StoreRoot, stats.Call{ID: "dst", At: at, Kind: stats.ModelCall, Runtime: "native"}); err != nil {
			t.Fatal(err)
		}
	}
	report, err = service.Query(context.Background(), records[0].ID, 1, newYork, time.Date(2026, 11, 1, 17, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.Usage.Totals.Requests != 2 {
		t.Fatalf("daylight-saving usage = %+v", report.Usage.Totals)
	}
}
