package toolresult

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	workspacechange "denova/internal/workspace/change"
)

// Manifest is the durable, bounded projection of a registered definition.
// It deliberately excludes the concrete implementation and result payload.
type Manifest struct {
	Name string `json:"name"`
	agent.ToolDescriptor
}

// UnknownManifest returns the conservative lifecycle policy used when a
// definition cannot be resolved. Unknown tools are never treated as harmless.
func UnknownManifest(name string) Manifest {
	normalized := normalizeToolName(name)
	if normalized == "" {
		normalized = "unknown_tool"
	}
	return Manifest{
		Name: normalized,
		ToolDescriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceOther, Execution: agent.ToolExecutionWorkspaceExclusive,
			MutationScope: agent.ToolMutationExternal, PostCheck: agent.ToolPostCheckNone,
			Recovery: agent.ToolRecoveryNonIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
			ResultRetention: agent.ToolResultProtected,
			Steering:        agent.SteeringFinishCurrent, MaxResultBytes: DefaultMaxBytes,
		},
	}
}

func ManifestForDefinition(name string, descriptor agent.ToolDescriptor) Manifest {
	return Manifest{Name: normalizeToolName(name), ToolDescriptor: descriptor}
}

// Filtered keeps compatibility for lifecycle consumers while the
// ToolResult itself remains the source of truth for model/display/details.
type Filtered struct {
	Result         agent.ToolResult `json:"result"`
	Content        string           `json:"content"`
	Manifest       Manifest         `json:"manifest"`
	OriginalBytes  int              `json:"original_bytes"`
	ReturnedBytes  int              `json:"returned_bytes"`
	Truncated      bool             `json:"truncated"`
	Target         string           `json:"target,omitempty"`
	IdempotencyKey string           `json:"idempotency_key"`
}

// DefaultMaxBytes is the ordinary model-visible result budget.
const DefaultMaxBytes = config.DefaultAgentToolResultLimitKB * 1024

func Filter(toolName, args, content string) Filtered {
	return FilterWithLimit(toolName, args, content, 0)
}

func FilterWithLimit(toolName, args, content string, maxBytes int) Filtered {
	return filterWithManifest(
		UnknownManifest(toolName), args, agent.TextToolResult(content), maxBytes,
	)
}

func FilterText(toolName string, descriptor agent.ToolDescriptor, args, content string, maxBytes int) Filtered {
	return FilterStructured(toolName, descriptor, args, agent.TextToolResult(content), maxBytes)
}

func FilterStructured(toolName string, descriptor agent.ToolDescriptor, args string, result agent.ToolResult, maxBytes int) Filtered {
	return filterWithManifest(ManifestForDefinition(toolName, descriptor), args, result, maxBytes)
}

func filterWithManifest(manifest Manifest, args string, result agent.ToolResult, maxBytes int) Filtered {
	manifest.MaxResultBytes = NormalizeLimitBytes(firstPositive(maxBytes, manifest.MaxResultBytes))
	result.ModelContent = workspacechange.ToolReceiptForModel(manifest.Name, result.ModelContent)
	prepareToolResultProjectionMetadata(manifest, args, &result)

	normalized, err := agent.NormalizeToolResult(result, manifest.ToolDescriptor)
	if err != nil {
		normalized = agent.ToolErrorResult("Invalid structured tool result: "+err.Error(), "Invalid structured tool result: "+err.Error())
		prepareToolResultProjectionMetadata(manifest, args, &normalized)
		normalized, _ = agent.NormalizeToolResult(normalized, manifest.ToolDescriptor)
	}
	normalized = ProjectReceipt(manifest, args, normalized)
	return Filtered{
		Result: normalized, Content: normalized.ModelContent, Manifest: manifest,
		OriginalBytes: normalized.Metadata.OriginalModelBytes,
		ReturnedBytes: normalized.Metadata.ReturnedModelBytes,
		Truncated:     normalized.Metadata.ModelTruncated,
		Target:        normalized.Metadata.Target, IdempotencyKey: normalized.Metadata.IdempotencyKey,
	}
}

func prepareToolResultProjectionMetadata(manifest Manifest, args string, result *agent.ToolResult) {
	if result == nil {
		return
	}
	if argumentTarget := strings.TrimSpace(TargetFromArguments(args)); argumentTarget != "" {
		result.Metadata.Target = filepath.ToSlash(argumentTarget)
	} else {
		result.Metadata.Target = filepath.ToSlash(strings.TrimSpace(result.Metadata.Target))
	}
	result.Metadata.IdempotencyKey = toolIdempotencyKey(manifest.Name, args)
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return DefaultMaxBytes
}

// NormalizeLimitBytes applies the product default to an absent result budget.
func NormalizeLimitBytes(maxBytes int) int {
	if maxBytes <= 0 {
		return DefaultMaxBytes
	}
	return maxBytes
}

// LimitBytes resolves the configured inline model-result budget.
func LimitBytes(cfg *config.Config) int {
	if cfg == nil || cfg.AgentToolResultLimitKB <= 0 {
		return config.DefaultAgentToolResultLimitKB * 1024
	}
	return cfg.AgentToolResultLimitKB * 1024
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
