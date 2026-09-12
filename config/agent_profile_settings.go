package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"denova/internal/revisionfile"

	"github.com/gofrs/flock"
)

// MutateUserSettings handles ordinary config.toml mutations. Agent Profile
// writes must go through the Agents Project change service so they share its
// conflict checks and recoverable history.
func MutateUserSettings(dataDir, expectedRevision string, mutate func(Settings) (Settings, error)) (string, error) {
	plan, err := PlanUserSettingsMutation(dataDir, UserSettingsMutationRequest{ExpectedRevision: expectedRevision, Mutate: mutate})
	if err != nil {
		return "", err
	}
	if len(plan.Profiles) != 0 {
		return "", errors.New("Agent Profile mutations require the Agents Project change service")
	}
	if plan.Config != nil {
		if err := WriteSettingsFileIfRevision(UserConfigPath(dataDir), plan.Config.Settings, plan.Config.BaseRevision); err != nil {
			return "", err
		}
	}
	return UserSettingsRevision(dataDir)
}

// writeAgentProfileSettingsLocked imports the backed-up released configuration
// during initial migration only. Runtime edits use file-local change intents.
func writeAgentProfileSettingsLocked(dataDir string, settings Settings) error {
	root := AgentProfilesRoot(dataDir)
	if err := os.MkdirAll(filepath.Join(root, "main"), 0o755); err != nil {
		return err
	}
	defaults, err := encodeAgentProfileDefaults(settings)
	if err != nil {
		return err
	}
	if err := writeAgentProfileFile(filepath.Join(root, "main", agentProfileDefaultsFilename), defaults); err != nil {
		return err
	}
	for _, profile := range fixedAgentProfiles {
		content, err := encodeMainAgentProfile(settings, profile)
		if err != nil {
			return err
		}
		if err := writeAgentProfileFile(filepath.Join(root, "main", profile.Filename), content); err != nil {
			return err
		}
	}
	if err := syncCustomAgentProfiles(filepath.Join(root, "custom"), SanitizeCustomAgents(settings.CustomAgents)); err != nil {
		return err
	}
	if err := syncSubAgentProfiles(filepath.Join(root, "subagents"), SanitizeSubAgents(settings.SubAgents)); err != nil {
		return err
	}
	return nil
}

func syncCustomAgentProfiles(directory string, agents []CustomAgentConfig) error {
	desired := make(map[string]bool, len(agents))
	for _, agent := range agents {
		name := agent.ID + ".toml"
		desired[name] = true
		content, err := encodeCustomAgentProfile(agent)
		if err != nil {
			return err
		}
		if err := writeAgentProfileFile(filepath.Join(directory, name), content); err != nil {
			return err
		}
	}
	return removeMissingValidProfiles(directory, desired, func(path string, content []byte) error {
		_, err := decodeCustomAgentProfile(path, content)
		return err
	})
}

func syncSubAgentProfiles(directory string, agents []SubAgentConfig) error {
	desired := make(map[string]bool, len(agents))
	for _, agent := range agents {
		name := agent.ID + ".toml"
		desired[name] = true
		content, err := encodeSubAgentProfile(agent)
		if err != nil {
			return err
		}
		if err := writeAgentProfileFile(filepath.Join(directory, name), content); err != nil {
			return err
		}
	}
	return removeMissingValidProfiles(directory, desired, func(path string, content []byte) error {
		_, err := decodeSubAgentProfile(path, content)
		return err
	})
}

func removeMissingValidProfiles(directory string, desired map[string]bool, validate func(string, []byte) error) error {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || desired[entry.Name()] || !strings.EqualFold(filepath.Ext(entry.Name()), ".toml") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil || validate(path, content) != nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove retired Agent Profile %s: %w", path, err)
		}
	}
	return nil
}

func writeAgentProfileFile(path string, content []byte) error {
	if _, err := revisionfile.ReplaceIfRevision(
		context.Background(), path, "", content,
		revisionfile.Options{FileMode: 0o644, DirectoryMode: 0o755},
	); err != nil {
		return fmt.Errorf("write Agent Profile %s: %w", path, err)
	}
	return nil
}

func hasAgentProfileSettings(settings Settings) bool {
	return !reflect.DeepEqual(agentProfileSettings(settings), Settings{})
}

// UserSettingsRevision covers the ordinary config plus every Profile file for
// stale UI snapshot detection. It is not a cross-file commit or lock token.
func UserSettingsRevision(dataDir string) (string, error) {
	if _, err := os.Stat(agentProfilesMarkerPath(dataDir)); errors.Is(err, fs.ErrNotExist) {
		return SettingsFileRevision(UserConfigPath(dataDir))
	} else if err != nil {
		return "", err
	}
	lock := flock.New(agentProfilesLockPath(dataDir))
	if err := lock.Lock(); err != nil {
		return "", err
	}
	defer func() { _ = lock.Unlock() }()
	return userSettingsRevisionLocked(dataDir)
}

func userSettingsRevisionLocked(dataDir string) (string, error) {
	plain, files, err := readUserSettingsFiles(dataDir)
	if err != nil {
		return "", err
	}
	return userSettingsFilesRevision(plain, files), nil
}

func userSettingsFilesRevision(plain revisionfile.Snapshot, files map[string]revisionfile.Snapshot) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("config\x00" + plain.Revision + "\x00"))
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		_, _ = hash.Write([]byte(path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(files[path].Content)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
