package versions

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitstorage "github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// GitStore owns the go-git repository operations used by Denova versions.
type GitStore struct {
	workspace  string
	repository string
}

func (s *Service) gitStore() GitStore {
	return GitStore{workspace: s.workspace, repository: s.repository}
}

func (g GitStore) OpenExisting() (*git.Repository, error) {
	repo, err := openStateRepository(g.workspace, g.repository)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		migrated, migrationErr := MigrateLegacyRepository(g.workspace, g.repository)
		if migrationErr != nil {
			return nil, migrationErr
		}
		if !migrated {
			return nil, nil
		}
		return openStateRepository(g.workspace, g.repository)
	}
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (g GitStore) Open() (*git.Repository, error) {
	repo, err := g.OpenExisting()
	if repo != nil || err != nil {
		return repo, err
	}
	repo, err = initializeStateRepository(g.workspace, g.repository)
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func openStateRepository(workspace, repository string) (*git.Repository, error) {
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(repository) == "" {
		return nil, fmt.Errorf("version workspace and repository are required")
	}
	if _, err := os.Stat(repository); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, git.ErrRepositoryNotExists
		}
		return nil, err
	}
	return git.Open(detachedRepositoryStorage(repository), osfs.New(workspace))
}

// detachedStorage intentionally exposes only the stable storage.Storer
// contract. Hiding filesystem.Storage's extra Filesystem method prevents
// go-git from creating a .git indirection file in user content.
type detachedStorage struct {
	gitstorage.Storer
}

func detachedRepositoryStorage(repository string) gitstorage.Storer {
	return detachedStorage{Storer: filesystem.NewStorage(
		osfs.New(repository),
		cache.NewObjectLRUDefault(),
	)}
}

func initializeStateRepository(workspace, repository string) (*git.Repository, error) {
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(repository) == "" {
		return nil, fmt.Errorf("version workspace and repository are required")
	}
	if err := os.MkdirAll(filepath.Dir(repository), 0o700); err != nil {
		return nil, err
	}
	temp, err := os.MkdirTemp(filepath.Dir(repository), ".version-repository-*")
	if err != nil {
		return nil, err
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.RemoveAll(temp)
		}
	}()
	if _, err := git.Init(detachedRepositoryStorage(temp), osfs.New(workspace)); err != nil {
		return nil, err
	}
	if err := os.Rename(temp, repository); err != nil {
		if existing, openErr := openStateRepository(workspace, repository); openErr == nil {
			return existing, nil
		}
		return nil, err
	}
	removeTemp = false
	return openStateRepository(workspace, repository)
}

// MigrateLegacyRepository copies the released workspace-local .git repository
// into Project-owned state. The source repository is deliberately retained as
// a rollback path. Copying through go-git also supports .git indirection files
// and linked worktrees without persisting paths back to their original root.
func MigrateLegacyRepository(workspace, repository string) (bool, error) {
	if _, err := openStateRepository(workspace, repository); err == nil {
		return false, nil
	} else if !errors.Is(err, git.ErrRepositoryNotExists) {
		return false, fmt.Errorf("open Project version repository: %w", err)
	}
	legacy, err := git.PlainOpen(workspace)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open released workspace version repository: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(repository), 0o700); err != nil {
		return false, err
	}
	temp, err := os.MkdirTemp(filepath.Dir(repository), ".version-migration-*")
	if err != nil {
		return false, err
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.RemoveAll(temp)
		}
	}()
	destination, err := git.Init(detachedRepositoryStorage(temp), osfs.New(workspace))
	if err != nil {
		return false, err
	}
	objects, err := legacy.Storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return false, err
	}
	if err := objects.ForEach(func(object plumbing.EncodedObject) error {
		_, copyErr := destination.Storer.SetEncodedObject(object)
		return copyErr
	}); err != nil {
		objects.Close()
		return false, fmt.Errorf("copy released version objects: %w", err)
	}
	objects.Close()
	references, err := legacy.References()
	if err != nil {
		return false, err
	}
	if err := references.ForEach(func(reference *plumbing.Reference) error {
		return destination.Storer.SetReference(reference)
	}); err != nil {
		references.Close()
		return false, fmt.Errorf("copy released version references: %w", err)
	}
	references.Close()
	if err := validateMigratedRepository(legacy, destination); err != nil {
		return false, err
	}
	if err := os.Rename(temp, repository); err != nil {
		if _, openErr := openStateRepository(workspace, repository); openErr == nil {
			return false, nil
		}
		return false, err
	}
	removeTemp = false
	return true, nil
}

func validateMigratedRepository(source, destination *git.Repository) error {
	sourceHead, sourceErr := source.Head()
	if errors.Is(sourceErr, plumbing.ErrReferenceNotFound) {
		return nil
	}
	if sourceErr != nil {
		return sourceErr
	}
	destinationHead, err := destination.Head()
	if err != nil {
		return fmt.Errorf("read migrated version HEAD: %w", err)
	}
	if destinationHead.Hash() != sourceHead.Hash() {
		return fmt.Errorf("migrated version HEAD does not match released repository")
	}
	if _, err := destination.CommitObject(destinationHead.Hash()); err != nil {
		return fmt.Errorf("validate migrated version commit: %w", err)
	}
	return nil
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

func (s *Service) commitWorkspaceSnapshot(repo *git.Repository, snapshot workspaceSnapshot, message, source string, metadata versionCommitMetadata, now time.Time) (plumbing.Hash, error) {
	if err := stageWorkspaceSnapshot(repo, snapshot); err != nil {
		return plumbing.ZeroHash, err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	options := &git.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Denova",
			Email: "denova@local",
			When:  now,
		},
	}
	parent, parentErr := versionHistoryHeadHash(repo)
	if parentErr == nil {
		options.Parents = []plumbing.Hash{parent}
	} else if !errors.Is(parentErr, plumbing.ErrReferenceNotFound) {
		return plumbing.ZeroHash, parentErr
	}
	commitMessage, err := formatCommitMessage(message, source, metadata)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	hash, err := worktree.Commit(commitMessage, options)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(versionHistoryReference, hash)); err != nil {
		return plumbing.ZeroHash, err
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(versionCurrentReference, hash)); err != nil {
		return plumbing.ZeroHash, err
	}
	return hash, nil
}

// stageWorkspaceSnapshot replaces the index in one write with the exact blob
// hashes persisted during collection. This avoids go-git's worktree status
// scan followed by a second per-file read, and naturally records deletions.
func stageWorkspaceSnapshot(repo *git.Repository, snapshot workspaceSnapshot) error {
	idx := &index.Index{
		Version: 2,
		Entries: make([]*index.Entry, 0, len(snapshot.files)),
	}
	for _, file := range snapshot.files {
		entry := idx.Add(file.Path)
		entry.Hash = plumbing.NewHash(file.Hash)
		entry.CreatedAt = file.ModifiedAt
		entry.ModifiedAt = file.ModifiedAt
		entry.Mode = file.Mode
		entry.Size = uint32(file.Size)
	}
	return repo.Storer.SetIndex(idx)
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
		data, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		state := versionFileStateFromBytes(data, file.Hash)
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

// commitFileIndex projects a commit from tree/blob metadata only. Callers that
// only compare versions never decompress or copy file contents into memory.
func (s *Service) commitFileIndex(id string) (map[string]versionFileData, error) {
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
		files[file.Name] = versionFileData{
			Path: file.Name,
			Hash: file.Hash.String(),
			Size: file.Size,
			Mode: file.Mode,
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
	if err := s.removeVisibleFilesAbsentFromCommit(id); err != nil {
		return err
	}
	return repo.Storer.SetReference(plumbing.NewHashReference(versionCurrentReference, plumbing.NewHash(strings.TrimSpace(id))))
}
