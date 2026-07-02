package workspacepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathUsesLegacyTargetWhenCurrentOnlyHasEmptyGeneratedLore(t *testing.T) {
	workspace := t.TempDir()
	currentLore := filepath.Join(workspace, DataDirName, "lore", "items.json")
	legacyLore := filepath.Join(workspace, LegacyDataDirName, "lore", "items.json")
	writeFile(t, currentLore, `{"version":1,"items":[]}`)
	writeFile(t, legacyLore, `{"version":1,"items":[{"id":"hero"}]}`)

	if got := Path(workspace, "lore", "items.json"); got != legacyLore {
		t.Fatalf("Path should keep using legacy lore with data: want=%s got=%s", legacyLore, got)
	}
	if got := Rel(workspace, "lore", "items.json"); got != ".nova/lore/items.json" {
		t.Fatalf("Rel should match the selected legacy lore path: %s", got)
	}
}

func TestPathKeepsCurrentTargetWhenBothTargetsHaveData(t *testing.T) {
	workspace := t.TempDir()
	currentLore := filepath.Join(workspace, DataDirName, "lore", "items.json")
	legacyLore := filepath.Join(workspace, LegacyDataDirName, "lore", "items.json")
	writeFile(t, currentLore, `{"version":1,"items":[{"id":"current"}]}`)
	writeFile(t, legacyLore, `{"version":1,"items":[{"id":"legacy"}]}`)

	if got := Path(workspace, "lore", "items.json"); got != currentLore {
		t.Fatalf("Path should keep current target when it has data: want=%s got=%s", currentLore, got)
	}
}

func TestDirNameFallsBackToLegacyWhenCurrentOnlyHasEphemeralData(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, DataDirName, "runs", "run.jsonl"), `{"type":"run_created"}`)
	writeFile(t, filepath.Join(workspace, LegacyDataDirName, "lore", "items.json"), `{"version":1,"items":[{"id":"hero"}]}`)

	if got := DirName(workspace); got != LegacyDataDirName {
		t.Fatalf("DirName should keep legacy active when current only has ephemeral data: %s", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
