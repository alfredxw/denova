package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type ToolSource string

const (
	ToolSourceOther ToolSource = "other"
	ToolSourceRead  ToolSource = "read"
	ToolSourceWrite ToolSource = "write"
	ToolSourceShell ToolSource = "shell"
	ToolSourceLore  ToolSource = "lore"
	ToolSourceWeb   ToolSource = "web"
)

// ToolManifest describes the loop-level contract for a model-visible tool result.
type ToolManifest struct {
	Name              string     `json:"name"`
	Source            ToolSource `json:"source"`
	MutatesWorkspace  bool       `json:"mutates_workspace"`
	MaxResultBytes    int        `json:"max_result_bytes"`
	RequiresPostCheck bool       `json:"requires_post_check"`
}

type FilteredToolResult struct {
	Content        string       `json:"content"`
	Manifest       ToolManifest `json:"manifest"`
	OriginalBytes  int          `json:"original_bytes"`
	ReturnedBytes  int          `json:"returned_bytes"`
	Truncated      bool         `json:"truncated"`
	Target         string       `json:"target,omitempty"`
	IdempotencyKey string       `json:"idempotency_key"`
}

const (
	defaultToolResultMaxBytes = 24 * 1024
	readToolResultMaxBytes    = 32 * 1024
	writeToolResultMaxBytes   = 8 * 1024
	loreToolResultMaxBytes    = 32 * 1024
)

func ManifestForTool(name string) ToolManifest {
	normalized := normalizeToolName(name)
	manifest := ToolManifest{
		Name:           normalized,
		Source:         ToolSourceOther,
		MaxResultBytes: defaultToolResultMaxBytes,
	}
	switch {
	case normalized == "write_lore_items":
		manifest.Source = ToolSourceLore
		manifest.MutatesWorkspace = true
		manifest.RequiresPostCheck = true
		manifest.MaxResultBytes = writeToolResultMaxBytes
	case normalized == "read_lore_items" || normalized == "list_lore_items":
		manifest.Source = ToolSourceLore
		manifest.MaxResultBytes = loreToolResultMaxBytes
	case isToolWriteLike(normalized):
		manifest.Source = ToolSourceWrite
		manifest.MutatesWorkspace = true
		manifest.RequiresPostCheck = true
		manifest.MaxResultBytes = writeToolResultMaxBytes
	case isToolReadLike(normalized):
		manifest.Source = ToolSourceRead
		manifest.MaxResultBytes = readToolResultMaxBytes
	case isToolShellLike(normalized):
		manifest.Source = ToolSourceShell
	case isToolWebLike(normalized):
		manifest.Source = ToolSourceWeb
	}
	if manifest.Name == "" {
		manifest.Name = "unknown_tool"
	}
	return manifest
}

func FilterToolResultForModel(toolName, args, content string) FilteredToolResult {
	manifest := ManifestForTool(toolName)
	body, truncated := truncateUTF8Bytes(content, normalizedToolResultLimit(manifest))
	return filteredToolResultFromBody(manifest, args, body, len(content), truncated)
}

func filteredToolResultFromBody(manifest ToolManifest, args, body string, originalBytes int, truncated bool) FilteredToolResult {
	limit := manifest.MaxResultBytes
	if limit <= 0 {
		limit = defaultToolResultMaxBytes
	}
	if !truncated {
		body, truncated = truncateUTF8Bytes(body, limit)
	}
	target := toolPathFromArgs(args)
	idempotencyKey := toolIdempotencyKey(manifest.Name, args)
	metadata := formatToolResultMetadata(manifest, originalBytes, len(body), truncated, target, idempotencyKey)
	result := strings.TrimRight(body, "\n")
	if result != "" {
		result += "\n\n"
	}
	result += metadata
	return FilteredToolResult{
		Content:        result,
		Manifest:       manifest,
		OriginalBytes:  originalBytes,
		ReturnedBytes:  len(result),
		Truncated:      truncated,
		Target:         target,
		IdempotencyKey: idempotencyKey,
	}
}

func normalizedToolResultLimit(manifest ToolManifest) int {
	if manifest.MaxResultBytes > 0 {
		return manifest.MaxResultBytes
	}
	return defaultToolResultMaxBytes
}

func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isToolWriteLike(name string) bool {
	switch name {
	case "write_file", "edit_file", "delete_file", "create_file", "move_file", "copy_file", "rename_file", "mkdir", "remove_file":
		return true
	}
	return strings.HasPrefix(name, "write_") ||
		strings.HasPrefix(name, "edit_") ||
		strings.HasPrefix(name, "delete_") ||
		strings.HasPrefix(name, "create_") ||
		strings.HasPrefix(name, "move_") ||
		strings.HasPrefix(name, "copy_") ||
		strings.HasPrefix(name, "rename_") ||
		strings.HasPrefix(name, "remove_")
}

func isToolReadLike(name string) bool {
	switch name {
	case "read_file", "list_files", "ls", "glob", "grep", "search_file", "search_workspace":
		return true
	default:
		return strings.HasPrefix(name, "read_") ||
			strings.HasPrefix(name, "list_") ||
			strings.HasPrefix(name, "search_")
	}
}

func isToolShellLike(name string) bool {
	switch name {
	case "bash", "shell", "execute", "execute_command", "run_command", "terminal":
		return true
	default:
		return strings.Contains(name, "shell") || strings.Contains(name, "command")
	}
}

func isToolWebLike(name string) bool {
	return strings.Contains(name, "web") ||
		strings.Contains(name, "search") ||
		strings.Contains(name, "duckduckgo") ||
		strings.Contains(name, "browser")
}

func truncateUTF8Bytes(content string, limit int) (string, bool) {
	if limit <= 0 || len(content) <= limit {
		return content, false
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	if limit <= 0 {
		return "", true
	}
	return content[:limit] + "\n[tool result truncated]", true
}

func toolIdempotencyKey(toolName, args string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(args)))
	return fmt.Sprintf("%s:%s", normalizeToolName(toolName), hex.EncodeToString(hash[:8]))
}

func formatToolResultMetadata(manifest ToolManifest, originalBytes, returnedBodyBytes int, truncated bool, target, idempotencyKey string) string {
	fields := []string{
		"[Nova tool result metadata]",
		"schema: tool_result.v1",
		"source: " + string(manifest.Source),
		fmt.Sprintf("mutates_workspace: %t", manifest.MutatesWorkspace),
		fmt.Sprintf("requires_post_check: %t", manifest.RequiresPostCheck),
		fmt.Sprintf("truncated: %t", truncated),
		fmt.Sprintf("original_bytes: %d", originalBytes),
		fmt.Sprintf("returned_body_bytes: %d", returnedBodyBytes),
		"idempotency_key: " + idempotencyKey,
	}
	if target = filepath.ToSlash(strings.TrimSpace(target)); target != "" {
		fields = append(fields, "target: "+target)
	}
	return strings.Join(fields, "\n")
}
