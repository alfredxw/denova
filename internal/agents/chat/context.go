package chat

import (
	"strings"

	"denova/internal/agents/prompts"
	"denova/internal/book"
)

// styleRulesSystemInstruction 把工作区配置的「场景 → 文风参考」规则集作为 system prompt 片段。
func styleRulesSystemInstruction(rules []prompts.StyleRule) string {
	return prompts.StyleRulesInstruction(rules)
}

func boundedStyleRules(rules []prompts.StyleRule, maxChars int) []prompts.StyleRule {
	if maxChars <= 0 {
		return nil
	}
	result := make([]prompts.StyleRule, 0, len(rules))
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
				content = string(runes[:remain]) + "\n\n[Style content truncated]"
				used = maxChars
			} else {
				used += len(runes)
			}
			contents = append(contents, content)
		}
		if len(contents) > 0 || len(refs) > 0 {
			used += len([]rune(scene)) + 16
			result = append(result, prompts.StyleRule{Global: rule.Global, Scene: scene, StyleReferences: refs, StyleContents: contents})
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

// readReferencedFile reads one explicit workspace reference without applying a
// second, caller-local budget. The shared context assembler is the sole owner
// of UTF-8-safe truncation and model-visible truncation notices.
func readReferencedFile(bookService *book.Service, relPath string) (string, error) {
	content, err := bookService.ReadFile(relPath)
	if err != nil {
		return "", err
	}
	return content, nil
}
