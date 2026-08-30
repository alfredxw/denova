package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/book/versions"
	workspacelayout "denova/internal/workspace"
)

const (
	releasedStateMigrationVersion = 3
	stateMigrationVersion         = 4
)

type migrationReceipt struct {
	Version     int       `json:"version"`
	CompletedAt time.Time `json:"completed_at"`
	Copied      []string  `json:"copied"`
}

// EnsureState creates the user-owned state root and performs a copy-only
// migration from legacy workspace-private state. Source files are retained as
// an explicit rollback path; subsequent calls are idempotent.
func (registry *Registry) EnsureState(record Record) (Layout, error) {
	// Multiple project surfaces can resolve the same project concurrently. Keep
	// the one-time legacy migration serialized so each caller observes either the
	// legacy source or the completed atomic destination, never a competing rename.
	registry.stateMu.Lock()
	defer registry.stateMu.Unlock()

	layout, err := registry.Layout(record)
	if err != nil {
		return Layout{}, err
	}
	if err := os.MkdirAll(layout.StateRoot, 0o700); err != nil {
		return Layout{}, fmt.Errorf("create project state root: %w", err)
	}
	receiptPath := filepath.Join(layout.StateRoot, "migration.json")
	complete, err := ensureCurrentStateMigrationReceipt(receiptPath, record.ID)
	if err != nil {
		return Layout{}, err
	}
	if complete {
		return layout, nil
	}
	legacy := []struct {
		name        string
		source      string
		destination string
	}{
		{name: "sessions", source: workspacelayout.Path(layout.ContentRoot, "sessions"), destination: layout.SessionsDir()},
		{name: "config", source: workspacelayout.Path(layout.ContentRoot, "config.toml"), destination: layout.ConfigPath()},
		{name: "changes", source: workspacelayout.Path(layout.ContentRoot, "changes"), destination: layout.ChangesDir()},
		{name: "reviews", source: workspacelayout.Path(layout.ContentRoot, "reviews"), destination: layout.ReviewsDir()},
		{name: "runs", source: workspacelayout.Path(layout.ContentRoot, "runs"), destination: layout.RunsDir()},
		{name: "artifacts", source: workspacelayout.Path(layout.ContentRoot, "artifacts"), destination: layout.ArtifactsDir()},
		{name: "automations", source: workspacelayout.Path(layout.ContentRoot, "automations"), destination: layout.AutomationsDir()},
	}
	copied := make([]string, 0, len(legacy))
	if layout.Type == TypeBook {
		migrated, migrationErr := versions.MigrateLegacyRepository(
			layout.ContentRoot,
			layout.VersionRepositoryDir(),
		)
		if migrationErr != nil {
			return Layout{}, fmt.Errorf("migrate released Project versions: %w", migrationErr)
		}
		if migrated {
			copied = append(copied, "versions")
		}
	}
	for _, item := range legacy {
		if _, statErr := os.Lstat(item.source); errors.Is(statErr, os.ErrNotExist) {
			continue
		} else if statErr != nil {
			return Layout{}, fmt.Errorf("inspect legacy project state %s: %w", item.name, statErr)
		}
		if _, statErr := os.Lstat(item.destination); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return Layout{}, fmt.Errorf("inspect project state destination %s: %w", item.name, statErr)
		}
		if err := copyStateEntry(item.source, item.destination); err != nil {
			return Layout{}, fmt.Errorf("migrate project state %s: %w", item.name, err)
		}
		copied = append(copied, item.name)
	}
	receipt := migrationReceipt{
		Version: stateMigrationVersion, CompletedAt: time.Now().UTC(), Copied: copied,
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Layout{}, err
	}
	if err := writeFileAtomic(receiptPath, append(raw, '\n'), 0o600); err != nil {
		return Layout{}, fmt.Errorf("write project state migration receipt: %w", err)
	}
	return layout, nil
}

// ensureCurrentStateMigrationReceipt accepts the current receipt, upgrades the
// v0.3.3 receipt without replaying its completed copy, and reports false only
// when no receipt exists yet.
func ensureCurrentStateMigrationReceipt(receiptPath, projectID string) (bool, error) {
	raw, err := os.ReadFile(receiptPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read project state migration receipt: %w", err)
	}
	var receipt migrationReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return false, fmt.Errorf("decode project state migration receipt: %w", err)
	}
	switch receipt.Version {
	case stateMigrationVersion:
		return true, nil
	case releasedStateMigrationVersion:
		receipt.Version = stateMigrationVersion
		migrated, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			return false, fmt.Errorf("encode migrated project state receipt: %w", err)
		}
		if err := writeFileAtomic(receiptPath, append(migrated, '\n'), 0o600); err != nil {
			return false, fmt.Errorf("migrate released project state receipt: %w", err)
		}
		slog.Info("[internal/project/migration.go] migrated released Project state receipt",
			"project_id", projectID,
			"from_version", releasedStateMigrationVersion,
			"to_version", stateMigrationVersion,
		)
		return true, nil
	default:
		return false, fmt.Errorf(
			"project state migration receipt version %d does not match supported version %d",
			receipt.Version,
			stateMigrationVersion,
		)
	}
}

func (registry *Registry) migrateReleasedStateReceipts(projects []Record) error {
	for _, record := range projects {
		receiptPath := filepath.Join(registry.denovaDir, StateDirectoryName, record.StateDirName, "migration.json")
		if _, err := ensureCurrentStateMigrationReceipt(receiptPath, record.ID); err != nil {
			return fmt.Errorf("migrate Project %s state receipt: %w", record.ID, err)
		}
	}
	return nil
}

func copyStateEntry(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("legacy state symlink is not migrated: %s", source)
	}
	if info.IsDir() {
		return copyStateDirectory(source, destination)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported legacy state entry: %s", source)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeFileAtomic(destination, data, info.Mode().Perm())
}

func copyStateDirectory(source, destination string) error {
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".migration-*")
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.RemoveAll(temp)
		}
	}()
	if err := copyDirectoryContents(source, temp); err != nil {
		return err
	}
	if err := os.Rename(temp, destination); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func copyDirectoryContents(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		destinationPath := filepath.Join(destination, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("legacy state symlink is not migrated: %s", sourcePath)
		}
		if entry.IsDir() {
			if err := os.MkdirAll(destinationPath, info.Mode().Perm()); err != nil {
				return err
			}
			if err := copyDirectoryContents(sourcePath, destinationPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported legacy state entry: %s", sourcePath)
		}
		input, err := os.Open(sourcePath)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := errors.Join(input.Close(), output.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("state path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if mode == 0 {
		mode = 0o600
	}
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}
