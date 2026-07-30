package agents

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	producttools "denova/internal/agents/tools"
)

const (
	retainedToolReceiptSchema       = "tool_result.receipt.v2"
	retainedToolArgumentsSchema     = "tool_call.arguments.v1"
	retainedContextProjectionBytes  = 256 * 1024
	retainedContextStringBytes      = 4 * 1024
	retainedContextCollectionValues = 64
	retainedContextRedactedValue    = "[redacted from retained tool context]"
	retainedTargetTruncationMarker  = "...[truncated]"
	protectedReceiptArgumentsBytes  = 4 * 1024
	protectedReceiptOutcomeBytes    = 8 * 1024
)

type retainedToolReceipt struct {
	Schema          string                    `json:"schema"`
	ToolName        string                    `json:"tool_name"`
	Status          agent.ToolResultStatus    `json:"status"`
	SyntheticReason agent.ToolSyntheticReason `json:"synthetic_reason,omitempty"`
	Source          agent.ToolSource          `json:"source"`
	MutationScope   agent.ToolMutationScope   `json:"mutation_scope"`
	Recovery        agent.ToolRecoveryClass   `json:"recovery"`
	Target          string                    `json:"target,omitempty"`
	IdempotencyKey  string                    `json:"idempotency_key,omitempty"`
	OriginalBytes   int                       `json:"original_bytes,omitempty"`
	ModelTruncated  bool                      `json:"model_truncated,omitempty"`
	Details         json.RawMessage           `json:"details,omitempty"`
	Diagnostic      *retainedTextProjection   `json:"diagnostic,omitempty"`
	SourceIDs       []string                  `json:"source_ids,omitempty"`
	Names           []string                  `json:"names,omitempty"`
	Artifacts       []retainedArtifactRef     `json:"artifacts,omitempty"`
	Note            string                    `json:"note"`
}

// retainedArtifactRef is the bounded artifact contract carried by receipts.
// Purpose distinguishes recoverable primary output from auxiliary attachments.
type retainedArtifactRef struct {
	Purpose         agent.ToolArtifactPurpose `json:"purpose,omitempty"`
	ReadablePath    string                    `json:"readable_path"`
	ContentType     string                    `json:"content_type,omitempty"`
	EstimatedBytes  int64                     `json:"estimated_bytes,omitempty"`
	EstimatedTokens int                       `json:"estimated_tokens,omitempty"`
}

type retainedTextProjection struct {
	Preview       string `json:"preview"`
	OriginalBytes int    `json:"original_bytes"`
	Omitted       bool   `json:"omitted"`
}

type retainedArgumentsFallback struct {
	Schema        string `json:"schema"`
	OriginalBytes int    `json:"original_bytes"`
	Note          string `json:"note"`
}

// projectProtectedToolResultReceipt creates the stable continuity evidence
// consumed by checkpoint compaction. Unlike the deprecated retained-context
// fields, this projection is produced for every protected, mutating,
// unresolved, or artifact-backed result on the live execution path.
func projectProtectedToolResultReceipt(manifest ToolManifest, arguments string, result agent.ToolResult) agent.ToolResult {
	protected := result.EffectiveResultRetention() == agent.ToolResultProtected ||
		result.Status != agent.ToolResultSuccess || result.SyntheticReason != "" ||
		manifest.MutationScope != agent.ToolMutationNone ||
		result.Metadata.ArtifactPersistence != nil || len(result.Artifacts) > 0
	if !protected {
		result.ProtectedReceipt = nil
		return result
	}
	fieldLimit := firstPositive(manifest.MaxResultBytes)
	receipt := &agent.ToolResultProtectedReceipt{
		SanitizedArguments: projectRetainedToolArguments(arguments, min(protectedReceiptArgumentsBytes, fieldLimit)),
		Outcome:            projectProtectedToolReceiptOutcome(manifest, result, min(protectedReceiptOutcomeBytes, fieldLimit)),
	}
	if receipt.SanitizedArguments == "" && receipt.Outcome == "" {
		result.ProtectedReceipt = nil
	} else {
		result.ProtectedReceipt = receipt
	}
	return result
}

func projectProtectedToolReceiptOutcome(manifest ToolManifest, result agent.ToolResult, limit int) string {
	receipt := buildRetainedToolReceipt(manifest, result)
	if result.Status != agent.ToolResultSuccess {
		diagnostic := projectRetainedText(result.ModelContent)
		// A protected checkpoint receipt records identity and size, never a copy
		// of the raw error/result body that compaction is meant to remove.
		diagnostic.Preview = ""
		diagnostic.Omitted = true
		receipt.Diagnostic = diagnostic
	}
	return marshalRetainedProjection(receipt, limit, manifest.Name, result.Status, result.ModelContent)
}

func buildRetainedToolReceipt(manifest ToolManifest, result agent.ToolResult) retainedToolReceipt {
	artifacts := retainedArtifactRefs(result.Artifacts)
	receipt := retainedToolReceipt{
		Schema: retainedToolReceiptSchema, ToolName: manifest.Name,
		Status: result.Status, SyntheticReason: result.SyntheticReason,
		Source: manifest.Source, MutationScope: manifest.MutationScope, Recovery: manifest.Recovery,
		Target: projectRetainedTarget(result.Metadata.Target), IdempotencyKey: result.Metadata.IdempotencyKey,
		OriginalBytes: result.Metadata.OriginalModelBytes, ModelTruncated: result.Metadata.ModelTruncated,
		Artifacts: artifacts,
		Note:      retainedToolResultNote(manifest.Source, retainedRecoverableArtifactAvailable(result.Artifacts)),
	}
	if len(result.Details) > 0 {
		modelSafeDetails := producttools.WorkspaceChangeResultForModel(manifest.Name, string(result.Details))
		receipt.Details = compactRetainedJSON(json.RawMessage(modelSafeDetails))
	}
	if manifest.Source == agent.ToolSourceLore && result.Status == agent.ToolResultSuccess {
		receipt.SourceIDs, receipt.Names = retainedLoreEvidence(result.ModelContent)
	}
	return receipt
}

func retainedToolResultNote(source agent.ToolSource, hasArtifact bool) string {
	if hasArtifact {
		return "The complete tool output is stored in the referenced artifact; retrieve it only if exact evidence is needed."
	}
	switch source {
	case agent.ToolSourceRead, agent.ToolSourceLore, agent.ToolSourceHistory, agent.ToolSourceWeb:
		return "The result body was available in the source turn and is omitted across turns; repeat the retained call if exact evidence is needed."
	default:
		return "The result body was available in the source turn; this stable receipt preserves the outcome and recovery metadata."
	}
}

func marshalRetainedProjection(receipt retainedToolReceipt, limit int, toolName string, status agent.ToolResultStatus, original string) string {
	encoded, err := json.Marshal(receipt)
	if err == nil && len(encoded) <= limit {
		return string(encoded)
	}
	fallback := map[string]any{
		"schema": retainedToolReceiptSchema, "tool_name": toolName, "status": status,
		"source": receipt.Source, "mutation_scope": receipt.MutationScope, "recovery": receipt.Recovery,
		"original_bytes": len(original), "artifacts": receipt.Artifacts,
		"note": "Receipt details exceeded the retained-context budget; repeat the call or retrieve its artifact if exact evidence is needed.",
	}
	if receipt.Target != "" {
		fallback["target"] = receipt.Target
	}
	encoded, _ = json.Marshal(fallback)
	if len(encoded) <= limit {
		return string(encoded)
	}
	minimal, _ := json.Marshal(map[string]any{
		"schema": retainedToolReceiptSchema, "tool_name": toolName, "status": status,
	})
	if len(minimal) <= limit {
		return string(minimal)
	}
	return ""
}

func projectRetainedToolArguments(arguments string, limit int) string {
	trimmed := strings.TrimSpace(strings.ToValidUTF8(arguments, "\uFFFD"))
	if trimmed == "" {
		return "{}"
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	projected := compactRetainedValue(value, 0)
	encoded, err := json.Marshal(projected)
	if err == nil && len(encoded) <= limit {
		return string(encoded)
	}
	fallback, _ := json.Marshal(retainedArgumentsFallback{
		Schema: retainedToolArgumentsSchema, OriginalBytes: len(trimmed),
		Note: "Large arguments were omitted from cross-turn context. Repeat the call with fresh arguments if needed.",
	})
	if len(fallback) <= limit {
		return string(fallback)
	}
	return ""
}

func compactRetainedJSON(raw json.RawMessage) json.RawMessage {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	encoded, err := json.Marshal(compactRetainedValue(value, 0))
	if err != nil || len(encoded) > retainedContextProjectionBytes {
		fallback, _ := json.Marshal(map[string]any{
			"schema": "tool_result.details_omitted.v1", "original_bytes": len(raw),
		})
		return fallback
	}
	return encoded
}

func compactRetainedValue(value any, depth int) any {
	if depth >= 12 {
		encoded, _ := json.Marshal(value)
		return retainedValueMarker("nested", len(encoded))
	}
	switch typed := value.(type) {
	case string:
		if agent.ContainsSensitiveToolContextMaterial(typed) {
			return retainedContextRedactedValue
		}
		if len(typed) <= retainedContextStringBytes {
			return typed
		}
		return retainedValueMarker("string", len(typed))
	case []any:
		limit := min(len(typed), retainedContextCollectionValues)
		result := make([]any, 0, limit+1)
		for _, item := range typed[:limit] {
			result = append(result, compactRetainedValue(item, depth+1))
		}
		if limit < len(typed) {
			result = append(result, map[string]any{"_denova_omitted_items": len(typed) - limit})
		}
		return result
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		limit := min(len(keys), retainedContextCollectionValues)
		result := make(map[string]any, limit+1)
		for _, key := range keys[:limit] {
			if agent.IsSensitiveToolContextKey(key) {
				result[key] = retainedContextRedactedValue
				continue
			}
			result[key] = compactRetainedValue(typed[key], depth+1)
		}
		if limit < len(keys) {
			result["_denova_omitted_fields"] = len(keys) - limit
		}
		return result
	default:
		return value
	}
}

func retainedValueMarker(kind string, bytes int) string {
	return fmt.Sprintf("[omitted %s: %d bytes]", kind, bytes)
}

func projectRetainedText(content string) *retainedTextProjection {
	content = strings.ToValidUTF8(content, "\uFFFD")
	preview, omitted := truncateRetainedUTF8(content, retainedContextStringBytes)
	return &retainedTextProjection{
		Preview: preview, OriginalBytes: len(content), Omitted: omitted,
	}
}

// projectRetainedTarget keeps useful resource identity without allowing
// display-only metadata to bypass the receipt's privacy and size boundary.
// The live ToolResult metadata remains untouched for UI and lifecycle use.
func projectRetainedTarget(target string) string {
	target = strings.TrimSpace(strings.ToValidUTF8(target, "\uFFFD"))
	if target == "" {
		return ""
	}
	if agent.ContainsSensitiveToolContextMaterial(target) {
		return retainedContextRedactedValue
	}
	preview, omitted := truncateRetainedUTF8(target, retainedContextStringBytes)
	if !omitted {
		return preview
	}
	prefix, _ := truncateRetainedUTF8(target, retainedContextStringBytes-len(retainedTargetTruncationMarker))
	return strings.TrimSpace(prefix) + retainedTargetTruncationMarker
}

func truncateRetainedUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	if limit <= 0 {
		return "", value != ""
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.TrimSpace(value[:end]), true
}

func retainedArtifactRefs(artifacts []agent.ToolArtifactRef) []retainedArtifactRef {
	result := make([]retainedArtifactRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifact = canonicalToolResultArtifact(artifact)
		if !artifact.Complete || !retainedArtifactPathAllowed(artifact.ReadablePath) {
			continue
		}
		result = append(result, retainedArtifactRef{
			Purpose: artifact.Purpose, ReadablePath: artifact.ReadablePath, ContentType: artifact.ContentType,
			EstimatedBytes: artifact.EstimatedBytes, EstimatedTokens: artifact.EstimatedTokens,
		})
	}
	return result
}

// retainedArtifactPathAllowed is deliberately stricter than runtime artifact
// metadata: a readable path is executable recovery data once copied into a
// checkpoint, so credential-shaped paths fail closed instead of being echoed.
func retainedArtifactPathAllowed(readablePath string) bool {
	readablePath = strings.TrimSpace(strings.ToValidUTF8(readablePath, "\uFFFD"))
	return readablePath != "" && !agent.ContainsSensitiveToolContextMaterial(readablePath)
}

func retainedRecoverableArtifactAvailable(artifacts []agent.ToolArtifactRef) bool {
	safeArtifacts := make([]agent.ToolArtifactRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifact = canonicalToolResultArtifact(artifact)
		if retainedArtifactPathAllowed(artifact.ReadablePath) {
			safeArtifacts = append(safeArtifacts, artifact)
		}
	}
	return recoverableToolResultArtifact(safeArtifacts) != nil
}

func retainedLoreEvidence(content string) ([]string, []string) {
	var sourceIDs, names []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "## "):
			name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if before, _, ok := strings.Cut(name, "（"); ok {
				name = strings.TrimSpace(before)
			}
			names = appendUniqueRetainedValue(names, name)
		case strings.HasPrefix(line, "ID："):
			sourceIDs = appendUniqueRetainedValue(sourceIDs, strings.TrimSpace(strings.TrimPrefix(line, "ID：")))
		case strings.HasPrefix(line, "ID:"):
			sourceIDs = appendUniqueRetainedValue(sourceIDs, strings.TrimSpace(strings.TrimPrefix(line, "ID:")))
		}
	}
	return sourceIDs, names
}

func appendUniqueRetainedValue(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || len(values) >= retainedContextCollectionValues {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
