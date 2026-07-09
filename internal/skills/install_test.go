package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseGitHubSource(t *testing.T) {
	tests := []struct {
		name    string
		source  GitHubSource
		want    GitHubRepository
		wantErr bool
	}{
		{
			name:   "shorthand",
			source: GitHubSource{URL: "owner/repo"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo"},
		},
		{
			name:   "full url",
			source: GitHubSource{URL: "https://github.com/owner/repo.git"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo"},
		},
		{
			name:   "tree url",
			source: GitHubSource{URL: "https://github.com/owner/repo/tree/main/skills/foo"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo", Ref: "main", Subdir: "skills/foo"},
		},
		{
			name:   "tree url with nested skill path",
			source: GitHubSource{URL: "https://github.com/anthropics/skills/tree/main/skills/canvas-design"},
			want:   GitHubRepository{Owner: "anthropics", Repo: "skills", Ref: "main", Subdir: "skills/canvas-design"},
		},
		{
			name:   "overrides",
			source: GitHubSource{URL: "https://github.com/owner/repo/tree/main/skills/foo", Ref: "release", Subdir: "skills/bar"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo", Ref: "release", Subdir: "skills/bar"},
		},
		{
			name:    "invalid host",
			source:  GitHubSource{URL: "https://gitlab.com/owner/repo"},
			wantErr: true,
		},
		{
			name:    "empty",
			source:  GitHubSource{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGitHubSource(tt.source)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseGitHubSource() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGitHubSource() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseGitHubSource() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRemoteArchiveDetectsGitHubRepositorySources(t *testing.T) {
	tests := []struct {
		name   string
		source RemoteArchiveSource
		want   GitHubRepository
	}{
		{
			name:   "shorthand",
			source: RemoteArchiveSource{URL: "owner/repo"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo"},
		},
		{
			name:   "tree URL",
			source: RemoteArchiveSource{URL: "https://github.com/owner/repo/tree/main/skills/foo"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo", Ref: "main", Subdir: "skills/foo"},
		},
		{
			name:   "anthropic skills tree URL",
			source: RemoteArchiveSource{URL: "https://github.com/anthropics/skills/tree/main/skills/canvas-design"},
			want:   GitHubRepository{Owner: "anthropics", Repo: "skills", Ref: "main", Subdir: "skills/canvas-design"},
		},
		{
			name:   "archive URL",
			source: RemoteArchiveSource{URL: "https://github.com/owner/repo/archive/refs/heads/main.zip"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo", Ref: "refs/heads/main"},
		},
		{
			name:   "overrides",
			source: RemoteArchiveSource{URL: "owner/repo", Ref: "release", Subdir: "skills/bar"},
			want:   GitHubRepository{Owner: "owner", Repo: "repo", Ref: "release", Subdir: "skills/bar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := githubRepositoryFromRemoteSource(tt.source)
			if err != nil {
				t.Fatalf("githubRepositoryFromRemoteSource() error = %v", err)
			}
			if !ok {
				t.Fatalf("githubRepositoryFromRemoteSource() did not detect GitHub source")
			}
			if got != tt.want {
				t.Fatalf("githubRepositoryFromRemoteSource() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPreviewRemoteArchiveDownloadsHTTPSZipWithSubdir(t *testing.T) {
	ctx := context.Background()
	serverURL, restoreHTTPClient := useSkillInstallHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archives/skills.zip" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(makeSkillZip(t, map[string]string{
			"pkg/catalog/skills/remote-one/SKILL.md": DefaultContent("remote-one", "remote desc"),
			"pkg/skills/outside/SKILL.md":            DefaultContent("outside", "outside desc"),
		}))
	})
	defer restoreHTTPClient()

	preview, err := PreviewRemoteArchive(ctx, nil, "", RemoteArchiveSource{
		URL:    serverURL + "/archives/skills.zip",
		Subdir: "catalog",
	})
	if err != nil {
		t.Fatalf("PreviewRemoteArchive() error = %v", err)
	}
	got := candidateNames(preview.Candidates)
	if !got["remote-one"] {
		t.Fatalf("remote-one candidate missing from %#v", preview.Candidates)
	}
	if got["outside"] {
		t.Fatalf("outside candidate should be excluded by subdir: %#v", preview.Candidates)
	}
}

func TestInstallRemoteArchiveInstallsSelectedSkill(t *testing.T) {
	ctx := context.Background()
	userDir := filepath.Join(t.TempDir(), "user")
	dirs := []Directory{{Scope: ScopeUser, Path: userDir, Writable: true}}
	serverURL, restoreHTTPClient := useSkillInstallHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(makeSkillZip(t, map[string]string{
			"repo/skills/remote-one/SKILL.md":     DefaultContent("remote-one", "remote desc"),
			"repo/skills/remote-one/assets/a.txt": "asset",
			"repo/skills/remote-two/SKILL.md":     DefaultContent("remote-two", "remote desc"),
		}))
	})
	defer restoreHTTPClient()
	source := RemoteArchiveSource{URL: serverURL + "/skills.zip"}

	preview, err := PreviewRemoteArchive(ctx, dirs, ScopeUser, source)
	if err != nil {
		t.Fatalf("PreviewRemoteArchive() error = %v", err)
	}
	result, err := InstallRemoteArchive(ctx, dirs, ScopeUser, source, []string{candidateIDByName(t, preview.Candidates, "remote-one")})
	if err != nil {
		t.Fatalf("InstallRemoteArchive() error = %v", err)
	}
	if len(result.Installed) != 1 || result.Installed[0].Name != "remote-one" {
		t.Fatalf("installed = %#v, want remote-one", result.Installed)
	}
	if _, err := os.Stat(filepath.Join(userDir, "remote-one", "assets", "a.txt")); err != nil {
		t.Fatalf("asset missing: %v", err)
	}
}

func TestRemoteArchiveRejectsInvalidAndOversizedSources(t *testing.T) {
	ctx := context.Background()
	for _, source := range []RemoteArchiveSource{
		{},
		{URL: "http://example.com/skills.zip"},
	} {
		if _, err := PreviewRemoteArchive(ctx, nil, "", source); err == nil {
			t.Fatalf("PreviewRemoteArchive(%#v) expected error", source)
		}
	}

	serverURL, restoreHTTPClient := useSkillInstallHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(MaxInstallArchiveBytes+1, 10))
		w.WriteHeader(http.StatusOK)
	})
	defer restoreHTTPClient()
	_, err := PreviewRemoteArchive(ctx, nil, "", RemoteArchiveSource{
		URL: serverURL + "/big.zip",
	})
	if err == nil || !strings.Contains(err.Error(), "32MB") {
		t.Fatalf("oversized remote archive error = %v, want size limit", err)
	}
}

func TestRemoteArchiveRejectsNonZipResponseWithActionableError(t *testing.T) {
	ctx := context.Background()
	serverURL, restoreHTTPClient := useSkillInstallHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not a zip</html>"))
	})
	defer restoreHTTPClient()
	_, err := PreviewRemoteArchive(ctx, nil, "", RemoteArchiveSource{
		URL: serverURL + "/tree/main/skills",
	})
	if err == nil {
		t.Fatalf("PreviewRemoteArchive() expected non-zip error")
	}
	for _, want := range []string{"not a ZIP archive", "owner/repo", ".zip"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want to contain %q", err.Error(), want)
		}
	}
}

func TestPreviewRemoteArchiveRejectsUnsafeZip(t *testing.T) {
	ctx := context.Background()
	serverURL, restoreHTTPClient := useSkillInstallHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(makeSymlinkZip(t))
	})
	defer restoreHTTPClient()
	_, err := PreviewRemoteArchive(ctx, nil, "", RemoteArchiveSource{
		URL: serverURL + "/unsafe.zip",
	})
	if err == nil {
		t.Fatalf("PreviewRemoteArchive() should reject unsafe zip")
	}
}

func TestPreviewDirectoryDiscoversRootFlatAndCatalogSkills(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeSkillFile(t, root, "skills/flat", "flat", "flat desc")
	writeSkillFile(t, root, "skills/category/nested", "nested", "nested desc")
	writeSkillFile(t, root, ".agents/skills/agent-skill", "agent-skill", "agent desc")
	writeSkillFile(t, root, ".", "root-skill", "root desc")

	preview, err := PreviewDirectory(ctx, nil, "", root)
	if err != nil {
		t.Fatalf("PreviewDirectory() error = %v", err)
	}
	got := candidateNames(preview.Candidates)
	for _, name := range []string{"root-skill", "flat", "nested", "agent-skill"} {
		if !got[name] {
			t.Fatalf("candidate %q missing from %#v", name, preview.Candidates)
		}
	}
}

func TestPreviewDirectoryMarksDuplicateAndInvalidCandidates(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeSkillFile(t, root, "skills/a", "same", "a desc")
	writeSkillFile(t, root, "skills/b", "same", "b desc")
	invalidDir := filepath.Join(root, "skills", "bad")
	if err := os.MkdirAll(invalidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, SkillFileName), []byte("---\nname: bad\n---\nmissing description\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewDirectory(ctx, nil, "", root)
	if err != nil {
		t.Fatalf("PreviewDirectory() error = %v", err)
	}
	var duplicateConflicts int
	var invalidFound bool
	for _, candidate := range preview.Candidates {
		if candidate.Name == "same" && candidate.Conflict {
			duplicateConflicts++
		}
		if candidate.SourcePath == "skills/bad" && candidate.InvalidReason != "" {
			invalidFound = true
		}
	}
	if duplicateConflicts != 2 {
		t.Fatalf("duplicate conflicts = %d, want 2; candidates=%#v", duplicateConflicts, preview.Candidates)
	}
	if !invalidFound {
		t.Fatalf("invalid candidate not reported: %#v", preview.Candidates)
	}
}

func TestInstallZipInstallsOnlySelectedSkillWithAssets(t *testing.T) {
	ctx := context.Background()
	userDir := filepath.Join(t.TempDir(), "user")
	dirs := []Directory{{Scope: ScopeUser, Path: userDir, Writable: true}}
	zipData := makeSkillZip(t, map[string]string{
		"repo/skills/one/SKILL.md":     DefaultContent("one", "one desc"),
		"repo/skills/one/assets/a.txt": "asset",
		"repo/skills/two/SKILL.md":     DefaultContent("two", "two desc"),
		"repo/skills/two/assets/b.txt": "skip",
		"repo/README.md":               "readme",
	})
	preview, err := PreviewZip(ctx, dirs, ScopeUser, zipData)
	if err != nil {
		t.Fatalf("PreviewZip() error = %v", err)
	}
	selected := candidateIDByName(t, preview.Candidates, "one")
	result, err := InstallZip(ctx, dirs, ScopeUser, zipData, []string{selected})
	if err != nil {
		t.Fatalf("InstallZip() error = %v", err)
	}
	if len(result.Installed) != 1 || result.Installed[0].Name != "one" {
		t.Fatalf("installed = %#v, want one", result.Installed)
	}
	if _, err := os.Stat(filepath.Join(userDir, "one", "assets", "a.txt")); err != nil {
		t.Fatalf("asset missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(userDir, "two", SkillFileName)); !os.IsNotExist(err) {
		t.Fatalf("unselected skill should not be installed, stat err=%v", err)
	}
}

func TestInstallZipRejectsConflictWithoutPartialInstall(t *testing.T) {
	ctx := context.Background()
	userDir := filepath.Join(t.TempDir(), "user")
	dirs := []Directory{{Scope: ScopeUser, Path: userDir, Writable: true}}
	writeSkillFile(t, userDir, "existing", "existing", "existing desc")
	zipData := makeSkillZip(t, map[string]string{
		"skills/existing/SKILL.md": DefaultContent("existing", "new desc"),
		"skills/new-one/SKILL.md":  DefaultContent("new-one", "new desc"),
	})
	preview, err := PreviewZip(ctx, dirs, ScopeUser, zipData)
	if err != nil {
		t.Fatalf("PreviewZip() error = %v", err)
	}
	_, err = InstallZip(ctx, dirs, ScopeUser, zipData, []string{
		candidateIDByName(t, preview.Candidates, "existing"),
		candidateIDByName(t, preview.Candidates, "new-one"),
	})
	if err == nil {
		t.Fatalf("InstallZip() expected conflict error")
	}
	if _, err := os.Stat(filepath.Join(userDir, "new-one", SkillFileName)); !os.IsNotExist(err) {
		t.Fatalf("new-one should not be partially installed, stat err=%v", err)
	}
}

func TestPreviewZipRejectsPathTraversalAndSymlink(t *testing.T) {
	ctx := context.Background()
	if _, err := PreviewZip(ctx, nil, "", makeSkillZip(t, map[string]string{"../bad": "bad"})); err == nil {
		t.Fatalf("PreviewZip() should reject path traversal")
	}
	if _, err := PreviewZip(ctx, nil, "", makeSymlinkZip(t)); err == nil {
		t.Fatalf("PreviewZip() should reject symlink")
	}
}

func TestPreviewZipWithNoSkillsReturnsEmptyCandidates(t *testing.T) {
	ctx := context.Background()
	preview, err := PreviewZip(ctx, nil, "", makeSkillZip(t, map[string]string{"repo/README.md": "readme"}))
	if err != nil {
		t.Fatalf("PreviewZip() error = %v", err)
	}
	if len(preview.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want empty", preview.Candidates)
	}
}

func makeSkillZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeSymlinkZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: "skills/link"}
	header.SetMode(os.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("target")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func candidateNames(candidates []InstallCandidate) map[string]bool {
	out := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.Name != "" {
			out[candidate.Name] = true
		}
	}
	return out
}

func candidateIDByName(t *testing.T, candidates []InstallCandidate, name string) string {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Name == name {
			return candidate.ID
		}
	}
	t.Fatalf("candidate %q not found in %#v", name, candidates)
	return ""
}

func useSkillInstallHTTPServer(t *testing.T, handler http.HandlerFunc) (string, func()) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	previous := skillInstallHTTPClient
	skillInstallHTTPClient = server.Client()
	return server.URL, func() {
		skillInstallHTTPClient = previous
		server.Close()
	}
}
