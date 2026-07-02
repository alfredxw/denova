package messages

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

const (
	maxChangelogMessages  = 12
	maxMessageBodyRunes   = 20000
	maxMessageSummaryRune = 160
)

var changelogVersionHeadingRe = regexp.MustCompile(`^\[?([^\]]+)\]?(?:\s*-\s*(\d{4}-\d{2}-\d{2}))?$`)

func parseChangelogMessages(content string) []Message {
	return parseChangelogMessagesForLocale(content, "")
}

func parseChangelogMessagesForLocale(content, locale string) []Message {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	messages := make([]Message, 0, maxChangelogMessages)
	currentHeading := ""
	body := []string{}

	flush := func() {
		if strings.TrimSpace(currentHeading) == "" {
			return
		}
		label, publishedAt := parseChangelogHeading(currentHeading)
		if strings.TrimSpace(label) == "" {
			label = strings.TrimSpace(currentHeading)
		}
		if isUnreleasedChangelogLabel(label) {
			currentHeading = ""
			body = body[:0]
			return
		}
		rawBodyText := trimBlankLines(strings.Join(body, "\n"))
		if strings.TrimSpace(rawBodyText) == "" {
			currentHeading = ""
			body = body[:0]
			return
		}
		bodyText := localizeChangelogBody(rawBodyText, locale)
		if strings.TrimSpace(bodyText) == "" {
			currentHeading = ""
			body = body[:0]
			return
		}
		message := Message{
			ID:          "changelog:" + changelogID(label, rawBodyText),
			Type:        MessageTypeChangelog,
			Title:       label,
			Summary:     changelogSummary(bodyText),
			Body:        truncateRunes(bodyText, maxMessageBodyRunes),
			PublishedAt: publishedAt,
		}
		messages = append(messages, message)
		currentHeading = ""
		body = body[:0]
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			flush()
			currentHeading = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if len(messages) >= maxChangelogMessages {
				break
			}
			continue
		}
		if currentHeading != "" {
			body = append(body, line)
		}
	}
	if len(messages) < maxChangelogMessages {
		flush()
	}
	if len(messages) > maxChangelogMessages {
		return messages[:maxChangelogMessages]
	}
	return messages
}

func localizeChangelogBody(body, locale string) string {
	target := changelogBodyLanguage(locale)
	lines := strings.Split(body, "\n")
	result := []string{}
	heading := ""
	section := []string{}

	flush := func() {
		filtered := filterChangelogLines(section, target)
		if len(filtered) == 0 {
			heading = ""
			section = section[:0]
			return
		}
		if len(result) > 0 {
			result = append(result, "")
		}
		if heading != "" {
			result = append(result, localizeChangelogHeading(heading, target), "")
		}
		result = append(result, filtered...)
		heading = ""
		section = section[:0]
	}

	for _, line := range lines {
		if isChangelogSectionHeading(line) {
			flush()
			heading = line
			continue
		}
		section = append(section, line)
	}
	flush()

	return trimBlankLines(strings.Join(compactBlankLines(result), "\n"))
}

func filterChangelogLines(lines []string, target string) []string {
	out := []string{}
	scopedLanguage := ""
	for i := 0; i < len(lines); {
		line := lines[i]
		if isChangelogSubheading(line) {
			title := strings.TrimSpace(strings.TrimPrefix(line, "#### "))
			if language, ok := explicitChangelogLanguage(title); ok {
				scopedLanguage = language
				i++
				continue
			}
			scopedLanguage = ""
			out = append(out, localizeChangelogHeading(line, target))
			i++
			continue
		}

		if marker, content, ok := splitChangelogListItem(line); ok {
			group := []string{line}
			i++
			for i < len(lines) && !isChangelogSectionHeading(lines[i]) && !isChangelogSubheading(lines[i]) && !isChangelogListItem(lines[i]) {
				group = append(group, lines[i])
				i++
			}
			if scopedLanguage != "" && scopedLanguage != target {
				continue
			}
			language, stripped, explicit := stripChangelogLanguageLabel(content)
			if !explicit {
				language = detectChangelogLanguage(strings.Join(group, "\n"))
			}
			if language != "" && language != target {
				continue
			}
			if explicit {
				stripped = strings.TrimSpace(stripped)
				if stripped == "" {
					continue
				}
				group[0] = marker + stripped
			}
			out = append(out, group...)
			continue
		}

		i++
		if scopedLanguage != "" && scopedLanguage != target {
			continue
		}
		if strings.TrimSpace(line) == "" {
			out = append(out, line)
			continue
		}
		language, stripped, explicit := stripChangelogLanguageLabel(strings.TrimSpace(line))
		if !explicit {
			language = detectChangelogLanguage(line)
		}
		if language != "" && language != target {
			continue
		}
		if explicit {
			line = leadingWhitespace(line) + strings.TrimSpace(stripped)
		}
		out = append(out, line)
	}
	return compactBlankLines(out)
}

func changelogBodyLanguage(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		return "en"
	}
	return "zh"
}

func isChangelogSectionHeading(line string) bool {
	return strings.HasPrefix(line, "### ")
}

func isChangelogSubheading(line string) bool {
	return strings.HasPrefix(line, "#### ")
}

func localizeChangelogHeading(line, target string) string {
	marker, title, ok := splitMarkdownHeading(line)
	if !ok {
		return line
	}
	if localized, ok := localizedChangelogHeadingTitle(title, target); ok {
		return marker + " " + localized
	}
	return line
}

func splitMarkdownHeading(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "### ") {
		return "###", strings.TrimSpace(strings.TrimPrefix(trimmed, "### ")), true
	}
	if strings.HasPrefix(trimmed, "#### ") {
		return "####", strings.TrimSpace(strings.TrimPrefix(trimmed, "#### ")), true
	}
	return "", "", false
}

func localizedChangelogHeadingTitle(title, target string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(title))
	if strings.Contains(normalized, "brief") && strings.Contains(title, "简要说明") {
		if target == "en" {
			return "Brief", true
		}
		return "简要说明", true
	}
	headings := map[string]struct {
		zh string
		en string
	}{
		"added":      {zh: "新增", en: "Added"},
		"新增":         {zh: "新增", en: "Added"},
		"changed":    {zh: "变更", en: "Changed"},
		"变更":         {zh: "变更", en: "Changed"},
		"fixed":      {zh: "修复", en: "Fixed"},
		"修复":         {zh: "修复", en: "Fixed"},
		"removed":    {zh: "移除", en: "Removed"},
		"移除":         {zh: "移除", en: "Removed"},
		"deprecated": {zh: "弃用", en: "Deprecated"},
		"弃用":         {zh: "弃用", en: "Deprecated"},
		"security":   {zh: "安全", en: "Security"},
		"安全":         {zh: "安全", en: "Security"},
	}
	value, ok := headings[normalized]
	if !ok {
		return "", false
	}
	if target == "en" {
		return value.en, true
	}
	return value.zh, true
}

func explicitChangelogLanguage(value string) (string, bool) {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(value)), "*_`")
	switch normalized {
	case "中文", "简体中文", "chinese", "zh", "zh-cn":
		return "zh", true
	case "english", "英文", "en", "en-us":
		return "en", true
	default:
		return "", false
	}
}

func stripChangelogLanguageLabel(text string) (string, string, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	labels := []struct {
		prefix   string
		language string
	}{
		{prefix: "zh-cn:", language: "zh"},
		{prefix: "zh-cn：", language: "zh"},
		{prefix: "chinese:", language: "zh"},
		{prefix: "chinese：", language: "zh"},
		{prefix: "简体中文:", language: "zh"},
		{prefix: "简体中文：", language: "zh"},
		{prefix: "中文:", language: "zh"},
		{prefix: "中文：", language: "zh"},
		{prefix: "zh:", language: "zh"},
		{prefix: "zh：", language: "zh"},
		{prefix: "english:", language: "en"},
		{prefix: "english：", language: "en"},
		{prefix: "en-us:", language: "en"},
		{prefix: "en-us：", language: "en"},
		{prefix: "英文:", language: "en"},
		{prefix: "英文：", language: "en"},
		{prefix: "en:", language: "en"},
		{prefix: "en：", language: "en"},
	}
	for _, label := range labels {
		if strings.HasPrefix(lower, label.prefix) {
			return label.language, strings.TrimSpace(trimmed[len(label.prefix):]), true
		}
	}
	return "", text, false
}

func splitChangelogListItem(line string) (string, string, bool) {
	trimmedLeft := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmedLeft)]
	if len(trimmedLeft) < 2 {
		return "", "", false
	}
	marker := trimmedLeft[:2]
	if marker != "- " && marker != "* " && marker != "+ " {
		return "", "", false
	}
	return indent + marker, trimmedLeft[2:], true
}

func isChangelogListItem(line string) bool {
	_, _, ok := splitChangelogListItem(line)
	return ok
}

func detectChangelogLanguage(text string) string {
	hasCJK := false
	hasLatin := false
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			hasCJK = true
			break
		}
		if unicode.Is(unicode.Latin, r) {
			hasLatin = true
		}
	}
	if hasCJK {
		return "zh"
	}
	if hasLatin {
		return "en"
	}
	return ""
}

func leadingWhitespace(line string) string {
	trimmedLeft := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmedLeft)]
}

func compactBlankLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	out := make([]string, 0, end-start)
	lastBlank := false
	for _, line := range lines[start:end] {
		blank := strings.TrimSpace(line) == ""
		if blank && lastBlank {
			continue
		}
		out = append(out, line)
		lastBlank = blank
	}
	return out
}

func parseChangelogHeading(heading string) (string, string) {
	heading = strings.TrimSpace(heading)
	matches := changelogVersionHeadingRe.FindStringSubmatch(heading)
	if len(matches) != 3 {
		return heading, ""
	}
	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2])
}

func isUnreleasedChangelogLabel(label string) bool {
	normalized := strings.TrimSpace(strings.ToLower(label))
	return normalized == "unreleased"
}

func changelogID(label, body string) string {
	label = strings.TrimSpace(strings.ToLower(label))
	var b strings.Builder
	lastDash := false
	for _, r := range label {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
			lastDash = r == '-'
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "entry"
	}
	sum := sha256.Sum256([]byte(body))
	return slug + ":" + hex.EncodeToString(sum[:4])
}

func changelogSummary(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "### ") {
			continue
		}
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "* "))
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "> "))
		trimmed = strings.Trim(trimmed, "`")
		if trimmed != "" {
			return truncateRunes(trimmed, maxMessageSummaryRune)
		}
	}
	return ""
}

func trimBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
