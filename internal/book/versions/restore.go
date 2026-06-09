package versions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

func (s *Service) Restore(id string, settings VersionAutoSettings) (VersionCommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version, err := s.findVersion(id)
	if err != nil {
		return VersionCommandResult{}, err
	}
	settings = normalizeVersionAutoSettings(settings)
	status, err := s.statusLocked(settings)
	if err != nil {
		return VersionCommandResult{}, err
	}
	if !status.Clean {
		if _, err := s.createLocked("回滚前自动备份", VersionSourceRollbackBackup, settings); err != nil && !errors.Is(err, ErrVersionClean) {
			return VersionCommandResult{}, fmt.Errorf("创建回滚前自动备份失败: %w", err)
		}
	}
	if err := s.restoreCommitToWorkspace(version.ID); err != nil {
		return VersionCommandResult{}, err
	}
	nextStatus, statusErr := s.statusLocked(settings)
	result := VersionCommandResult{Message: "已恢复到所选版本", Version: &version}
	if statusErr == nil {
		result.Status = &nextStatus
	}
	return result, nil
}

func (s *Service) removeEmptyVisibleDirs() error {
	dirs := []string{}
	err := filepath.WalkDir(s.workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == s.workspace {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.workspace, path)
		if err != nil {
			return nil
		}
		if isVersionExcludedRelPath(filepath.ToSlash(rel)) {
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.SliceStable(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
				continue
			}
			return err
		}
	}
	return nil
}
