package platform

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"denova/internal/revisionfile"
)

// Permissions are host policy, separate from extension-defined configuration.
type PackagePermissions struct {
	ReleaseID string   `json:"releaseId"`
	Grants    []string `json:"grants"`
}

func (m *Manager) currentPackage(kind Kind, id string) (Release, Installed, error) {
	if err := validateID(id); err != nil {
		return Release{}, Installed{}, err
	}
	items, err := m.List(kind)
	if err != nil {
		return Release{}, Installed{}, err
	}
	for _, item := range items {
		if item.ID == id && !item.Removed {
			for _, release := range item.Releases {
				if release.Ref.ReleaseID == item.CurrentRelease {
					return release, item, nil
				}
			}
		}
	}
	return Release{}, Installed{}, failure("NOT_FOUND", "Installed extension %s is unavailable", id)
}

func (m *Manager) PackageConfiguration(kind Kind, id, locale string) (ConfigurationDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, _, err := m.currentPackage(kind, id)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	snapshot, err := revisionfile.Read(context.Background(), filepath.Join(m.packagePath(release.Ref.Package), "settings.toml"))
	if err != nil {
		return ConfigurationDocument{}, err
	}
	if len(snapshot.Content) > MaxDefinitionBytes {
		return ConfigurationDocument{}, failure("LIMIT_EXCEEDED", "Configuration exceeds %d bytes", MaxDefinitionBytes)
	}
	overrides, parseErr := parseConfiguration(snapshot.Content)
	if parseErr != nil {
		overrides = map[string]any{}
	}
	document, err := m.configurationDocument(release, overrides, locale)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	document.Revision = snapshot.Revision
	if parseErr != nil {
		// Keep the editor available when a hand-edited file needs repair.
		_, document.Problem = ErrorResponse(parseErr)
		document.TOML = string(snapshot.Content)
	}
	return document, nil
}

func configurationInput(input ConfigurationInput) (map[string]any, error) {
	switch input.Format {
	case "toml":
		return parseConfiguration([]byte(input.TOML))
	case "values":
		raw, err := json.Marshal(input.Overrides)
		if err != nil {
			return nil, err
		}
		if len(raw) > MaxDefinitionBytes {
			return nil, failure("LIMIT_EXCEEDED", "Configuration exceeds %d bytes", MaxDefinitionBytes)
		}
		parsed, err := configurationValues(input.Overrides)
		if err != nil {
			return nil, err
		}
		return parsed.(map[string]any), nil
	default:
		return nil, failure("INVALID_ARGUMENT", "Configuration format must be values or toml")
	}
}

func (m *Manager) ValidatePackageConfiguration(kind Kind, id, locale string, input ConfigurationInput) (ConfigurationDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, _, err := m.currentPackage(kind, id)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	return m.configurationFromInput(release, locale, input)
}

// Both conversion and saving validate the same release-bound editor input.
func (m *Manager) configurationFromInput(release Release, locale string, input ConfigurationInput) (ConfigurationDocument, error) {
	if input.ReleaseID != release.Ref.ReleaseID {
		return ConfigurationDocument{}, failure("DOCUMENT_CONFLICT", "Installed release changed; reload settings")
	}
	overrides, err := configurationInput(input)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	document, err := m.configurationDocument(release, overrides, locale)
	if err == nil && document.Problem != nil {
		return ConfigurationDocument{}, document.Problem
	}
	return document, err
}

// The caller holds both manager locks. Only current consumers, pinned saves and
// running tasks constrain shared settings; unused historic packages do not.
func (m *Manager) settingsReferences(ref PackageRef) ([]Release, error) {
	refs := map[ReleaseRef]bool{}
	add := func(candidate ReleaseRef) {
		if candidate.Package == ref {
			refs[candidate] = true
		}
	}
	for _, kind := range []Kind{Plugin, Game} {
		items, err := m.List(kind)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Removed {
				continue
			}
			for _, release := range item.Releases {
				if release.Ref.ReleaseID != item.CurrentRelease {
					continue
				}
				add(release.Ref)
				if item.Enabled {
					pins, err := m.resolveDependencies(release.Manifest, nil)
					// An already unavailable consumer cannot activate these settings.
					if err == nil {
						for _, pin := range pins {
							add(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: pin.PluginID}, ReleaseID: pin.ReleaseID})
						}
					}
				}
			}
		}
	}
	instances, err := m.Instances()
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		if instance.Preview {
			continue
		}
		add(ReleaseRef{Package: PackageRef{Kind: Game, ID: instance.GameID}, ReleaseID: instance.ReleaseID})
		for _, pin := range instance.Dependencies {
			add(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: pin.PluginID}, ReleaseID: pin.ReleaseID})
		}
	}
	for _, runtime := range m.runtimes {
		for _, provider := range runtime.providers {
			if provider.context.Environment == "installed" {
				add(provider.release.Ref)
			}
		}
	}
	result := []Release{}
	for ref := range refs {
		release, _, err := m.release(ref)
		if err != nil {
			return nil, err
		}
		result = append(result, release)
	}
	slices.SortFunc(result, func(a, b Release) int { return strings.Compare(a.Ref.ReleaseID, b.Ref.ReleaseID) })
	return result, nil
}

func (m *Manager) SavePackageConfiguration(kind Kind, id, locale string, input ConfigurationInput) (ConfigurationDocument, error) {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	release, _, err := m.currentPackage(kind, id)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	document, err := m.configurationFromInput(release, locale, input)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	references, err := m.settingsReferences(release.Ref.Package)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	for _, referenced := range references {
		// Releases with no settings declaration receive an empty settings context.
		// They cannot constrain configuration introduced by a later release.
		if referenced.Manifest.Settings == nil {
			continue
		}
		form, err := readConfiguration(referenced.Manifest.Settings, func(name string) ([]byte, error) { return m.readReleaseFile(referenced, name) })
		if err != nil {
			return ConfigurationDocument{}, err
		}
		if _, err := validateConfiguration(form, document.Overrides); err != nil {
			_, problem := ErrorResponse(err)
			problem.Code, problem.MessageKey = "CONFIGURATION_CONFLICT", "platform.errors.CONFIGURATION_CONFLICT"
			problem.PackageID, problem.Version = id, referenced.Manifest.Version
			return ConfigurationDocument{}, problem
		}
	}
	raw := []byte(document.TOML)
	root := m.packagePath(release.Ref.Package)
	path := filepath.Join(root, "settings.toml")
	if input.ExpectedRevision == "" {
		return ConfigurationDocument{}, failure("INVALID_ARGUMENT", "Expected settings revision is required")
	}
	result, err := revisionfile.Mutate(context.Background(), path, revisionfile.Options{FileMode: 0o600, DirectoryMode: 0o700}, func(prior revisionfile.Snapshot) ([]byte, error) {
		if prior.Revision != input.ExpectedRevision {
			return nil, failure("DOCUMENT_CONFLICT", "Settings changed since they were read; reload before saving")
		}
		if prior.Exists && string(prior.Content) != string(raw) {
			backup := filepath.Join(root, "backups", "settings-"+time.Now().UTC().Format("20060102T150405.000000000")+".toml")
			if err := writeBytes(backup, prior.Content); err != nil {
				return nil, err
			}
		}
		return raw, nil
	})
	if err != nil {
		return ConfigurationDocument{}, err
	}
	document.Revision = result.Revision
	slog.Info("platform_extension_settings_saved", "kind", kind, "package", id, "applies", "next-start")
	return document, nil
}

func (m *Manager) SetPackagePermissions(ctx context.Context, kind Kind, id string, input PackagePermissions) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	release, item, err := m.currentPackage(kind, id)
	if err != nil {
		return err
	}
	if item.CurrentRelease != input.ReleaseID {
		return failure("DOCUMENT_CONFLICT", "Installed release changed; reload permissions")
	}
	grants, err := validateGrants(release.Manifest, input.Grants)
	if err != nil {
		return err
	}
	if slices.Equal(release.Grants, grants) {
		return nil
	}
	for runtimeID, runtime := range m.runtimes {
		if runtime.uses(kind, id) {
			if err := m.stopLocked(ctx, runtimeID); err != nil {
				return err
			}
		}
	}
	for index := range item.Releases {
		if item.Releases[index].Ref == release.Ref {
			item.Releases[index].Grants = grants
		}
	}
	item.Grants = slices.Clone(grants)
	if err := m.saveInstalled(kind, item); err != nil {
		return err
	}
	slog.Info("platform_extension_permissions_saved", "kind", kind, "package", id)
	return nil
}

func validateGrants(manifest Manifest, grants []string) ([]string, error) {
	for _, permission := range manifest.Permissions.Required {
		if !slices.Contains(grants, permission) {
			return nil, failure("PERMISSION_DENIED", "Permission %s must be granted explicitly", permission)
		}
	}
	for _, grant := range grants {
		if !slices.Contains(manifest.Permissions.Required, grant) && !slices.Contains(manifest.Permissions.Optional, grant) {
			return nil, failure("PERMISSION_DENIED", "Undeclared permission %s", grant)
		}
	}
	result := slices.Clone(grants)
	slices.Sort(result)
	return slices.Compact(result), nil
}
