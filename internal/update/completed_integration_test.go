package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeNotesSurvivePreparingNextUpdateAndMovingInstallation(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "original")
	service := &Service{currentVersion: "0.5.1", executablePath: filepath.Join(install, "nova")}
	writeUpdateTestPackage(t, install, updateTestPackageContents{Executable: "old"})
	source := filepath.Join(root, "first")
	writeUpdateTestPackage(t, source, updateTestPackageContents{Executable: "new"})
	if _, err := service.stageUpdate(source, "0.6.3"); err != nil {
		t.Fatal(err)
	}
	path, err := readPendingManifestRef(updateDataDir(install))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := setPhase(path, &manifest, PhaseSucceeded, nil); err != nil {
		t.Fatal(err)
	}
	service.currentVersion = "0.6.3"
	next := filepath.Join(root, "next")
	writeUpdateTestPackage(t, next, updateTestPackageContents{Executable: "next"})
	const notes = "## [v0.6.4]\n### Brief\n- Next release\n"
	if err := os.WriteFile(filepath.Join(next, "CHANGELOG.md"), []byte(notes), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.stageUpdate(next, "0.6.4"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "moved")
	if err := os.Rename(install, moved); err != nil {
		t.Fatal(err)
	}
	service.executablePath = filepath.Join(moved, "nova")
	status, err := service.Status()
	if err != nil || status.Phase != PhaseStaged || status.CompletedUpdate == nil {
		t.Fatalf("lost completed update while staging: %+v, %v", status, err)
	}
	completed := status.CompletedUpdate
	if completed.ID != manifest.ID || completed.PreviousVersion != "0.5.1" || completed.Version != "0.6.3" || completed.CompletedAt.IsZero() {
		t.Fatalf("incorrect comparison baseline: %+v", completed)
	}
	// A local upload's bundled history must remain readable without a network.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("staged notes attempted a network request")
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	service.githubRawBase = server.URL
	actual, err := service.Changelog(context.Background(), "0.6.4")
	if err != nil || actual != notes {
		t.Fatalf("offline notes = %q, %v", actual, err)
	}
	service.currentVersion = "0.6.5"
	if completed, err := service.CompletedUpdate(); err != nil || completed != nil {
		t.Fatalf("unrelated running version inherited an old upgrade: %+v, %v", completed, err)
	}
}

func TestUpdateChangelogUsesRequestedReleaseTag(t *testing.T) {
	const notes = "## [v0.6.10]\n- Release history\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alfredxw/denova/v0.6.10/CHANGELOG.md" {
			t.Errorf("incorrect source: %s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(notes))
	}))
	defer server.Close()
	service := &Service{repository: "alfredxw/denova", executablePath: filepath.Join(t.TempDir(), "nova"), githubRawBase: server.URL, httpClient: server.Client()}
	actual, err := service.Changelog(context.Background(), "v0.6.10")
	if err != nil || actual != notes {
		t.Fatalf("release notes = %q, %v", actual, err)
	}
}
