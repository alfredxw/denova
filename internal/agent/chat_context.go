package agent

import (
	"errors"
	"fmt"
	"strings"

	"nova/internal/book"
	"nova/internal/prompts"
)

// appendReferenceContext 将用户引用的文件内容追加到本次 Agent 输入。
func appendReferenceContext(bookService *book.Service, message string, references []string, logs ...*contextBuildLog) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString(prompts.ReferenceHeader)

	total := 0
	seen := make(map[string]bool)
	for _, ref := range references {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true

		sb.WriteString("\n## @")
		sb.WriteString(ref)
		sb.WriteString("\n")

		if total >= maxReferenceTotalBytes {
			sb.WriteString(prompts.ReferenceOverflowHint)
			addContextLog(logs, "文件引用", "@"+ref, prompts.ReferenceOverflowHint, "未读取：引用内容总量已超过限制")
			continue
		}

		content, n, err := readReferencedFile(bookService, ref, maxReferenceFileBytes, maxReferenceTotalBytes-total)
		total += n
		if err != nil {
			sb.WriteString("读取失败：")
			sb.WriteString(err.Error())
			sb.WriteString("\n")
			addContextLog(logs, "文件引用", "@"+ref, err.Error(), "读取失败")
			continue
		}
		addContextLog(logs, "文件引用", "@"+ref, content, "")

		sb.WriteString("```markdown\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// appendLoreReferenceContext 将用户本轮明确引用的结构化资料条目追加到 Agent 输入。
func appendLoreReferenceContext(bookService *book.Service, message string, references []string, logs ...*contextBuildLog) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\n# 本轮明确引用的资料库条目\n\n以下资料来自结构化资料库，优先级高于泛化摘要；请在本轮创作或判断中优先遵守这些条目的已确认设定。\n")

	if bookService == nil || bookService.Workspace() == "" {
		sb.WriteString("\n资料库读取失败：当前 workspace 不可用。\n")
		addContextLog(logs, "资料库引用", "workspace", "当前 workspace 不可用", "读取失败")
		return sb.String()
	}

	items, err := book.NewLoreStore(bookService.Workspace()).List()
	if err != nil {
		sb.WriteString("\n资料库读取失败：")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
		addContextLog(logs, "资料库引用", ".nova/lore/items.json", err.Error(), "读取失败")
		return sb.String()
	}

	byID := make(map[string]book.LoreItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	seen := make(map[string]bool)
	for _, ref := range references {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		item, ok := byID[ref]
		if !ok {
			sb.WriteString("\n## @资料:")
			sb.WriteString(ref)
			sb.WriteString("\n读取失败：资料条目不存在\n")
			addContextLog(logs, "资料库引用", "@资料:"+ref, "资料条目不存在", "读取失败")
			continue
		}
		content := formatLoreReference(item)
		addContextLog(logs, "资料库引用", "@资料:"+item.Name, content, item.ID)
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	return sb.String()
}

// appendStyleReferenceContext 将本轮指定的风格参考追加到 Agent 输入。
func appendStyleReferenceContext(bookService *book.Service, message string, styleReferences []string, logs ...*contextBuildLog) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString(prompts.StyleReferenceHeader)

	total := 0
	seen := make(map[string]bool)
	for _, ref := range styleReferences {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true

		sb.WriteString("\n## #")
		sb.WriteString(ref)
		sb.WriteString("\n")

		if total >= maxStyleReferenceTotalBytes {
			sb.WriteString(prompts.StyleReferenceOverflowHint)
			addContextLog(logs, "风格参考", "#"+ref, prompts.StyleReferenceOverflowHint, "未读取：风格参考内容总量已超过限制")
			continue
		}

		content, n, err := readStyleReferencedFile(bookService, ref, maxStyleReferenceFileBytes, maxStyleReferenceTotalBytes-total)
		total += n
		if err != nil {
			sb.WriteString("读取失败：")
			sb.WriteString(err.Error())
			sb.WriteString("\n")
			addContextLog(logs, "风格参考", "#"+ref, err.Error(), "读取失败")
			continue
		}
		addContextLog(logs, "风格参考", "#"+ref, content, "")

		sb.WriteString("```markdown\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// appendStyleRulesHint 在用户本轮未通过 # 指定风格时，
// 把工作区配置的「场景 → 风格文件」规则集作为建议附加到上下文。
// 不直接读取文件内容，由 Agent 基于本轮章节内容自行判断。
func appendStyleRulesHint(message string, rules []StyleRule) string {
	return prompts.StyleRulesHint(message, rules)
}

// appendSelectionContext 将用户在编辑器中选中的文本片段追加到消息上下文。
func appendSelectionContext(message string, selections []TextSelectionRef) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString(prompts.SelectionHeader)

	for _, sel := range selections {
		sb.WriteString("\n## 选中内容来自 ")
		sb.WriteString(sel.FileName)
		sb.WriteString(fmt.Sprintf(":L%d-L%d\n", sel.StartLine, sel.EndLine))
		sb.WriteString("```\n")
		sb.WriteString(sel.Content)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// readReferencedFile 安全读取引用文件，并按单文件和总大小限制截断。
func readReferencedFile(bookService *book.Service, relPath string, fileLimit, remainLimit int) (string, int, error) {
	limit := fileLimit
	if remainLimit < limit {
		limit = remainLimit
	}
	if limit <= 0 {
		return "", 0, errors.New("引用内容总量已超过限制")
	}

	content, err := bookService.ReadFile(relPath)
	if err != nil {
		return "", 0, err
	}

	data := []byte(content)
	truncated := false
	if len(data) > limit {
		data = data[:limit]
		truncated = true
	}

	result := string(data)
	if truncated {
		result += "\n\n[内容已截断]"
	}
	return result, len(data), nil
}

func formatLoreReference(item book.LoreItem) string {
	var sb strings.Builder
	sb.WriteString("## ")
	sb.WriteString(item.Name)
	sb.WriteString("（")
	sb.WriteString(item.Type)
	sb.WriteString(" / ")
	sb.WriteString(item.Importance)
	sb.WriteString(" / ")
	sb.WriteString(item.LoadMode)
	sb.WriteString("）\n")
	sb.WriteString("ID：")
	sb.WriteString(item.ID)
	sb.WriteString("\n")
	if len(item.Tags) > 0 {
		sb.WriteString("标签：")
		sb.WriteString(strings.Join(item.Tags, "、"))
		sb.WriteString("\n")
	}
	if item.BriefDescription != "" {
		sb.WriteString("简介：")
		sb.WriteString(item.BriefDescription)
		sb.WriteString("\n")
	}
	content := strings.TrimSpace(item.Content)
	if content != "" {
		sb.WriteString("\n```markdown\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}
	return strings.TrimSpace(sb.String())
}

// readStyleReferencedFile 安全读取风格参考文件，并按单文件和总大小限制截断。
func readStyleReferencedFile(bookService *book.Service, stylePath string, fileLimit, remainLimit int) (string, int, error) {
	limit := fileLimit
	if remainLimit < limit {
		limit = remainLimit
	}
	if limit <= 0 {
		return "", 0, errors.New("风格参考内容总量已超过限制")
	}

	content, err := bookService.ReadStyleFile(stylePath)
	if err != nil {
		return "", 0, err
	}

	data := []byte(content)
	truncated := false
	if len(data) > limit {
		data = data[:limit]
		truncated = true
	}

	result := string(data)
	if truncated {
		result += "\n\n[内容已截断]"
	}
	return result, len(data), nil
}
