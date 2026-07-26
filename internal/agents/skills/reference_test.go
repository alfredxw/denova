package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendReadReferenceUsesActiveSkillAndLineWindow(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	writeSkillFile(t, user, "guide", "guide", "user guide")
	writeSkillFile(t, workspace, "guide", "guide", "workspace guide")
	writeReferenceFixture(t, user, "guide", "references/flow.md", "user\n")
	writeReferenceFixture(t, workspace, "guide", "references/flow.md", "first\nsecond\nthird\n")

	backend := NewAgentBackend([]Directory{
		{Scope: ScopeUser, Path: user},
		{Scope: ScopeWorkspace, Path: workspace},
	}, "ide", nil)
	got, err := backend.ReadReference(context.Background(), "skill://guide/references/flow.md", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.URI != "skill://guide/references/flow.md" || got.Content != "second\n" || got.Offset != 2 || got.Limit != 1 || got.Total != 3 {
		t.Fatalf("reference read = %+v", got)
	}
}

func TestBackendReadReferenceAppliesBoundedDefault(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "guide", "guide", "guide")
	var content strings.Builder
	for index := 1; index <= defaultSkillReferenceLines+1; index++ {
		fmt.Fprintf(&content, "line-%d\n", index)
	}
	writeReferenceFixture(t, root, "guide", "references/long.md", content.String())
	backend := NewBackend([]Directory{{Scope: ScopeUser, Path: root}})

	got, err := backend.ReadReference(context.Background(), "skill://guide/references/long.md", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Offset != 1 || got.Limit != defaultSkillReferenceLines || got.Total != defaultSkillReferenceLines+1 || strings.Contains(got.Content, fmt.Sprintf("line-%d", defaultSkillReferenceLines+1)) {
		t.Fatalf("default reference window = offset=%d limit=%d total=%d tail=%q", got.Offset, got.Limit, got.Total, got.Content[len(got.Content)-32:])
	}
}

func TestBackendReadReferenceRejectsUnavailableAndUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	writeSkillFileForAgents(t, root, "guide", "guide", "guide", "interactive_story")
	writeReferenceFixture(t, root, "guide", "references/flow.md", "safe\n")
	writeReferenceFixture(t, root, "guide", "assets/hidden.md", "hidden\n")
	backend := NewAgentBackend([]Directory{{Scope: ScopeUser, Path: root}}, "ide", nil)

	for _, rawURI := range []string{
		"skill://guide/references/flow.md",
		"skill://guide/references/../assets/hidden.md",
		"skill://guide/assets/hidden.md",
		"https://guide/references/flow.md",
		"skill://guide/references/flow.md?raw=true",
	} {
		if _, err := backend.ReadReference(context.Background(), rawURI, 1, 10); err == nil {
			t.Fatalf("ReadReference(%q) should fail", rawURI)
		}
	}
}

func TestBackendReadReferenceRejectsSymlinkAndNonUTF8(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "guide", "guide", "guide")
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	references := filepath.Join(root, "guide", "references")
	if err := os.MkdirAll(references, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(references, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(references, "binary.md"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend([]Directory{{Scope: ScopeUser, Path: root}})
	for _, rawURI := range []string{"skill://guide/references/escape.md", "skill://guide/references/binary.md"} {
		if _, err := backend.ReadReference(context.Background(), rawURI, 1, 10); err == nil {
			t.Fatalf("ReadReference(%q) should fail", rawURI)
		}
	}
}

func writeReferenceFixture(t *testing.T, root, name, relative, content string) {
	t.Helper()
	filePath := filepath.Join(root, name, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
