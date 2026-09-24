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
	grants, err := validateGrants(candidate.Manifest, grants)
	if err != nil {
		return Release{}, err
	}
	if _, err := m.resolveDependencies(candidate.Manifest, nil); err != nil {
		return Release{}, err
	}
	release := Release{Ref: ReleaseRef{Package: PackageRef{Kind: candidate.Kind, ID: candidate.Manifest.ID}, ReleaseID: "preview-" + uuid.NewString()}, Manifest: candidate.Manifest, Digest: candidate.Digest, InstalledAt: time.Now().UTC(), Grants: slices.Clone(grants)}
	directory := m.releasePath(release.Ref)
	if err := m.publishCandidate(candidate, release.Ref); err != nil {
		return Release{}, err
	}
	installed := Installed{ID: candidate.Manifest.ID, Enabled: true, CurrentRelease: release.Ref.ReleaseID, Grants: slices.Clone(grants), Releases: []Release{release}}
	if err := writeJSON(filepath.Join(filepath.Dir(directory), "release.json"), installed); err != nil {
		_ = os.RemoveAll(filepath.Dir(directory))
		return Release{}, err
	}
	return release, nil
}
