package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// testSource imports minimal contract fixtures without reading product examples.
func testSource(t *testing.T, m *Manager, projectID, relative, fixture, id string, kind Kind) Development {
	t.Helper()
	_, layout, err := m.registry.Resolve(projectID, true)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(layout.ContentRoot, filepath.FromSlash(relative))
	for _, source := range []string{"runtime", fixture} {
		if err := os.CopyFS(directory, os.DirFS(filepath.Join("testdata", source))); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(directory, kind.manifestFile())
	var manifest Manifest
	if err := readJSON(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ID = id
	if err := writeJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	development, err := m.LinkDevelopment(kind, projectID, relative)
	if err != nil {
		t.Fatal(err)
	}
	return development
}
