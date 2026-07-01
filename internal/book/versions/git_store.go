package versions

import (
	"errors"
	"io"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitStore owns the go-git repository operations used by Denova versions.
type GitStore struct {
	workspace string
}

func (s *Service) gitStore() GitStore {
	return GitStore{workspace: s.workspace}
}

func (g GitStore) OpenExisting() (*git.Repository, error) {
	repo, err := git.PlainOpen(g.workspace)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (g GitStore) Open() (*git.Repository, error) {
	repo, err := git.PlainOpen(g.workspace)
	if err == nil {
		return repo, nil
	}
	if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, err
	}
	repo, err = git.PlainInit(g.workspace, false)
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (g GitStore) CheckoutWhole(repo *git.Repository, id string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}
	return worktree.Checkout(&git.CheckoutOptions{
		Hash:  plumbing.NewHash(strings.TrimSpace(id)),
		Force: true,
	})
}

func (s *Service) openExistingVersionRepo() (*git.Repository, error) {
	return s.gitStore().OpenExisting()
}

func (s *Service) openVersionRepo() (*git.Repository, error) {
	return s.gitStore().Open()
}

func (s *Service) commitWorkspaceSnapshot(repo *git.Repository, files []versionFileData, message, source string, now time.Time) (plumbing.Hash, error) {
	if err := s.stageWorkspaceFiles(repo, files); err != nil {
		return plumbing.ZeroHash, err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return worktree.Commit(formatCommitMessage(message, source), &git.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Denova",
			Email: "denova@local",
			When:  now,
		},
	})
}

func (s *Service) stageWorkspaceFiles(repo *git.Repository, files []versionFileData) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	for _, file := range files {
		if err := worktree.AddWithOptions(&git.AddOptions{Path: file.Path, SkipStatus: true}); err != nil {
			return err
		}
	}
	return removeVersionExcludedIndexEntries(repo)
}

func removeVersionExcludedIndexEntries(repo *git.Repository) error {
	idx, err := repo.Storer.Index()
	if err != nil {
		return err
	}
	kept := idx.Entries[:0]
	changed := false
	for _, entry := range idx.Entries {
		if isVersionExcludedRelPath(entry.Name) {
			changed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !changed {
		return nil
	}
	idx.Entries = kept
	return repo.Storer.SetIndex(idx)
}

func (s *Service) commitFiles(id string) (map[string]versionFileData, error) {
	repo, err := s.openVersionRepo()
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(plumbing.NewHash(strings.TrimSpace(id)))
	if err != nil {
		return nil, err
	}
	iter, err := commit.Files()
	if err != nil {
		return nil, err
	}
	files := map[string]versionFileData{}
	err = iter.ForEach(func(file *object.File) error {
		reader, err := file.Reader()
		if err != nil {
			return err
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		state := versionFileStateFromBytes(data)
		files[file.Name] = versionFileData{
			Path:  file.Name,
			Hash:  state.Hash,
			Size:  state.Size,
			Chars: state.Chars,
			Text:  state.Text,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (s *Service) readCommitFile(id, path string) ([]byte, error) {
	repo, err := s.openVersionRepo()
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(plumbing.NewHash(strings.TrimSpace(id)))
	if err != nil {
		return nil, err
	}
	file, err := commit.File(path)
	if err != nil {
		return nil, err
	}
	reader, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (s *Service) restoreCommitToWorkspace(id string) error {
	repo, err := s.openVersionRepo()
	if err != nil {
		return err
	}
	if err := s.withProtectedExcludedWorkspaceDirs(func() error {
		return s.gitStore().CheckoutWhole(repo, id)
	}); err != nil {
		return err
	}
	if err := removeVersionExcludedIndexEntries(repo); err != nil {
		return err
	}
	return s.removeVisibleFilesAbsentFromCommit(id)
}
