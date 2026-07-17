package interactive

import (
	"fmt"
	"log"
	"strings"

	"denova/internal/book"
)

const (
	directorLoreContextFile = "lore-context.md"

	// DirectorLoreActiveContextMaxBytes bounds the complete lore bodies selected
	// for automatic Game Agent loading. Candidates and offstage references are
	// not part of this budget because only the Director sees them.
	DirectorLoreActiveContextMaxBytes = DirectorContextMaxBytes
)

var requiredDirectorLoreContextHeadings = []string{
	"当前",
	"候场",
	"暂离场",
}

var activeDirectorLoreContextSections = map[string]bool{
	"当前": true,
}

// DirectorLoreContextReferences is the parsed, name-based working set from
// lore-context.md. Names are canonical user-facing lore keys; the lore store
// guarantees that enabled entries cannot share a name.
type DirectorLoreContextReferences struct {
	Active     []string
	Candidates []string
	Offstage   []string
}

func defaultDirectorLoreContextDocument() string {
	return strings.TrimSpace(`# 分支资料工作集

> 使用 [[资料名称]] 精确引用资料库，不复制正文。二级标题只表示生命周期状态；可按角色、势力、地点、物品等需要自由增加三级标题。常驻资料已由系统完整加载，不重复写入本文件。

## 当前

### 角色

记录当前场景或最近分支持续参与的角色，并用一句话说明本阶段功能。

### 势力

记录正在施加资源、制度、舆论、追捕或关系压力的势力。

### 地点、物品与其他设定

记录当前阶段必须完整加载的背景、地点、物品或其他稳定设定。

## 候场

### 角色与势力

记录可能在明确触发条件下入场的角色或势力。候场资料只供 Director 规划，不自动注入正文 Agent。

## 暂离场

### 角色与势力

记录阶段作用已完成、暂不自动召回，但可能被玩家主动寻找或在后续阶段回归的角色或势力。`)
}

// ParseDirectorLoreContextReferences extracts exact [[name]] references and
// classifies them by their validated Markdown lifecycle section.
func ParseDirectorLoreContextReferences(content string) DirectorLoreContextReferences {
	refs := DirectorLoreContextReferences{}
	seenActive := map[string]bool{}
	seenCandidates := map[string]bool{}
	seenOffstage := map[string]bool{}
	section := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			continue
		}
		for _, name := range loreReferenceNamesInText(line) {
			switch {
			case activeDirectorLoreContextSections[section]:
				appendUniqueLoreName(&refs.Active, seenActive, name)
			case section == "候场":
				appendUniqueLoreName(&refs.Candidates, seenCandidates, name)
			case section == "暂离场":
				appendUniqueLoreName(&refs.Offstage, seenOffstage, name)
			}
		}
	}
	return refs
}

func (r DirectorLoreContextReferences) All() []string {
	result := make([]string, 0, len(r.Active)+len(r.Candidates)+len(r.Offstage))
	seen := map[string]bool{}
	for _, names := range [][]string{r.Active, r.Candidates, r.Offstage} {
		for _, name := range names {
			appendUniqueLoreName(&result, seen, name)
		}
	}
	return result
}

func loreReferenceNamesInText(text string) []string {
	result := []string{}
	for {
		start := strings.Index(text, "[[")
		if start < 0 {
			return result
		}
		text = text[start+2:]
		end := strings.Index(text, "]]")
		if end < 0 {
			return result
		}
		name := strings.TrimSpace(text[:end])
		if name != "" {
			result = append(result, name)
		}
		text = text[end+2:]
	}
}

func appendUniqueLoreName(target *[]string, seen map[string]bool, name string) {
	name = strings.TrimSpace(name)
	key := strings.ToLower(name)
	if name == "" || seen[key] {
		return
	}
	seen[key] = true
	*target = append(*target, name)
}

func (s *Store) validateDirectorLoreContext(content string) error {
	if err := validateDirectorLoreContextDoc(content); err != nil {
		return err
	}
	refs := ParseDirectorLoreContextReferences(content)
	if len(refs.All()) == 0 {
		return nil
	}
	items, err := book.NewLoreStore(s.root).List()
	if err != nil {
		return fmt.Errorf("读取资料库以校验导演资料工作集失败: %w", err)
	}
	byName := make(map[string]book.LoreItem, len(items))
	for _, item := range items {
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}
	for _, name := range refs.All() {
		item, ok := byName[strings.ToLower(name)]
		if !ok {
			log.Printf("[director-lore-context] ignoring unavailable lore reference name=%q source=lore-context.md location=internal/interactive/director_lore_context.go", name)
			continue
		}
		if item.LoadMode == book.LoreLoadModeResident {
			return fmt.Errorf("常驻资料已由系统完整加载，不应重复写入 lore-context.md: %s", name)
		}
	}
	activeNames := make(map[string]bool, len(refs.Active))
	for _, name := range refs.Active {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, ok := byName[key]; ok {
			activeNames[key] = true
		}
	}
	for _, name := range append(append([]string{}, refs.Candidates...), refs.Offstage...) {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, ok := byName[key]; ok && activeNames[key] {
			return fmt.Errorf("同一资料不能同时处于当前和候场/暂离场区段: %s", name)
		}
	}
	candidateNames := make(map[string]bool, len(refs.Candidates))
	for _, name := range refs.Candidates {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, ok := byName[key]; ok {
			candidateNames[key] = true
		}
	}
	for _, name := range refs.Offstage {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, ok := byName[key]; ok && candidateNames[key] {
			return fmt.Errorf("同一角色不能同时处于候场和暂离场区段: %s", name)
		}
	}
	activeBytes := 0
	for _, name := range refs.Active {
		if item, ok := byName[strings.ToLower(strings.TrimSpace(name))]; ok {
			activeBytes += len([]byte(item.Content))
		}
	}
	if activeBytes > DirectorLoreActiveContextMaxBytes {
		return fmt.Errorf("当前资料正文合计 %d bytes，超过自动加载上限 %d bytes；请减少当前引用并把未登场角色移到候场或暂离场", activeBytes, DirectorLoreActiveContextMaxBytes)
	}
	return nil
}

func validateDirectorLoreContextDoc(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("导演资料工作集不能为空")
	}
	if len([]byte(content)) > maxDirectorPlanDocBytes {
		return fmt.Errorf("导演资料工作集超过大小上限 %d bytes", maxDirectorPlanDocBytes)
	}
	allowed := map[string]bool{}
	seen := map[string]bool{}
	for _, heading := range requiredDirectorLoreContextHeadings {
		allowed[heading] = true
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		if !allowed[heading] {
			return fmt.Errorf("导演资料工作集包含未知二级标题 %q；请改为 当前、候场或暂离场，内容分类请使用三级标题", heading)
		}
		if seen[heading] {
			return fmt.Errorf("导演资料工作集二级标题重复: %s", heading)
		}
		seen[heading] = true
	}
	for _, heading := range requiredDirectorLoreContextHeadings {
		if !seen[heading] {
			return fmt.Errorf("导演资料工作集缺少必填标题: %s", heading)
		}
	}
	return nil
}
