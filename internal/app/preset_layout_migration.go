package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"denova/internal/presetlayout"
)

const (
	presetLayoutMigrationVersion = 1
	presetLayoutBackupDirectory  = "preset-layout-v1"
)

type presetLayoutMigrationReceipt struct {
	Version     int       `json:"version"`
	CompletedAt time.Time `json:"completed_at"`
	Backups     []string  `json:"backups,omitempty"`
}

type presetLayoutMigrationSource struct {
	oldDirectory string
	destinations []presetLayoutMigrationDestination
}

type presetLayoutMigrationDestination struct {
	sourceSubdirectory string
	path               string
}

// migratePresetLayout moves every previous preset catalog behind
// one stable root. The startup lease serializes this migration. Each original
// directory is first atomically moved into backups, then copied to its final
// location so a crash can retry without losing the source data.
func migratePresetLayout(dataRoot string) error {
	complete, err := presetLayoutMigrationComplete(dataRoot)
	if err != nil {
		return err
	}
	if complete {
		return nil
	}

	backups := make([]string, 0, 5)
	for _, source := range presetLayoutMigrationSources(dataRoot) {
		backupPath, found, err := preservePresetLayoutSource(dataRoot, source)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		for _, destination := range source.destinations {
			copySource := backupPath
			if destination.sourceSubdirectory != "" {
				copySource = filepath.Join(copySource, destination.sourceSubdirectory)
			}
			if err := copyPresetLayoutDirectory(copySource, destination.path); err != nil {
				return fmt.Errorf("restore preset catalog %s: %w", destination.path, err)
			}
		}
		relative, err := filepath.Rel(dataRoot, backupPath)
		if err != nil {
			return fmt.Errorf("resolve preset backup path %s: %w", backupPath, err)
		}
		backups = append(backups, filepath.ToSlash(relative))
	}

	receipt := presetLayoutMigrationReceipt{
		Version: presetLayoutMigrationVersion, CompletedAt: time.Now().UTC(), Backups: backups,
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preset layout migration receipt: %w", err)
	}
	if err := writeMigrationFileAtomic(presetLayoutMigrationReceiptPath(dataRoot), append(raw, '\n')); err != nil {
		return fmt.Errorf("write preset layout migration receipt: %w", err)
	}
	if len(backups) > 0 {
		slog.Info("[internal/app/preset_layout_migration.go] migrated preset catalogs",
			"preset_root", presetlayout.Root(dataRoot), "backups", backups)
	}
	return nil
}

func presetLayoutMigrationSources(dataRoot string) []presetLayoutMigrationSource {
	return []presetLayoutMigrationSource{
		{
			oldDirectory: "story-tellers",
			destinations: []presetLayoutMigrationDestination{{path: presetlayout.NarrativeStyles(dataRoot)}},
		},
		{
			oldDirectory: "image-presets",
			destinations: []presetLayoutMigrationDestination{{path: presetlayout.Image(dataRoot)}},
		},
		{
			oldDirectory: "game-planning-templates",
			destinations: []presetLayoutMigrationDestination{{path: presetlayout.GamePlanning(dataRoot)}},
		},
		{
			oldDirectory: "story-director-modules",
			destinations: []presetLayoutMigrationDestination{
				{sourceSubdirectory: "event-packages", path: presetlayout.EventPackages(dataRoot)},
				{sourceSubdirectory: "rule-systems", path: presetlayout.RuleSystems(dataRoot)},
				{sourceSubdirectory: "actor-states", path: presetlayout.ActorStates(dataRoot)},
			},
		},
		{
			oldDirectory: "story-directors",
			destinations: []presetLayoutMigrationDestination{{path: presetlayout.LegacyGamePresets(dataRoot)}},
		},
	}
}

func preservePresetLayoutSource(dataRoot string, source presetLayoutMigrationSource) (string, bool, error) {
	sourcePath := filepath.Join(dataRoot, source.oldDirectory)
	backupPath := filepath.Join(dataRoot, "backups", presetLayoutBackupDirectory, source.oldDirectory)
	sourceExists, err := presetLayoutDirectoryExists(sourcePath)
	if err != nil {
		return "", false, err
	}
	backupExists, err := presetLayoutDirectoryExists(backupPath)
	if err != nil {
		return "", false, err
	}
	if !sourceExists {
		return backupPath, backupExists, nil
	}
	if backupExists {
		return "", false, fmt.Errorf("preset layout migration found both source and backup directories: %s and %s", sourcePath, backupPath)
	}
	for _, destination := range source.destinations {
		destinationExists, err := presetLayoutDirectoryExists(destination.path)
		if err != nil {
			return "", false, err
		}
		if destinationExists {
			return "", false, fmt.Errorf("preset layout migration found both source and destination directories: %s and %s", sourcePath, destination.path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		return "", false, fmt.Errorf("create preset migration backup root: %w", err)
	}
	if err := os.Rename(sourcePath, backupPath); err != nil {
		return "", false, fmt.Errorf("preserve preset directory %s: %w", sourcePath, err)
	}
	return backupPath, true, nil
}

func copyPresetLayoutDirectory(source, destination string) error {
	sourceExists, err := presetLayoutDirectoryExists(source)
	if err != nil {
		return err
	}
	if !sourceExists {
		return nil
	}
	destinationExists, err := presetLayoutDirectoryExists(destination)
	if err != nil {
		return err
	}
	if destinationExists {
		return nil
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".preset-layout-*")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := os.CopyFS(temporary, os.DirFS(source)); err != nil {
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return err
	}
	committed = true
	return nil
}

func presetLayoutDirectoryExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect preset directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("preset layout path is not a directory: %s", path)
	}
	return true, nil
}

func presetLayoutMigrationComplete(dataRoot string) (bool, error) {
	path := presetLayoutMigrationReceiptPath(dataRoot)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read preset layout migration receipt: %w", err)
	}
	var receipt presetLayoutMigrationReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return false, fmt.Errorf("decode preset layout migration receipt: %w", err)
	}
	if receipt.Version != presetLayoutMigrationVersion {
		return false, fmt.Errorf(
			"preset layout migration receipt version %d does not match supported version %d",
			receipt.Version, presetLayoutMigrationVersion,
		)
	}
	return true, nil
}

func presetLayoutMigrationReceiptPath(dataRoot string) string {
	return filepath.Join(dataRoot, "backups", presetLayoutBackupDirectory, "migration.json")
}
