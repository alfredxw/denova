package workspacechange

import (
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

func (s *Service) atomicWriteVisibleFile(rel string, content []byte) (mutationResult, error) {
	result := mutationResult{Stage: mutationStageUnchanged, ParentRel: visibleParentRel(rel)}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return result, err
	}
	defer root.Close()
	parent := result.ParentRel
	if err := s.ensureVisibleParentDurable(root, parent); err != nil {
		return result, err
	}
	mode := os.FileMode(0o644)
	if info, err := root.Stat(filepath.FromSlash(rel)); err == nil {
		if !info.Mode().IsRegular() {
			return result, newError(ErrorCodeConflict, "workspace path is not a regular file", map[string]any{"path": rel})
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return result, err
	}
	var random [8]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return result, err
	}
	tempRel := path.Join(parent, fmt.Sprintf(".%s.denova-%x.tmp", path.Base(rel), random[:]))
	tempPath := filepath.FromSlash(tempRel)
	targetPath := filepath.FromSlash(rel)
	file, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return result, err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = root.Remove(tempPath)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return result, err
	}
	if err := file.Sync(); err != nil {
		return result, err
	}
	if err := file.Close(); err != nil {
		return result, err
	}
	if err := root.Rename(tempPath, targetPath); err != nil {
		return result, err
	}
	removeTemp = false
	result.Stage = mutationStageVisible
	result.WorkspaceMutated = true
	if err := s.durability.syncRootDir(root, parent); err != nil {
		return result, err
	}
	result.Stage = mutationStageDurable
	return result, nil
}

func (s *Service) atomicRemoveVisibleFile(rel string) (mutationResult, error) {
	result := mutationResult{Stage: mutationStageUnchanged, ParentRel: visibleParentRel(rel)}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if err := root.Remove(filepath.FromSlash(rel)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// The desired after-state is already visible, but synchronize the
			// directory before any durable journal claims that state.
			result.Stage = mutationStageVisible
		} else {
			return result, err
		}
	} else {
		result.Stage = mutationStageVisible
		result.WorkspaceMutated = true
	}
	if err := s.durability.syncRootDir(root, result.ParentRel); err != nil {
		return result, err
	}
	result.Stage = mutationStageDurable
	return result, nil
}
