package platform

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"denova/internal/portablepath"
	"denova/internal/project"
	"denova/internal/revisionfile"
	"github.com/google/uuid"
)

type Candidate struct {
	Source   *GitHubSource `json:"source,omitempty"`
	ID       string        `json:"candidateId"`
	Kind     Kind          `json:"kind"`
	Manifest Manifest      `json:"manifest"`
	Digest   string        `json:"digest"`
	Files    []string      `json:"files"`
	Bytes    int           `json:"bytes"`
	files    map[string][]byte
	replaces *ReleaseRef
}

// Manager owns package and instance mutations. Runtime handles and candidate
// bytes are process-local; only stable references enter the data directory.
type Manager struct {
	root       string
	registry   *project.Registry
	mu         sync.Mutex
	candidates map[string]*Candidate
	runtimeMu  sync.Mutex
	runtimes   map[string]*Runtime
	agents     *AgentService
	resources  *ResourceService
	stories    StoryHost
	githubHTTP *http.Client
}

func New(root string, registry *project.Registry) *Manager {
	return &Manager{root: root, registry: registry, candidates: map[string]*Candidate{}, runtimes: map[string]*Runtime{}, githubHTTP: newGitHubClient()}
}

func (m *Manager) PreviewDirectory(kind Kind, directory string) (Candidate, error) {
	if !kind.valid() {
		return Candidate{}, failure("INVALID_ARGUMENT", "Invalid package kind")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Candidate{}, err
	}
	defer root.Close()
	raw, err := root.ReadFile(kind.manifestFile())
	if err != nil {
		return Candidate{}, err
	}
	var manifest Manifest
	if err := decodeJSON(raw, &manifest); err != nil {
		return Candidate{}, err
	}
	if _, err := root.Stat(Plugin.manifestFile()); err == nil {
		if _, err := root.Stat(Game.manifestFile()); err == nil {
			return Candidate{}, failure("INVALID_ARGUMENT", "A product root cannot have both manifests")
		}
	}
	paths := []string{"."}
	if manifest.Distribution != nil {
		paths = append(slices.Clone(manifest.Distribution.Files), kind.manifestFile())
	}
	files, err := readPackageFiles(root, paths)
	if err != nil {
		return Candidate{}, err
	}
	return m.freeze(kind, files)
}

// Source collection follows its distribution list; installed exports collect
// every frozen file so their content identity survives an export/import roundtrip.
func readPackageFiles(root *os.Root, paths []string) (map[string][]byte, error) {
	files := map[string][]byte{}
	total := 0
	for _, path := range paths {
		if path != "." {
			if err := portablepath.Validate(path); err != nil {
				return nil, failure("INVALID_ARGUMENT", "%v", err)
			}
		}
		err := fs.WalkDir(root.FS(), path, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if name == "." {
				return nil
			}
			if err := portablepath.Validate(name); err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return failure("INVALID_ARGUMENT", "Package entry %s is not a regular file or directory", name)
			}
			if entry.IsDir() {
				return nil
			}
			if _, exists := files[name]; exists {
				return nil
			}
			if len(files) >= MaxPackageFiles || info.Size() > MaxFileBytes || int64(total)+info.Size() > MaxPackageBytes {
				return failure("LIMIT_EXCEEDED", "Package exceeds file or byte limit")
			}
			file, err := root.Open(name)
			if err != nil {
				return err
			}
			data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
			_ = file.Close()
			if err != nil {
				return err
			}
			if len(data) > MaxFileBytes || total+len(data) > MaxPackageBytes {
				return failure("LIMIT_EXCEEDED", "Package changed beyond byte limit")
			}
			files[name] = data
			total += len(data)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func (m *Manager) PreviewZIP(kind Kind, raw []byte) (Candidate, error) {
	files, err := readPackageZIP(raw)
	if err != nil {
		return Candidate{}, err
	}
	return m.freeze(kind, files)
}

func readPackageZIP(raw []byte) (map[string][]byte, error) {
	if len(raw) > MaxPackageBytes {
		return nil, failure("LIMIT_EXCEEDED", "Archive is too large")
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, failure("INVALID_ARGUMENT", "Invalid ZIP archive: %v", err)
	}
	if len(reader.File) > MaxPackageFiles {
		return nil, failure("LIMIT_EXCEEDED", "Archive exceeds entry limits")
	}
	files := map[string][]byte{}
	seen := map[string]bool{}
	total := 0
	for _, file := range reader.File {
		name := strings.TrimSuffix(file.Name, "/")
		if err := portablepath.Validate(name); err != nil {
			return nil, failure("INVALID_ARGUMENT", "%v", err)
		}
		if seen[portablepath.FoldKey(name)] {
			return nil, failure("INVALID_ARGUMENT", "Duplicate or case-conflicting ZIP path %s", name)
		}
		seen[portablepath.FoldKey(name)] = true
		if file.Mode()&os.ModeSymlink != 0 || (!file.FileInfo().IsDir() && !file.Mode().IsRegular()) {
			return nil, failure("INVALID_ARGUMENT", "ZIP contains a special file: %s", name)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if len(files) >= MaxPackageFiles || file.UncompressedSize64 > MaxFileBytes {
			return nil, failure("LIMIT_EXCEEDED", "Archive exceeds file limits")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(stream, MaxFileBytes+1))
		_ = stream.Close()
		if err != nil {
			return nil, err
		}
		total += len(data)
		if len(data) > MaxFileBytes || total > MaxPackageBytes {
			return nil, failure("LIMIT_EXCEEDED", "Archive exceeds byte limits")
		}
		files[name] = data
	}
	if _, err := packageFileNames(files); err != nil {
		return nil, err
	}
	return files, nil
}

func packageFileNames(files map[string][]byte) ([]string, error) {
	// Check every path prefix, including implicit ZIP directories, so A/a never
	// silently alias on Windows even when the archive omitted directory entries.
	names := make([]string, 0, len(files))
	seen := map[string]string{}
	for name := range files {
		if err := portablepath.Validate(name); err != nil {
			return nil, err
		}
		parts := strings.Split(name, "/")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			key := portablepath.FoldKey(prefix)
			if prior, ok := seen[key]; ok && prior != prefix {
				return nil, failure("INVALID_ARGUMENT", "Case-conflicting package paths %s and %s", prior, prefix)
			}
			seen[key] = prefix
			if i < len(parts)-1 {
				if _, exists := files[prefix]; exists {
					return nil, failure("INVALID_ARGUMENT", "File is also a directory: %s", prefix)
				}
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func checkPackage(kind Kind, files map[string][]byte) (Candidate, error) {
	names, err := packageFileNames(files)
	if err != nil {
		return Candidate{}, err
	}
	manifest, err := validateManifest(kind, files)
	if err != nil {
		return Candidate{}, err
	}
	digest := sha256.New()
	total := 0
	for _, name := range names {
		fmt.Fprintf(digest, "%d:%s:%d:", len(name), name, len(files[name]))
		_, _ = digest.Write(files[name])
		total += len(files[name])
	}
	return Candidate{Kind: kind, Manifest: manifest, Digest: hex.EncodeToString(digest.Sum(nil)), Files: names, Bytes: total, files: files}, nil
}

func (m *Manager) freeze(kind Kind, files map[string][]byte) (Candidate, error) {
	candidate, err := checkPackage(kind, files)
	if err != nil {
		return Candidate{}, err
	}
	candidate.ID = uuid.NewString()
	m.mu.Lock()
	defer m.mu.Unlock()
	// Bounded process-local previews are disposable. Refuse new ones rather than
	// evicting bytes a user is currently reviewing for installation.
	used := candidate.Bytes
	for _, current := range m.candidates {
		used += current.Bytes
	}
	if len(m.candidates) >= 32 || used > MaxPackageBytes {
		return Candidate{}, failure("LIMIT_EXCEEDED", "Discard unused package previews before creating another")
	}
	m.candidates[candidate.ID] = &candidate
	return candidate, nil
}

func (m *Manager) DiscardCandidate(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.candidates, id)
}

func (m *Manager) ExportCandidate(id string, writer io.Writer) error {
	m.mu.Lock()
	candidate := m.candidates[id]
	m.mu.Unlock()
	if candidate == nil {
		return failure("NOT_FOUND", "Candidate %s is unavailable", id)
	}
	return writePackageArchive(*candidate, writer)
}

func writePackageArchive(candidate Candidate, writer io.Writer) error {
	archive := zip.NewWriter(writer)
	for _, name := range candidate.Files {
		entry, err := archive.Create(name)
		if err != nil {
			_ = archive.Close()
			return err
		}
		if _, err := entry.Write(candidate.files[name]); err != nil {
			_ = archive.Close()
			return err
		}
	}
	return archive.Close()
}

func (m *Manager) Install(candidateID string, grants []string) (Release, error) {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate := m.candidates[candidateID]
	if candidate == nil {
		return Release{}, failure("NOT_FOUND", "Candidate %s is unavailable", candidateID)
	}
	manifest := candidate.Manifest
	form, err := readConfiguration(manifest.Settings, func(name string) ([]byte, error) { return candidate.files[name], nil })
	if err != nil {
		return Release{}, err
	}
	overrides, err := m.settingsOverrides(PackageRef{Kind: candidate.Kind, ID: manifest.ID})
	if err != nil {
		return Release{}, err
	}
	if _, err := validateConfiguration(form, overrides); err != nil {
		return Release{}, err
	}
	grants, err = validateGrants(manifest, grants)
	if err != nil {
		return Release{}, err
	}
	if _, err := m.resolveDependencies(manifest, nil); err != nil {
		return Release{}, err
	}
	items, err := m.List(candidate.Kind)
	if err != nil {
		return Release{}, err
	}
	index := slices.IndexFunc(items, func(item Installed) bool { return item.ID == manifest.ID })
	if candidate.replaces != nil && (index < 0 || items[index].Removed || items[index].CurrentRelease != candidate.replaces.ReleaseID) {
		return Release{}, failure("DOCUMENT_CONFLICT", "Installed package changed while its update was being reviewed")
	}
	if index < 0 {
		items = append(items, Installed{ID: manifest.ID, Releases: []Release{}})
		index = len(items) - 1
	}
	release := Release{Ref: ReleaseRef{Package: PackageRef{Kind: candidate.Kind, ID: manifest.ID}, ReleaseID: candidate.Digest}, Manifest: manifest, Digest: candidate.Digest, InstalledAt: time.Now().UTC(), Grants: slices.Clone(grants)}
	for priorIndex, prior := range items[index].Releases {
		if prior.Digest == candidate.Digest {
			release = prior
			if !slices.Equal(prior.Grants, grants) {
				// Reauthorization changes policy, never package bytes. Revoke
				// existing credentials before publishing the new grant snapshot.
				for id, runtime := range m.runtimes {
					if runtime.uses(candidate.Kind, manifest.ID) {
						if err := m.stopLocked(context.Background(), id); err != nil {
							return Release{}, err
						}
					}
				}
				release.Grants = grants
				items[index].Releases[priorIndex] = release
			}
		}
	}
	destination := m.releasePath(release.Ref)
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		parent := filepath.Dir(destination)
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return Release{}, err
		}
		staging, err := os.MkdirTemp(parent, ".install-")
		if err != nil {
			return Release{}, err
		}
		defer os.RemoveAll(staging)
		for _, name := range candidate.Files {
			path := filepath.Join(staging, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return Release{}, err
			}
			if err := writeBytes(path, candidate.files[name]); err != nil {
				return Release{}, err
			}
		}
		if err := os.Rename(staging, destination); err != nil {
			return Release{}, err
		}
	} else if err != nil {
		return Release{}, err
	}
	if !slices.ContainsFunc(items[index].Releases, func(r Release) bool { return r.Digest == release.Digest }) {
		items[index].Releases = append(items[index].Releases, release)
	}
	items[index].CurrentRelease = release.Ref.ReleaseID
	items[index].Source = candidate.Source
	items[index].Enabled = true
	items[index].Removed = false
	items[index].Grants = slices.Clone(grants)
	if err := m.saveInstalled(candidate.Kind, items[index]); err != nil {
		return Release{}, err
	}
	slog.Info("platform_package_installed", "kind", candidate.Kind, "package", manifest.ID, "release", release.Ref.ReleaseID)
	return release, nil
}

func (m *Manager) releasePath(ref ReleaseRef) string {
	if strings.HasPrefix(ref.ReleaseID, "preview-") {
		return filepath.Join(m.packagePath(ref.Package), "previews", ref.ReleaseID, "package")
	}
	return filepath.Join(m.packagePath(ref.Package), "releases", ref.ReleaseID)
}

func (m *Manager) release(ref ReleaseRef) (Release, Installed, error) {
	if !ref.Package.Kind.valid() {
		return Release{}, Installed{}, failure("INVALID_ARGUMENT", "Invalid package kind")
	}
	if err := validateID(ref.Package.ID); err != nil {
		return Release{}, Installed{}, err
	}
	if strings.HasPrefix(ref.ReleaseID, "preview-") {
		if _, err := uuid.Parse(strings.TrimPrefix(ref.ReleaseID, "preview-")); err != nil {
			return Release{}, Installed{}, failure("INVALID_ARGUMENT", "Invalid preview release")
		}
		var item Installed
		if err := readJSON(filepath.Join(filepath.Dir(m.releasePath(ref)), "release.json"), &item); err != nil {
			return Release{}, Installed{}, err
		}
		if len(item.Releases) != 1 || item.Releases[0].Ref != ref {
			return Release{}, Installed{}, failure("NOT_FOUND", "Preview identity mismatch")
		}
		return item.Releases[0], item, nil
	}
	items, err := m.List(ref.Package.Kind)
	if err != nil {
		return Release{}, Installed{}, err
	}
	for _, item := range items {
		if item.ID == ref.Package.ID {
			for _, release := range item.Releases {
				if release.Ref.ReleaseID == ref.ReleaseID {
					return release, item, nil
				}
			}
		}
	}
	return Release{}, Installed{}, failure("NOT_FOUND", "Release %s/%s is not installed", ref.Package.ID, ref.ReleaseID)
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytes(path, data)
}
func writeBytes(path string, data []byte) error {
	_, err := revisionfile.ReplaceIfRevision(context.Background(), path, "", data, revisionfile.Options{FileMode: 0o600, DirectoryMode: 0o700})
	return err
}
