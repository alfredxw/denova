package platform

import (
	"slices"
	"sort"

	"github.com/Masterminds/semver/v3"
)

func (m *Manager) resolveDependencies(manifest Manifest, requested []DependencyPin) ([]DependencyPin, error) {
	installed, err := m.List(Plugin)
	if err != nil {
		return nil, err
	}
	return resolveAvailableDependencies(manifest, requested, installed)
}

func resolveAvailableDependencies(manifest Manifest, requested []DependencyPin, installed []Installed) ([]DependencyPin, error) {
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

// ModelRequirement is the resolved binding used by setup forms and admission.
// Keys include the provider namespace; definitions and credentials stay private.
type ModelRequirement struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}

func (m *Manager) modelRequirements(release Release, pins []DependencyPin) ([]ModelRequirement, error) {
	releases := []Release{release}
	for _, pin := range pins {
		dep, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: pin.PluginID}, ReleaseID: pin.ReleaseID})
		if err != nil {
			return nil, err
		}
		releases = append(releases, dep)
	}
	result := []ModelRequirement{}
	for _, current := range releases {
		for _, slot := range current.Manifest.ModelSlots {
			key := current.Manifest.ID + "/" + slot.ID
			if current.Ref.Package.Kind == Game {
				key = "local:" + slot.ID
			}
			result = append(result, ModelRequirement{Key: key, Kind: slot.Kind, Required: slot.Required})
		}
	}
	if release.Manifest.Game != nil && slices.Contains(release.Manifest.Game.Uses.Agents, "builtin/assistant") {
		result = append(result, ModelRequirement{Key: "builtin/assistant", Kind: "text", Required: true})
	}
	return result, nil
}

func (m *Manager) validateModels(release Release, pins []DependencyPin, projectID string, models map[string]string) error {
	slots, err := m.modelRequirements(release, pins)
	if err != nil {
		return err
	}
	for _, slot := range slots {
		if slot.Required && (projectID == "" || models[slot.Key] == "") {
			return failure("NOT_CONFIGURED", "Select a Project and model for %s", slot.Key)
		}
	}
	return nil
}

// ExportDependencies resolves the same current plugin releases that a new
// consumer would use. Export callers include those releases in their bundle.
func (m *Manager) ExportDependencies(manifest Manifest) ([]DependencyPin, error) {
	return m.resolveDependencies(manifest, nil)
}
