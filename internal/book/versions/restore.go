package versions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	if err := s.restoreSnapshot(version); err != nil {
		return VersionCommandResult{}, err
	}
	index, err := s.loadIndex()
	if err != nil {
		return VersionCommandResult{}, err
	}
	index.CurrentID = version.ID
	if err := s.saveIndex(index); err != nil {
		return VersionCommandResult{}, err
	}
	nextStatus, statusErr := s.statusLocked(settings)
	result := VersionCommandResult{Message: "已恢复到所选版本", Version: &version}
	if statusErr == nil {
		result.Status = &nextStatus
	}
	return result, nil
}

func (s *Service) restoreSnapshot(version VersionEntry) error {
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
	srcRoot := s.snapshotDir(version.ID)
	return filepath.WalkDir(srcRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcRoot || entry.IsDir() {
			return nil
		}
		if entry.Name() == "manifest.json" && filepath.Dir(path) == srcRoot {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(s.workspace, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyVersionFile(path, dst)
	})
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
		if strings.HasPrefix(entry.Name(), ".") {
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
