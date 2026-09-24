package platform

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"time"

	"denova/internal/revisionfile"
	toml "github.com/pelletier/go-toml/v2"
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

func (m *Manager) PackageConfiguration(ref ReleaseRef, locale string) (ConfigurationDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, _, err := m.configurationRelease(ref)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	snapshot, err := revisionfile.Read(context.Background(), m.settingsPath(release.Ref))
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
	}
	return document, nil
}

// An omitted release selects the current installation; bound views always use
// their exact release so newer settings cannot alter an older saved game.
func (m *Manager) configurationRelease(ref ReleaseRef) (Release, Installed, error) {
	if ref.ReleaseID == "" {
		return m.currentPackage(ref.Package.Kind, ref.Package.ID)
	}
	return m.release(ref)
}

func (m *Manager) SavePackageConfiguration(kind Kind, id, locale string, input ConfigurationInput) (ConfigurationDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: kind, ID: id}, ReleaseID: input.ReleaseID})
	if err != nil {
		return ConfigurationDocument{}, err
	}
	document, err := m.configurationDocument(release, input.Overrides, locale)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	if document.Problem != nil {
		return ConfigurationDocument{}, document.Problem
	}
	raw, err := toml.Marshal(input.Overrides)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	if len(raw) > MaxDefinitionBytes {
		return ConfigurationDocument{}, failure("LIMIT_EXCEEDED", "Configuration exceeds %d bytes", MaxDefinitionBytes)
	}
	path := m.settingsPath(release.Ref)
	root := filepath.Dir(path)
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
