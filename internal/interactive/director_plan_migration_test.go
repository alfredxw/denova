package interactive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func legacyDefaultStoryDirectorPlanningTemplates() StoryDirectorPlanningTemplates {
	return StoryDirectorPlanningTemplates{Plan: strings.TrimSpace(`# 导演规划

## 正文Agent可读

### 阶段钩子与阅读欲望
公开的阶段目标与行动钩子。

### 资料库锚点
公开的角色与势力锚点。

### 核心角色与关系张力
当前可见关系。

### 当前场景与行动空间
当前可见场景与行动空间。

### 信息揭示与线索密度
已经公开或可以发现的线索。

### 遭遇、检定与代价
玩家可以感知的检定与代价。

### 状态连续性
当前可见状态。

### 最近分支安排
最近分支的可见承接。

## 后台导演私密

### 阶段钩子与阅读欲望
隐藏真相与后续反转。

### 资料库锚点
后台必须遵守的资料设定。

### 核心角色与关系张力
未公开动机与关系变化。

### 当前场景与行动空间
场景背后的隐藏资源。

### 信息揭示与线索密度
暂缓揭示的信息。

### 遭遇、检定与代价
隐藏代价与失败推进。

### 状态连续性
未公开状态。

### 最近分支安排
多条后台承接策略。`)}
}

func TestDirectorDocumentsMigrateLegacyCombinedPlanWithBackup(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "旧版导演文档"})
	if err != nil {
		t.Fatal(err)
	}
	dir := store.directorPlanBranchDir(story.ID, "main")
	legacyPlan := legacyDefaultStoryDirectorPlanningTemplates().Plan
	legacyLore := `# 分支资料上下文

## 当前背景与地点
- [[雾港]]
## 当前势力
- [[巡夜会]]
## 当前角色
- [[沈凝]]
## 候场角色
- [[罗衡]]
## 暂离场角色
- [[旧友]]
## 当前物品与其他设定
- [[残卷]]`
	if err := os.WriteFile(filepath.Join(dir, directorPlanFile), []byte(legacyPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, directorLoreContextFile), []byte(legacyLore), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, directorAgentBriefFile)); err != nil {
		t.Fatal(err)
	}

	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.Docs.Plan, "正文Agent可读") || !strings.Contains(plan.Docs.Plan, "## 选角覆盖") {
		t.Fatalf("legacy plan was not converted to a private document:\n%s", plan.Docs.Plan)
	}
	if !strings.Contains(plan.Docs.AgentBrief, "## 当前目标与可见钩子") || plan.VisibleDocs.AgentBrief != strings.TrimSpace(plan.Docs.AgentBrief) {
		t.Fatalf("agent brief was not split into its own visible document:\n%s", plan.Docs.AgentBrief)
	}
	if !strings.Contains(plan.Docs.LoreContext, "## 当前") || !strings.Contains(plan.Docs.LoreContext, "### 角色") || !strings.Contains(plan.Docs.LoreContext, "[[沈凝]]") {
		t.Fatalf("legacy lore workset was not migrated:\n%s", plan.Docs.LoreContext)
	}
	backups, err := filepath.Glob(filepath.Join(workspace, "backups", directorDocumentsMigrationVersion, story.ID, "main", "*", directorPlanFile))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one rollback backup, matches=%v err=%v", backups, err)
	}
}

func TestNormalizeStoryDirectorPlanningTemplatesMigratesLegacyCombinedTemplate(t *testing.T) {
	legacy := legacyDefaultStoryDirectorPlanningTemplates()
	normalized := NormalizeStoryDirectorPlanningTemplates(legacy)
	if !strings.Contains(normalized.Plan, "## 选角覆盖") || strings.Contains(normalized.Plan, "## 正文Agent可读") {
		t.Fatalf("legacy private sections should migrate into director.md template:\n%s", normalized.Plan)
	}
	if !strings.Contains(normalized.AgentBrief, "## 当前目标与可见钩子") || !strings.Contains(normalized.AgentBrief, "## 当前角色与可见关系") {
		t.Fatalf("legacy public sections should migrate into agent-brief.md template:\n%s", normalized.AgentBrief)
	}
}

func TestDirectorLoreMigrationStopsOnUnknownH2AndKeepsBackup(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "未知旧分组"})
	if err != nil {
		t.Fatal(err)
	}
	dir := store.directorPlanBranchDir(story.ID, "main")
	if err := os.WriteFile(filepath.Join(dir, directorPlanFile), []byte(legacyDefaultStoryDirectorPlanningTemplates().Plan), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, directorLoreContextFile), []byte("# 旧工作集\n\n## 自定义秘密分组\n\n- [[不可猜测]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, directorAgentBriefFile)); err != nil {
		t.Fatal(err)
	}

	if _, err := store.DirectorPlan(story.ID, "main"); err == nil || !strings.Contains(err.Error(), "无法安全迁移未知二级标题") {
		t.Fatalf("unknown structure should stop migration with a clear error: %v", err)
	}
	backups, globErr := filepath.Glob(filepath.Join(workspace, "backups", directorDocumentsMigrationVersion, story.ID, "main", "*", directorLoreContextFile))
	if globErr != nil || len(backups) != 1 {
		t.Fatalf("failed migration should still preserve a backup, matches=%v err=%v", backups, globErr)
	}
}

func TestDirectorLoreMigrationRejectsUnstructuredContentInsteadOfDroppingIt(t *testing.T) {
	content := "# 自定义工作集\n\n- [[不可丢失的资料]]"
	if _, err := migrateLegacyDirectorLoreContext(content); err == nil || !strings.Contains(err.Error(), "未包含二级标题") {
		t.Fatalf("unstructured user content should require manual migration: %v", err)
	}
}
