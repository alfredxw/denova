package agent

import (
	"errors"
	"fmt"
	"strings"

	"nova/internal/book"
	"nova/internal/prompts"
)

const maxStyleRuleContextChars = 32000

// appendWritingSkillContext 将本轮生效的写作 Skill 注入用户消息，作为 Agent 的工作流约束。
func appendWritingSkillContext(message string, ctx WritingSkillContext, logs ...*contextBuildLog) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\n# 本轮选中的 Writing Skill\n\n")
	sb.WriteString("以下 SKILL.md 是本轮 IDE 创作 Agent 必须遵循的有效写作 Skill。它定义写作流程、角色协作和输出要求；用户本轮自然语言指令仍然决定具体写作范围，不存在单独的 writing_scope 字段。\n\n")
	sb.WriteString("- name: ")
	sb.WriteString(ctx.Name)
	sb.WriteString("\n- source: ")
	sb.WriteString(ctx.Source)
	sb.WriteString("\n- scope: ")
	sb.WriteString(ctx.Scope)
	if ctx.Path != "" {
		sb.WriteString("\n- path: ")
		sb.WriteString(ctx.Path)
	}
	sb.WriteString(fmt.Sprintf("\n- size_limit_chars: %d", ctx.MaxChars))
	if ctx.Truncated {
		sb.WriteString("\n- truncated: true")
	}
	sb.WriteString("\n\n```markdown\n")
	sb.WriteString(ctx.Content)
	sb.WriteString("\n```\n")

	note := fmt.Sprintf("source=%s scope=%s path=%s max_chars=%d", ctx.Source, ctx.Scope, ctx.Path, ctx.MaxChars)
	addContextLog(logs, "写作 Skill", ctx.Name, sb.String()[len(message):], note)
	return sb.String()
}

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

// styleRulesSystemInstruction 把工作区配置的「场景 → 风格内容」规则集作为 system prompt 片段。
func styleRulesSystemInstruction(rules []StyleRule) string {
	return prompts.StyleRulesInstruction(boundedStyleRules(rules, maxStyleRuleContextChars))
}

func boundedStyleRules(rules []StyleRule, maxChars int) []StyleRule {
	if maxChars <= 0 {
		return nil
	}
	result := make([]StyleRule, 0, len(rules))
	used := 0
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" || len(rule.StyleContents) == 0 {
			continue
		}
		contents := make([]string, 0, len(rule.StyleContents))
		for _, content := range rule.StyleContents {
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			remain := maxChars - used
			if remain <= 0 {
				break
			}
			runes := []rune(content)
			if len(runes) > remain {
				content = string(runes[:remain]) + "\n\n[风格内容已截断]"
				used = maxChars
			} else {
				used += len(runes)
			}
			contents = append(contents, content)
		}
		if len(contents) > 0 {
			result = append(result, StyleRule{Scene: scene, StyleContents: contents})
		}
		if used >= maxChars {
			break
		}
	}
	return result
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
