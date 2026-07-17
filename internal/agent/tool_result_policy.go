package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"denova/config"
)

type ToolSource string

const (
	ToolSourceOther   ToolSource = "other"
	ToolSourceRead    ToolSource = "read"
	ToolSourceWrite   ToolSource = "write"
	ToolSourceShell   ToolSource = "shell"
	ToolSourceLore    ToolSource = "lore"
	ToolSourceHistory ToolSource = "history"
	ToolSourceWeb     ToolSource = "web"
	ToolSourceImage   ToolSource = "image"
)

// ToolManifest describes the loop-level contract for a model-visible tool result.
type ToolManifest struct {
	Name              string     `json:"name"`
	Source            ToolSource `json:"source"`
	Capability        string     `json:"capability,omitempty"`
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
	defaultToolResultMaxBytes = config.DefaultAgentToolResultLimitKB * 1024
	toolResultMetadataHeader  = "[Denova tool result metadata]"
)

func ManifestForTool(name string) ToolManifest {
	normalized := normalizeToolName(name)
	manifest := ToolManifest{
		Name:           normalized,
		Source:         ToolSourceOther,
		MaxResultBytes: defaultToolResultMaxBytes,
	}
	switch {
	case normalized == generateImageToolName || normalized == generateChapterIllustrationToolName:
		manifest.Source = ToolSourceImage
		manifest.Capability = config.AgentToolImageGeneration
		manifest.MutatesWorkspace = true
		manifest.RequiresPostCheck = true
	case normalized == "write_lore_items":
		manifest.Source = ToolSourceLore
		manifest.Capability = config.AgentToolLoreWrite
		manifest.MutatesWorkspace = true
		manifest.RequiresPostCheck = true
	case normalized == "read_lore_items" || normalized == "list_lore_items":
		manifest.Source = ToolSourceLore
		manifest.Capability = config.AgentToolLoreRead
	case normalized == "search_story_history":
		manifest.Source = ToolSourceHistory
	case capabilityForConfigManagerTool(normalized) != "":
		manifest.Capability = capabilityForConfigManagerTool(normalized)
		if strings.HasPrefix(normalized, "write_") {
			manifest.Source = ToolSourceWrite
			manifest.MutatesWorkspace = true
			manifest.RequiresPostCheck = true
		} else {
			manifest.Source = ToolSourceRead
		}
	case isToolWriteLike(normalized):
		manifest.Source = ToolSourceWrite
		manifest.Capability = config.AgentToolFileWrite
		manifest.MutatesWorkspace = true
		manifest.RequiresPostCheck = true
	case isToolReadLike(normalized):
		manifest.Source = ToolSourceRead
		manifest.Capability = config.AgentToolFileRead
	case isToolShellLike(normalized):
		manifest.Source = ToolSourceShell
		manifest.Capability = config.AgentToolShellExecute
	case isToolWebLike(normalized):
		manifest.Source = ToolSourceWeb
		manifest.Capability = config.AgentToolWebSearch
	}
	if manifest.Name == "" {
		manifest.Name = "unknown_tool"
	}
	return manifest
}

func capabilityForConfigManagerTool(name string) string {
	switch name {
	case "list_style_references", "list_tellers", "read_tellers", "list_story_directors", "read_story_directors", "list_actor_states", "read_actor_states", "list_image_presets", "read_image_presets":
		return config.AgentToolLoreRead
	case "write_style_references", "write_tellers", "write_story_directors", "write_actor_states", "write_image_presets":
		return config.AgentToolLoreWrite
	case "list_automations", "read_automations", "write_automations":
		return config.AgentToolTodo
	case "list_skills", "read_skills", "write_skills":
		return config.AgentToolSkills
	case "list_agent_configs":
		return config.AgentToolAgentConfigRead
	case "write_agent_configs":
		return config.AgentToolAgentConfigWrite
	default:
		return ""
	}
}

func FilterToolResultForModel(toolName, args, content string) FilteredToolResult {
	return FilterToolResultForModelWithLimit(toolName, args, content, 0)
}

func FilterToolResultForModelWithLimit(toolName, args, content string, maxBytes int) FilteredToolResult {
	manifest := ManifestForTool(toolName)
	manifest.MaxResultBytes = normalizeToolResultLimitBytes(maxBytes)
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
	if truncated && !strings.Contains(body, "[tool result truncated]") {
		body = strings.TrimRight(body, "\n")
		if body != "" {
			body += "\n"
		}
		body += "[tool result truncated]"
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
	return normalizeToolResultLimitBytes(manifest.MaxResultBytes)
}

func normalizeToolResultLimitBytes(maxBytes int) int {
	if maxBytes <= 0 {
		return defaultToolResultMaxBytes
	}
	return maxBytes
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
		toolResultMetadataHeader,
		"schema: tool_result.v1",
		"source: " + string(manifest.Source),
		"capability: " + firstNonEmpty(manifest.Capability, "unclassified"),
		fmt.Sprintf("mutates_workspace: %t", manifest.MutatesWorkspace),
		fmt.Sprintf("requires_post_check: %t", manifest.RequiresPostCheck),
		fmt.Sprintf("max_result_bytes: %d", manifest.MaxResultBytes),
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
