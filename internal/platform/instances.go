package platform

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"denova/internal/portablepath"
	"github.com/google/uuid"
)

type CreateInstance struct {
	GameID      string            `json:"gameId"`
	ReleaseID   string            `json:"releaseId"`
	Title       string            `json:"title"`
	ProjectID   string            `json:"projectId,omitempty"`
	StoryID     string            `json:"storyId,omitempty"`
	StoryOrigin string            `json:"storyOrigin,omitempty"`
	Setup       map[string]any    `json:"setup"`
	Models      map[string]string `json:"models"`
}

func (m *Manager) CreateInstance(request CreateInstance) (Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, item, err := m.release(ReleaseRef{Package: PackageRef{Kind: Game, ID: request.GameID}, ReleaseID: request.ReleaseID})
	if err != nil {
		return Instance{}, err
	}
	if item.Removed || !item.Enabled {
		return Instance{}, failure("DEPENDENCY_UNAVAILABLE", "Game %s is unavailable", item.ID)
	}
	if release.Manifest.APIMajor != APIMajor {
		return Instance{}, failure("API_INCOMPATIBLE", "Game requires API %d", release.Manifest.APIMajor)
	}
	pins, err := m.resolveDependencies(release.Manifest, nil)
	if err != nil {
		return Instance{}, err
	}
	if request.ProjectID != "" {
		if _, _, err := m.registry.Resolve(request.ProjectID, true); err != nil {
			return Instance{}, err
		}
	}
	configuration, err := m.gameSetup(release, request.Setup)
	if err != nil {
		return Instance{}, err
	}
	if err := m.validateModels(release, pins, request.ProjectID, request.Models); err != nil {
		return Instance{}, err
	}
	title := strings.TrimSpace(request.Title)
	if title == "" || len(title) > 512 {
		return Instance{}, failure("INVALID_ARGUMENT", "Instance title must contain 1..512 bytes")
	}
	instance := Instance{ID: uuid.NewString(), GameID: request.GameID, ReleaseID: request.ReleaseID, Title: title, ProjectID: request.ProjectID, Dependencies: pins, Setup: configuration, Models: request.Models, Preview: release.Ref.Environment() == "preview", CreatedAt: time.Now().UTC()}
	if instance.Models == nil {
		instance.Models = map[string]string{}
	}
	if release.Manifest.Game.Storage.Kind == "story" {
		if m.stories == nil {
			return Instance{}, failure("UNSUPPORTED", "Story host is unavailable")
		}
		if request.ProjectID == "" {
			return Instance{}, failure("NOT_CONFIGURED", "Select a Project for this Story")
		}
		instance.StoryID = request.StoryID
		options := StoryBindingOptions{Origin: request.StoryOrigin, ModelProfile: request.Models["local:"+release.Manifest.Game.Story.ModelSlot]}
		instance, err = m.stories.Bind(context.Background(), instance, options)
		if err != nil {
			return Instance{}, err
		}
		slog.Info("platform_story_instance_created", "game", instance.GameID, "instance", instance.ID, "story", instance.StoryID)
		return instance, nil
	}
	if request.StoryID != "" {
		return Instance{}, failure("INVALID_ARGUMENT", "Only Story games can bind an existing Story")
	}
	if err := os.MkdirAll(filepath.Join(m.instancePath(instance.GameID, instance.ID), "data"), 0o700); err != nil {
		return Instance{}, err
	}
	if err := writeJSON(filepath.Join(m.instancePath(instance.GameID, instance.ID), "instance.json"), instance); err != nil {
		return Instance{}, err
	}
	slog.Info("platform_game_instance_created", "game", instance.GameID, "instance", instance.ID, "preview", instance.Preview)
	return instance, nil
}

// RenameInstance changes display metadata only, preserving the save and release binding.
func (m *Manager) RenameInstance(id, title string) (Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 512 {
		return Instance{}, failure("INVALID_ARGUMENT", "Instance title must contain 1..512 bytes")
	}
	instance, err := m.Instance(id)
	if err != nil {
		return Instance{}, err
	}
	instance.Title = title
	if instance.StoryID != "" {
		if err := m.stories.SaveBinding(context.Background(), instance); err != nil {
			return Instance{}, err
		}
		slog.Info("platform_story_instance_renamed", "instance", id)
		return instance, nil
	}
	if err := writeJSON(filepath.Join(m.instancePath(instance.GameID, id), "instance.json"), instance); err != nil {
		return Instance{}, err
	}
	slog.Info("platform_game_instance_renamed", "instance", id)
	return instance, nil
}

func (m *Manager) UpgradeInstance(ctx context.Context, id, releaseID string, pins []DependencyPin) (Instance, error) {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	if err := m.stopLocked(ctx, id); err != nil {
		return Instance{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	instance, err := m.Instance(id)
	if err != nil {
		return Instance{}, err
	}
	old, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Game, ID: instance.GameID}, ReleaseID: instance.ReleaseID})
	if err != nil {
		return Instance{}, err
	}
	next, _, err := m.release(ReleaseRef{Package: old.Ref.Package, ReleaseID: releaseID})
	if err != nil {
		return Instance{}, err
	}
	if next.Manifest.Game.Storage.SaveFormat == "" || next.Manifest.Game.Storage.SaveFormat != old.Manifest.Game.Storage.SaveFormat {
		return Instance{}, failure("SAVE_INCOMPATIBLE", "Author must declare the same nonempty saveFormat for an instance upgrade")
	}
	if next.Manifest.Game.Storage.Kind != old.Manifest.Game.Storage.Kind {
		return Instance{}, failure("SAVE_INCOMPATIBLE", "Storage ownership cannot change during an upgrade")
	}
	if next.Manifest.APIMajor != APIMajor {
		return Instance{}, failure("API_INCOMPATIBLE", "Target game release requires API %d", next.Manifest.APIMajor)
	}
	// A game update must not implicitly replace a saved NPC's provider. New
	// dependencies may be selected, but existing pins remain unless explicitly
	// changed by the caller. Incompatible ranges fail before touching save data.
	if pins == nil {
		pins = instance.Dependencies
	}
	dependencies, err := m.resolveDependencies(next.Manifest, pins)
	if err != nil {
		return Instance{}, err
	}
	configuration, err := m.gameSetup(next, instance.Setup)
	if err != nil {
		return Instance{}, err
	}
	if err := m.validateModels(next, dependencies, instance.ProjectID, instance.Models); err != nil {
		return Instance{}, err
	}
	if _, err := m.backupInstance(instance); err != nil {
		return Instance{}, err
	}
	instance.Setup = configuration
	if err := m.checkUpgrade(ctx, instance, next, dependencies); err != nil {
		return Instance{}, err
	}
	instance.ReleaseID = releaseID
	instance.Dependencies = dependencies
	if instance.StoryID != "" {
		if err := m.stories.SaveBinding(ctx, instance); err != nil {
			return Instance{}, err
		}
		slog.Info("platform_story_instance_upgraded", "instance", id, "release", releaseID)
		return instance, nil
	}
	if err := writeJSON(filepath.Join(m.instancePath(instance.GameID, id), "instance.json"), instance); err != nil {
		return Instance{}, err
	}
	slog.Info("platform_game_instance_upgraded", "instance", id, "release", releaseID)
	return instance, nil
}

func (m *Manager) ExportInstance(ctx context.Context, id string, writer io.Writer) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	if err := m.stopLocked(ctx, id); err != nil {
		return err
	}
	instance, err := m.Instance(id)
	if err != nil {
		return err
	}
	if instance.StoryID != "" {
		return m.stories.Export(ctx, instance, writer)
	}
	return zipDirectory(m.instancePath(instance.GameID, id), writer)
}

func (m *Manager) RemoveInstance(ctx context.Context, id string) (string, error) {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	if err := m.stopLocked(ctx, id); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	instance, err := m.Instance(id)
	if err != nil {
		return "", err
	}
	backup, err := m.backupInstance(instance)
	if err != nil {
		return "", err
	}
	if instance.StoryID != "" {
		if err := m.stories.RemoveBinding(ctx, instance); err != nil {
			return "", err
		}
		slog.Info("platform_story_instance_removed", "instance", id, "backup", backup)
		return backup, nil
	}
	if err := os.RemoveAll(m.instancePath(instance.GameID, id)); err != nil {
		return "", err
	}
	slog.Info("platform_game_instance_removed", "instance", id, "backup", backup)
	return backup, nil
}

func (m *Manager) backupInstance(instance Instance) (string, error) {
	relative := "games/" + instance.GameID + "/backups/" + instance.ID + "-" + uuid.NewString() + ".zip"
	path := filepath.Join(m.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if instance.StoryID != "" {
		err = m.stories.Export(context.Background(), instance, file)
	} else {
		err = zipDirectory(m.instancePath(instance.GameID, instance.ID), file)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return relative, nil
}

func zipDirectory(directory string, writer io.Writer) error {
	if err := portablepath.PreflightTree(directory); err != nil {
		return err
	}
	archive := zip.NewWriter(writer)
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		target, err := archive.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(target, file)
		return err
	})
	closeErr := archive.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// PackageAvailability controls catalog visibility and new activations. Removed
// packages retain their releases and saves until explicitly reinstalled.
type PackageAvailability struct {
	Enabled bool `json:"enabled"`
	Removed bool `json:"removed"`
}

// Disabling blocks new activations while current journeys and tasks finish.
// Removal explicitly stops affected runtimes, retaining releases and saves.
func (m *Manager) SetAvailability(ctx context.Context, kind Kind, id string, state PackageAvailability) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	if state.Removed {
		for key, runtime := range m.runtimes {
			if runtime.uses(kind, id) {
				if err := m.stopLocked(ctx, key); err != nil {
					return err
				}
			}
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items, err := m.List(kind)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(items, func(item Installed) bool { return item.ID == id })
	if index < 0 {
		return failure("NOT_FOUND", "Package %s is not installed", id)
	}
	items[index].Enabled = state.Enabled && !state.Removed
	items[index].Removed = state.Removed
	if err := m.saveInstalled(kind, items[index]); err != nil {
		return err
	}
	slog.Info("platform_package_availability_changed", "kind", kind, "package", id, "enabled", items[index].Enabled, "removed", state.Removed)
	return nil
}
