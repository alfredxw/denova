package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

// agentFilesystemBackend wraps filesystem.Backend with Nova Agent-specific
// safety and recovery behavior while delegating ordinary filesystem operations.
type agentFilesystemBackend struct {
	filesystem.Backend
}

const agentFileReadDefaultLimitLines = 2000

func newAgentFilesystemBackend(inner filesystem.Backend) filesystem.Backend {
	if inner == nil {
		return nil
	}
	return &agentFilesystemBackend{Backend: inner}
}

func (b *agentFilesystemBackend) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	if req == nil {
		return nil, fmt.Errorf("read request is nil")
	}
	if b.Backend == nil {
		return nil, fmt.Errorf("filesystem backend is nil")
	}
	next := *req
	if next.Offset <= 0 {
		next.Offset = 1
	}
	if next.Limit <= 0 {
		next.Limit = agentFileReadDefaultLimitLines
	}
	return b.Backend.Read(ctx, &next)
}

func (b *agentFilesystemBackend) Edit(ctx context.Context, req *filesystem.EditRequest) error {
	if req == nil {
		return fmt.Errorf("edit request is nil")
	}
	err := b.Backend.Edit(ctx, req)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "string not found") {
		return err
	}

	content, readErr := os.ReadFile(req.FilePath)
	if readErr != nil {
		return err
	}
	text := string(content)
	normalizedText, indexMap := normalizeEditWhitespace(text)
	normalizedOld, _ := normalizeEditWhitespace(req.OldString)
	if normalizedOld == "" {
		return err
	}

	count := strings.Count(normalizedText, normalizedOld)
	if count == 0 {
		return err
	}
	if count > 1 {
		return fmt.Errorf("string (after trailing whitespace normalization) appears %d times; refusing fuzzy edit", count)
	}

	newText, ok := replaceNormalizedSpans(text, normalizedText, indexMap, normalizedOld, req.NewString)
	if !ok {
		return err
	}
	return os.WriteFile(req.FilePath, []byte(newText), 0644)
}

func normalizeEditWhitespace(s string) (string, []int) {
	var normalized strings.Builder
	indexMap := make([]int, 0, len(s))
	lineStart := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			appendTrimmedLine(&normalized, &indexMap, s, lineStart, i)
			normalized.WriteByte('\n')
			indexMap = append(indexMap, i)
			lineStart = i + 1
		}
	}
	appendTrimmedLine(&normalized, &indexMap, s, lineStart, len(s))
	return normalized.String(), indexMap
}

func appendTrimmedLine(normalized *strings.Builder, indexMap *[]int, s string, start, end int) {
	trimmedEnd := end
	for trimmedEnd > start && (s[trimmedEnd-1] == ' ' || s[trimmedEnd-1] == '\t') {
		trimmedEnd--
	}
	for i := start; i < trimmedEnd; i++ {
		normalized.WriteByte(s[i])
		*indexMap = append(*indexMap, i)
	}
}

func replaceNormalizedSpans(original, normalized string, indexMap []int, oldString, newString string) (string, bool) {
	var out strings.Builder
	searchFrom := 0
	originalFrom := 0
	replaced := false
	for {
		rel := strings.Index(normalized[searchFrom:], oldString)
		if rel < 0 {
			break
		}
		matchStart := searchFrom + rel
		matchEnd := matchStart + len(oldString)
		if matchStart >= len(indexMap) {
			return "", false
		}
		originalStart := indexMap[matchStart]
		originalEnd := len(original)
		if matchEnd < len(indexMap) {
			originalEnd = indexMap[matchEnd]
		}
		if originalStart < originalFrom || originalEnd < originalStart {
			return "", false
		}
		out.WriteString(original[originalFrom:originalStart])
		out.WriteString(newString)
		originalFrom = originalEnd
		replaced = true
		break
	}
	if !replaced {
		return "", false
	}
	out.WriteString(original[originalFrom:])
	return out.String(), true
}
