package interactive

import "strings"

// ParseLoreReferences extracts unique [[name]] references from free-form
// model-visible content without assigning lifecycle semantics.
func ParseLoreReferences(content string) []string {
	result := []string{}
	seen := map[string]bool{}
	for {
		start := strings.Index(content, "[[")
		if start < 0 {
			return result
		}
		content = content[start+2:]
		end := strings.Index(content, "]]")
		if end < 0 {
			return result
		}
		name := strings.TrimSpace(content[:end])
		key := strings.ToLower(name)
		if name != "" && !seen[key] {
			seen[key] = true
			result = append(result, name)
		}
		content = content[end+2:]
	}
}
