package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinWritingPresetInstructionsCoverScopeInference(t *testing.T) {
	for _, name := range []string{"novel-lite", "novel-standard"} {
		content := readBuiltinWritingPreset(t, name)
		for _, required := range []string{
			"agent: ide",
			"category: writing",
			"writing-workflow",
			"Do not assume the task is the next chapter",
			"There is no `writing_scope` field",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s missing required instruction %q", name, required)
			}
		}
	}
}

func TestBuiltinWritingPresetInstructionsCoverMultiChapterPlanning(t *testing.T) {
	for _, name := range []string{"novel-standard"} {
		content := readBuiltinWritingPreset(t, name)
		for _, required := range []string{
			"overall",
			"per-chapter plan",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s missing multi-chapter planning instruction %q", name, required)
			}
		}
	}
}

func TestBuiltinWritingPresetInstructionsCoverRequiredTools(t *testing.T) {
	for _, name := range []string{"novel-lite", "novel-standard"} {
		content := readBuiltinWritingPreset(t, name)
		for _, required := range []string{
			"`read`",
			"`write`",
			"`edit`",
			"[tool error]",
			"do not claim completion",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s missing required tool instruction %q", name, required)
			}
		}
		for _, obsolete := range []string{"read_file", "write_file", "edit_file"} {
			if strings.Contains(content, obsolete) {
				t.Fatalf("%s contains obsolete tool name %q", name, obsolete)
			}
		}
	}
}

func TestBuiltinWritingPresetInstructionsCoverTaskDelegation(t *testing.T) {
	for _, name := range []string{"novel-standard"} {
		content := readBuiltinWritingPreset(t, name)
		for _, required := range []string{
			"task",
			"description",
			"general-purpose",
			"unavailable",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s missing task delegation instruction %q", name, required)
			}
		}
		if strings.Contains(content, "`reviewer`") {
			t.Fatalf("%s still requires the removed reviewer SubAgent", name)
		}
	}
}

func TestBuiltinSkillCatalogContainsOnlyDurableProductCapabilities(t *testing.T) {
	root := filepath.Join("..", "..", "..", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), SkillFileName)); err == nil {
			names = append(names, entry.Name())
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	want := []string{
		"chapter-illustration",
		"config-manager",
		"interactive-image",
		"novel-lite",
		"novel-standard",
		"web-research",
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Fatalf("built-in Skills = %q, want %q", names, want)
	}
}

func TestBuiltinChapterIllustrationSkillIsIDEOnly(t *testing.T) {
	content := readBuiltinWritingPreset(t, "chapter-illustration")
	for _, required := range []string{
		"name: chapter-illustration",
		"agent: ide",
		"generate_image",
		"Do not edit the chapter prose automatically",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("chapter-illustration missing required instruction %q", required)
		}
	}
}

func readBuiltinWritingPreset(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "skills", name, SkillFileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
