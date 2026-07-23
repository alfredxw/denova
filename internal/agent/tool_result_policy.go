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

// ToolExecutionClass declares the workspace lease held while a tool runs.
// Unknown tools are deliberately exclusive until their registration provides a
// stronger contract.
type ToolExecutionClass string

const (
	ToolExecutionParallelRead       ToolExecutionClass = "parallel_read"
	ToolExecutionWorkspaceExclusive ToolExecutionClass = "workspace_exclusive"
	ToolExecutionChild              ToolExecutionClass = "child"
)

// ToolRecoveryClass declares what recovery may safely do after observing a
// durable tool-start record without a matching completion record.
type ToolRecoveryClass string

const (
	ToolRecoveryReadOnly      ToolRecoveryClass = "read_only"
	ToolRecoveryIdempotent    ToolRecoveryClass = "idempotent"
	ToolRecoveryReconcilable  ToolRecoveryClass = "reconcilable"
	ToolRecoveryNonIdempotent ToolRecoveryClass = "non_idempotent"
)

type ToolResultProjection string

const (
	ToolResultBoundedModelContext ToolResultProjection = "bounded_model_context"
)

// ToolDescriptor is the loop-level contract for a model-visible tool. The
// contract is registered explicitly; the runtime must never infer side effects
// or retry safety from a tool-name prefix.
type ToolDescriptor struct {
	Name              string               `json:"name"`
	Source            ToolSource           `json:"source"`
	Capability        string               `json:"capability,omitempty"`
	Execution         ToolExecutionClass   `json:"execution"`
	Recovery          ToolRecoveryClass    `json:"recovery"`
	ResultProjection  ToolResultProjection `json:"result_projection"`
	MutatesWorkspace  bool                 `json:"mutates_workspace"`
	MaxResultBytes    int                  `json:"max_result_bytes"`
	RequiresPostCheck bool                 `json:"requires_post_check"`
}

// ToolManifest remains an alias while callers migrate to descriptor wording.
// It is intentionally not a second representation of tool semantics.
type ToolManifest = ToolDescriptor

/*
Tool descriptors form an explicit catalog because recovery is a correctness
boundary. Adding a new model-visible tool requires choosing its execution and
recovery classes instead of accidentally inheriting behavior from its name.
*/
var toolDescriptorCatalog = buildToolDescriptorCatalog()

func buildToolDescriptorCatalog() map[string]ToolDescriptor {
	catalog := make(map[string]ToolDescriptor)
	register := func(names []string, descriptor ToolDescriptor) {
		for _, name := range names {
			entry := descriptor
			entry.Name = normalizeToolName(name)
			entry.MaxResultBytes = defaultToolResultMaxBytes
			entry.ResultProjection = ToolResultBoundedModelContext
			catalog[entry.Name] = entry
		}
	}

	register([]string{"read_file", "list_files", "ls", "glob", "grep", "search_file", "search_workspace"}, ToolDescriptor{
		Source: ToolSourceRead, Capability: config.AgentToolFileRead,
		Execution: ToolExecutionParallelRead, Recovery: ToolRecoveryReadOnly,
	})
	register([]string{"write_file", "edit_file", "delete_file", "create_file", "move_file", "copy_file", "rename_file", "mkdir", "remove_file"}, ToolDescriptor{
		Source: ToolSourceWrite, Capability: config.AgentToolFileWrite,
		Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryReconcilable,
		MutatesWorkspace: true, RequiresPostCheck: true,
	})
	register([]string{"bash", "shell", "execute", "execute_shell", "execute_command", "run_command", "terminal"}, ToolDescriptor{
		Source: ToolSourceShell, Capability: config.AgentToolShellExecute,
		Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryNonIdempotent,
		MutatesWorkspace: true, RequiresPostCheck: true,
	})
	register([]string{"read_lore_items", "list_lore_items"}, ToolDescriptor{
		Source: ToolSourceLore, Capability: config.AgentToolLoreRead,
		Execution: ToolExecutionParallelRead, Recovery: ToolRecoveryReadOnly,
	})
	register([]string{"write_lore_items"}, ToolDescriptor{
		Source: ToolSourceLore, Capability: config.AgentToolLoreWrite,
		Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryReconcilable,
		MutatesWorkspace: true, RequiresPostCheck: true,
	})
	register([]string{"search_story_history", "read_event_cards"}, ToolDescriptor{
		Source: ToolSourceHistory, Execution: ToolExecutionParallelRead, Recovery: ToolRecoveryReadOnly,
	})
	register([]string{"prepare_interactive_turn", initializeStoryStateSchemaToolName, interactiveTurnSubmissionToolName, legacyActorStatePatchesToolName, legacyInteractiveChoicesToolName, submitDirectorPlanUpdateToolName}, ToolDescriptor{
		Source: ToolSourceHistory, Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryReconcilable,
	})
	register([]string{generateImageToolName, generateChapterIllustrationToolName, "generate_interactive_image"}, ToolDescriptor{
		Source: ToolSourceImage, Capability: config.AgentToolImageGeneration,
		Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryNonIdempotent,
		MutatesWorkspace: true, RequiresPostCheck: true,
	})
	register([]string{"web_search", "search_web", "duckduckgo_search", "browser_search"}, ToolDescriptor{
		Source: ToolSourceWeb, Capability: config.AgentToolWebSearch,
		Execution: ToolExecutionParallelRead, Recovery: ToolRecoveryReadOnly,
	})
	register([]string{"write_todos"}, ToolDescriptor{
		Source: ToolSourceOther, Capability: config.AgentToolTodo,
		Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryIdempotent,
	})
	register([]string{"task"}, ToolDescriptor{
		Source: ToolSourceOther, Execution: ToolExecutionChild, Recovery: ToolRecoveryNonIdempotent,
	})
	register([]string{"skill"}, ToolDescriptor{
		Source: ToolSourceOther, Capability: config.AgentToolSkills,
		Execution: ToolExecutionParallelRead, Recovery: ToolRecoveryReadOnly,
	})

	registerConfigManagerDescriptors(register)
	return catalog
}

func registerConfigManagerDescriptors(register func([]string, ToolDescriptor)) {
	readGroups := map[string][]string{
		config.AgentToolLoreRead:        {"list_style_references", "list_tellers", "read_tellers", "list_story_directors", "read_story_directors", "list_event_packages", "read_event_packages", "list_actor_states", "read_actor_states", "list_image_presets", "read_image_presets"},
		config.AgentToolTodo:            {"list_automations", "read_automations"},
		config.AgentToolSkills:          {"list_skills", "read_skills"},
		config.AgentToolAgentConfigRead: {"list_agent_configs"},
	}
	for capability, names := range readGroups {
		register(names, ToolDescriptor{
			Source: ToolSourceRead, Capability: capability,
			Execution: ToolExecutionParallelRead, Recovery: ToolRecoveryReadOnly,
		})
	}
	writeGroups := map[string][]string{
		config.AgentToolLoreWrite:        {"write_style_references", "write_tellers", "write_story_directors", "write_event_packages", "write_actor_states", "write_image_presets"},
		config.AgentToolTodo:             {"write_automations"},
		config.AgentToolSkills:           {"write_skills"},
		config.AgentToolAgentConfigWrite: {"write_agent_configs"},
	}
	for capability, names := range writeGroups {
		register(names, ToolDescriptor{
			Source: ToolSourceWrite, Capability: capability,
			Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryReconcilable,
			MutatesWorkspace: true, RequiresPostCheck: true,
		})
	}
}

// DescriptorForTool resolves a declared contract. Unknown tools receive a
// conservative, non-retryable descriptor; their spelling does not grant them
// read-only concurrency or implicit idempotency.
func DescriptorForTool(name string) ToolDescriptor {
	if descriptor, ok := declaredToolDescriptor(name); ok {
		return descriptor
	}
	normalized := normalizeToolName(name)
	if normalized == "" {
		normalized = "unknown_tool"
	}
	return ToolDescriptor{
		Name: normalized, Source: ToolSourceOther,
		Execution: ToolExecutionWorkspaceExclusive, Recovery: ToolRecoveryNonIdempotent,
		ResultProjection: ToolResultBoundedModelContext,
		MaxResultBytes:   defaultToolResultMaxBytes,
	}
}

func declaredToolDescriptor(name string) (ToolDescriptor, bool) {
	descriptor, ok := toolDescriptorCatalog[normalizeToolName(name)]
	return descriptor, ok
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
	return DescriptorForTool(name)
}

func FilterToolResultForModel(toolName, args, content string) FilteredToolResult {
	return FilterToolResultForModelWithLimit(toolName, args, content, 0)
}

func FilterToolResultForModelWithLimit(toolName, args, content string, maxBytes int) FilteredToolResult {
	manifest := ManifestForTool(toolName)
	manifest.MaxResultBytes = normalizeToolResultLimitBytes(maxBytes)
	content = workspaceChangeToolResultForModel(toolName, content)
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
