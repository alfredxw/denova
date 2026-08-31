package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"denova/internal/revisionfile"

	"github.com/gofrs/flock"
)

const (
	AgentProfilesDirectoryName = "agents"
	agentProfilesMarkerName    = ".profiles-v1"
	agentProfilesBackupSuffix  = ".v0.3.3-agent-profiles.bak"
)

const agentsProjectInstructions = `# Agents Project

This Project contains the user's editable Agent Profiles. Built-in runtime contracts remain product-owned and are not copied here.

- main/defaults.toml stores shared Agent defaults; the remaining main/*.toml files store one main Agent override partitioned by runtime.
- custom/*.toml stores one complete Custom Main Agent per stable ID.
- subagents/*.toml stores one delegated SubAgent per stable ID. SubAgents are never main Agents.
- Read trajectory://index only when evidence is needed. Trajectory is a read-only projection of Sessions, Runs, and Outcomes.
- Make the smallest evidence-backed change. Keep each file independently valid so one bad profile cannot invalidate the catalog.
- Use Project Versions to review or restore changes. There is one working state; there are no Draft, Candidate, or Published profile states.
`

// AgentProfilesRoot is both the editable Agent Profile root and the managed
// Agents Project content directory.
func AgentProfilesRoot(dataDir string) string {
	return filepath.Join(strings.TrimSpace(dataDir), AgentProfilesDirectoryName)
}

func agentProfilesMarkerPath(dataDir string) string {
	return filepath.Join(AgentProfilesRoot(dataDir), agentProfilesMarkerName)
}

func agentProfilesLockPath(dataDir string) string {
	return filepath.Join(strings.TrimSpace(dataDir), "runtime", "agent-profiles.lock")
}

// EnsureAgentProfiles performs the single v0.3.3 user-settings migration and
// seeds the system-managed Agents Project. The marker is written last, so an
// interrupted migration remains safely retryable without dual reads.
func EnsureAgentProfiles(dataDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return errors.New("Agent Profile data directory is required")
	}
	lockPath := agentProfilesLockPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return fmt.Errorf("create Agent Profile runtime directory: %w", err)
	}
	lock := flock.New(lockPath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("lock Agent Profiles: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	return ensureAgentProfilesLocked(dataDir)
}

func ensureAgentProfilesLocked(dataDir string) error {
	root := AgentProfilesRoot(dataDir)
	if err := os.MkdirAll(filepath.Join(root, "main"), 0o755); err != nil {
		return fmt.Errorf("create main Agent Profile directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "custom"), 0o755); err != nil {
		return fmt.Errorf("create Custom Main Agent Profile directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "subagents"), 0o755); err != nil {
		return fmt.Errorf("create SubAgent Profile directory: %w", err)
	}
	if err := ensureAgentsProjectInstructions(root); err != nil {
		return err
	}
	if _, err := os.Stat(agentProfilesMarkerPath(dataDir)); err == nil {
		return ensureFixedAgentProfiles(root)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect Agent Profile marker: %w", err)
	}

	userPath := UserConfigPath(dataDir)
	legacy, err := ReadSettingsFile(userPath)
	if err != nil {
		return err
	}
	legacyProfiles := agentProfileSettings(legacy)
	if hasAgentProfileSettings(legacyProfiles) {
		if err := preserveAgentProfileMigrationBackup(userPath); err != nil {
			return err
		}
		if err := writeAgentProfileSettingsLocked(dataDir, legacyProfiles); err != nil {
			return fmt.Errorf("import v0.3.3 Agent settings: %w", err)
		}
		cleaned := legacy
		clearAgentProfileSettings(&cleaned)
		if err := WriteSettingsFile(userPath, cleaned); err != nil {
			return fmt.Errorf("remove migrated Agent settings from user config: %w", err)
		}
		slog.Info("[config/agent_profiles.go] migrated v0.3.3 Agent settings into Agents Project",
			"source", userPath,
			"destination", root,
		)
	} else if err := ensureFixedAgentProfiles(root); err != nil {
		return err
	}
	if _, err := revisionfile.ReplaceIfRevision(
		context.Background(),
		agentProfilesMarkerPath(dataDir),
		"",
		[]byte("schema_version = 1\n"),
		revisionfile.Options{FileMode: 0o644, DirectoryMode: 0o755},
	); err != nil {
		return fmt.Errorf("write Agent Profile marker: %w", err)
	}
	return nil
}

func ensureAgentsProjectInstructions(root string) error {
	path := filepath.Join(root, "AGENTS.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect Agents Project instructions: %w", err)
	}
	if _, err := revisionfile.ReplaceIfRevision(
		context.Background(), path, "", []byte(agentsProjectInstructions),
		revisionfile.Options{FileMode: 0o644, DirectoryMode: 0o755},
	); err != nil {
		return fmt.Errorf("write Agents Project instructions: %w", err)
	}
	return nil
}

func ensureFixedAgentProfiles(root string) error {
	defaultsPath := filepath.Join(root, "main", agentProfileDefaultsFilename)
	if _, err := os.Stat(defaultsPath); errors.Is(err, fs.ErrNotExist) {
		content, encodeErr := encodeAgentProfileDefaults(Settings{})
		if encodeErr != nil {
			return encodeErr
		}
		if writeErr := writeAgentProfileFile(defaultsPath, content); writeErr != nil {
			return writeErr
		}
	} else if err != nil {
		return fmt.Errorf("inspect Agent Profile defaults %s: %w", defaultsPath, err)
	}
	for _, profile := range fixedAgentProfiles {
		path := filepath.Join(root, "main", profile.Filename)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("inspect main Agent Profile %s: %w", path, err)
		}
		content, err := encodeMainAgentProfile(Settings{}, profile)
		if err != nil {
			return err
		}
		if err := writeAgentProfileFile(path, content); err != nil {
			return err
		}
	}
	return nil
}

func preserveAgentProfileMigrationBackup(path string) error {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Agent settings migration source: %w", err)
	}
	backupPath := path + agentProfilesBackupSuffix
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect Agent settings migration backup: %w", err)
	}
	if _, err := revisionfile.ReplaceIfRevision(
		context.Background(), backupPath, "", content,
		revisionfile.Options{FileMode: 0o600, DirectoryMode: 0o755},
	); err != nil {
		return fmt.Errorf("write Agent settings migration backup: %w", err)
	}
	return nil
}

// LoadAgentProfileSettings loads only valid files. A malformed profile is
// isolated and logged so unrelated Agents remain runnable and repairable.
func LoadAgentProfileSettings(dataDir string) (Settings, bool, error) {
	lock := flock.New(agentProfilesLockPath(dataDir))
	if err := lock.Lock(); err != nil {
		return Settings{}, false, fmt.Errorf("lock Agent Profiles for reading: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	return loadAgentProfileSettingsLocked(dataDir)
}

func loadAgentProfileSettingsLocked(dataDir string) (Settings, bool, error) {
	if _, err := os.Stat(agentProfilesMarkerPath(dataDir)); errors.Is(err, fs.ErrNotExist) {
		return Settings{}, false, nil
	} else if err != nil {
		return Settings{}, false, fmt.Errorf("inspect Agent Profile marker: %w", err)
	}
	settings, err := loadAgentProfileSettingsFromRoot(AgentProfilesRoot(dataDir))
	return settings, true, err
}

// loadUserSettingsWithProfiles returns one serialized snapshot spanning the
// legacy user config and the Agent Profile files that replaced its Agent fields.
func loadUserSettingsWithProfiles(dataDir string) (Settings, error) {
	if err := EnsureAgentProfiles(dataDir); err != nil {
		return Settings{}, err
	}
	lock := flock.New(agentProfilesLockPath(dataDir))
	if err := lock.Lock(); err != nil {
		return Settings{}, fmt.Errorf("lock user settings for reading: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	plain, err := ReadSettingsFile(UserConfigPath(dataDir))
	if err != nil {
		return Settings{}, err
	}
	profiles, ready, err := loadAgentProfileSettingsLocked(dataDir)
	if err != nil {
		return Settings{}, err
	}
	if ready {
		plain = mergeAgentProfileLayer(plain, profiles)
	}
	return plain, nil
}

func loadAgentProfileSettingsFromRoot(root string) (Settings, error) {
	settings := Settings{}
	defaultsPath := filepath.Join(root, "main", agentProfileDefaultsFilename)
	if content, err := os.ReadFile(defaultsPath); err == nil {
		document, decodeErr := decodeAgentProfileDefaults(defaultsPath, content)
		if decodeErr != nil {
			slog.Warn("[config/agent_profiles.go] ignored invalid Agent Profile defaults", "path", defaultsPath, "error", decodeErr)
		} else {
			applyAgentProfileDefaults(&settings, document)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Settings{}, fmt.Errorf("read Agent Profile defaults %s: %w", defaultsPath, err)
	}
	for _, profile := range fixedAgentProfiles {
		path := filepath.Join(root, "main", profile.Filename)
		content, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Settings{}, fmt.Errorf("read main Agent Profile %s: %w", path, err)
		}
		document, err := decodeMainAgentProfile(path, content, profile.Kind)
		if err != nil {
			slog.Warn("[config/agent_profiles.go] ignored invalid main Agent Profile", "path", path, "error", err)
			continue
		}
		if err := applyMainAgentProfile(&settings, document, profile.Kind); err != nil {
			slog.Warn("[config/agent_profiles.go] ignored invalid main Agent Profile", "path", path, "error", err)
		}
	}
	settings.CustomAgents = loadCustomAgentProfiles(filepath.Join(root, "custom"))
	settings.SubAgents = loadSubAgentProfiles(filepath.Join(root, "subagents"))
	return sanitizeEditableSettings(settings), nil
}

func loadCustomAgentProfiles(directory string) []CustomAgentConfig {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		slog.Warn("[config/agent_profiles.go] could not list Custom Main Agent Profiles", "directory", directory, "error", err)
		return nil
	}
	result := make([]CustomAgentConfig, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toml") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			slog.Warn("[config/agent_profiles.go] ignored unreadable Custom Main Agent Profile", "path", path, "error", readErr)
			continue
		}
		agent, decodeErr := decodeCustomAgentProfile(path, content)
		if decodeErr != nil {
			slog.Warn("[config/agent_profiles.go] ignored invalid Custom Main Agent Profile", "path", path, "error", decodeErr)
			continue
		}
		result = append(result, agent)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func loadSubAgentProfiles(directory string) []SubAgentConfig {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		slog.Warn("[config/agent_profiles.go] could not list SubAgent Profiles", "directory", directory, "error", err)
		return nil
	}
	result := make([]SubAgentConfig, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toml") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			slog.Warn("[config/agent_profiles.go] ignored unreadable SubAgent Profile", "path", path, "error", readErr)
			continue
		}
		agent, decodeErr := decodeSubAgentProfile(path, content)
		if decodeErr != nil {
			slog.Warn("[config/agent_profiles.go] ignored invalid SubAgent Profile", "path", path, "error", decodeErr)
			continue
		}
		result = append(result, agent)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
