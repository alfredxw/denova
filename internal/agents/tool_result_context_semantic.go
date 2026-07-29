package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

const (
	retainedToolReceiptSchema       = "tool_result.receipt.v2"
	retainedToolArgumentsSchema     = "tool_call.arguments.v1"
	retainedContextProjectionBytes  = 256 * 1024
	retainedContextStringBytes      = 4 * 1024
	retainedContextCollectionValues = 64
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
	Artifacts       []agent.ToolArtifactRef   `json:"artifacts,omitempty"`
	Note            string                    `json:"note"`
}

type retainedTextProjection struct {
	Preview       string `json:"preview"`
	OriginalBytes int    `json:"original_bytes"`
	SHA256        string `json:"sha256"`
	Omitted       bool   `json:"omitted"`
}

type retainedArgumentsFallback struct {
	Schema        string `json:"schema"`
	OriginalBytes int    `json:"original_bytes"`
	SHA256        string `json:"sha256"`
	Note          string `json:"note"`
}

func projectRetainedToolResult(manifest ToolManifest, arguments string, result agent.ToolResult) agent.ToolResult {
	result.ContextRetention = manifest.ContextRetention
	if manifest.ContextRetention != agent.ToolContextReceipt {
		result.RetainedArguments = ""
		result.RetainedContent = ""
		return result
	}
	limit := min(firstPositive(manifest.MaxResultBytes), retainedContextProjectionBytes)
	result.RetainedArguments = projectRetainedToolArguments(arguments, limit)
	result.RetainedContent = projectRetainedToolReceipt(manifest, result, limit)
	return result
}

func projectRetainedToolReceipt(manifest ToolManifest, result agent.ToolResult, limit int) string {
	receipt := retainedToolReceipt{
		Schema: retainedToolReceiptSchema, ToolName: manifest.Name,
		Status: result.Status, SyntheticReason: result.SyntheticReason,
		Source: manifest.Source, MutationScope: manifest.MutationScope, Recovery: manifest.Recovery,
		Target: result.Metadata.Target, IdempotencyKey: result.Metadata.IdempotencyKey,
		OriginalBytes: result.Metadata.OriginalModelBytes, ModelTruncated: result.Metadata.ModelTruncated,
		Artifacts: append([]agent.ToolArtifactRef(nil), result.Artifacts...),
		Note:      retainedToolResultNote(manifest.Source, len(result.Artifacts) > 0),
	}
	if len(result.Details) > 0 {
		receipt.Details = compactRetainedJSON(result.Details)
	}
	if result.Status != agent.ToolResultSuccess {
		receipt.Diagnostic = projectRetainedText(result.ModelContent)
	}
	if manifest.Source == agent.ToolSourceLore && result.Status == agent.ToolResultSuccess {
		receipt.SourceIDs, receipt.Names = retainedLoreEvidence(result.ModelContent)
	}
	return marshalRetainedProjection(receipt, limit, manifest.Name, result.Status, result.ModelContent)
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
		"original_bytes": len(original), "sha256": retainedSHA256(original), "artifacts": receipt.Artifacts,
		"note": "Receipt details exceeded the retained-context budget; repeat the call or retrieve its artifact if exact evidence is needed.",
	}
	if receipt.Target != "" {
		fallback["target"] = receipt.Target
	}
	encoded, _ = json.Marshal(fallback)
	if len(encoded) <= limit {
		return string(encoded)
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
		Schema: retainedToolArgumentsSchema, OriginalBytes: len(trimmed), SHA256: retainedSHA256(trimmed),
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
			"sha256": retainedSHA256(string(raw)),
		})
		return fallback
	}
	return encoded
}

func compactRetainedValue(value any, depth int) any {
	if depth >= 12 {
		encoded, _ := json.Marshal(value)
		return retainedValueMarker("nested", len(encoded), retainedSHA256(string(encoded)))
	}
	switch typed := value.(type) {
	case string:
		if len(typed) <= retainedContextStringBytes {
			return typed
		}
		return retainedValueMarker("string", len(typed), retainedSHA256(typed))
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
			if sensitiveToolContextKey(key) {
				result[key] = "[redacted from retained tool context]"
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

func sensitiveToolContextKey(key string) bool {
	normalized := strings.Map(func(value rune) rune {
		if value >= 'A' && value <= 'Z' {
			return value + ('a' - 'A')
		}
		if (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') {
			return value
		}
		return -1
	}, strings.TrimSpace(key))
	for _, marker := range []string{"password", "passwd", "secret", "token", "apikey", "authorization", "cookie", "credential", "privatekey", "bearer"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func retainedValueMarker(kind string, bytes int, digest string) string {
	return fmt.Sprintf("[omitted %s: %d bytes, sha256:%s]", kind, bytes, digest)
}

func projectRetainedText(content string) *retainedTextProjection {
	content = strings.ToValidUTF8(content, "\uFFFD")
	preview := content
	omitted := len(preview) > retainedContextStringBytes
	if omitted {
		end := retainedContextStringBytes
		for end > 0 && !utf8.RuneStart(preview[end]) {
			end--
		}
		preview = strings.TrimSpace(preview[:end])
	}
	return &retainedTextProjection{
		Preview: preview, OriginalBytes: len(content), SHA256: retainedSHA256(content), Omitted: omitted,
	}
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

func retainedSHA256(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func isRetainedToolReceipt(content string) bool {
	var envelope struct {
		Schema string `json:"schema"`
	}
	return json.Unmarshal([]byte(content), &envelope) == nil && envelope.Schema == retainedToolReceiptSchema
}
