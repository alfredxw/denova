package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
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
