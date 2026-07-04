package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinWritingPresetInstructionsCoverScopeInference(t *testing.T) {
	for _, name := range []string{"novel-lite", "novel-standard", "novel-heavy"} {
		content := readBuiltinWritingPreset(t, name)
		for _, required := range []string{
			"agent: ide",
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
	for _, name := range []string{"novel-standard", "novel-heavy"} {
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
	for _, name := range []string{"novel-lite", "novel-standard", "novel-heavy"} {
		content := readBuiltinWritingPreset(t, name)
		for _, required := range []string{
			"read_file",
			"write_file",
			"edit_file",
			"[tool error]",
			"不得宣称已完成",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s missing required tool instruction %q", name, required)
			}
		}
	}
}

func TestBuiltinWritingPresetInstructionsCoverTaskDelegation(t *testing.T) {
	for _, name := range []string{"novel-standard", "novel-heavy"} {
		content := readBuiltinWritingPreset(t, name)
		for _, required := range []string{
			"task",
			"description",
			"reviewer",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s missing task delegation instruction %q", name, required)
			}
		}
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

func TestBuiltinLoreSkillCoversToolUsage(t *testing.T) {
	content := readBuiltinWritingPreset(t, "lore")
	for _, required := range []string{
		"name: lore",
		"agent: ide,config_manager,interactive_story",
		"list_lore_items",
		"read_lore_items",
		"write_lore_items",
		"`list_lore_items` 全量索引",
		"`read_lore_items` 批量读取正文",
		"`write_lore_items` 创建或更新条目",
		`"delete_ids": []`,
		`"items":[],"delete_ids":["old-hero-draft"]`,
		"`delete_ids` 必须是数组",
		"不要传字符串 `\"[]\"`",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("lore skill missing required instruction %q", required)
		}
	}
}

func readBuiltinWritingPreset(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", name, SkillFileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
