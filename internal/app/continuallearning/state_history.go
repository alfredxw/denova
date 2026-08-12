package continuallearning

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	agentstate "github.com/alfredxw/denova/agent/state"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/flock"
)

const stateVersionIDPrefix = "harness-state-v1:"

var errStopStateVersions = errors.New("stop Harness State version iteration")

// stateHistory is an application-owned projection of the visible State
// directory. Current files remain authoritative; Git is never exposed to the
// Agent State module or upper application contracts.
type stateHistory struct {
	repo *git.Repository
	lock *flock.Flock
	mu   sync.Mutex
}

func openStateHistory(root, lockPath string) (*stateHistory, error) {
	gitPath := filepath.Join(root, ".git")
	if info, err := os.Lstat(gitPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, errors.New("open Harness State history: .git must be a private directory")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("inspect Harness State history: %w", err)
	}
	repository, err := git.PlainOpen(root)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repository, err = git.PlainInit(root, false)
	}
	if err != nil {
		return nil, fmt.Errorf("open Harness State history: %w", err)
	}
	return &stateHistory{repo: repository, lock: flock.New(lockPath)}, nil
}

func (history *stateHistory) withLock(ctx context.Context, operation func() error) error {
	if history == nil || history.repo == nil || history.lock == nil {
		return errors.New("Harness State history is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	locked, err := history.lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire Harness State history lock: %w", err)
	}
	if !locked {
		return context.Canceled
	}
	defer history.lock.Unlock()
	return operation()
}

func (history *stateHistory) versions(ctx context.Context, snapshot agentstate.Snapshot, limit int) ([]StateVersion, error) {
	versions := make([]StateVersion, 0)
	err := history.withLock(ctx, func() error {
		if _, _, err := history.record(snapshot, "Observe current Harness State"); err != nil {
			return err
		}
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		iterator, err := history.repo.Log(&git.LogOptions{})
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		defer iterator.Close()
		return iterator.ForEach(func(commit *object.Commit) error {
			if len(versions) >= limit {
				return errStopStateVersions
			}
			version, err := stateVersionFromCommit(commit)
			if err != nil {
				return err
			}
			versions = append(versions, version)
			return nil
		})
	})
	if errors.Is(err, errStopStateVersions) {
		err = nil
	}
	return versions, err
}

func (history *stateHistory) diff(ctx context.Context, from, to StateVersionID) (StateVersionDiff, error) {
	var result StateVersionDiff
	err := history.withLock(ctx, func() error {
		fromCommit, err := stateCommitForVersion(history.repo, from)
		if err != nil {
			return err
		}
		toCommit, err := stateCommitForVersion(history.repo, to)
		if err != nil {
			return err
		}
		fromTree, err := fromCommit.Tree()
		if err != nil {
			return err
		}
		toTree, err := toCommit.Tree()
		if err != nil {
			return err
		}
		changes, err := object.DiffTree(fromTree, toTree)
		if err != nil {
			return err
		}
		patch, err := changes.Patch()
		if err != nil {
			return err
		}
		result = StateVersionDiff{From: from, To: to, Patch: patch.String()}
		return nil
	})
	return result, err
}

func (history *stateHistory) record(snapshot agentstate.Snapshot, summary string) (*StateVersion, bool, error) {
	worktree, err := history.repo.Worktree()
	if err != nil {
		return nil, false, err
	}
	status, err := worktree.Status()
	if err != nil {
		return nil, false, err
	}
	if status.IsClean() {
		head, headErr := history.repo.Head()
		if errors.Is(headErr, plumbing.ErrReferenceNotFound) {
			return nil, false, nil
		}
		if headErr != nil {
			return nil, false, headErr
		}
		version, err := stateVersionFromHash(history.repo, head.Hash())
		if err != nil {
			return nil, false, err
		}
		if version.Revision != snapshot.Revision {
			return nil, false, fmt.Errorf("Harness State changed while observing version history: expected=%s actual=%s", snapshot.Revision, version.Revision)
		}
		return &version, false, nil
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return nil, false, err
	}
	hash, err := worktree.Commit(normalizeStateVersionSummary(summary), &git.CommitOptions{
		Author: &object.Signature{Name: "Denova", Email: "state@denova.local", When: time.Now().UTC()},
	})
	if err != nil {
		if errors.Is(err, git.ErrEmptyCommit) {
			return nil, false, nil
		}
		return nil, false, err
	}
	version, err := stateVersionFromHash(history.repo, hash)
	if err != nil {
		return nil, false, err
	}
	if version.Revision != snapshot.Revision {
		return nil, true, fmt.Errorf("Harness State changed while recording version: expected=%s actual=%s", snapshot.Revision, version.Revision)
	}
	return &version, true, nil
}

func stateVersionFromHash(repository *git.Repository, hash plumbing.Hash) (StateVersion, error) {
	commit, err := repository.CommitObject(hash)
	if err != nil {
		return StateVersion{}, err
	}
	return stateVersionFromCommit(commit)
}

func stateVersionFromCommit(commit *object.Commit) (StateVersion, error) {
	files, err := stateFilesFromCommit(commit)
	if err != nil {
		return StateVersion{}, err
	}
	return StateVersion{
		ID:        StateVersionID(stateVersionIDPrefix + commit.Hash.String()),
		Revision:  agentstate.RevisionForFiles(files),
		Summary:   strings.TrimSpace(commit.Message),
		CreatedAt: commit.Author.When.UTC(),
	}, nil
}

func stateCommitForVersion(repository *git.Repository, id StateVersionID) (*object.Commit, error) {
	encoded := strings.TrimSpace(string(id))
	if !strings.HasPrefix(encoded, stateVersionIDPrefix) {
		return nil, ErrStateVersionNotFound
	}
	raw := strings.TrimPrefix(encoded, stateVersionIDPrefix)
	if len(raw) != 40 {
		return nil, ErrStateVersionNotFound
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return nil, ErrStateVersionNotFound
	}
	commit, err := repository.CommitObject(plumbing.NewHash(raw))
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) || errors.Is(err, fs.ErrNotExist) {
			return nil, ErrStateVersionNotFound
		}
		return nil, err
	}
	return commit, nil
}

func stateFilesFromCommit(commit *object.Commit) ([]agentstate.File, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	files := make([]agentstate.File, 0)
	err = tree.Files().ForEach(func(file *object.File) error {
		content, err := file.Contents()
		if err != nil {
			return err
		}
		files = append(files, agentstate.File{Path: filepath.ToSlash(file.Name), Content: []byte(content)})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func stateReplacementChanges(current, target []agentstate.File) []agentstate.Change {
	targetByPath := make(map[string][]byte, len(target))
	for _, file := range target {
		targetByPath[file.Path] = file.Content
	}
	changes := make([]agentstate.Change, 0, len(current)+len(target))
	for _, file := range current {
		if _, ok := targetByPath[file.Path]; !ok {
			changes = append(changes, agentstate.Change{Path: file.Path, Delete: true})
		}
	}
	for _, file := range target {
		changes = append(changes, agentstate.Change{Path: file.Path, Content: file.Content})
	}
	return changes
}

func normalizeStateVersionSummary(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return "Update Harness State"
	}
	if len(value) > 240 {
		end := 240
		for end > 0 && !utf8.ValidString(value[:end]) {
			end--
		}
		value = strings.TrimSpace(value[:end])
	}
	return value
}
