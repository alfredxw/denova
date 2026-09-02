package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryMigratesLegacyProjectStateRootToStores(t *testing.T) {
	denovaDir := t.TempDir()
	workspace := t.TempDir()
	legacyRoot := filepath.Join(denovaDir, "project-state")
	legacyProjectRoot := filepath.Join(legacyRoot, "Book")
	if err := os.MkdirAll(legacyProjectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyProjectRoot, "marker.txt"), []byte("preserved\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := map[string]any{
		"version": legacyProjectStateRegistryVersion,
		"projects": []any{map[string]any{
			"id":        "project-one",
			"type":      TypeGeneral,
			"name":      "Book",
			"state_dir": "Book",
			"location":  ProjectLocation{Kind: LocationExternal, Path: workspace},
		}},
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(denovaDir, "projects.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	projects, err := NewRegistry(denovaDir).List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("Projects = %#v", projects)
	}
	marker, err := os.ReadFile(filepath.Join(denovaDir, "stores", "Book", "marker.txt"))
	if err != nil || string(marker) != "preserved\n" {
		t.Fatalf("migrated Project Store marker = %q, err = %v", marker, err)
	}
	if _, err := os.Stat(legacyRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy project-state root still exists: %v", err)
	}
	persisted, err := os.ReadFile(filepath.Join(denovaDir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(persisted, []byte(`"store_dir": "Book"`)) || bytes.Contains(persisted, []byte(`"state_dir"`)) {
		t.Fatalf("Project Registry did not adopt store_dir: %s", persisted)
	}
}

func TestRegistryNeverMergesLegacyProjectStateRootIntoStores(t *testing.T) {
	denovaDir := t.TempDir()
	legacyRoot := filepath.Join(denovaDir, "project-state")
	storeRoot := filepath.Join(denovaDir, "stores")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(storeRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := NewRegistry(denovaDir).List(true); err == nil {
		t.Fatal("migration merged conflicting project-state and stores roots")
	}
	for _, root := range []string{legacyRoot, storeRoot} {
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			t.Fatalf("migration changed conflicting root %s: info=%v err=%v", root, info, err)
		}
	}
}
