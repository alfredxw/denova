package platform

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/uuid"
)

// PreparePreview freezes a candidate outside the installed catalog. Preview
// releases survive restart solely so their separate test instances can reopen.
func (m *Manager) PreparePreview(candidateID string, grants []string) (Release, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate := m.candidates[candidateID]
	if candidate == nil {
		return Release{}, failure("NOT_FOUND", "Preview candidate is unavailable")
	}
	for _, permission := range candidate.Manifest.Permissions.Required {
		if !slices.Contains(grants, permission) {
			return Release{}, failure("PERMISSION_DENIED", "Required preview permission %s was not granted", permission)
		}
	}
	for _, grant := range grants {
		if !slices.Contains(candidate.Manifest.Permissions.Required, grant) && !slices.Contains(candidate.Manifest.Permissions.Optional, grant) {
			return Release{}, failure("PERMISSION_DENIED", "Undeclared preview permission")
		}
	}
	if _, err := m.resolveDependencies(candidate.Manifest, nil); err != nil {
		return Release{}, err
	}
	release := Release{Ref: ReleaseRef{Package: PackageRef{Kind: candidate.Kind, ID: candidate.Manifest.ID}, ReleaseID: "preview-" + uuid.NewString()}, Manifest: candidate.Manifest, Digest: candidate.Digest, InstalledAt: time.Now().UTC(), Grants: slices.Clone(grants)}
	directory := m.releasePath(release.Ref)
	for _, name := range candidate.Files {
		if err := writeBytes(filepath.Join(directory, filepath.FromSlash(name)), candidate.files[name]); err != nil {
			return Release{}, err
		}
	}
	installed := Installed{ID: candidate.Manifest.ID, Enabled: true, CurrentRelease: release.Ref.ReleaseID, Grants: slices.Clone(grants), Releases: []Release{release}}
	if err := writeJSON(filepath.Join(filepath.Dir(directory), "release.json"), installed); err != nil {
		_ = os.RemoveAll(filepath.Dir(directory))
		return Release{}, err
	}
	return release, nil
}
