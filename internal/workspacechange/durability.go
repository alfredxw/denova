package workspacechange

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type mutationStage string

const (
	mutationStageUnchanged mutationStage = "unchanged"
	mutationStageVisible   mutationStage = "visible"
	mutationStageDurable   mutationStage = "durable"
)

// mutationResult distinguishes a visible namespace mutation from one whose
// parent directory has crossed the durability barrier.
type mutationResult struct {
	Stage            mutationStage
	ParentRel        string
	WorkspaceMutated bool
}

type durabilityOps struct {
	syncRootDirFn func(*os.Root, string) error
}

func defaultDurabilityOps() *durabilityOps {
	return &durabilityOps{
		syncRootDirFn: syncRootDirectory,
	}
}

func (o *durabilityOps) syncRootDir(root *os.Root, rel string) error {
	if o == nil || o.syncRootDirFn == nil {
		return syncRootDirectory(root, rel)
	}
	return o.syncRootDirFn(root, rel)
}

func syncRootDirectory(root *os.Root, rel string) error {
	if rel == "" {
		rel = "."
	}
	file, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Sync(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}

// mkdirAllRootDurable creates a private directory chain beneath an opened
// workspace root. Existing symlinks are rejected even when they point back
// inside the workspace, keeping ledger/blob storage on an unambiguous path.
func mkdirAllRootDurable(root *os.Root, rel string, mode os.FileMode, ops *durabilityOps) error {
	current := "."
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		if component == "" || component == "." {
			continue
		}
		next := path.Join(current, component)
		info, err := root.Lstat(filepath.FromSlash(next))
		if errors.Is(err, os.ErrNotExist) {
			if err := root.Mkdir(filepath.FromSlash(next), mode); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = root.Lstat(filepath.FromSlash(next))
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return newError(ErrorCodeConflict, "workspace change storage path contains a symbolic link", map[string]any{"path": next})
		}
		if !info.IsDir() {
			return &os.PathError{Op: "mkdir", Path: next, Err: errors.New("path is not a directory")}
		}
		if err := ops.syncRootDir(root, next); err != nil {
			return err
		}
		if err := ops.syncRootDir(root, current); err != nil {
			return err
		}
		current = next
	}
	return nil
}

func visibleParentRel(rel string) string {
	parent := path.Dir(filepath.ToSlash(rel))
	if parent == "" {
		return "."
	}
	return parent
}

func (s *Service) ensureVisibleParentDurable(root *os.Root, parent string) error {
	if parent == "." {
		return nil
	}
	if err := root.MkdirAll(filepath.FromSlash(parent), 0o755); err != nil {
		return err
	}
	current := "."
	for _, component := range strings.Split(parent, "/") {
		if component == "" || component == "." {
			continue
		}
		next := path.Join(current, component)
		if err := s.durability.syncRootDir(root, next); err != nil {
			return err
		}
		if err := s.durability.syncRootDir(root, current); err != nil {
			return err
		}
		current = next
	}
	return nil
}

func (s *Service) syncVisibleParent(rel string) error {
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return err
	}
	defer root.Close()
	return s.durability.syncRootDir(root, visibleParentRel(rel))
}

func (s *Service) markPendingParentSync(path, parent string) {
	if parent == "" {
		parent = visibleParentRel(path)
	}
	s.pendingParentSync[parent] = path
}

func (s *Service) syncPendingParentsLocked() error {
	parents := make([]string, 0, len(s.pendingParentSync))
	for parent := range s.pendingParentSync {
		parents = append(parents, parent)
	}
	sort.Strings(parents)
	if len(parents) == 0 {
		return nil
	}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		parent := parents[0]
		return durabilityPendingError(s.pendingParentSync[parent], "", "", mutationResult{
			Stage:            mutationStageVisible,
			ParentRel:        parent,
			WorkspaceMutated: true,
		}, err)
	}
	defer root.Close()
	for _, parent := range parents {
		path := s.pendingParentSync[parent]
		if err := s.durability.syncRootDir(root, parent); err != nil {
			return durabilityPendingError(path, "", "", mutationResult{
				Stage:            mutationStageVisible,
				ParentRel:        parent,
				WorkspaceMutated: true,
			}, err)
		}
		delete(s.pendingParentSync, parent)
		for rel, pending := range s.pendingSaves {
			if pending.ParentRel == parent {
				pending.Durable = true
				s.pendingSaves[rel] = pending
			}
		}
	}
	return nil
}

func (s *Service) reconcilePendingDurabilityLocked() error {
	if err := s.syncPendingParentsLocked(); err != nil {
		return err
	}
	for rel, pending := range s.pendingSaves {
		if !pending.Durable || pending.RedoInvalidated {
			continue
		}
		if err := s.invalidateRedoExcept(OriginUser); err != nil {
			return durabilityPendingError(rel, "", "", mutationResult{
				Stage:            mutationStageDurable,
				ParentRel:        pending.ParentRel,
				WorkspaceMutated: true,
			}, err)
		}
		pending.RedoInvalidated = true
		s.pendingSaves[rel] = pending
	}
	if err := s.recoverOperations(); err != nil {
		return err
	}
	return s.recoverPrepared()
}

func durabilityPendingError(path, changeSetID, operationID string, result mutationResult, cause error) error {
	details := map[string]any{
		"path":              path,
		"mutation_stage":    result.Stage,
		"recovery_pending":  true,
		"workspace_mutated": result.WorkspaceMutated,
	}
	if changeSetID != "" {
		details["change_set_id"] = changeSetID
	}
	if operationID != "" {
		details["operation_id"] = operationID
	}
	pending := newError(ErrorCodeDurabilityPending, "workspace mutation durability or journal finalization is pending", details)
	if cause == nil {
		return pending
	}
	return errors.Join(pending, cause)
}
