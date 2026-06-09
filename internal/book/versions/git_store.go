package versions

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (s *Service) openVersionRepo() (*git.Repository, error) {
	repo, err := git.PlainOpen(s.workspace)
	if err == nil {
		return repo, s.ensureGitExclude()
	}
	if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, err
	}
	repo, err = git.PlainInit(s.workspace, false)
	if err != nil {
		return nil, err
	}
	return repo, s.ensureGitExclude()
}

func (s *Service) ensureGitExclude() error {
	path := filepath.Join(s.workspace, ".git", "info", "exclude")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	next := make([]string, 0, len(lines)+1)
	hasVersionsExclude := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".nova/" {
			continue
		}
		if trimmed == ".nova/versions/" {
			hasVersionsExclude = true
		}
		next = append(next, line)
	}
	if len(next) > 0 && next[len(next)-1] == "" {
		next = next[:len(next)-1]
	}
	if !hasVersionsExclude {
		next = append(next, ".nova/versions/")
	}
	content := strings.Join(next, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (s *Service) commitWorkspaceSnapshot(repo *git.Repository, files []versionFileData, message string, now time.Time) (plumbing.Hash, error) {
	if err := s.stageWorkspaceFiles(repo, files); err != nil {
		return plumbing.ZeroHash, err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return worktree.Commit(message, &git.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Nova",
			Email: "nova@local",
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
	return nil
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
	files, err := s.collectVisibleFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.Remove(file.Abs); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := s.removeEmptyVisibleDirs(); err != nil {
		return err
	}

	repo, err := s.openVersionRepo()
	if err != nil {
		return err
	}
	commit, err := repo.CommitObject(plumbing.NewHash(strings.TrimSpace(id)))
	if err != nil {
		return err
	}
	iter, err := commit.Files()
	if err != nil {
		return err
	}
	err = iter.ForEach(func(file *object.File) error {
		reader, err := file.Reader()
		if err != nil {
			return err
		}
		defer reader.Close()
		dst := filepath.Join(s.workspace, filepath.FromSlash(file.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, reader); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		return err
	}
	return s.setRepoHeadAndIndex(repo, id)
}

func (s *Service) setRepoHeadAndIndex(repo *git.Repository, id string) error {
	hash := plumbing.NewHash(strings.TrimSpace(id))
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, hash)); err != nil {
		return err
	}
	files, err := s.collectVisibleFiles()
	if err != nil {
		return err
	}
	return s.stageWorkspaceFiles(repo, files)
}

func versionEntriesContain(items []VersionEntry, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
