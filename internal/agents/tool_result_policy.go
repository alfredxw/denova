package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
)

type ToolSource = agent.ToolSource
type ToolExecutionClass = agent.ToolExecutionClass
type ToolRecoveryClass = agent.ToolRecoveryClass

const (
	ToolSourceOther   = agent.ToolSourceOther
	ToolSourceRead    = agent.ToolSourceRead
	ToolSourceWrite   = agent.ToolSourceWrite
	ToolSourceShell   = agent.ToolSourceShell
	ToolSourceLore    = agent.ToolSourceLore
	ToolSourceHistory = agent.ToolSourceHistory
	ToolSourceWeb     = agent.ToolSourceWeb
	ToolSourceImage   = agent.ToolSourceImage
)

const (
	ToolExecutionParallelRead       = agent.ToolExecutionParallelRead
	ToolExecutionWorkspaceExclusive = agent.ToolExecutionWorkspaceExclusive
	ToolExecutionChild              = agent.ToolExecutionChild
)

const (
	ToolRecoveryReadOnly      = agent.ToolRecoveryReadOnly
	ToolRecoveryIdempotent    = agent.ToolRecoveryIdempotent
	ToolRecoveryReconcilable  = agent.ToolRecoveryReconcilable
	ToolRecoveryNonIdempotent = agent.ToolRecoveryNonIdempotent
)

const ToolResultBoundedModelContext = agent.ToolResultBoundedModelContext

// ToolManifest is the durable, bounded projection of a registered definition.
// It deliberately excludes the concrete implementation and result payload.
type ToolManifest struct {
	Name string `json:"name"`
	agent.ToolDescriptor
}

func unknownToolManifest(name string) ToolManifest {
	normalized := normalizeToolName(name)
	if normalized == "" {
		normalized = "unknown_tool"
	}
	return ToolManifest{
		Name: normalized,
		ToolDescriptor: agent.ToolDescriptor{
			Source: ToolSourceOther, Execution: ToolExecutionWorkspaceExclusive,
			Recovery: ToolRecoveryNonIdempotent, ResultProjection: ToolResultBoundedModelContext,
			Steering: agent.SteeringFinishCurrent, MaxResultBytes: defaultToolResultMaxBytes,
		},
	}
}

func manifestForDefinition(name string, descriptor agent.ToolDescriptor) ToolManifest {
	return ToolManifest{Name: normalizeToolName(name), ToolDescriptor: descriptor}
}

// FilteredToolResult keeps compatibility for lifecycle consumers while the
// ToolResult itself remains the source of truth for model/display/details.
type FilteredToolResult struct {
	Result         agent.ToolResult `json:"result"`
	Content        string           `json:"content"`
	Manifest       ToolManifest     `json:"manifest"`
	OriginalBytes  int              `json:"original_bytes"`
	ReturnedBytes  int              `json:"returned_bytes"`
	Truncated      bool             `json:"truncated"`
	Target         string           `json:"target,omitempty"`
	IdempotencyKey string           `json:"idempotency_key"`
}

const (
	defaultToolResultMaxBytes = config.DefaultAgentToolResultLimitKB * 1024
	toolResultMetadataHeader  = "[Denova tool result metadata]" // legacy history parser only
)

func FilterToolResultForModel(toolName, args, content string) FilteredToolResult {
	return FilterToolResultForModelWithLimit(toolName, args, content, 0)
}

func FilterToolResultForModelWithLimit(toolName, args, content string, maxBytes int) FilteredToolResult {
	return filterStructuredToolResultWithManifest(
		unknownToolManifest(toolName), args, agent.TextToolResult(content), maxBytes,
	)
}

func filterToolResultForModelWithDescriptor(toolName string, descriptor agent.ToolDescriptor, args, content string, maxBytes int) FilteredToolResult {
	return filterStructuredToolResultWithDescriptor(toolName, descriptor, args, agent.TextToolResult(content), maxBytes)
}

func filterStructuredToolResultWithDescriptor(toolName string, descriptor agent.ToolDescriptor, args string, result agent.ToolResult, maxBytes int) FilteredToolResult {
	return filterStructuredToolResultWithManifest(manifestForDefinition(toolName, descriptor), args, result, maxBytes)
}

func filterStructuredToolResultWithManifest(manifest ToolManifest, args string, result agent.ToolResult, maxBytes int) FilteredToolResult {
	manifest.MaxResultBytes = normalizeToolResultLimitBytes(firstPositive(maxBytes, manifest.MaxResultBytes))
	result.ModelContent = producttools.WorkspaceChangeResultForModel(manifest.Name, result.ModelContent)
	result.Metadata.Target = filepath.ToSlash(strings.TrimSpace(toolPathFromArgs(args)))
	result.Metadata.IdempotencyKey = toolIdempotencyKey(manifest.Name, args)

	normalized, err := agent.NormalizeToolResult(result, manifest.ToolDescriptor)
	if err != nil {
		normalized = agent.ToolErrorResult("Invalid structured tool result: "+err.Error(), "Invalid structured tool result: "+err.Error())
		normalized, _ = agent.NormalizeToolResult(normalized, manifest.ToolDescriptor)
	}
	return FilteredToolResult{
		Result: normalized, Content: normalized.ModelContent, Manifest: manifest,
		OriginalBytes: normalized.Metadata.OriginalModelBytes,
		ReturnedBytes: normalized.Metadata.ReturnedModelBytes,
		Truncated:     normalized.Metadata.ModelTruncated,
		Target:        normalized.Metadata.Target, IdempotencyKey: normalized.Metadata.IdempotencyKey,
	}
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return defaultToolResultMaxBytes
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
	const suffix = "\n[tool result truncated]"
	bodyLimit := limit - len(suffix)
	if bodyLimit <= 0 {
		bodyLimit = limit
		for bodyLimit > 0 && !utf8.RuneStart(content[bodyLimit]) {
			bodyLimit--
		}
		return content[:bodyLimit], true
	}
	for bodyLimit > 0 && !utf8.RuneStart(content[bodyLimit]) {
		bodyLimit--
	}
	return strings.TrimRight(content[:bodyLimit], "\n") + suffix, true
}

func toolIdempotencyKey(toolName, args string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(args)))
	return fmt.Sprintf("%s:%s", normalizeToolName(toolName), hex.EncodeToString(hash[:8]))
}

// parseToolResultManifest reads only the legacy text metadata format. New
// transcripts and lifecycle events carry descriptor and metadata explicitly.
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
	return manifestForDefinition(name, agent.ToolDescriptor{
		Source: agent.ToolSource(values["source"]), Capability: values["capability"],
		Execution: agent.ToolExecutionClass(values["execution"]), Recovery: agent.ToolRecoveryClass(values["recovery"]),
		ResultProjection: agent.ToolResultProjection(values["result_projection"]),
		Steering:         agent.SteeringFinishCurrent,
		MutatesWorkspace: mutates, MaxResultBytes: maxBytes, RequiresPostCheck: postCheck,
	}), true
}
