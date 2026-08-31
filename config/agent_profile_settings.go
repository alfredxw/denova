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

// MutateUserSettings atomically serializes all Denova-owned user settings
// mutations while persisting Agent fields only into the Agents Project.
func MutateUserSettings(
	dataDir string,
	expectedRevision string,
	mutate func(Settings) (Settings, error),
) (string, error) {
	if mutate == nil {
		return "", errors.New("settings mutator is nil")
	}
	if err := EnsureAgentProfiles(dataDir); err != nil {
		return "", err
	}
	lock := flock.New(agentProfilesLockPath(dataDir))
	if err := lock.Lock(); err != nil {
		return "", fmt.Errorf("lock user settings: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	currentRevision, err := userSettingsRevisionLocked(dataDir)
	if err != nil {
		return "", err
	}
	if expectedRevision != "" && currentRevision != expectedRevision {
		return "", ErrSettingsRevisionConflict
	}
	userPath := UserConfigPath(dataDir)
	plain, err := ReadSettingsFile(userPath)
	if err != nil {
		return "", err
	}
	profiles, _, err := loadAgentProfileSettingsLocked(dataDir)
	if err != nil {
		return "", err
	}
	current := mergeAgentProfileLayer(plain, profiles)
	next, err := mutate(current)
	if err != nil {
		return "", err
	}
	next = sanitizeEditableSettings(next)

	nextPlain := next
	clearAgentProfileSettings(&nextPlain)
	currentPlain := plain
	clearAgentProfileSettings(&currentPlain)
	if !reflect.DeepEqual(currentPlain, nextPlain) {
		if err := WriteSettingsFile(userPath, nextPlain); err != nil {
			return "", err
		}
	}
	nextProfiles := agentProfileSettings(next)
	if !reflect.DeepEqual(agentProfileSettings(current), nextProfiles) {
		if err := writeAgentProfileSettingsLocked(dataDir, nextProfiles); err != nil {
			return "", err
		}
	}
	return userSettingsRevisionLocked(dataDir)
}

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

// UserSettingsRevision covers the ordinary user config plus every Agent
// Profile TOML file. UI saves therefore cannot silently overwrite a profile
// edited by the Agents Project's General Agent.
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
	hash := sha256.New()
	configSnapshot, err := revisionfile.Read(context.Background(), UserConfigPath(dataDir))
	if err != nil {
		return "", err
	}
	_, _ = hash.Write([]byte("config\x00" + configSnapshot.Revision + "\x00"))
	root := AgentProfilesRoot(dataDir)
	var paths []string
	for _, directory := range []string{"main", "custom", "subagents"} {
		entries, readErr := os.ReadDir(filepath.Join(root, directory))
		if errors.Is(readErr, fs.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return "", readErr
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toml") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)
	for _, relative := range paths {
		content, readErr := os.ReadFile(filepath.Join(root, relative))
		if readErr != nil {
			return "", readErr
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
