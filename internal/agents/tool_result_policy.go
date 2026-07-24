package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	producttools "denova/internal/agents/tools"
)

type ToolSource = agenttools.Source
type ToolExecutionClass = agenttools.ExecutionClass
type ToolRecoveryClass = agenttools.RecoveryClass

const (
	ToolSourceOther   = agenttools.SourceOther
	ToolSourceRead    = agenttools.SourceRead
	ToolSourceWrite   = agenttools.SourceWrite
	ToolSourceShell   = agenttools.SourceShell
	ToolSourceLore    = agenttools.Source("lore")
	ToolSourceHistory = agenttools.Source("history")
	ToolSourceWeb     = agenttools.Source("web")
	ToolSourceImage   = agenttools.Source("image")
)

const (
	ToolExecutionParallelRead       = agenttools.ExecutionParallelRead
	ToolExecutionWorkspaceExclusive = agenttools.ExecutionWorkspaceExclusive
	ToolExecutionChild              = agenttools.ExecutionChild
)

const (
	ToolRecoveryReadOnly      = agenttools.RecoveryReadOnly
	ToolRecoveryIdempotent    = agenttools.RecoveryIdempotent
	ToolRecoveryReconcilable  = agenttools.RecoveryReconcilable
	ToolRecoveryNonIdempotent = agenttools.RecoveryNonIdempotent
)

const ToolResultBoundedModelContext = agenttools.ResultBoundedModelContext

// ToolManifest is the runtime projection of an attached Descriptor. Name is
// derived from Tool.Info and is never supplied when the Definition is built.
type ToolManifest struct {
	Name string `json:"name"`
	agenttools.Descriptor
}

func unknownToolManifest(name string) ToolManifest {
	normalized := normalizeToolName(name)
	if normalized == "" {
		normalized = "unknown_tool"
	}
	return ToolManifest{
		Name: normalized,
		Descriptor: agenttools.Descriptor{
			Source:    ToolSourceOther,
			Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryNonIdempotent,
			ResultProjection: ToolResultBoundedModelContext,
			MaxResultBytes:   defaultToolResultMaxBytes,
		},
	}
}

func manifestForDefinition(name string, descriptor agenttools.Descriptor) ToolManifest {
	return ToolManifest{Name: normalizeToolName(name), Descriptor: descriptor}
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

func FilterToolResultForModel(toolName, args, content string) FilteredToolResult {
	return FilterToolResultForModelWithLimit(toolName, args, content, 0)
}

func FilterToolResultForModelWithLimit(toolName, args, content string, maxBytes int) FilteredToolResult {
	manifest := unknownToolManifest(toolName)
	return filterToolResultForModelWithManifest(manifest, args, content, maxBytes)
}

func filterToolResultForModelWithDescriptor(toolName string, descriptor agenttools.Descriptor, args, content string, maxBytes int) FilteredToolResult {
	return filterToolResultForModelWithManifest(manifestForDefinition(toolName, descriptor), args, content, maxBytes)
}

func filterToolResultForModelWithManifest(manifest ToolManifest, args, content string, maxBytes int) FilteredToolResult {
	manifest.MaxResultBytes = normalizeToolResultLimitBytes(maxBytes)
	content = producttools.WorkspaceChangeResultForModel(manifest.Name, content)
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
		"execution: " + string(manifest.Execution),
		"recovery: " + string(manifest.Recovery),
		"result_projection: " + string(manifest.ResultProjection),
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

func parseToolResultManifest(name, content string) (ToolManifest, bool) {
	marker := strings.LastIndex(content, toolResultMetadataHeader)
	if marker < 0 {
		return ToolManifest{}, false
	}
	values := make(map[string]string)
	for _, line := range strings.Split(content[marker+len(toolResultMetadataHeader):], "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if found {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if values["schema"] != "tool_result.v1" {
		return ToolManifest{}, false
	}
	maxBytes, _ := strconv.Atoi(values["max_result_bytes"])
	mutates, _ := strconv.ParseBool(values["mutates_workspace"])
	postCheck, _ := strconv.ParseBool(values["requires_post_check"])
	return manifestForDefinition(name, agenttools.Descriptor{
		Source: agenttools.Source(values["source"]), Capability: values["capability"],
		Execution: agenttools.ExecutionClass(values["execution"]), Recovery: agenttools.RecoveryClass(values["recovery"]),
		ResultProjection: agenttools.ResultProjection(values["result_projection"]),
		MutatesWorkspace: mutates, MaxResultBytes: maxBytes, RequiresPostCheck: postCheck,
	}), true
}
