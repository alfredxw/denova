package agents

import (
	"errors"
	"strings"

	"denova/internal/book"
	"denova/internal/prompts"
)

// styleRulesSystemInstruction 把工作区配置的「场景 → 文风参考」规则集作为 system prompt 片段。
func styleRulesSystemInstruction(rules []StyleRule) string {
	return prompts.StyleRulesInstruction(rules)
}

func boundedStyleRules(rules []StyleRule, maxChars int) []StyleRule {
	if maxChars <= 0 {
		return nil
	}
	result := make([]StyleRule, 0, len(rules))
	used := 0
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		refs := make([]prompts.StyleReference, 0, len(rule.StyleReferences))
		for _, ref := range rule.StyleReferences {
			name := strings.TrimSpace(ref.Name)
			path := strings.TrimSpace(ref.Path)
			displayPath := strings.TrimSpace(ref.DisplayPath)
			if name == "" {
				name = displayPath
			}
			if path == "" {
				path = displayPath
			}
			if name == "" || path == "" {
				continue
			}
			desc := truncateRunes(strings.TrimSpace(ref.Description), 240)
			errText := truncateRunes(strings.TrimSpace(ref.Error), 240)
			remain := maxChars - used
			if remain <= 0 {
				break
			}
			cost := styleReferencePromptCost(name, desc, path, displayPath, errText)
			if cost > remain {
				used = maxChars
				break
			}
			used += cost
			refs = append(refs, prompts.StyleReference{
				Name:        name,
				Description: desc,
				Path:        path,
				DisplayPath: displayPath,
				Missing:     ref.Missing,
				Error:       errText,
			})
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
		if len(contents) > 0 || len(refs) > 0 {
			used += len([]rune(scene)) + 16
			result = append(result, StyleRule{Global: rule.Global, Scene: scene, StyleReferences: refs, StyleContents: contents})
		}
		if used >= maxChars {
			break
		}
	}
	return result
}

func styleReferencePromptCost(name, description, path, displayPath, errText string) int {
	return len([]rune(name)) + len([]rune(description)) + len([]rune(path)) + len([]rune(displayPath)) + len([]rune(errText)) + 64
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
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
