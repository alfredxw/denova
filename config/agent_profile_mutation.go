package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"denova/internal/revisionfile"
	"github.com/gofrs/flock"
)

var ErrInvalidAgentProfile = errors.New("invalid Agent Profile")

// UserSettingsMutationRequest describes a settings edit. ProfilePaths names
// explicitly targeted profiles, including no-op resets, so invalid targets
// cannot be silently interpreted as empty settings. Paths are relative to the
// Agents Project. Mutate must not perform I/O or retain its settings argument.
type UserSettingsMutationRequest struct {
	ExpectedRevision string
	ProfilePaths     []string
	Mutate           func(Settings) (Settings, error)
}

type AgentProfileOperation string

const (
	AgentProfileReplace AgentProfileOperation = "replace"
	AgentProfileDelete  AgentProfileOperation = "delete"
)

// AgentProfileChange is an ephemeral, single-file intent. The caller must
// commit it through the shared Project change service using BaseRevision.
// Err is local to this target and must never be treated as an empty document.
type AgentProfileChange struct {
	Path         string
	BaseRevision string
	Operation    AgentProfileOperation
	Content      []byte
	Err          error
}

type UserConfigChange struct {
	BaseRevision string
	Settings     Settings
}

// UserSettingsMutation is a set of independent commits, never a transaction.
// Only changed files and invalid explicit targets appear in the plan.
type UserSettingsMutation struct {
	Config   *UserConfigChange
	Profiles []AgentProfileChange
}

// PlanUserSettingsMutation compares decoded partitions while retaining the
// exact bytes used to read them. Untouched files, including invalid files and
// comments, are never serialized back. The aggregate revision is a stale-view
// check; each commit must still check its own captured file revision.
func PlanUserSettingsMutation(dataDir string, request UserSettingsMutationRequest) (UserSettingsMutation, error) {
	if request.Mutate == nil {
		return UserSettingsMutation{}, errors.New("settings mutator is nil")
	}
	if err := EnsureAgentProfiles(dataDir); err != nil {
		return UserSettingsMutation{}, err
	}
	lock := flock.New(agentProfilesLockPath(dataDir))
	if err := lock.Lock(); err != nil {
		return UserSettingsMutation{}, err
	}
	defer func() { _ = lock.Unlock() }()
	// Complete released config migrations before capturing the mutation base.
	if _, err := ReadSettingsFile(UserConfigPath(dataDir)); err != nil {
		return UserSettingsMutation{}, err
	}
	plainFile, files, err := readUserSettingsFiles(dataDir)
	if err != nil {
		return UserSettingsMutation{}, err
	}
	if request.ExpectedRevision != "" && request.ExpectedRevision != userSettingsFilesRevision(plainFile, files) {
		return UserSettingsMutation{}, ErrSettingsRevisionConflict
	}
	plain, err := decodeSettingsFile(UserConfigPath(dataDir), plainFile.Content)
	if err != nil {
		return UserSettingsMutation{}, err
	}
	root := AgentProfilesRoot(dataDir)
	profiles, err := loadAgentProfileSettingsUsing(root, func(path string) ([]byte, error) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		file, ok := files[filepath.ToSlash(rel)]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return file.Content, nil
	})
	if err != nil {
		return UserSettingsMutation{}, err
	}
	current := mergeAgentProfileLayer(plain, profiles)
	before, err := encodeAgentProfileFiles(current)
	if err != nil {
		return UserSettingsMutation{}, err
	}
	// Compare the ordinary layer before invoking a callback that may edit slices.
	clearAgentProfileSettings(&plain)
	plainBefore, err := encodePlainSettings(plain)
	if err != nil {
		return UserSettingsMutation{}, err
	}
	next, err := request.Mutate(current)
	if err != nil {
		return UserSettingsMutation{}, err
	}
	next = sanitizeEditableSettings(next)
	after, err := encodeAgentProfileFiles(next)
	if err != nil {
		return UserSettingsMutation{}, err
	}
	clearAgentProfileSettings(&next)
	plainAfter, err := encodePlainSettings(next)
	if err != nil {
		return UserSettingsMutation{}, err
	}
	plan := UserSettingsMutation{}
	if !bytes.Equal(plainBefore, plainAfter) {
		plan.Config = &UserConfigChange{BaseRevision: plainFile.Revision, Settings: next}
	}
	paths := map[string]bool{}
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	for _, path := range request.ProfilePaths {
		paths[path] = true
	}
	explicit := map[string]bool{}
	for _, path := range request.ProfilePaths {
		explicit[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		changed := !bytes.Equal(before[path], after[path])
		if !changed && !explicit[path] {
			continue
		}
		base := files[path]
		if !base.Exists {
			base.Revision = revisionfile.MissingRevision
		}
		change := AgentProfileChange{Path: path, BaseRevision: base.Revision, Operation: AgentProfileReplace, Content: after[path]}
		if base.Exists {
			if err := validateAgentProfileFile(path, base.Content); err != nil {
				change.Err = fmt.Errorf("%w: %s: %v", ErrInvalidAgentProfile, path, err)
			}
		}
		if change.Err == nil && !changed {
			continue
		}
		if _, exists := after[path]; !exists {
			change.Operation = AgentProfileDelete
		}
		plan.Profiles = append(plan.Profiles, change)
	}
	return plan, nil
}

func readUserSettingsFiles(dataDir string) (revisionfile.Snapshot, map[string]revisionfile.Snapshot, error) {
	plain, err := revisionfile.Read(context.Background(), UserConfigPath(dataDir))
	if err != nil {
		return plain, nil, err
	}
	files := map[string]revisionfile.Snapshot{}
	for _, dir := range []string{"main", "custom", "subagents"} {
		entries, err := os.ReadDir(filepath.Join(AgentProfilesRoot(dataDir), dir))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return plain, nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toml") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := dir + "/" + entry.Name()
			file, err := revisionfile.Read(context.Background(), filepath.Join(AgentProfilesRoot(dataDir), filepath.FromSlash(path)))
			if err != nil {
				return plain, nil, err
			}
			if file.Exists {
				files[path] = file
			}
		}
	}
	return plain, files, nil
}
