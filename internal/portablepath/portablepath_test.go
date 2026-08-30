package portablepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortablePathRules(t *testing.T) {
	t.Parallel()
	valid := []string{"chapters/第一章.md", "assets/Café.png", "emoji/✨.txt"}
	for _, value := range valid {
		if err := Validate(value); err != nil {
			t.Errorf("Validate(%q): %v", value, err)
		}
	}
	invalid := []string{
		"CON.txt",
		"COM¹.txt",
		"lpt³.log",
		"folder/name?.md",
		"folder/trailing. ",
		"Cafe\u0301.md",
		`folder\file.md`,
	}
	for _, value := range invalid {
		if err := Validate(value); err == nil {
			t.Errorf("Validate(%q) unexpectedly succeeded", value)
		}
	}
}

func TestCheckNoCollisionUsesUnicodeCaseFold(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Résumé.md"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckNoCollision(root, "résumé.md"); err == nil {
		t.Fatal("case-fold collision was accepted")
	}
	if err := CheckNoCollision(root, "Résumé.md"); err != nil {
		t.Fatalf("exact existing path was rejected: %v", err)
	}
}

func TestPreflightRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := PreflightTree(root); err == nil {
		t.Fatal("symlink tree passed portable preflight")
	}
}
