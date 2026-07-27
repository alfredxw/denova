package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/workspacepath"
)

func TestNewDirectoriesUsesPublicBookSkillsDirectory(t *testing.T) {
	workspace := t.TempDir()
	dirs := NewDirectories("", "", workspace)
	if len(dirs) != 1 {
		t.Fatalf("directories = %#v", dirs)
	}
	if got, want := dirs[0].Path, filepath.Join(workspace, "skills"); got != want {
		t.Fatalf("workspace Skills path = %s, want %s", got, want)
	}
}

func TestNewDirectoriesMigratesLegacySkillBundlesWithPublicPrecedence(t *testing.T) {
	workspace := t.TempDir()
	legacyRoot := filepath.Join(workspace, workspacepath.LegacyDataDirName, "skills")
	currentRoot := filepath.Join(workspace, workspacepath.DataDirName, "skills")
	publicRoot := filepath.Join(workspace, "skills")
	writeSkillFixture(t, filepath.Join(legacyRoot, "legacy-only"), "legacy-only", "legacy body")
	if err := os.MkdirAll(filepath.Join(legacyRoot, "legacy-only", "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "legacy-only", "references", "notes.md"), []byte("supporting notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkillFixture(t, filepath.Join(legacyRoot, "shared"), "shared", "legacy shared")
	writeSkillFixture(t, filepath.Join(currentRoot, "shared"), "shared", "current shared")
	writeSkillFixture(t, filepath.Join(publicRoot, "public-only"), "public-only", "public body")
	writeSkillFixture(t, filepath.Join(legacyRoot, "public-wins"), "public-wins", "legacy candidate")
	writeSkillFixture(t, filepath.Join(currentRoot, "public-wins"), "public-wins", "current candidate")
	writeSkillFixture(t, filepath.Join(publicRoot, "public-wins"), "public-wins", "public authority")

	dirs := NewDirectories("", "", workspace)
	if len(dirs) != 1 || dirs[0].Path != publicRoot {
		t.Fatalf("directories = %#v", dirs)
	}
	for _, name := range []string{"legacy-only", "shared", "public-only", "public-wins"} {
		if _, err := os.Stat(filepath.Join(publicRoot, name, SkillFileName)); err != nil {
			t.Fatalf("public Skill %s missing: %v", name, err)
		}
	}
	shared, err := os.ReadFile(filepath.Join(publicRoot, "shared", SkillFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shared), "current shared") || strings.Contains(string(shared), "legacy shared") {
		t.Fatalf(".denova Skill should win over .nova during migration:\n%s", shared)
	}
	public, err := os.ReadFile(filepath.Join(publicRoot, "public-wins", SkillFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(public), "public authority") || strings.Contains(string(public), "candidate") {
		t.Fatalf("public Skill should win over hidden candidates:\n%s", public)
	}
	if notes, err := os.ReadFile(filepath.Join(publicRoot, "legacy-only", "references", "notes.md")); err != nil || string(notes) != "supporting notes" {
		t.Fatalf("supporting file migration failed: content=%q err=%v", notes, err)
	}
	if _, err := os.Stat(filepath.Join(legacyRoot, "legacy-only", SkillFileName)); err != nil {
		t.Fatalf("legacy Skill should remain as a backup: %v", err)
	}
}

func writeSkillFixture(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	content := DefaultContent(name, name+" description") + "\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(root, SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
