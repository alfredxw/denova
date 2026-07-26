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
			"不要假设任务一定是下一章",
			"没有 `writing_scope` 字段",
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
			"整体计划",
			"分章计划",
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
			"不得宣称已完成",
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
			"不可用",
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
		"不要自动编辑章节正文",
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
