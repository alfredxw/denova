package style

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

func fileRevision(info os.FileInfo) string {
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}

func NormalizeStoragePath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if !isStyleFile(base) {
		base += ".md"
	}
	base = sanitizeFilename(base)
	if base == "" {
		return ""
	}
	return StoragePath(base)
}

func StoragePath(filename string) string {
	filename = sanitizeFilename(filepath.Base(strings.TrimSpace(filename)))
	if filename == "" {
		return ""
	}
	if !isStyleFile(filename) {
		filename += ".md"
	}
	return filepath.ToSlash(filepath.Join(DisplayDir, filename))
}

func isStyleFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	return ext == ".md" || ext == ".markdown" || ext == ".txt"
}

func filenameForWrite(filename, name string) string {
	filename = sanitizeFilename(filepath.Base(strings.TrimSpace(filename)))
	if filename == "" {
		filename = slugFilename(name)
	}
	if filename == "" {
		filename = fmt.Sprintf("style-%d.md", time.Now().UnixNano())
	}
	if !isStyleFile(filename) {
		filename += ".md"
	}
	if strings.ToLower(filepath.Ext(filename)) != ".md" {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".md"
	}
	return filename
}

func sanitizeFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = filepath.Base(filename)
	filename = strings.Trim(filename, ". ")
	if filename == "" || filename == "." || filename == ".." {
		return ""
	}
	var out strings.Builder
	for _, r := range filename {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			out.WriteRune(r)
		case r == '.', r == '-', r == '_':
			out.WriteRune(r)
		case unicode.IsSpace(r):
			out.WriteByte('-')
		}
	}
	cleaned := strings.Trim(out.String(), ".- _")
	if cleaned == "" {
		return ""
	}
	return cleaned
}

func slugFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var out strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			out.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r), r == '-', r == '_':
			if !lastDash && out.Len() > 0 {
				out.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(out.String(), "-") + ".md"
}

func summarizeMarkdown(filename, content string) (string, string) {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	desc := ""
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" || trimmed == "---" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") && name == strings.TrimSuffix(filename, filepath.Ext(filename)) {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
		if trimmed != "" {
			desc = truncateRunes(trimmed, MaxDescriptionSize)
			break
		}
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	return strings.TrimSpace(name), desc
}

func ensureTrailingNewline(value string) string {
	if strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}

func ensureReferenceHeader(content, name, description string) string {
	content = strings.TrimSpace(content)
	if hasMarkdownH1(content) {
		return content
	}
	name = oneLine(name)
	description = truncateRunes(oneLine(description), MaxDescriptionSize)
	if name == "" && description == "" {
		return content
	}
	var sb strings.Builder
	if name != "" {
		fmt.Fprintf(&sb, "# %s\n\n", name)
	}
	if description != "" {
		fmt.Fprintf(&sb, "> %s\n\n", description)
	}
	sb.WriteString(content)
	return sb.String()
}

func hasMarkdownH1(content string) bool {
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			return true
		}
	}
	return false
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
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
