package skills

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteUpdatesPersistConsentProtectEditsAndKeepBackup(t *testing.T) {
	ctx := context.Background()
	var version atomic.Int32
	version.Store(1)
	var requests atomic.Int32
	archive := func(body string) []byte {
		return makeSkillZip(t, map[string]string{"repo/skills/remote/SKILL.md": DefaultContent("remote", body), "repo/skills/remote/references/help.md": body})
	}
	v1, v2 := archive("first version"), archive("second version")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if version.Load() == 1 {
			_, _ = w.Write(v1)
		} else {
			_, _ = w.Write(v2)
		}
	}))
	defer server.Close()
	previous := skillInstallHTTPClient
	skillInstallHTTPClient = server.Client()
	t.Cleanup(func() { skillInstallHTTPClient = previous })
	dirs := []Directory{{Scope: ScopeUser, Path: t.TempDir(), Writable: true}}
	source := RemoteArchiveSource{URL: server.URL + "/skills.zip"}
	preview, err := PreviewRemoteArchive(ctx, dirs, ScopeUser, source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := InstallRemoteArchive(ctx, dirs, ScopeUser, source, []string{preview.Candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if installed.Installed[0].Remote == nil || installed.Installed[0].Remote.AutoUpdate {
		t.Fatalf("consent not off: %+v", installed)
	}
	before := requests.Load()
	if got := UpdateDue(ctx, dirs, time.Now()); len(got) != 0 || requests.Load() != before {
		t.Fatal("disabled updater fetched an archive")
	}
	version.Store(2)
	checked := RefreshRemote(ctx, dirs, ScopeUser, "remote", CheckUpdate)
	if checked.ErrorKey != "" || !checked.Remote.UpdateAvailable {
		t.Fatalf("check: %+v", checked)
	}
	doc, _ := ReadDocument(ctx, dirs, ScopeUser, "remote")
	if doc.Description != "first version" {
		t.Fatal("check installed an update")
	}
	if _, err := SetAutoUpdate(ctx, dirs, ScopeUser, "remote", true); err != nil {
		t.Fatal(err)
	}
	preferenceDoc, err := ReadDocument(ctx, dirs, ScopeUser, "remote")
	if err != nil || preferenceDoc.Revision != doc.Revision {
		t.Fatal("automatic update consent invalidated the content revision")
	}
	now := time.Now().Add(25 * time.Hour)
	updated := UpdateDue(ctx, dirs, now)
	if len(updated) != 1 || updated[0].ErrorKey != "" || updated[0].Remote.Status != "updated" {
		t.Fatalf("automatic update: %+v", updated)
	}
	doc, err = ReadDocument(ctx, dirs, ScopeUser, "remote")
	if err != nil || doc.Description != "second version" {
		t.Fatalf("updated doc: %+v %v", doc, err)
	}
	backups, _ := os.ReadDir(filepath.Join(dirs[0].Path, ".denova-backups", "remote"))
	if len(backups) != 1 {
		t.Fatalf("missing backup: %v", backups)
	}
	backup, err := os.ReadFile(filepath.Join(dirs[0].Path, ".denova-backups", "remote", backups[0].Name(), "references", "help.md"))
	if err != nil || string(backup) != "first version" {
		t.Fatal("backup lost previous content")
	}
	before = requests.Load()
	if got := UpdateDue(ctx, dirs, now.Add(23*time.Hour)); len(got) != 0 || requests.Load() != before {
		t.Fatal("daily cadence lost after reread")
	}
	if err := os.WriteFile(filepath.Join(dirs[0].Path, "remote", "references", "help.md"), []byte("local edits"), 0o644); err != nil {
		t.Fatal(err)
	}
	version.Store(1)
	modified := UpdateDue(ctx, dirs, now.Add(25*time.Hour))
	if len(modified) != 1 || modified[0].Remote.Status != "modified" {
		t.Fatalf("local edits not detected: %+v", modified)
	}
	content, _ := os.ReadFile(filepath.Join(dirs[0].Path, "remote", "references", "help.md"))
	if string(content) != "local edits" {
		t.Fatal("local content overwritten")
	}
}

func TestRemoteFailurePersistsAttemptAndLeavesContent(t *testing.T) {
	ctx := context.Background()
	var fail atomic.Bool
	var invalid atomic.Bool
	var requests atomic.Int32
	archive := makeSkillZip(t, map[string]string{"repo/skills/remote/SKILL.md": DefaultContent("remote", "original")})
	unportable := makeSkillZip(t, map[string]string{
		"repo/skills/remote/SKILL.md":               DefaultContent("remote", "new upstream"),
		"repo/skills/remote/references/invalid?.md": "Not portable",
	})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if invalid.Load() {
			_, _ = w.Write(unportable)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	previous := skillInstallHTTPClient
	skillInstallHTTPClient = server.Client()
	t.Cleanup(func() { skillInstallHTTPClient = previous })
	dirs := []Directory{{Scope: ScopeUser, Path: t.TempDir(), Writable: true}}
	source := RemoteArchiveSource{URL: server.URL + "/skills.zip"}
	preview, err := PreviewRemoteArchive(ctx, dirs, ScopeUser, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InstallRemoteArchive(ctx, dirs, ScopeUser, source, []string{preview.Candidates[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetAutoUpdate(ctx, dirs, ScopeUser, "remote", true); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	now := time.Now().UTC()
	results := UpdateDue(ctx, dirs, now)
	if len(results) != 1 || results[0].ErrorKey == "" || results[0].Remote.Status != "error" {
		t.Fatalf("failure result: %+v", results)
	}
	state, err := readRemoteState(filepath.Join(dirs[0].Path, "remote"))
	if err != nil || !state.CheckedAt.Equal(now) || !state.AutoUpdate {
		t.Fatalf("failure state: %+v %v", state, err)
	}
	before := requests.Load()
	if got := UpdateDue(ctx, dirs, now.Add(time.Hour)); len(got) != 0 || requests.Load() != before {
		t.Fatal("failed source retried before daily interval")
	}
	fail.Store(false)
	invalid.Store(true)
	if got := UpdateDue(ctx, dirs, now.Add(25*time.Hour)); len(got) != 1 || got[0].ErrorKey == "" {
		t.Fatalf("unportable update was not rejected: %+v", got)
	}
	doc, err := ReadDocument(ctx, dirs, ScopeUser, "remote")
	if err != nil || doc.Description != "original" {
		t.Fatal("failed update changed local content")
	}
}

func TestArchiveCannotImportAutomaticUpdateConsent(t *testing.T) {
	ctx := context.Background()
	archive := makeSkillZip(t, map[string]string{
		"repo/skills/imported/SKILL.md":           DefaultContent("imported", "Local archive"),
		"repo/skills/imported/" + remoteStateFile: `{"source":{"url":"https://example.com/archive.zip"},"source_path":".","digest":"fake","auto_update":true}`,
	})
	dirs := []Directory{{Scope: ScopeUser, Path: t.TempDir(), Writable: true}}
	preview, err := PreviewZip(ctx, dirs, ScopeUser, archive)
	if err != nil {
		t.Fatal(err)
	}
	result, err := InstallZip(ctx, dirs, ScopeUser, archive, []string{preview.Candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Installed[0].Remote != nil {
		t.Fatal("ZIP archive imported remote update consent")
	}
}

func TestRecoverInterruptedRemoteDirectoryReplacement(t *testing.T) {
	dir := Directory{Scope: ScopeUser, Path: t.TempDir(), Writable: true}
	writeSkillFile(t, dir.Path, "remote", "remote", "original")
	backup := filepath.Join(dir.Path, ".denova-backups", "remote")
	if err := os.MkdirAll(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir.Path, "remote"), filepath.Join(backup, "pending")); err != nil {
		t.Fatal(err)
	}
	recoverDirectoryUpdates(context.Background(), dir)
	doc, err := ReadDocument(context.Background(), []Directory{dir}, ScopeUser, "remote")
	if err != nil || doc.Description != "original" {
		t.Fatalf("recovery failed: %+v %v", doc, err)
	}
}
