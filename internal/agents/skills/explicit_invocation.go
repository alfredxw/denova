package skills

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ResolveExplicitInvocations returns active Skills explicitly referenced as
// /<skill-name> anywhere in the message. Results preserve first-occurrence
// order and contain each Skill at most once.
func (b *Backend) ResolveExplicitInvocations(ctx context.Context, message string) []Skill {
	if b == nil || strings.TrimSpace(message) == "" {
		return nil
	}
	available := make(map[string]Skill)
	for _, rec := range b.activeRecords(ctx) {
		available[rec.skill.Name] = rec.skill
	}
	if len(available) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	resolved := make([]Skill, 0)
	for index := 0; index < len(message); index++ {
		if message[index] != '/' || !explicitInvocationStartBoundary(message, index) {
			continue
		}
		nameStart := index + 1
		if nameStart >= len(message) || !isSkillNameByte(message[nameStart]) {
			continue
		}
		nameEnd := nameStart
		for nameEnd < len(message) && isSkillNameByte(message[nameEnd]) {
			nameEnd++
		}
		if !explicitInvocationEndBoundary(message, nameEnd) {
			continue
		}
		name := message[nameStart:nameEnd]
		skill, ok := available[name]
		if !ok || seen[name] {
			index = nameEnd - 1
			continue
		}
		seen[name] = true
		resolved = append(resolved, skill)
		index = nameEnd - 1
	}
	return resolved
}

// FormatForModel is the canonical model-visible representation shared by the
// model-callable skill tool and deterministic explicit Skill preloading. The
// limit covers the complete formatted result, not only the SKILL.md body.
func FormatForModel(skill Skill, maxBytes int) string {
	content := strings.TrimSpace(skill.Content)
	prefix := fmt.Sprintf("# Skill: %s\n\nDescription: %s\nContext mode: %s\n\n", skill.Name, skill.Description, skill.Context)
	formatted := prefix + content
	if maxBytes <= 0 || len(formatted) <= maxBytes {
		return formatted
	}
	const marker = "\n\n[Skill instructions truncated at configured context fragment limit]"
	bodyLimit := maxBytes - len(prefix) - len(marker)
	if bodyLimit <= 0 {
		return truncateUTF8Bytes(formatted, maxBytes)
	}
	return prefix + strings.TrimSpace(truncateUTF8Bytes(content, bodyLimit)) + marker
}

func explicitInvocationStartBoundary(message string, slash int) bool {
	if slash == 0 {
		return true
	}
	previous := message[slash-1]
	return !isSkillNameByte(previous) && previous != '/' && previous != '\\'
}

func explicitInvocationEndBoundary(message string, end int) bool {
	if end >= len(message) {
		return true
	}
	next := message[end]
	return !isSkillNameByte(next) && next != '/' && next != '\\'
}

func isSkillNameByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '_' || value == '-'
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
