package resourceexchange

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"denova/internal/agents/skills"
	"denova/internal/buildinfo"
	"denova/internal/platform"
	"denova/internal/portablepath"
	"denova/internal/revisionfile"
	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
)

func decode(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

func archiveBytes(files map[string][]byte) ([]byte, error) {
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		entry, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func readFiles(directory string) (map[string][]byte, error) {
	if err := portablepath.PreflightTree(directory); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files := map[string][]byte{}
	total := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if len(files) >= platform.MaxPackageFiles {
			return fmt.Errorf("too many package files")
		}
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(io.LimitReader(file, platform.MaxFileBytes+1))
		_ = file.Close()
		if err != nil {
			return err
		}
		total += len(raw)
		if len(raw) > platform.MaxFileBytes || total > platform.MaxPackageBytes {
			return fmt.Errorf("package exceeds byte limit")
		}
		files[name] = raw
		return nil
	})
	return files, err
}

func writeFiles(directory string, files map[string][]byte) error {
	for name, data := range files {
		if err := portablepath.Validate(name); err != nil {
			return err
		}
		file := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(file, data, 0600); err != nil {
			return err
		}
	}
	return nil
}

func subtree(files map[string][]byte, prefix string) map[string][]byte {
	if prefix == "" || prefix == "." {
		return files
	}
	result := map[string][]byte{}
	for name, raw := range files {
		if strings.HasPrefix(name, prefix+"/") {
			result[strings.TrimPrefix(name, prefix+"/")] = raw
		}
	}
	return result
}

func unwrapArchive(files map[string][]byte) map[string][]byte {
	root := ""
	for name := range files {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			return files
		}
		if root == "" {
			root = parts[0]
		}
		if parts[0] != root {
			return files
		}
	}
	return subtree(files, root)
}

func (s *Service) previewPath(id string) (string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("invalid preview ID")
	}
	return filepath.Join(s.root, "resource-exchange", "previews", id), nil
}

func (s *Service) Preview(ctx context.Context, source Source, data []byte) (Preview, error) {
	var files map[string][]byte
	var err error
	if source.Kind == "file" {
		files, err = platform.ArchiveFiles(data)
		if err != nil {
			return s.previewCharacter(ctx, source, data)
		}
		files = unwrapArchive(files)
		source = Source{Kind: "file", Filename: filepath.Base(source.Filename)}
	} else {
		if err := validateSource(source); err != nil {
			return Preview{}, err
		}
		if source.Kind == "github" {
			resolved, downloaded, downloadErr := s.platform.SourceFiles(ctx, platform.GitHubSource{URL: source.URL, Ref: source.Ref, Path: source.Path, Commit: source.Commit})
			if downloadErr != nil {
				return Preview{}, downloadErr
			}
			source.Ref, source.Commit = resolved.Ref, resolved.Commit
			files = subtree(downloaded, source.Path)
		} else {
			data, err = skills.DownloadRemoteArchive(ctx, source.URL)
			if err != nil {
				return Preview{}, err
			}
			files, err = platform.ArchiveFiles(data)
			if err != nil {
				return Preview{}, err
			}
			files = subtree(unwrapArchive(files), source.Path)
		}
	}
	if len(files) == 0 {
		return Preview{}, fmt.Errorf("source contains no package files")
	}
	if source.Kind == "github" && (files["denova.plugin.json"] != nil || files["denova.game.json"] != nil) {
		directory, err := os.MkdirTemp("", "denova-release-")
		if err != nil {
			return Preview{}, err
		}
		defer os.RemoveAll(directory)
		if err := writeFiles(directory, files); err != nil {
			return Preview{}, err
		}
		kind := platform.Plugin
		if files["denova.game.json"] != nil {
			kind = platform.Game
		}
		candidate, err := s.platform.PreviewDirectory(kind, directory)
		if err != nil {
			return Preview{}, err
		}
		defer s.platform.DiscardCandidate(candidate.ID)
		var archive bytes.Buffer
		if err := s.platform.ExportCandidate(candidate.ID, &archive); err != nil {
			return Preview{}, err
		}
		files, err = platform.ArchiveFiles(archive.Bytes())
		if err != nil {
			return Preview{}, err
		}
	}
	return s.previewFiles(ctx, source, files)
}

func (s *Service) previewFiles(ctx context.Context, source Source, files map[string][]byte) (result Preview, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := filepath.Join(s.root, "resource-exchange", "previews")
	if err := os.MkdirAll(base, 0700); err != nil {
		return Preview{}, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return Preview{}, err
	}
	active := 0
	for _, entry := range entries {
		if _, err := uuid.Parse(entry.Name()); err != nil || !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return Preview{}, err
		}
		if time.Since(info.ModTime()) > 24*time.Hour {
			if err := os.RemoveAll(filepath.Join(base, entry.Name())); err != nil {
				return Preview{}, err
			}
		} else {
			active++
		}
	}
	if active >= 32 {
		return Preview{}, fmt.Errorf("close unused import previews before opening another")
	}
	result = Preview{ID: uuid.NewString(), Source: source, ExpiresAt: time.Now().UTC().Add(24 * time.Hour), Candidates: []PackagePreview{}}
	dir, _ := s.previewPath(result.ID)
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	if err = writeFiles(filepath.Join(dir, "files"), files); err != nil {
		return Preview{}, err
	}
	roots := []string{}
	if _, ok := files["denova-pack.json"]; ok {
		roots = []string{"."}
	} else {
		for name := range files {
			if path.Base(name) == "denova-pack.json" {
				roots = append(roots, path.Dir(name))
			}
		}
	}
	slices.Sort(roots)
	if len(roots) > 0 {
		for _, root := range roots {
			var manifest Manifest
			if err = decode(subtree(files, root)["denova-pack.json"], &manifest); err != nil {
				return Preview{}, err
			}
			candidate, prepareErr := s.preparePackage(ctx, dir, root, manifest)
			if prepareErr != nil {
				return Preview{}, prepareErr
			}
			result.Candidates = append(result.Candidates, candidate)
		}
	} else if files["denova.plugin.json"] != nil || files["denova.game.json"] != nil {
		if files["denova.plugin.json"] != nil && files["denova.game.json"] != nil {
			return Preview{}, fmt.Errorf("ambiguous extension manifests")
		}
		kind := "extension.plugin"
		if files["denova.game.json"] != nil {
			kind = "extension.game"
		}
		manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "native", Name: "Extension"}, Resources: []Resource{{ID: "extension", Kind: kind, Path: "."}}}
		candidate, prepareErr := s.preparePackage(ctx, dir, ".", manifest)
		if prepareErr != nil {
			return Preview{}, prepareErr
		}
		candidate.Format = kind
		ext := candidate.Resources[0].Extension
		candidate.Package = PackageInfo{ID: "ext-" + strings.ReplaceAll(ext.Manifest.ID, ".", "-"), Name: ext.Manifest.Name.English, Version: ext.Manifest.Version}
		result.Candidates = append(result.Candidates, candidate)
	} else {
		preview, prepareErr := skills.PreviewDirectory(ctx, nil, skills.ScopeUser, filepath.Join(dir, "files"))
		if prepareErr != nil {
			return Preview{}, prepareErr
		}
		for _, skill := range preview.Candidates {
			if skill.InvalidReason != "" {
				return Preview{}, fmt.Errorf("invalid Skill %s: %s", skill.Name, skill.InvalidReason)
			}
			manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "skill-" + skill.Name, Name: skill.Name, Description: skill.Description}, Resources: []Resource{{ID: "skill", Kind: "skill", Path: skill.SourcePath}}}
			candidate, prepareErr := s.preparePackage(ctx, dir, ".", manifest)
			if prepareErr != nil {
				return Preview{}, prepareErr
			}
			candidate.ID = skill.ID
			candidate.Format = "skill"
			result.Candidates = append(result.Candidates, candidate)
		}
	}
	if len(result.Candidates) == 0 {
		return Preview{}, fmt.Errorf("no supported package found")
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return Preview{}, err
	}
	_, err = revisionfile.ReplaceIfRevision(ctx, filepath.Join(dir, "preview.json"), "", raw, revisionfile.Options{})
	return result, err
}

func (s *Service) preparePackage(ctx context.Context, dir, root string, manifest Manifest) (PackagePreview, error) {
	if manifest.Format != "denova.resource-pack" || manifest.SchemaVersion != 1 || !resourceID.MatchString(manifest.Package.ID) || strings.TrimSpace(manifest.Package.Name) == "" || len(manifest.Resources) == 0 || len(manifest.Resources) > 256 {
		return PackagePreview{}, fmt.Errorf("invalid package manifest")
	}
	if manifest.Package.MinVersion != "" {
		minimum, err := semver.StrictNewVersion(manifest.Package.MinVersion)
		if err != nil {
			return PackagePreview{}, err
		}
		if buildinfo.Version != "dev" {
			current, err := semver.NewVersion(buildinfo.Version)
			if err != nil || current.LessThan(minimum) {
				return PackagePreview{}, fmt.Errorf("package requires Denova %s", minimum)
			}
		}
	}
	if err := validateResourcePaths(manifest.Resources); err != nil {
		return PackagePreview{}, err
	}
	result := PackagePreview{ID: root, Package: manifest.Package, Format: manifest.Format, Resources: []PreviewResource{}}
	seen := map[string]bool{}
	for _, resource := range manifest.Resources {
		if !resourceID.MatchString(resource.ID) || seen[resource.ID] || !validKind(resource.Kind) {
			return result, fmt.Errorf("invalid resource identity or kind")
		}
		seen[resource.ID] = true
		if resource.Path != "." {
			if err := portablepath.Validate(resource.Path); err != nil {
				return result, err
			}
		}
		resource.Path = path.Join(root, resource.Path)
		for i, asset := range resource.Assets {
			if err := portablepath.Validate(asset); err != nil {
				return result, err
			}
			resource.Assets[i] = path.Join(root, asset)
		}
		for _, asset := range resource.Assets {
			info, err := os.Stat(filepath.Join(dir, "files", filepath.FromSlash(asset)))
			if err != nil {
				return result, err
			}
			if !info.Mode().IsRegular() {
				return result, fmt.Errorf("asset must be a regular file")
			}
		}
		item := PreviewResource{Resource: resource, Root: root}
		location := filepath.Join(dir, "files", filepath.FromSlash(resource.Path))
		var raw []byte
		switch resource.Kind {
		case "extension.plugin", "extension.game":
			files, err := readFiles(location)
			if err != nil {
				return result, err
			}
			for name := range files {
				if path.Base(name) == "denova-pack.json" {
					return result, fmt.Errorf("nested package manifest in extension")
				}
			}
			raw, err = archiveBytes(files)
			if err != nil {
				return result, err
			}
			kind := platform.Kind(strings.TrimPrefix(resource.Kind, "extension."))
			ext, err := s.platform.PreviewZIP(kind, raw)
			if err != nil {
				return result, err
			}
			item.Extension, item.Name = &ext, ext.Manifest.Name.English
			defer s.platform.DiscardCandidate(ext.ID)
		case "skill":
			preview, err := skills.PreviewDirectory(ctx, nil, skills.ScopeUser, location)
			if err != nil {
				return result, err
			}
			if len(preview.Candidates) != 1 || preview.Candidates[0].InvalidReason != "" {
				return result, fmt.Errorf("resource must contain exactly one valid Skill")
			}
			item.Name, item.Description = preview.Candidates[0].Name, preview.Candidates[0].Description
			files, err := readFiles(location)
			if err != nil {
				return result, err
			}
			raw, err = archiveBytes(files)
			if err != nil {
				return result, err
			}
		default:
			var err error
			raw, err = os.ReadFile(location)
			if err != nil {
				return result, err
			}
			if resource.Kind != "style.reference" {
				if err := validatePayload(resource.Kind, raw); err != nil {
					return result, err
				}
			}
			if resource.Kind == "project.cover" {
				item.Name = "Cover"
			} else if resource.Kind == "style.reference" {
				item.Name = path.Base(resource.Path)
			} else {
				var metadata struct {
					Name             string `json:"name"`
					Title            string `json:"title"`
					Description      string `json:"description"`
					BriefDescription string `json:"brief_description"`
				}
				if err := json.Unmarshal(raw, &metadata); err != nil {
					return result, err
				}
				item.Name, item.Description = metadata.Name, metadata.Description
				if item.Description == "" {
					item.Description = metadata.BriefDescription
				}
				if item.Name == "" {
					item.Name = metadata.Title
				}
			}
		}
		digestFiles := map[string][]byte{"resource": raw}
		for _, asset := range resource.Assets {
			data, err := os.ReadFile(filepath.Join(dir, "files", filepath.FromSlash(asset)))
			if err != nil {
				return result, err
			}
			digestFiles[asset] = data
		}
		if len(resource.Assets) > 0 {
			payload, err := archiveBytes(digestFiles)
			if err != nil {
				return result, err
			}
			item.Digest = revisionfile.Revision(payload)
		} else {
			item.Digest = revisionfile.Revision(raw)
		}
		if item.Extension != nil {
			item.Digest = item.Extension.Digest
		}
		result.Resources = append(result.Resources, item)
	}
	for i := range result.Resources {
		item := &result.Resources[i]
		if item.Extension != nil {
			for _, dependency := range item.Extension.Manifest.Requires {
				for _, possible := range result.Resources {
					if possible.Kind == "extension.plugin" && possible.Extension.Manifest.ID == dependency.PluginID && !slices.Contains(item.Requires, possible.ID) {
						item.Requires = append(item.Requires, possible.ID)
					}
				}
			}
		}
	}
	if _, err := selectResources(result.Resources, []string{result.Resources[0].ID}); err != nil {
		return result, err
	}
	if err := inferReferences(dir, &result); err != nil {
		return result, err
	}
	// Validate every component, including resources not selected by default.
	for _, resource := range result.Resources {
		if _, err := selectResources(result.Resources, []string{resource.ID}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func selectResources(resources []PreviewResource, selected []string) ([]PreviewResource, error) {
	byID := map[string]PreviewResource{}
	for _, resource := range resources {
		byID[resource.ID] = resource
	}
	state := map[string]int{}
	result := []PreviewResource{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("resource dependency cycle")
		}
		if state[id] == 2 {
			return nil
		}
		resource, ok := byID[id]
		if !ok {
			return fmt.Errorf("missing resource dependency %s", id)
		}
		state[id] = 1
		for _, dependency := range resource.Requires {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		result = append(result, resource)
		return nil
	}
	for _, id := range selected {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("select at least one resource")
	}
	return result, nil
}

func (s *Service) loadPreview(id string) (Preview, string, error) {
	dir, err := s.previewPath(id)
	if err != nil {
		return Preview{}, "", err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "preview.json"))
	if err != nil {
		return Preview{}, "", err
	}
	var preview Preview
	if err := json.Unmarshal(raw, &preview); err != nil {
		return Preview{}, "", err
	}
	if preview.ID != id || time.Now().After(preview.ExpiresAt) {
		return Preview{}, "", fmt.Errorf("import preview expired")
	}
	return preview, dir, nil
}
