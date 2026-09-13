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
	"sort"
	"strings"
	"time"

	"denova/internal/portablepath"
	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
)

func (m *Manager) resolveDependencies(manifest Manifest, requested []DependencyPin) ([]DependencyPin, error) {
	installed, err := m.List(Plugin)
	if err != nil {
		return nil, err
	}
	available := map[string]Installed{}
	for _, item := range installed {
		available[item.ID] = item
	}
	chosen := map[string]Release{}
	visiting := map[string]bool{}
	if manifest.Contributes != nil {
		visiting[manifest.ID] = true
	}
	var visit func(Manifest) error
	visit = func(parent Manifest) error {
		for _, dependency := range parent.Requires {
			if visiting[dependency.PluginID] {
				return failure("DEPENDENCY_UNAVAILABLE", "Dependency cycle at %s", dependency.PluginID)
			}
			item, ok := available[dependency.PluginID]
			if !ok || !item.Enabled || item.Removed {
				return failure("DEPENDENCY_UNAVAILABLE", "Plugin %s is unavailable", dependency.PluginID)
			}
			constraint, err := semver.NewConstraint(dependency.VersionRange)
			if err != nil {
				return failure("DEPENDENCY_UNAVAILABLE", "Invalid version range: %v", err)
			}
			release, exists := chosen[dependency.PluginID]
			if !exists {
				pin := ""
				for _, requestedPin := range requested {
					if requestedPin.PluginID == dependency.PluginID {
						pin = requestedPin.ReleaseID
					}
				}
				// New consumers use the current installation. Existing consumers
				// retain explicit snapshots, including same-version source updates.
				if pin == "" {
					pin = item.CurrentRelease
				}
				for _, candidate := range item.Releases {
					version, err := semver.StrictNewVersion(candidate.Manifest.Version)
					if err == nil && constraint.Check(version) && candidate.Ref.ReleaseID == pin {
						release = candidate
						break
					}
				}
			}
			version, parseErr := semver.StrictNewVersion(release.Manifest.Version)
			if parseErr != nil || !constraint.Check(version) {
				return failure("DEPENDENCY_UNAVAILABLE", "No single selected release of %s satisfies %s", dependency.PluginID, dependency.VersionRange)
			}
			if release.Manifest.APIMajor != APIMajor {
				return failure("API_INCOMPATIBLE", "Plugin %s requires API %d", dependency.PluginID, release.Manifest.APIMajor)
			}
			ids := contributionIDs(release.Manifest.contributions())
			for _, id := range dependency.Contributions {
				if !slices.Contains(ids, id) {
					return failure("DEPENDENCY_UNAVAILABLE", "Plugin %s has no contribution %s", dependency.PluginID, id)
				}
			}
			if !exists {
				chosen[dependency.PluginID] = release
				visiting[dependency.PluginID] = true
				if err := visit(release.Manifest); err != nil {
					return err
				}
				delete(visiting, dependency.PluginID)
			}
		}
		return nil
	}
	if err := visit(manifest); err != nil {
		return nil, err
	}
	pins := make([]DependencyPin, 0, len(chosen))
	for id, release := range chosen {
		pins = append(pins, DependencyPin{PluginID: id, ReleaseID: release.Ref.ReleaseID})
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].PluginID < pins[j].PluginID })
	for _, pin := range requested {
		if _, ok := chosen[pin.PluginID]; !ok {
			return nil, failure("DEPENDENCY_UNAVAILABLE", "Unneeded dependency pin %s", pin.PluginID)
		}
	}
	return pins, nil
}

func contributionIDs(c Contributions) []string {
	ids := []string{}
	for _, item := range c.Tools {
		ids = append(ids, item.ID)
	}
	for _, item := range c.Toolsets {
		ids = append(ids, item.ID)
	}
	return ids
}

type CreateInstance struct {
	GameID    string            `json:"gameId"`
	ReleaseID string            `json:"releaseId"`
	Title     string            `json:"title"`
	ProjectID string            `json:"projectId,omitempty"`
	Setup     map[string]any    `json:"setup"`
	Models    map[string]string `json:"models"`
	Preview   bool              `json:"preview"`
}

func (m *Manager) CreateInstance(request CreateInstance) (Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, item, err := m.release(ReleaseRef{Package: PackageRef{Kind: Game, ID: request.GameID}, ReleaseID: request.ReleaseID})
	if err != nil {
		return Instance{}, err
	}
	if request.Preview != strings.HasPrefix(request.ReleaseID, "preview-") {
		return Instance{}, failure("INVALID_ARGUMENT", "Preview releases require isolated test instances")
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
	instance := Instance{ID: uuid.NewString(), GameID: request.GameID, ReleaseID: request.ReleaseID, Title: title, ProjectID: request.ProjectID, Dependencies: pins, Setup: configuration, Models: request.Models, Preview: request.Preview, CreatedAt: time.Now().UTC()}
	if instance.Models == nil {
		instance.Models = map[string]string{}
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
	if err := writeJSON(filepath.Join(m.instancePath(instance.GameID, id), "instance.json"), instance); err != nil {
		return Instance{}, err
	}
	slog.Info("platform_game_instance_renamed", "instance", id)
	return instance, nil
}

func (m *Manager) validateModels(release Release, pins []DependencyPin, projectID string, models map[string]string) error {
	if release.Manifest.Game != nil && slices.Contains(release.Manifest.Game.Uses.Agents, "builtin/assistant") && (projectID == "" || models["builtin/assistant"] == "") {
		return failure("NOT_CONFIGURED", "Select a Project and model for builtin/assistant")
	}
	releases := []Release{release}
	for _, pin := range pins {
		dep, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: pin.PluginID}, ReleaseID: pin.ReleaseID})
		if err != nil {
			return err
		}
		releases = append(releases, dep)
	}
	for _, current := range releases {
		for _, slot := range current.Manifest.ModelSlots {
			key := current.Manifest.ID + "/" + slot.ID
			if current.Ref.Package.Kind == Game {
				key = "local:" + slot.ID
			}
			if slot.Required && (projectID == "" || models[key] == "") {
				return failure("NOT_CONFIGURED", "Select a Project and model for %s/%s", current.Manifest.ID, slot.ID)
			}
		}
	}
	return nil
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
	err = zipDirectory(m.instancePath(instance.GameID, instance.ID), file)
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
