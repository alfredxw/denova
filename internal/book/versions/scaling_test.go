package versions

import (
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestWorkspaceSnapshotUsesGitBlobHashes(t *testing.T) {
	workspace := t.TempDir()
	service := newVersionTestService(t, workspace)
	writeFile(t, workspace, "chapters/ch0001.md", "snapshot content")

	snapshot, err := service.collectWorkspaceSnapshot(nil)
	if err != nil {
		t.Fatalf("collect workspace snapshot: %v", err)
	}
	file, ok := snapshot.byPath["chapters/ch0001.md"]
	if !ok {
		t.Fatalf("snapshot file missing: %#v", snapshot.byPath)
	}
	want := plumbing.ComputeHash(plumbing.BlobObject, []byte("snapshot content")).String()
	if file.Hash != want {
		t.Fatalf("snapshot hash=%s want Git blob hash=%s", file.Hash, want)
	}
}

func TestCommitUsesCollectedSnapshotWhenWorkspaceChangesBeforeCommit(t *testing.T) {
	workspace := t.TempDir()
	service := newVersionTestService(t, workspace)
	writeFile(t, workspace, "chapters/ch0001.md", "collected content")
	repo, err := service.openVersionRepo()
	if err != nil {
		t.Fatalf("open version repository: %v", err)
	}
	snapshot, err := service.collectWorkspaceSnapshot(repo.Storer)
	if err != nil {
		t.Fatalf("collect workspace snapshot: %v", err)
	}

	writeFile(t, workspace, "chapters/ch0001.md", "newer workspace content")
	now := time.Now()
	metadata := newVersionCommitMetadata(snapshot, []VersionChange{{Path: "chapters/ch0001.md", Status: "added"}}, "", "")
	hash, err := service.commitWorkspaceSnapshot(repo, snapshot, "snapshot", VersionSourceManual, metadata, now)
	if err != nil {
		t.Fatalf("commit collected snapshot: %v", err)
	}
	content, err := service.readCommitFile(hash.String(), "chapters/ch0001.md")
	if err != nil {
		t.Fatalf("read committed file: %v", err)
	}
	if got := string(content); got != "collected content" {
		t.Fatalf("committed content=%q want collected snapshot", got)
	}
	status, err := service.Status(DefaultAutoSettings())
	if err != nil {
		t.Fatalf("status after concurrent workspace change: %v", err)
	}
	assertChange(t, status.Changes, "chapters/ch0001.md", "modified")
}

func TestHistoryStopsBeforeLoadingParentBeyondLimit(t *testing.T) {
	workspace := t.TempDir()
	service := newVersionTestService(t, workspace)
	writeFile(t, workspace, "chapters/ch0001.md", "base")
	base, err := service.Create("base", VersionSourceManual, DefaultAutoSettings())
	if err != nil {
		t.Fatalf("create base version: %v", err)
	}
	repo, err := service.openExistingVersionRepo()
	if err != nil || repo == nil {
		t.Fatalf("open existing repository: repo=%v err=%v", repo, err)
	}
	baseCommit, err := repo.CommitObject(plumbing.NewHash(base.Version.ID))
	if err != nil {
		t.Fatalf("load base commit: %v", err)
	}

	now := time.Now().Add(time.Second)
	metadata := versionCommitMetadata{
		Schema:       versionCommitMetadataSchema,
		FileCount:    1,
		TotalBytes:   int64(len("base")),
		ChangedPaths: []string{"chapters/ch0001.md"},
	}
	message, err := formatCommitMessage("bounded head", VersionSourceManual, metadata)
	if err != nil {
		t.Fatalf("format bounded head metadata: %v", err)
	}
	commit := &object.Commit{
		Author:       object.Signature{Name: "Denova", Email: "denova@local", When: now},
		Committer:    object.Signature{Name: "Denova", Email: "denova@local", When: now},
		Message:      message,
		TreeHash:     baseCommit.TreeHash,
		ParentHashes: []plumbing.Hash{plumbing.NewHash(strings.Repeat("f", 40))},
	}
	encoded := repo.Storer.NewEncodedObject()
	if err := commit.Encode(encoded); err != nil {
		t.Fatalf("encode bounded head: %v", err)
	}
	headHash, err := repo.Storer.SetEncodedObject(encoded)
	if err != nil {
		t.Fatalf("store bounded head: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("read HEAD: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(head.Name(), headHash)); err != nil {
		t.Fatalf("move HEAD: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(versionHistoryReference, headHash)); err != nil {
		t.Fatalf("move version history ref: %v", err)
	}

	history, err := service.History(1)
	if err != nil {
		t.Fatalf("History(1) loaded a parent beyond its limit: %v", err)
	}
	if len(history) != 1 || history[0].ID != headHash.String() {
		t.Fatalf("History(1)=%#v want only bounded head", history)
	}
	if _, err := service.History(2); err == nil {
		t.Fatal("History(2) should attempt to load the deliberately missing parent")
	}
}
