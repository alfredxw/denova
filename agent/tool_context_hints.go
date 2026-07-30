package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	toolResultHintMaxDepth             = 6
	toolResultHintMaxCollectionEntries = 32
	toolResultHintMaxStringBytes       = 1024
	toolResultHintMaxPathBytes         = 4096
	toolResultHintMaxBytes             = 32 * 1024
	toolResultHintRedactedValue        = "[REDACTED]"
	toolResultHintTruncatedValue       = "[TRUNCATED]"
)

func normalizeToolResultContextHints(hints *ToolResultContextHints) (*ToolResultContextHints, error) {
	if hints == nil {
		return nil, nil
	}
	normalized := *hints
	switch normalized.ContextValue {
	case "":
		normalized.ContextValue = ToolResultContextNormal
	case ToolResultContextNormal, ToolResultContextDiscardable:
	default:
		return nil, fmt.Errorf("invalid tool result context value %q", normalized.ContextValue)
	}

	recovery := normalized.Recovery
	switch recovery.Kind {
	case "", ToolResultRecoveryRead, ToolResultRecoveryRefetch, ToolResultRecoveryRerun, ToolResultRecoveryArtifact:
	default:
		return nil, fmt.Errorf("invalid tool result recovery kind %q", recovery.Kind)
	}
	if recovery.EstimatedBytes < 0 || recovery.EstimatedTokens < 0 {
		return nil, errors.New("tool result recovery estimates cannot be negative")
	}
	if recovery.Reference != nil {
		reference, err := normalizeToolResultReference(recovery.Reference)
		if err != nil {
			return nil, err
		}
		recovery.Reference = reference
	}
	recovery.ArtifactPath = strings.TrimSpace(strings.ToValidUTF8(recovery.ArtifactPath, "\uFFFD"))
	if len(recovery.ArtifactPath) > toolResultHintMaxPathBytes || strings.ContainsRune(recovery.ArtifactPath, '\x00') {
		return nil, errors.New("tool result artifact path is invalid")
	}
	if ContainsSensitiveToolContextMaterial(recovery.ArtifactPath) {
		return nil, errors.New("tool result artifact path contains sensitive material")
	}
	if recovery.Kind == "" && (len(recovery.Reference) != 0 || recovery.ArtifactPath != "" ||
		recovery.EstimatedBytes != 0 || recovery.EstimatedTokens != 0) {
		return nil, errors.New("tool result recovery kind is required")
	}
	if recovery.Kind != "" && len(recovery.Reference) == 0 && recovery.ArtifactPath == "" {
		return nil, errors.New("tool result recovery reference is required")
	}
	if recovery.Kind == ToolResultRecoveryArtifact && recovery.ArtifactPath == "" {
		return nil, errors.New("artifact recovery requires a readable path")
	}
	normalized.Recovery = recovery
	normalized.SupersessionKey = normalizeToolResultSupersessionKey(hints.SupersessionKey)

	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode tool result context hints: %w", err)
	}
	if len(encoded) > toolResultHintMaxBytes {
		return nil, fmt.Errorf("tool result context hints exceed %d bytes", toolResultHintMaxBytes)
	}
	return &normalized, nil
}

func normalizeToolResultReference(reference map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(reference)
	if err != nil {
		return nil, fmt.Errorf("encode tool result recovery reference: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	var generic map[string]any
	if err := decoder.Decode(&generic); err != nil {
		return nil, fmt.Errorf("decode tool result recovery reference: %w", err)
	}
	normalized, err := normalizeToolResultHintMap(generic, 1)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeToolResultHintMap(values map[string]any, depth int) (map[string]any, error) {
	if depth > toolResultHintMaxDepth {
		return map[string]any{"_truncated": toolResultHintTruncatedValue}, nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > toolResultHintMaxCollectionEntries {
		keys = keys[:toolResultHintMaxCollectionEntries]
	}
	normalized := make(map[string]any, len(keys))
	for _, key := range keys {
		safeKey := boundedToolResultHintString(strings.ToValidUTF8(key, "\uFFFD"))
		value, err := normalizeToolResultHintValue(values[key], depth+1, IsSensitiveToolContextKey(key))
		if err != nil {
			return nil, err
		}
		normalized[safeKey] = value
	}
	return normalized, nil
}

func normalizeToolResultHintValue(value any, depth int, redact bool) (any, error) {
	if redact {
		return toolResultHintRedactedValue, nil
	}
	if depth > toolResultHintMaxDepth {
		return toolResultHintTruncatedValue, nil
	}
	switch typed := value.(type) {
	case nil, bool, json.Number:
		return typed, nil
	case string:
		if ContainsSensitiveToolContextMaterial(typed) {
			return toolResultHintRedactedValue, nil
		}
		return boundedToolResultHintString(strings.ToValidUTF8(typed, "\uFFFD")), nil
	case map[string]any:
		return normalizeToolResultHintMap(typed, depth)
	case []any:
		limit := min(len(typed), toolResultHintMaxCollectionEntries)
		result := make([]any, 0, limit)
		for index := 0; index < limit; index++ {
			normalized, err := normalizeToolResultHintValue(typed[index], depth+1, false)
			if err != nil {
				return nil, err
			}
			result = append(result, normalized)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported tool result recovery value %T", value)
	}
}

func boundedToolResultHintString(value string) string {
	if len(value) <= toolResultHintMaxStringBytes {
		return value
	}
	const marker = "...[truncated]"
	end := toolResultHintMaxStringBytes - len(marker)
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + marker
}

func normalizeToolResultSupersessionKey(value string) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD"))
	if value == "" {
		return ""
	}
	if containsSensitiveHintIdentifier(value) {
		digest := sha256.Sum256([]byte(value))
		return "redacted:" + hex.EncodeToString(digest[:8])
	}
	return boundedToolResultHintString(value)
}

func containsSensitiveHintIdentifier(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{"authorization", "password", "secret", "cookie", "api_key", "apikey", "token", "bearer"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// IsSensitiveToolContextKey is the shared fail-closed classifier for keys that
// may be copied into a model-visible receipt, recovery hint, or checkpoint.
// It deliberately ignores punctuation so snake_case, kebab-case, headers,
// and environment-style names follow the same policy.
func IsSensitiveToolContextKey(key string) bool {
	normalized := normalizeSensitiveToolContextIdentifier(key)
	for _, marker := range []string{
		"authorization", "authentication", "password", "passwd", "secret",
		"token", "apikey", "cookie", "credential", "privatekey", "accesskey",
		"sessionkey", "signingkey", "bearer",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	// Header maps commonly use names such as X-Custom-Auth or Proxy-Auth.
	// Matching only the suffix avoids treating ordinary fields such as author
	// as credentials.
	return normalized == "auth" || strings.HasSuffix(normalized, "auth")
}

// ContainsSensitiveToolContextMaterial catches credential-shaped scalar
// values even when they sit below an innocuous custom header key.
func ContainsSensitiveToolContextMaterial(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{
		"authorization=", "authorization:", "password=", "password:",
		"passwd=", "passwd:", "secret=", "secret:", "cookie=", "cookie:",
		"token=", "token:", "api_key=", "api_key:", "apikey=", "apikey:",
		"credential=", "credential:", "private_key=", "private_key:",
		"access_key=", "access_key:", "session_key=", "session_key:",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	fields := strings.Fields(normalized)
	for index := 0; index+1 < len(fields); index++ {
		if strings.Trim(fields[index], " :\t\r\n") == "bearer" && strings.TrimSpace(fields[index+1]) != "" {
			return true
		}
	}
	return false
}

func normalizeSensitiveToolContextIdentifier(value string) string {
	return strings.Map(func(character rune) rune {
		switch {
		case character >= 'A' && character <= 'Z':
			return character + ('a' - 'A')
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			return character
		default:
			return -1
		}
	}, strings.TrimSpace(value))
}

func validateToolArtifactPersistence(persistence ToolArtifactPersistence) error {
	if !persistence.Attempted {
		return errors.New("tool artifact persistence must be marked attempted")
	}
	if persistence.Complete {
		if persistence.FailureReason != "" {
			return errors.New("complete tool artifact persistence cannot have a failure reason")
		}
		return nil
	}
	switch persistence.FailureReason {
	case ToolArtifactFailureStoreUnavailable, ToolArtifactFailureBegin,
		ToolArtifactFailureWrite, ToolArtifactFailureCommit:
		return nil
	default:
		return fmt.Errorf("invalid tool artifact persistence failure %q", persistence.FailureReason)
	}
}

func estimateToolResultTokens(byteCount int64) int {
	if byteCount <= 0 {
		return 0
	}
	estimate := byteCount / 4
	if byteCount%4 != 0 {
		estimate++
	}
	maxInt := int64(^uint(0) >> 1)
	if estimate > maxInt {
		return int(maxInt)
	}
	return int(estimate)
}
