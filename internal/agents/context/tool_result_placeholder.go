package context

import (
	"denova/internal/agents/toolresult"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const (
	ToolResultPlaceholderRendererVersion = "tool_result.placeholder.v1"
	maxToolResultPlaceholderBytes        = 8 * 1024
)

// renderedToolResultPlaceholder is persisted verbatim in cleanup records.
// Rendering happens once during planning so replay does not depend on future
// wording or implementation changes.
type renderedToolResultPlaceholder struct {
	Content string
	Version string
}

func renderToolResultPlaceholder(call agent.ToolCall, message *agent.Message, supersededBy string) (renderedToolResultPlaceholder, bool) {
	if message == nil || message.Role != agent.ToolRole || message.ToolResult == nil || message.ToolResult.ContextHints == nil {
		return renderedToolResultPlaceholder{}, false
	}
	hints := message.ToolResult.ContextHints
	recovery := hints.Recovery
	if recovery.Kind == "" || !usableToolResultRecoveryForCleanup(recovery, message.ToolResult.Artifacts) {
		return renderedToolResultPlaceholder{}, false
	}

	lines := []string{
		"[Older tool result removed to save context.",
		"Tool: " + boundedPlaceholderText(call.Function.Name, 256),
		"Status: " + boundedPlaceholderText(string(message.ToolResult.Status), 64),
		"Recovery: " + boundedPlaceholderText(string(recovery.Kind), 64),
	}
	if reference := marshalPlaceholderReference(recovery.Reference); reference != "" {
		lines = append(lines, "Reference: "+reference)
	}
	if artifact := strings.TrimSpace(recovery.ArtifactPath); artifact != "" {
		lines = append(lines, "Readable artifact: "+boundedPlaceholderText(artifact, 2048))
	}
	if recovery.EstimatedBytes > 0 {
		lines = append(lines, fmt.Sprintf("Estimated original bytes: %d", recovery.EstimatedBytes))
	}
	if recovery.EstimatedTokens > 0 {
		lines = append(lines, fmt.Sprintf("Estimated original tokens: %d", recovery.EstimatedTokens))
	}
	if supersededBy = strings.TrimSpace(supersededBy); supersededBy != "" {
		lines = append(lines, "Superseded by tool call: "+boundedPlaceholderText(supersededBy, 256))
	}
	lines = append(lines, recoveryInstruction(recovery.Kind)+"]")
	content := strings.Join(lines, "\n")
	if len(content) > maxToolResultPlaceholderBytes {
		return renderedToolResultPlaceholder{}, false
	}
	return renderedToolResultPlaceholder{Content: content, Version: ToolResultPlaceholderRendererVersion}, true
}

// usableToolResultRecovery rejects hints whose only identity was redacted or
// truncated. Such metadata is safe to persist but cannot justify deleting the
// rich result because a later Agent could not actually recover it.
func usableToolResultRecovery(recovery agent.ToolResultRecoveryHint) bool {
	switch recovery.Kind {
	case agent.ToolResultRecoveryRead:
		return recoveryReferenceHasNamedIdentity(recovery.Reference, readRecoveryIdentityKeys)
	case agent.ToolResultRecoveryRefetch:
		return recoveryReferenceHasHTTPURL(recovery.Reference) ||
			recoveryReferenceHasNamedIdentity(recovery.Reference, refetchQueryIdentityKeys) &&
				recoveryReferenceHasNamedIdentity(recovery.Reference, refetchScopeIdentityKeys)
	case agent.ToolResultRecoveryRerun:
		return recoveryReferenceHasNamedIdentity(recovery.Reference, rerunRecoveryIdentityKeys)
	case agent.ToolResultRecoveryArtifact:
		return recoveryIdentityValue(recovery.ArtifactPath)
	default:
		return false
	}
}

func usableToolResultRecoveryForCleanup(recovery agent.ToolResultRecoveryHint, artifacts []agent.ToolArtifactRef) bool {
	if !usableToolResultRecovery(recovery) {
		return false
	}
	if recovery.Kind != agent.ToolResultRecoveryArtifact {
		return true
	}
	want := strings.TrimSpace(recovery.ArtifactPath)
	for _, candidate := range artifacts {
		candidate = toolresult.CanonicalArtifact(candidate)
		if candidate.ReadablePath != want || !candidate.Complete || candidate.ContentType == "" {
			continue
		}
		if toolresult.RecoverableArtifactPurpose(candidate.Purpose) {
			return true
		}
	}
	return false
}

var readRecoveryIdentityKeys = map[string]struct{}{
	"path": {}, "paths": {}, "file": {}, "file_path": {}, "uri": {},
	"ref": {}, "reference": {}, "resource": {}, "resource_id": {},
}

var refetchQueryIdentityKeys = map[string]struct{}{"query": {}, "q": {}}
var refetchScopeIdentityKeys = map[string]struct{}{"scope": {}, "domain": {}, "site": {}, "provider": {}}

var rerunRecoveryIdentityKeys = map[string]struct{}{
	"action": {}, "operation": {}, "query": {}, "q": {}, "pattern": {},
	"keyword": {}, "keywords": {}, "path": {}, "paths": {}, "url": {},
	"name": {}, "names": {}, "id": {}, "ids": {}, "ref": {}, "reference": {},
	"resource": {}, "resource_id": {}, "target": {}, "skill": {}, "scope": {},
	"story_id": {}, "branch_id": {}, "before_turn_id": {}, "types": {}, "load_modes": {},
}

func recoveryReferenceHasNamedIdentity(reference map[string]any, allowed map[string]struct{}) bool {
	for key, value := range reference {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if _, ok := allowed[normalizedKey]; ok && recoveryNamedIdentityValue(value, allowed) {
			return true
		}
		if nested, ok := value.(map[string]any); ok && recoveryReferenceHasNamedIdentity(nested, allowed) {
			return true
		}
		if values, ok := value.([]any); ok {
			for _, child := range values {
				if nested, ok := child.(map[string]any); ok && recoveryReferenceHasNamedIdentity(nested, allowed) {
					return true
				}
			}
		}
	}
	return false
}

func recoveryNamedIdentityValue(value any, allowed map[string]struct{}) bool {
	switch typed := value.(type) {
	case map[string]any:
		return recoveryReferenceHasNamedIdentity(typed, allowed)
	case []any:
		for _, child := range typed {
			if recoveryNamedIdentityValue(child, allowed) {
				return true
			}
		}
		return false
	default:
		return recoveryIdentityValue(typed)
	}
}

func recoveryIdentityValue(value any) bool {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		return text != "" && text != "[REDACTED]" && text != "[TRUNCATED]" && !strings.HasSuffix(text, "...[truncated]")
	case map[string]any:
		for _, child := range typed {
			if recoveryIdentityValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if recoveryIdentityValue(child) {
				return true
			}
		}
	}
	return false
}

func recoveryReferenceHasHTTPURL(reference map[string]any) bool {
	for key, value := range reference {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey == "url" || normalizedKey == "uri" {
			text, ok := value.(string)
			if ok && usableRecoveryHTTPURL(text) {
				return true
			}
		}
		if nested, ok := value.(map[string]any); ok && recoveryReferenceHasHTTPURL(nested) {
			return true
		}
		if values, ok := value.([]any); ok {
			for _, child := range values {
				if nested, ok := child.(map[string]any); ok && recoveryReferenceHasHTTPURL(nested) {
					return true
				}
			}
		}
	}
	return false
}

func usableRecoveryHTTPURL(value string) bool {
	value = strings.TrimSpace(value)
	if !recoveryIdentityValue(value) {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return parsed.User == nil && parsed.Host != "" && (scheme == "http" || scheme == "https")
}

func marshalPlaceholderReference(reference map[string]any) string {
	if len(reference) == 0 {
		return ""
	}
	// encoding/json sorts map keys. Copying the top level makes that stable
	// guarantee explicit and prevents a caller from mutating during rendering.
	keys := make([]string, 0, len(reference))
	for key := range reference {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	copy := make(map[string]any, len(keys))
	for _, key := range keys {
		copy[key] = reference[key]
	}
	encoded, err := json.Marshal(copy)
	if err != nil || len(encoded) > maxToolResultPlaceholderBytes/2 {
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
		return "Rerun the retained, redacted invocation if exact output is required."
	case agent.ToolResultRecoveryArtifact:
		return "Use read on the readable artifact path if exact output is required."
	default:
		return "Recover the result from the retained reference if exact evidence is required."
	}
}

func boundedPlaceholderText(value string, limit int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD"))
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "…"
}
