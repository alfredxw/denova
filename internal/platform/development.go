package platform

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"denova/extensionassets"
	"denova/internal/portablepath"
	"github.com/google/uuid"
)

func validateDevelopmentPath(path string) error {
	if path == "." {
		return nil
	}
	return portablepath.Validate(path)
}

func checkDevelopmentLocation(root, relative string) error {
	if relative == "." {
		return nil
	}
	return portablepath.CheckNoCollision(root, relative)
}

type Development struct {
	// Imported provenance records the base checkout; local edits and builds are
	// identified by the resulting candidate digest, not by the upstream commit.
	Source    *GitHubSource `json:"source,omitempty"`
	ID        string        `json:"developmentId"`
	Kind      Kind          `json:"kind"`
	ProjectID string        `json:"projectId"`
	// A dot denotes the Project root; other values are portable subdirectories.
	RelativePath string `json:"relativePath"`
}

// CreateDevelopment initializes one neutral source scaffold per product kind.
// Concrete example applications are reference source, never initialization modes.
type CreateDevelopment struct {
	Kind         Kind          `json:"kind"`
	ProjectID    string        `json:"projectId"`
	RelativePath string        `json:"relativePath"`
	ID           string        `json:"id"`
	Name         LocalizedText `json:"name"`
}

func (m *Manager) Developments() ([]Development, error) {
	projects, err := m.registry.List(false)
	if err != nil {
		return nil, err
	}
	items := []Development{}
	for _, record := range projects {
		layout, err := m.registry.Layout(record)
		if err != nil {
			return nil, err
		}
		var linked []Development
		err = readJSON(filepath.Join(layout.StoreRoot, "development.json"), &linked)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		items = append(items, linked...)
		// A Project-root manifest is its own development binding. No second
		// project registry or explicit import step is needed for opened source.
		if !slices.ContainsFunc(linked, func(item Development) bool { return item.RelativePath == "." }) {
			if kind, err := DetectKind(layout.ContentRoot); err == nil {
				items = append(items, Development{ID: record.ID, Kind: kind, ProjectID: record.ID, RelativePath: "."})
			}
		}
	}
	return items, nil
}

func (m *Manager) Development(id string) (Development, string, error) {
	items, err := m.Developments()
	if err != nil {
		return Development{}, "", err
	}
	for _, item := range items {
		if item.ID == id {
			_, layout, err := m.registry.Resolve(item.ProjectID, true)
			if err != nil {
				return Development{}, "", err
			}
			if err := validateDevelopmentPath(item.RelativePath); err != nil {
				return Development{}, "", err
			}
			return item, filepath.Join(layout.ContentRoot, filepath.FromSlash(item.RelativePath)), nil
		}
	}
	return Development{}, "", failure("NOT_FOUND", "Development binding is unavailable")
}

func (m *Manager) LinkDevelopment(kind Kind, projectID, relative string) (Development, error) {
	if !kind.valid() {
		return Development{}, failure("INVALID_ARGUMENT", "Invalid development kind")
	}
	if err := validateDevelopmentPath(relative); err != nil {
		return Development{}, failure("INVALID_ARGUMENT", "%v", err)
	}
	_, layout, err := m.registry.Resolve(projectID, true)
	if err != nil {
		return Development{}, err
	}
	if err := checkDevelopmentLocation(layout.ContentRoot, relative); err != nil {
		return Development{}, failure("INVALID_ARGUMENT", "%v", err)
	}
	root, err := os.OpenRoot(layout.ContentRoot)
	if err != nil {
		return Development{}, err
	}
	defer root.Close()
	file, err := root.Open(filepath.ToSlash(filepath.Join(relative, kind.manifestFile())))
	if err != nil {
		return Development{}, err
	}
	_ = file.Close()
	if relative == "." {
		return Development{ID: projectID, Kind: kind, ProjectID: projectID, RelativePath: "."}, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	path := filepath.Join(layout.StoreRoot, "development.json")
	items := []Development{}
	if err := readJSON(path, &items); err != nil && !os.IsNotExist(err) {
		return Development{}, err
	}
	for _, item := range items {
		if item.RelativePath == relative {
			if item.Kind != kind {
				return Development{}, failure("INVALID_ARGUMENT", "Directory is already linked to another product kind")
			}
			return item, nil
		}
	}
	item := Development{ID: uuid.NewString(), Kind: kind, ProjectID: projectID, RelativePath: relative}
	items = append(items, item)
	return item, writeJSON(path, items)
}

func (m *Manager) CreateDevelopment(request CreateDevelopment) (Development, error) {
	if err := validateID(request.ID); err != nil {
		return Development{}, err
	}
	if err := validateDevelopmentPath(request.RelativePath); err != nil {
		return Development{}, err
	}
	if request.Name.Chinese == "" || request.Name.English == "" {
		return Development{}, failure("INVALID_ARGUMENT", "Both localized names are required")
	}
	if !request.Kind.valid() {
		return Development{}, failure("INVALID_ARGUMENT", "Invalid development kind")
	}
	_, layout, err := m.registry.Resolve(request.ProjectID, true)
	if err != nil {
		return Development{}, err
	}
	if err := checkDevelopmentLocation(layout.ContentRoot, request.RelativePath); err != nil {
		return Development{}, err
	}
	root, err := os.OpenRoot(layout.ContentRoot)
	if err != nil {
		return Development{}, err
	}
	defer root.Close()
	if err := root.MkdirAll(request.RelativePath, 0o700); err != nil {
		return Development{}, err
	}
	directory, err := root.OpenRoot(request.RelativePath)
	if err != nil {
		return Development{}, err
	}
	defer directory.Close()
	entries, err := fs.ReadDir(directory.FS(), ".")
	if err != nil {
		return Development{}, err
	}
	if len(entries) != 0 {
		return Development{}, failure("DOCUMENT_CONFLICT", "Development directory must be empty")
	}
	starterRoot := "starters/" + string(request.Kind)
	starterFiles := extensionassets.Files()
	err = fs.WalkDir(starterFiles, starterRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(path, starterRoot+"/")
		data, err := starterFiles.ReadFile(path)
		if err != nil {
			return err
		}
		if relative == request.Kind.manifestFile() {
			var manifest Manifest
			if err := decodeJSON(data, &manifest); err != nil {
				return err
			}
			manifest.ID = request.ID
			manifest.Name = request.Name
			data, err = json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return err
			}
		}
		if err := directory.MkdirAll(filepath.ToSlash(filepath.Dir(relative)), 0o700); err != nil {
			return err
		}
		file, err := directory.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(data)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	})
	if err != nil {
		return Development{}, err
	}
	sharedFiles := []string{"starters/DEVELOPMENT.md"}
	switch request.Kind {
	case Plugin:
		sharedFiles = append(sharedFiles, "sdk/runtime.mjs")
	case Game:
		sharedFiles = append(sharedFiles, "sdk/client.mjs")
	}
	for _, shared := range sharedFiles {
		data, err := starterFiles.ReadFile(shared)
		if err != nil {
			return Development{}, err
		}
		file, err := directory.OpenFile(filepath.Base(shared), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return Development{}, err
		}
		_, err = file.Write(data)
		closeErr := file.Close()
		if err != nil {
			return Development{}, err
		}
		if closeErr != nil {
			return Development{}, closeErr
		}
	}
	development, err := m.LinkDevelopment(request.Kind, request.ProjectID, request.RelativePath)
	if err == nil {
		slog.Info("platform_development_initialized", "kind", request.Kind, "project", request.ProjectID, "package", request.ID)
	}
	return development, err
}

// BuildRecipe is read-only. The UI displays and sends this recipe to the
// existing visible Project terminal, whose cancellation and logs stay intact.
func (m *Manager) BuildRecipe(id string) (*Command, string, error) {
	item, directory, err := m.Development(id)
	if err != nil {
		return nil, "", err
	}
	var manifest Manifest
	if err := readJSON(filepath.Join(directory, item.Kind.manifestFile()), &manifest); err != nil {
		return nil, "", err
	}
	if manifest.Development == nil || manifest.Development.Build == nil {
		return nil, directory, nil
	}
	command := *manifest.Development.Build
	if strings.TrimSpace(command.Command) == "" || filepath.IsAbs(command.Command) {
		return nil, "", failure("INVALID_ARGUMENT", "Build command must name a host executable")
	}
	return &command, directory, nil
}

func (m *Manager) CheckDevelopment(id string) (Candidate, error) {
	item, directory, err := m.Development(id)
	if err != nil {
		return Candidate{}, err
	}
	candidate, err := m.PreviewDirectory(item.Kind, directory)
	if err != nil {
		return Candidate{}, err
	}
	// Development provenance describes the imported checkout, not these locally
	// edited bytes. Publishing must not establish an upstream update baseline.
	return candidate, nil
}

func (m *Manager) Impact(kind Kind, id string) ([]string, error) {
	instances, err := m.Instances()
	if err != nil {
		return nil, err
	}
	affected := []string{}
	for _, instance := range instances {
		if kind == Game && instance.GameID == id || kind == Plugin && slices.ContainsFunc(instance.Dependencies, func(pin DependencyPin) bool { return pin.PluginID == id }) {
			affected = append(affected, instance.Title)
		}
	}
	for _, runtime := range m.RuntimeSnapshots() {
		if runtime.Context.Source.Package == (PackageRef{Kind: kind, ID: id}) && runtime.Context.Scope.Kind != "game-instance" {
			affected = append(affected, fmt.Sprintf("%s / %s", runtime.Context.Scope.ProjectID, runtime.Context.Scope.SessionID))
		}
	}
	return affected, nil
}
