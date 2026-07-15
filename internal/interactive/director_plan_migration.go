package interactive

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const directorDocumentsMigrationVersion = "director-doc-v2"

func (s *Store) ensureDirectorDocumentsV2Locked(storyID, branchID string) error {
	dir := s.directorPlanBranchDir(storyID, branchID)
	planPath := filepath.Join(dir, directorPlanFile)
	agentBriefPath := filepath.Join(dir, directorAgentBriefFile)
	lorePath := filepath.Join(dir, directorLoreContextFile)

	plan, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	agentBrief, briefErr := os.ReadFile(agentBriefPath)
	loreContext, loreErr := os.ReadFile(lorePath)
	if briefErr == nil && loreErr == nil {
		return nil
	}
	if briefErr != nil && !os.IsNotExist(briefErr) {
		return briefErr
	}
	if loreErr != nil && !os.IsNotExist(loreErr) {
		return loreErr
	}
	backupDir, err := s.backupDirectorDocumentsLocked(storyID, branchID)
	if err != nil {
		return fmt.Errorf("创建导演文档迁移备份失败: %w", err)
	}

	// A partially-created v2 directory is repaired without re-splitting its
	// private plan. A missing agent brief indicates the legacy combined format.
	docs := DirectorPlanDocs{Plan: string(plan), AgentBrief: string(agentBrief), LoreContext: string(loreContext)}
	legacy := os.IsNotExist(briefErr)
	if legacy {
		docs.Plan, docs.AgentBrief = migrateLegacyCombinedDirectorPlan(string(plan))
	}
	if os.IsNotExist(loreErr) {
		docs.LoreContext = defaultDirectorLoreContextDocument()
	} else {
		docs.LoreContext, err = migrateLegacyDirectorLoreContext(string(loreContext))
		if err != nil {
			return fmt.Errorf("迁移导演资料工作集失败，旧文件已备份到 %s: %w", filepath.ToSlash(backupDir), err)
		}
	}
	if !legacy && strings.TrimSpace(docs.AgentBrief) == "" {
		docs.AgentBrief = DefaultStoryDirectorPlanningTemplates().AgentBrief
	}
	if legacy {
		if err := validateDirectorPlanDoc(DirectorPlanDocPlan, docs.Plan); err != nil {
			return fmt.Errorf("迁移旧版 director.md 失败（备份：%s）: %w", filepath.ToSlash(backupDir), err)
		}
		if err := validateDirectorPlanDoc(DirectorPlanDocAgentBrief, docs.AgentBrief); err != nil {
			return fmt.Errorf("生成 agent-brief.md 失败（备份：%s）: %w", filepath.ToSlash(backupDir), err)
		}
	}
	if err := validateDirectorLoreContextDoc(docs.LoreContext); err != nil {
		return fmt.Errorf("迁移 lore-context.md 失败（备份：%s）: %w", filepath.ToSlash(backupDir), err)
	}
	if err := writeDirectorDocumentsAtomically(dir, docs); err != nil {
		return fmt.Errorf("写入新版导演文档失败（备份：%s）: %w", filepath.ToSlash(backupDir), err)
	}
	log.Printf("[director-doc-migration] migrated story=%q branch=%q backup=%q files=%q,%q,%q location=internal/interactive/director_plan_migration.go", storyID, branchID, filepath.ToSlash(backupDir), directorPlanFile, directorAgentBriefFile, directorLoreContextFile)
	return nil
}

func (s *Store) backupDirectorDocumentsLocked(storyID, branchID string) (string, error) {
	sourceDir := s.directorPlanBranchDir(storyID, branchID)
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	backupDir := filepath.Join(s.root, "backups", directorDocumentsMigrationVersion, storyID, branchID, stamp)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	copied := 0
	for _, name := range []string{directorPlanFile, directorAgentBriefFile, directorLoreContextFile, directorPlanMetadataFile} {
		data, err := os.ReadFile(filepath.Join(sourceDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(backupDir, name), data, 0o644); err != nil {
			return "", err
		}
		copied++
	}
	if copied == 0 {
		return "", errors.New("没有可备份的导演文档")
	}
	return backupDir, nil
}

func migrateLegacyCombinedDirectorPlan(content string) (string, string) {
	visible := legacyDirectorSection(content, "正文Agent可读", "后台导演私密")
	private := legacyDirectorSection(content, "后台导演私密", "")
	if visible == "" || private == "" {
		defaults := DefaultStoryDirectorPlanningTemplates()
		preserved := strings.TrimSpace(content)
		if preserved != "" {
			defaults.Plan += "\n\n## 旧版规划原文（迁移备查）\n\n" + preserved
		}
		return defaults.Plan, defaults.AgentBrief
	}

	visibleSections := markdownSectionBodies(visible, 3)
	privateSections := markdownSectionBodies(private, 3)
	privatePlan := renderMigratedDirectorDocument("# 导演私密规划", []migratedDirectorSection{
		{"阶段目标与隐藏钩子", sectionOrDefault(privateSections, "阶段钩子与阅读欲望", DirectorPlanDocPlan)},
		{"资料库锚点", sectionOrDefault(privateSections, "资料库锚点", DirectorPlanDocPlan)},
		{"选角覆盖", "场景规模：标准。请基于本轮实际阅读的资料正文补充当前与候场角色，并说明关系、信息、冲突等功能覆盖。"},
		{"核心角色与关系张力", sectionOrDefault(privateSections, "核心角色与关系张力", DirectorPlanDocPlan)},
		{"重要势力与阶段阻力", sectionOrDefault(privateSections, "重要势力与阶段阻力", DirectorPlanDocPlan)},
		{"当前场景幕后信息", sectionOrDefault(privateSections, "当前场景与行动空间", DirectorPlanDocPlan)},
		{"信息揭示与线索密度", sectionOrDefault(privateSections, "信息揭示与线索密度", DirectorPlanDocPlan)},
		{"遭遇、检定与代价", sectionOrDefault(privateSections, "遭遇、检定与代价", DirectorPlanDocPlan)},
		{"爽点、危机与反转", sectionOrDefault(privateSections, "爽点、危机与反转", DirectorPlanDocPlan)},
		{"状态连续性", sectionOrDefault(privateSections, "状态连续性", DirectorPlanDocPlan)},
		{"最近分支安排", sectionOrDefault(privateSections, "最近分支安排", DirectorPlanDocPlan)},
		{"伏笔与回收", sectionOrDefault(privateSections, "伏笔与回收", DirectorPlanDocPlan)},
	})
	agentBrief := renderMigratedDirectorDocument("# 正文 Agent 简报", []migratedDirectorSection{
		{"当前目标与可见钩子", sectionOrDefault(visibleSections, "阶段钩子与阅读欲望", DirectorPlanDocAgentBrief)},
		{"当前场景与行动空间", sectionOrDefault(visibleSections, "当前场景与行动空间", DirectorPlanDocAgentBrief)},
		{"当前角色与可见关系", joinLegacySections(visibleSections, "资料库锚点", "核心角色与关系张力", "重要势力与阶段阻力")},
		{"已公开信息与可发现线索", joinLegacySections(visibleSections, "信息揭示与线索密度", "伏笔与回收")},
		{"遭遇、检定与可见代价", joinLegacySections(visibleSections, "遭遇、检定与代价", "爽点、危机与反转")},
		{"状态连续性", sectionOrDefault(visibleSections, "状态连续性", DirectorPlanDocAgentBrief)},
		{"最近分支承接", sectionOrDefault(visibleSections, "最近分支安排", DirectorPlanDocAgentBrief)},
	})
	return privatePlan, agentBrief
}

func migrateLegacyDirectorLoreContext(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return defaultDirectorLoreContextDocument(), nil
	}
	headings := markdownH2Headings(content)
	if len(headings) == 0 {
		return "", fmt.Errorf("无法安全迁移未包含二级标题的旧资料工作集；请手动整理为 当前/候场/暂离场")
	}
	newHeadings := map[string]bool{"当前": true, "候场": true, "暂离场": true}
	allNew := len(headings) > 0
	for _, heading := range headings {
		if !newHeadings[heading] {
			allNew = false
			break
		}
	}
	if allNew {
		return content, nil
	}

	legacyHeadings := map[string]bool{
		"当前背景与地点":   true,
		"当前势力":      true,
		"当前角色":      true,
		"候场角色":      true,
		"候场势力":      true,
		"暂离场角色":     true,
		"当前物品与其他设定": true,
	}
	for _, heading := range headings {
		if !legacyHeadings[heading] {
			return "", fmt.Errorf("无法安全迁移未知二级标题 %q；请先将其改为旧版已知分组，或手动整理为 当前/候场/暂离场", heading)
		}
	}
	sections := markdownSectionBodies(content, 2)
	var sb strings.Builder
	sb.WriteString("# 分支资料工作集\n\n")
	sb.WriteString("> 已从旧版固定分类迁移。二级标题表示生命周期状态；内容类别使用三级标题。\n\n")
	sb.WriteString("## 当前\n\n")
	appendMigratedLoreGroup(&sb, "背景与地点", sections["当前背景与地点"])
	appendMigratedLoreGroup(&sb, "势力", sections["当前势力"])
	appendMigratedLoreGroup(&sb, "角色", sections["当前角色"])
	appendMigratedLoreGroup(&sb, "物品与其他设定", sections["当前物品与其他设定"])
	sb.WriteString("## 候场\n\n")
	appendMigratedLoreGroup(&sb, "角色", sections["候场角色"])
	appendMigratedLoreGroup(&sb, "势力", sections["候场势力"])
	sb.WriteString("## 暂离场\n\n")
	appendMigratedLoreGroup(&sb, "角色与势力", sections["暂离场角色"])
	return strings.TrimSpace(sb.String()), nil
}

func markdownH2Headings(content string) []string {
	result := []string{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			result = append(result, strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
		}
	}
	return result
}

func appendMigratedLoreGroup(sb *strings.Builder, heading, body string) {
	sb.WriteString("### ")
	sb.WriteString(heading)
	sb.WriteString("\n\n")
	if body = strings.TrimSpace(body); body != "" {
		sb.WriteString(body)
		sb.WriteString("\n\n")
		return
	}
	sb.WriteString("（暂无）\n\n")
}

type migratedDirectorSection struct {
	Heading string
	Body    string
}

func renderMigratedDirectorDocument(title string, sections []migratedDirectorSection) string {
	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n")
	for _, section := range sections {
		sb.WriteString("\n## ")
		sb.WriteString(section.Heading)
		sb.WriteString("\n\n")
		sb.WriteString(strings.TrimSpace(section.Body))
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func legacyDirectorSection(content, heading, endHeading string) string {
	marker := "## " + heading
	start := strings.Index(content, marker)
	if start < 0 {
		return ""
	}
	result := content[start+len(marker):]
	if endHeading != "" {
		if end := strings.Index(result, "## "+endHeading); end >= 0 {
			result = result[:end]
		}
	}
	return strings.TrimSpace(result)
}

func markdownSectionBodies(content string, level int) map[string]string {
	prefix := strings.Repeat("#", level) + " "
	sections := map[string]string{}
	current := ""
	var body strings.Builder
	flush := func() {
		if current != "" {
			sections[current] = strings.TrimSpace(body.String())
		}
		body.Reset()
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && level > 2 {
			flush()
			current = ""
			continue
		}
		if current != "" {
			body.WriteString(line)
			body.WriteString("\n")
		}
	}
	flush()
	return sections
}

func sectionOrDefault(sections map[string]string, heading, kind string) string {
	if value := strings.TrimSpace(sections[heading]); value != "" {
		return value
	}
	defaults := DefaultStoryDirectorPlanningTemplates()
	content := defaults.Plan
	if kind == DirectorPlanDocAgentBrief {
		content = defaults.AgentBrief
	}
	return strings.TrimSpace(markdownSectionBodies(content, 2)[heading])
}

func joinLegacySections(sections map[string]string, headings ...string) string {
	parts := make([]string, 0, len(headings))
	for _, heading := range headings {
		if value := strings.TrimSpace(sections[heading]); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return "请根据当前资料工作集补充可见信息。"
	}
	return strings.Join(parts, "\n\n")
}

func writeDirectorDocumentsAtomically(dir string, docs DirectorPlanDocs) error {
	contents := map[string]string{
		directorPlanFile:        docs.Plan,
		directorAgentBriefFile:  docs.AgentBrief,
		directorLoreContextFile: docs.LoreContext,
	}
	return writeDirectorDocumentContentsAtomically(dir, contents, []string{directorPlanFile, directorAgentBriefFile, directorLoreContextFile})
}

func writeDirectorDocumentChangesAtomically(dir string, before, after DirectorPlanDocs) error {
	contents := map[string]string{}
	order := make([]string, 0, 3)
	for _, document := range []struct {
		name   string
		before string
		after  string
	}{
		{name: directorPlanFile, before: before.Plan, after: after.Plan},
		{name: directorAgentBriefFile, before: before.AgentBrief, after: after.AgentBrief},
		{name: directorLoreContextFile, before: before.LoreContext, after: after.LoreContext},
	} {
		if strings.TrimSpace(document.before) == strings.TrimSpace(document.after) {
			continue
		}
		contents[document.name] = document.after
		order = append(order, document.name)
	}
	if len(order) == 0 {
		return nil
	}
	return writeDirectorDocumentContentsAtomically(dir, contents, order)
}

func writeDirectorDocumentContentsAtomically(dir string, contents map[string]string, order []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temps := map[string]string{}
	for _, name := range order {
		temp, err := os.CreateTemp(dir, "."+name+"-*")
		if err != nil {
			removeDirectorTempFiles(temps)
			return err
		}
		path := temp.Name()
		text := strings.TrimSpace(contents[name]) + "\n"
		if _, err = temp.WriteString(text); err == nil {
			err = temp.Sync()
		}
		closeErr := temp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Chmod(path, 0o644)
		}
		if err != nil {
			_ = os.Remove(path)
			removeDirectorTempFiles(temps)
			return err
		}
		temps[name] = path
	}

	previous := map[string][]byte{}
	existed := map[string]bool{}
	for name := range contents {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			previous[name] = data
			existed[name] = true
		} else if !os.IsNotExist(err) {
			removeDirectorTempFiles(temps)
			return err
		}
	}
	for _, name := range order {
		if err := os.Rename(temps[name], filepath.Join(dir, name)); err != nil {
			for restoreName := range contents {
				target := filepath.Join(dir, restoreName)
				if existed[restoreName] {
					_ = os.WriteFile(target, previous[restoreName], 0o644)
				} else {
					_ = os.Remove(target)
				}
			}
			removeDirectorTempFiles(temps)
			return err
		}
		delete(temps, name)
	}
	return nil
}

func removeDirectorTempFiles(paths map[string]string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
