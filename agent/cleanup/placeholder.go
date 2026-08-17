package cleanup

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const maxPlaceholderBytes = 8 * 1024

func renderPlaceholder(call agent.ToolCall, message *agent.Message, supersededBy string) (string, bool) {
	if message == nil || message.ToolResult == nil || message.ToolResult.ContextHints == nil {
		return "", false
	}
	recovery := message.ToolResult.ContextHints.Recovery
	if !usableRecovery(recovery, message.ToolResult.Artifacts) {
		return "", false
	}
	lines := []string{
		"[Older tool result removed to save context.",
		"Tool: " + bounded(call.Function.Name, 256),
		"Status: " + bounded(string(message.ToolResult.Status), 64),
		"Recovery: " + bounded(string(recovery.Kind), 64),
	}
	if reference := marshalReference(recovery.Reference); reference != "" {
		lines = append(lines, "Reference: "+reference)
	}
	if path := strings.TrimSpace(recovery.ArtifactPath); path != "" {
		lines = append(lines, "Readable artifact: "+bounded(path, 2048))
	}
	if recovery.EstimatedBytes > 0 {
		lines = append(lines, fmt.Sprintf("Estimated original bytes: %d", recovery.EstimatedBytes))
	}
	if recovery.EstimatedTokens > 0 {
		lines = append(lines, fmt.Sprintf("Estimated original tokens: %d", recovery.EstimatedTokens))
	}
	if supersededBy = strings.TrimSpace(supersededBy); supersededBy != "" {
		lines = append(lines, "Superseded by tool call: "+bounded(supersededBy, 256))
	}
	lines = append(lines, recoveryInstruction(recovery.Kind)+"]")
	content := strings.Join(lines, "\n")
	return content, content != "" && len(content) <= maxPlaceholderBytes
}

func usableRecovery(recovery agent.ToolResultRecoveryHint, artifacts []agent.ToolArtifactRef) bool {
	switch recovery.Kind {
	case agent.ToolResultRecoveryRead, agent.ToolResultRecoveryRefetch, agent.ToolResultRecoveryRerun:
		// For non-artifact recovery the public contract is deliberately generic:
		// Reference is the complete, normalized invocation needed to repeat the
		// operation. Product field names are not part of Cleanup policy.
		return completeReference(recovery.Reference)
	case agent.ToolResultRecoveryArtifact:
		want := strings.TrimSpace(recovery.ArtifactPath)
		if want == "" || incompleteReferenceMarker(want) {
			return false
		}
		for _, artifact := range artifacts {
			if strings.TrimSpace(artifact.ReadablePath) == want && artifact.Complete && strings.TrimSpace(artifact.ContentType) != "" &&
				(artifact.Purpose == agent.ToolArtifactPurposeCompleteModelOutput || artifact.Purpose == agent.ToolArtifactPurposeCompleteToolOutput) {
				return true
			}
		}
	}
	return false
}

func completeReference(reference map[string]any) bool {
	if len(reference) == 0 {
		return false
	}
	for key, value := range reference {
		if incompleteReferenceMarker(key) || !completeReferenceValue(value) {
			return false
		}
	}
	return true
}

func completeReferenceValue(value any) bool {
	switch typed := value.(type) {
	case nil, bool, json.Number:
		return true
	case string:
		return !incompleteReferenceMarker(typed)
	case []any:
		for _, child := range typed {
			if !completeReferenceValue(child) {
				return false
			}
		}
	case map[string]any:
		for key, child := range typed {
			if incompleteReferenceMarker(key) || !completeReferenceValue(child) {
				return false
			}
		}
	default:
		return false
	}
	return true
}

func incompleteReferenceMarker(value string) bool {
	value = strings.TrimSpace(value)
	return strings.EqualFold(value, "[REDACTED]") || strings.EqualFold(value, "[TRUNCATED]") ||
		strings.HasSuffix(strings.ToLower(value), "...[truncated]")
}

func marshalReference(reference map[string]any) string {
	keys := make([]string, 0, len(reference))
	for key := range reference {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(keys))
	for _, key := range keys {
		ordered[key] = reference[key]
	}
	encoded, err := json.Marshal(ordered)
	if err != nil || len(encoded) > maxPlaceholderBytes/2 || len(encoded) == 2 {
		return ""
	}
	return string(encoded)
}

func recoveryInstruction(kind agent.ToolResultRecoveryKind) string {
	switch kind {
	case agent.ToolResultRecoveryRead:
		return "Use read with the retained reference if the exact content is needed again."
	case agent.ToolResultRecoveryRefetch:
		return "Fetch the retained reference again if current content is required."
	case agent.ToolResultRecoveryRerun:
		return "Rerun the retained invocation if exact output is required."
	case agent.ToolResultRecoveryArtifact:
		return "Use read on the readable artifact path if exact output is required."
	default:
		return "Recover the result from the retained reference if exact evidence is required."
	}
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD"))
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "…"
}
