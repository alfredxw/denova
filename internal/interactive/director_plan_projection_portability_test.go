package interactive

import (
	"path/filepath"
	"testing"
)

func TestDirectorPlanDocInfosPersistContentRelativePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "interactive", "stories", "story-1", "director", "main")
	infos := directorPlanDocInfos(root, dir, DirectorPlanDocs{
		Plan: "plan", AgentBrief: "brief", LoreContext: "lore",
	})
	if got := infos[DirectorPlanDocPlan].Path; got != "interactive/stories/story-1/director/main/director.md" {
		t.Fatalf("Director plan path = %q", got)
	}
}
