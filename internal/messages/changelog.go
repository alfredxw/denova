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
		bodyText := trimBlankLines(strings.Join(body, "\n"))
		if strings.TrimSpace(bodyText) == "" {
			currentHeading = ""
			body = body[:0]
			return
		}
		message := Message{
			ID:          "changelog:" + changelogID(label, bodyText),
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

func parseChangelogHeading(heading string) (string, string) {
	heading = strings.TrimSpace(heading)
	matches := changelogVersionHeadingRe.FindStringSubmatch(heading)
	if len(matches) != 3 {
		return heading, ""
	}
	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2])
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
