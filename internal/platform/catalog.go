package platform

import (
	"archive/zip"
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
)

// CatalogEntry projects install state and activation readiness. It is not a
// second registry; the package's installed record remains authoritative.
type CatalogEntry struct {
	Kind Kind `json:"kind"`
	Installed
	UnavailableReason string `json:"unavailableReason,omitempty"`
}

func (m *Manager) Catalog() ([]CatalogEntry, error) {
	result := []CatalogEntry{}
	for _, kind := range []Kind{Plugin, Game} {
		items, err := m.List(kind)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			entry := CatalogEntry{Kind: kind, Installed: item}
			release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: kind, ID: item.ID}, ReleaseID: item.CurrentRelease})
			if err == nil && release.Manifest.APIMajor != APIMajor {
				err = failure("API_INCOMPATIBLE", "Package requires API %d", release.Manifest.APIMajor)
			}
			if err == nil {
				_, err = m.resolveDependencies(release.Manifest, nil)
			}
			if err != nil {
				_, body := ErrorResponse(err)
				entry.UnavailableReason = body.MessageKey
			}
			result = append(result, entry)
		}
	}
	return result, nil
}

// DetectKind recognizes an explicit package root without scanning user content.
func DetectKind(directory string) (Kind, error) {
	names := []string{}
	for _, kind := range []Kind{Plugin, Game} {
		if _, err := os.Stat(filepath.Join(directory, kind.manifestFile())); err == nil {
			names = append(names, kind.manifestFile())
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return detectManifestKind(names)
}

func detectManifestKind(names []string) (Kind, error) {
	var selected Kind
	for _, name := range names {
		for _, kind := range []Kind{Plugin, Game} {
			if name != kind.manifestFile() {
				continue
			}
			if selected != "" {
				return "", failure("INVALID_ARGUMENT", "A package must contain exactly one root manifest")
			}
			selected = kind
		}
	}
	if selected == "" {
		return "", failure("INVALID_ARGUMENT", "No supported extension manifest was found")
	}
	return selected, nil
}

func (m *Manager) PreviewExtensionDirectory(directory string) (Candidate, error) {
	kind, err := DetectKind(directory)
	if err != nil {
		return Candidate{}, err
	}
	return m.PreviewDirectory(kind, directory)
}

func (m *Manager) PreviewExtensionZIP(data []byte) (Candidate, error) {
	if len(data) > MaxPackageBytes {
		return Candidate{}, failure("LIMIT_EXCEEDED", "Archive exceeds package limit")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Candidate{}, err
	}
	names := make([]string, 0, len(archive.File))
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	kind, err := detectManifestKind(names)
	if err != nil {
		return Candidate{}, err
	}
	return m.PreviewZIP(kind, data)
}

// GamePreferences stores the user's default for new storylines only. Existing
// instances always retain their own game and release bindings.
type GamePreferences struct {
	DefaultGameID string `json:"defaultGameId"`
}

const BuiltinGameID = "builtin.story"

func (m *Manager) GamePreferences() (GamePreferences, error) {
	value := GamePreferences{DefaultGameID: BuiltinGameID}
	err := readJSON(filepath.Join(m.root, "games", "preferences.json"), &value)
	if err != nil && !os.IsNotExist(err) {
		return value, err
	}
	if value.DefaultGameID != BuiltinGameID {
		items, err := m.Catalog()
		if err != nil {
			return value, err
		}
		for _, item := range items {
			if item.Kind == Game && item.ID == value.DefaultGameID && item.Enabled && !item.Removed && item.UnavailableReason == "" {
				return value, nil
			}
		}
		value.DefaultGameID = BuiltinGameID
	}
	return value, nil
}

func (m *Manager) SetDefaultGame(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != BuiltinGameID {
		items, err := m.Catalog()
		if err != nil {
			return err
		}
		found := false
		for _, item := range items {
			if item.Kind == Game && item.ID == id && item.Enabled && !item.Removed && item.UnavailableReason == "" {
				found = true
			}
		}
		if !found {
			return failure("DEPENDENCY_UNAVAILABLE", "Default game is unavailable")
		}
	}
	if err := writeJSON(filepath.Join(m.root, "games", "preferences.json"), GamePreferences{DefaultGameID: id}); err != nil {
		return err
	}
	slog.Info("platform_default_game_updated", "game", id)
	return nil
}
