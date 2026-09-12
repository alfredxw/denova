package versions

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestVersionDiffFiltersExcludedHistoricalFiles(t *testing.T) {
	dir := t.TempDir()
	service := newVersionTestService(t, dir)
	settings := DefaultAutoSettings()
	writeFile(t, dir, "chapter.md", "first")
	first, err := service.Create("First", VersionSourceManual, settings)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a historical snapshot created before runtime files were excluded.
	repo, err := service.openVersionRepo()
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	excluded := []string{"session.jsonl.domain.lock", ".denova/runs/trace.json", ".nova/reviews/review.json"}
	for _, path := range append(excluded, "chapter.md", "Cargo.lock") {
		writeFile(t, dir, path, "second")
		if _, err := worktree.Add(path); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := worktree.Commit("Historical", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	for _, comparison := range []VersionDiffComparison{VersionDiffComparisonParent, VersionDiffComparisonWorkspace} {
		t.Run(string(comparison), func(t *testing.T) {
			diff, err := service.Diff(hash.String(), "", comparison)
			if err != nil {
				t.Fatal(err)
			}
			if comparison == VersionDiffComparisonParent {
				if diff.BaseVersion == nil || diff.BaseVersion.ID != first.Version.ID || len(diff.Changes) != 2 || len(diff.Files) != 2 {
					t.Fatalf("unexpected parent diff: %#v", diff)
				}
				assertVersionFileDiff(t, diff.Files, "chapter.md", "first", "second", false, false)
				assertVersionFileDiff(t, diff.Files, "Cargo.lock", "", "second", true, false)
			} else if len(diff.Changes) != 0 || len(diff.Files) != 0 {
				t.Fatalf("excluded files must not appear as workspace deletions: %#v", diff)
			}
			for _, path := range excluded {
				if _, err := service.Diff(hash.String(), path, comparison); err == nil {
					t.Fatalf("explicit access to excluded path %q must fail", path)
				}
			}
		})
	}
}
