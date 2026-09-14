package platform

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExportInstalledPreservesReleaseWithoutLocalData(t *testing.T) {
	for _, kind := range []Kind{Plugin, Game} {
		t.Run(string(kind), func(t *testing.T) {
			m, projectID := testManager(t)
			fixture := "static"
			if kind == Plugin {
				fixture = "tool"
			}
			candidate := testCandidate(t, m, projectID, fixture, "test.export", kind)
			m.DiscardCandidate(candidate.ID)
			// ZIP imports may contain extra runtime files outside distribution.files.
			candidate.files["extra.txt"] = []byte("keep the exact installed bytes")
			candidate, err := m.freeze(kind, candidate.files)
			if err != nil {
				t.Fatal(err)
			}
			release := testInstall(t, m, candidate)
			localSettings := filepath.Join(m.packagePath(release.Ref.Package), "settings.toml")
			if err := os.WriteFile(localSettings, []byte("private_setting = true"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.SetAvailability(context.Background(), kind, release.Manifest.ID, PackageAvailability{Enabled: false}); err != nil {
				t.Fatal(err)
			}
			m.DiscardCandidate(candidate.ID)
			var archive bytes.Buffer
			if err := m.ExportInstalled(release.Ref, &archive); err != nil {
				t.Fatal(err)
			}
			if len(m.candidates) != 0 {
				t.Fatal("export retained a temporary candidate")
			}
			imported, err := m.PreviewExtensionZIP(archive.Bytes())
			if err != nil || imported.Digest != release.Digest || !reflect.DeepEqual(imported.files, candidate.files) {
				t.Fatalf("export changed release content: %#v %v", imported, err)
			}
			if _, found := imported.files["settings.toml"]; found {
				t.Fatal("export included local settings")
			}
			if err := os.WriteFile(filepath.Join(m.releasePath(release.Ref), "extra.txt"), []byte("tampered"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.ExportInstalled(release.Ref, &bytes.Buffer{}); err == nil {
				t.Fatal("export accepted modified installed bytes")
			}
		})
	}
}
