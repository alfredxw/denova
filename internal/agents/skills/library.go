package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"denova/internal/localfs"
)

const librarySettingsFile = ".denova-library.json"

// LibrarySettings belongs to a Skill root, not an Agent profile. Global settings
// control shared discovery and builtin/user availability; Project roots only
// control their own Skills. Keys are scope:name, never host paths.
type LibrarySettings struct {
	SharedEnabled bool            `json:"shared_enabled"`
	Disabled      map[string]bool `json:"disabled,omitempty"`
}

func readLibrarySettings(root string) (LibrarySettings, error) {
	var settings LibrarySettings
	if root == "" {
		return settings, nil
	}
	data, err := os.ReadFile(filepath.Join(root, librarySettingsFile))
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	err = json.Unmarshal(data, &settings)
	return settings, err
}

func configureDirectory(ctx context.Context, dir Directory) Directory {
	settings, err := readLibrarySettings(dir.settingsRoot)
	if err != nil {
		slog.ErrorContext(ctx, "Read Skill library settings failed", "scope", dir.Scope, "error", err)
		dir.disabled = true
		return dir
	}
	dir.disabled = dir.Scope == ScopeShared && !settings.SharedEnabled
	dir.disabledSkills = settings.Disabled
	return dir
}

// SetLibraryEnabled changes library availability without rewriting SKILL.md or
// overriding an Agent's narrower visibility policy.
func SetLibraryEnabled(ctx context.Context, dirs []Directory, scope Scope, name string, enabled bool) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return err
	}
	if _, _, err := skillDirectory(dirs, scope, name); err != nil {
		return err
	}
	return editLibrarySettings(ctx, dir.settingsRoot, func(settings *LibrarySettings) {
		if settings.Disabled == nil {
			settings.Disabled = map[string]bool{}
		}
		key := string(scope) + ":" + name
		if enabled {
			delete(settings.Disabled, key)
		} else {
			settings.Disabled[key] = true
		}
	})
}

// SetSharedEnabled only changes Denova's use of the host's shared library. It
// never writes into ~/.agents/skills or changes another application's state.
func SetSharedEnabled(ctx context.Context, dirs []Directory, enabled bool) error {
	dir, err := directoryForScope(dirs, ScopeUser)
	if err != nil {
		return err
	}
	return editLibrarySettings(ctx, dir.Path, func(settings *LibrarySettings) { settings.SharedEnabled = enabled })
}

func editLibrarySettings(ctx context.Context, root string, edit func(*LibrarySettings)) (err error) {
	if root == "" {
		return fmt.Errorf("Skill library settings directory is not configured")
	}
	release, err := localfs.AcquireLease(ctx, filepath.Join(root, ".denova-locks", "library.lock"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	settings, err := readLibrarySettings(root)
	if err != nil {
		return err
	}
	edit(&settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return atomicWriteSkillFile(dir, librarySettingsFile, data, 0o600)
}
