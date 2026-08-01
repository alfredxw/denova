package skills

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"denova/internal/localfs"
	workspacelayout "denova/internal/workspace"
)

var workspaceSkillMigrationLocks sync.Map

// MigrateWorkspaceSkills copies legacy book-owned Skill bundles into the
// public <book>/skills directory. Existing public bundles always win, the
// legacy trees remain untouched as a recoverable backup, and every new bundle
// is published by directory rename so readers never observe a partial copy.
func MigrateWorkspaceSkills(workspace string) error {
	workspace = normalizePath(workspace)
	if workspace == "" {
		return nil
	}
	lockValue, _ := workspaceSkillMigrationLocks.LoadOrStore(workspace, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	targetRoot := filepath.Join(workspace, "skills")
	sources := []string{
		filepath.Join(workspace, workspacelayout.DataDirName, "skills"),
		filepath.Join(workspace, workspacelayout.LegacyDataDirName, "skills"),
	}
	var migrationErrors []error
	for _, sourceRoot := range sources {
		entries, err := os.ReadDir(sourceRoot)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			migrationErrors = append(migrationErrors, fmt.Errorf("read legacy Skills %s: %w", sourceRoot, err))
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			sourceSkill := filepath.Join(sourceRoot, entry.Name())
			if info, err := os.Stat(filepath.Join(sourceSkill, SkillFileName)); err != nil || !info.Mode().IsRegular() {
				continue
			}
			targetSkill := filepath.Join(targetRoot, entry.Name())
			if _, err := os.Lstat(targetSkill); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				migrationErrors = append(migrationErrors, fmt.Errorf("inspect public Skill %s: %w", targetSkill, err))
				continue
			}
			if err := migrateWorkspaceSkillBundle(sourceSkill, targetRoot, targetSkill, entry.Name()); err != nil {
				migrationErrors = append(migrationErrors, err)
				continue
			}
			slog.InfoContext(context.Background(), fmt.Sprintf("[skills] migrated legacy workspace Skill source=%s target=%s", sourceSkill, targetSkill))
		}
	}
	return errors.Join(migrationErrors...)
}

func migrateWorkspaceSkillBundle(sourceSkill, targetRoot, targetSkill, name string) error {
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("create public Skills directory %s: %w", targetRoot, err)
	}
	stageRoot, err := os.MkdirTemp(targetRoot, ".migrate-*")
	if err != nil {
		return fmt.Errorf("create Skill migration stage in %s: %w", targetRoot, err)
	}
	defer os.RemoveAll(stageRoot)
	stagedSkill := filepath.Join(stageRoot, name)
	if err := copySkillDir(sourceSkill, stagedSkill); err != nil {
		return fmt.Errorf("copy legacy Skill %s: %w", sourceSkill, err)
	}
	if _, err := os.Stat(targetSkill); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect public Skill %s: %w", targetSkill, err)
	}
	if err := os.Rename(stagedSkill, targetSkill); err != nil {
		if _, statErr := os.Stat(targetSkill); statErr == nil {
			return nil
		}
		return fmt.Errorf("publish migrated Skill %s: %w", targetSkill, err)
	}
	if err := localfs.SyncDirectory(targetRoot); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[skills] migrated Skill directory durability failed path=%s err=%v", targetRoot, err))
	}
	return nil
}
