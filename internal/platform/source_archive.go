package platform

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/project"
	"github.com/google/uuid"
)

func readGitHubArchive(raw []byte) (map[string][]byte, error) {
	files, err := readPackageZIP(raw)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]byte, len(files))
	root := ""
	for name, data := range files {
		parent, relative, found := strings.Cut(name, "/")
		if !found || (root != "" && parent != root) {
			return nil, failure("INVALID_ARGUMENT", "GitHub archive must contain exactly one enclosing directory")
		}
		root = parent
		result[relative] = data
	}
	return result, nil
}

func githubPackageRoot(relative string, repository map[string][]byte) (Kind, Manifest, map[string][]byte, error) {
	files := repository
	if relative != "." {
		files = map[string][]byte{}
		for name, data := range repository {
			if strings.HasPrefix(name, relative+"/") {
				files[strings.TrimPrefix(name, relative+"/")] = data
			}
		}
	}
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	kind, err := detectManifestKind(names)
	if err != nil {
		return "", Manifest{}, nil, failure("GITHUB_MANIFEST_MISSING", "Select a directory with exactly one Denova manifest: %s", relative)
	}
	raw := files[kind.manifestFile()]
	if len(raw) > MaxDefinitionBytes {
		return "", Manifest{}, nil, failure("LIMIT_EXCEEDED", "Source manifest is too large")
	}
	var manifest Manifest
	if err := decodeJSON(raw, &manifest); err != nil {
		return "", Manifest{}, nil, err
	}
	return kind, manifest, files, nil
}

func githubDistribution(kind Kind, manifest Manifest, files map[string][]byte) (map[string][]byte, error) {
	if manifest.Distribution == nil {
		return files, nil
	}
	result := map[string][]byte{kind.manifestFile(): files[kind.manifestFile()]}
	for _, path := range manifest.Distribution.Files {
		if err := validateDevelopmentPath(path); err != nil {
			return nil, failure("INVALID_ARGUMENT", "Invalid distribution path: %v", err)
		}
		found := false
		for name, data := range files {
			if path == "." || name == path || strings.HasPrefix(name, path+"/") {
				found = true
				result[name] = data
			}
		}
		if !found {
			return nil, failure("GITHUB_BUILD_REQUIRED", "Distributed path %s is absent; build the source in Workbench", path)
		}
	}
	return result, nil
}

// ImportGitHub creates an ordinary managed source Project only on explicit
// request. The full repository preserves shared build inputs for subpackages.
// Failures retain any created Project for inspection; no existing files change.
func (m *Manager) ImportGitHub(ctx context.Context, source GitHubSource) (DevelopmentSource, error) {
	source, files, err := m.downloadGitHubSource(ctx, source)
	if err != nil {
		return DevelopmentSource{}, err
	}
	kind, manifest, _, err := githubPackageRoot(source.Path, files)
	if err != nil {
		return DevelopmentSource{}, err
	}
	name := strings.TrimPrefix(source.URL, "https://github.com/")
	_, name, _ = strings.Cut(name, "/")
	record, err := m.registry.CreateDirectory(project.CreateDirectoryRequest{Name: name + "-" + uuid.NewString()[:8]})
	if err != nil {
		return DevelopmentSource{}, err
	}
	layout, err := m.registry.Layout(record)
	if err != nil {
		return DevelopmentSource{}, err
	}
	root, err := os.OpenRoot(layout.ContentRoot)
	if err != nil {
		return DevelopmentSource{}, err
	}
	defer root.Close()
	for name, data := range files {
		if err := ctx.Err(); err != nil {
			return DevelopmentSource{}, err
		}
		if err := root.MkdirAll(filepath.ToSlash(filepath.Dir(name)), 0700); err != nil {
			return DevelopmentSource{}, err
		}
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return DevelopmentSource{}, err
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			return DevelopmentSource{}, writeErr
		}
		if closeErr != nil {
			return DevelopmentSource{}, closeErr
		}
	}
	development := Development{ID: uuid.NewString(), Kind: kind, ProjectID: record.ID, RelativePath: source.Path, Source: &source}
	// The existing development binding owns provenance, including root imports.
	if err := writeJSON(filepath.Join(layout.StoreRoot, "development.json"), []Development{development}); err != nil {
		return DevelopmentSource{}, err
	}
	slog.Info("platform_github_source_imported", "project", record.ID, "repository", source.URL, "commit", source.Commit)
	return DevelopmentSource{Development: development, ProjectName: record.Name, Manifest: &manifest}, nil
}
