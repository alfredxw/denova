package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"denova/internal/revisionfile"
)

// InstallSelection identifies frozen code and consent supplied by a confirmed
// application plan. Reference selections never acquire installation ownership.
type InstallSelection struct {
	CandidateID string
	Grants      []string
	Reference   bool
}

// InstallWrite is a platform-owned metadata mutation, relative to the data
// directory. The application may combine it with its resource transaction; it
// must preserve bytes and CAS preconditions and finish while the callback runs.
type InstallWrite struct {
	Path     string
	Expected string
	Content  []byte
}

// ArchiveFiles shares extension-grade archive admission with resource imports.
// The returned map contains only portable, bounded regular files.
func ArchiveFiles(raw []byte) (map[string][]byte, error) { return readPackageZIP(raw) }

// SourceFiles freezes the selected subtree of a public GitHub checkout.
// It neither builds nor installs the downloaded content.
func (m *Manager) SourceFiles(ctx context.Context, source GitHubSource) (GitHubSource, map[string][]byte, error) {
	source, raw, err := m.downloadGitHubArchive(ctx, source)
	if err != nil {
		return source, nil, err
	}
	files, err := readGitHubSubtree(raw, source.Path)
	return source, files, err
}

// InstallState captures installed identities, grants, enabled state and current
// settings. A plan must be reviewed again if this state changes before commit.
func (m *Manager) InstallState() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.installState()
}

func (m *Manager) installState() (string, error) {
	states := map[string]string{}
	for _, kind := range []Kind{Plugin, Game} {
		items, err := m.List(kind)
		if err != nil {
			return "", err
		}
		for _, item := range items {
			raw, err := json.Marshal(item)
			if err != nil {
				return "", err
			}
			key := string(kind) + "/" + item.ID
			states[key] = revisionfile.Revision(raw)
			if item.CurrentRelease != "" {
				snapshot, err := revisionfile.Read(context.Background(), m.settingsPath(ReleaseRef{Package: PackageRef{Kind: kind, ID: item.ID}, ReleaseID: item.CurrentRelease}))
				if err != nil {
					return "", err
				}
				states[key+"/settings"] = snapshot.Revision
			}
		}
	}
	raw, err := json.Marshal(states)
	if err != nil {
		return "", err
	}
	return revisionfile.Revision(raw), nil
}

// WithInstallBatch prevents runtime admission and platform mutations until the
// application's durable commit finishes. Publishing immutable release bytes is
// harmless before commit; no installed pointer, grant or setting is written here.
func (m *Manager) WithInstallBatch(ctx context.Context, expectedState string, selections []InstallSelection, commit func([]InstallWrite) error) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.installState()
	if err != nil {
		return err
	}
	if current != expectedState {
		return failure("DOCUMENT_CONFLICT", "Extension state changed while reviewing installation")
	}
	plugins, err := m.List(Plugin)
	if err != nil {
		return err
	}
	writes := []InstallWrite{}
	candidates := []*Candidate{}
	seen := map[PackageRef]bool{}
	for _, selected := range selections {
		candidate := m.candidates[selected.CandidateID]
		if candidate == nil {
			return failure("NOT_FOUND", "Frozen extension candidate expired")
		}
		ref := PackageRef{Kind: candidate.Kind, ID: candidate.Manifest.ID}
		if seen[ref] {
			return failure("INVALID_ARGUMENT", "Repeated extension identity in batch")
		}
		seen[ref] = true
		items, err := m.List(candidate.Kind)
		if err != nil {
			return err
		}
		item := Installed{ID: ref.ID, Enabled: true, Releases: []Release{}}
		if index := slices.IndexFunc(items, func(i Installed) bool { return i.ID == ref.ID }); index >= 0 {
			item = items[index]
		}
		if selected.Reference {
			if item.Removed || !item.Enabled || item.CurrentRelease != candidate.Digest {
				return failure("DEPENDENCY_UNAVAILABLE", "Referenced extension changed or is unavailable")
			}
			if _, err := validateGrants(candidate.Manifest, item.Grants); err != nil {
				return err
			}
			candidates = append(candidates, candidate)
			continue
		}
		grants, err := validateGrants(candidate.Manifest, selected.Grants)
		if err != nil {
			return err
		}
		grants = append([]string{}, grants...)
		// New immutable releases leave existing consumers on their exact pins.
		// Reauthorizing an existing digest changes its credential policy and
		// therefore requires those consumers to stop before the transaction.
		for _, previous := range item.Releases {
			if previous.Digest != candidate.Digest || slices.Equal(previous.Grants, grants) {
				continue
			}
			for _, runtime := range m.runtimes {
				if runtime.uses(candidate.Kind, ref.ID) {
					return failure("DOCUMENT_CONFLICT", "Stop the extension runtime before changing release permissions")
				}
			}
		}
		release := Release{Ref: ReleaseRef{Package: ref, ReleaseID: candidate.Digest}, Manifest: candidate.Manifest, Digest: candidate.Digest, InstalledAt: time.Now().UTC(), Grants: slices.Clone(grants)}
		index := slices.IndexFunc(item.Releases, func(r Release) bool { return r.Digest == release.Digest })
		if index < 0 {
			item.Releases = append(item.Releases, release)
		} else {
			release.InstalledAt = item.Releases[index].InstalledAt
			item.Releases[index] = release
		}
		if err := m.publishCandidate(candidate, release.Ref); err != nil {
			return err
		}
		settings := m.settingsPath(release.Ref)
		nextSettings, err := revisionfile.Read(ctx, settings)
		if err != nil {
			return err
		}
		if !nextSettings.Exists && item.CurrentRelease != "" {
			previous, err := revisionfile.Read(ctx, m.settingsPath(ReleaseRef{Package: ref, ReleaseID: item.CurrentRelease}))
			if err != nil {
				return err
			}
			if previous.Exists {
				relative, err := filepath.Rel(m.root, settings)
				if err != nil {
					return err
				}
				writes = append(writes, InstallWrite{Path: filepath.ToSlash(relative), Expected: nextSettings.Revision, Content: previous.Content})
			}
		}
		item.CurrentRelease, item.Grants, item.Removed, item.Source = release.Digest, slices.Clone(grants), false, nil
		path := filepath.Join(m.packagePath(ref), "installed.json")
		before, err := revisionfile.Read(ctx, path)
		if err != nil {
			return err
		}
		raw, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(m.root, path)
		if err != nil {
			return err
		}
		writes = append(writes, InstallWrite{Path: filepath.ToSlash(relative), Expected: before.Revision, Content: raw})
		if candidate.Kind == Plugin {
			index := slices.IndexFunc(plugins, func(i Installed) bool { return i.ID == ref.ID })
			if index < 0 {
				plugins = append(plugins, item)
			} else {
				plugins[index] = item
			}
		}
		candidates = append(candidates, candidate)
	}
	for _, candidate := range candidates {
		if _, err := resolveAvailableDependencies(candidate.Manifest, nil, plugins); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if commit == nil {
		return fmt.Errorf("installation batch requires a durable commit callback")
	}
	return commit(writes)
}

// CandidateInfo exposes the frozen manifest and source, never mutable file data.
func (m *Manager) CandidateInfo(id string) (Candidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate := m.candidates[id]
	if candidate == nil {
		return Candidate{}, failure("NOT_FOUND", "Candidate is unavailable")
	}
	return *candidate, nil
}

// WithRestoreBarrier protects installed pointers restored by an application
// backup. It never writes platform state itself or changes running releases.
func (m *Manager) WithRestoreBarrier(expected string, packages []PackageRef, commit func() error) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.installState()
	if err != nil {
		return err
	}
	if state != expected {
		return failure("DOCUMENT_CONFLICT", "Extension state changed before restoring backup")
	}
	for _, ref := range packages {
		for _, runtime := range m.runtimes {
			if runtime.uses(ref.Kind, ref.ID) {
				return failure("DOCUMENT_CONFLICT", "Stop extension runtime before restoring backup")
			}
		}
	}
	return commit()
}
