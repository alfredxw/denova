// Package toolresult provides the reusable lossless result processor for
// public Agent Definitions. Product hosts supply storage; this package owns
// result bounds, artifact materialization, recovery hints, and safe receipts.
package toolresult

import (
	"context"
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
	DefaultMaxBytes                = 128 * 1024
	ProtectedArgumentsMaxBytes     = 4 * 1024
	ProtectedOutcomeMaxBytes       = 8 * 1024
	standardReceiptSchema          = "agent.tool_result.receipt.v1"
	toolResultArtifactContentType  = "text/plain; charset=utf-8"
	redactedValue                  = "[redacted from retained tool context]"
	eagerToolResultRetentionNotice = "[Context retention notice]\nThis is a very large, recoverable result and may be replaced by a compact receipt before the next user turn. Preserve only conclusions needed later; do not copy the entire result."
)

// Policy is semantic processor configuration and therefore participates in
// Definition restore identity. Zero MaxBytes selects DefaultMaxBytes.
type Policy struct {
	MaxBytes            int
	EagerMinTokens      int
	ContextWindowTokens int
}

type standardProcessor struct {
	policy   Policy
	identity agent.CapabilityIdentity
}

// Standard returns the built-in lossless result processor. It is suitable for
// standalone coding Agents as well as product-composed Definitions.
func Standard(policy Policy) agent.ToolResultProcessor {
	policy.MaxBytes = normalizeLimit(policy.MaxBytes)
	policy.EagerMinTokens = max(0, policy.EagerMinTokens)
	policy.ContextWindowTokens = max(0, policy.ContextWindowTokens)
	encoded, _ := json.Marshal(policy)
	hash := sha256.Sum256(encoded)
	return &standardProcessor{
		policy: policy,
		identity: agent.CapabilityIdentity{
			Kind: "tool_result_processor.standard", Version: 1,
			ConfigHash: hex.EncodeToString(hash[:]),
		},
	}
}

func (processor *standardProcessor) Identity() agent.CapabilityIdentity {
	if processor == nil {
		return agent.CapabilityIdentity{}
	}
	return processor.identity
}

func (processor *standardProcessor) Process(
	ctx context.Context,
	request agent.ToolResultProcessRequest,
) (agent.ToolResult, error) {
	if processor == nil {
		return request.Result, fmt.Errorf("standard ToolResultProcessor is nil")
	}
	descriptor := request.Definition.Descriptor
	limit := descriptor.MaxResultBytes
	if limit <= 0 || limit > processor.policy.MaxBytes {
		limit = processor.policy.MaxBytes
	}
	descriptor.MaxResultBytes = limit
	result := request.Result
	result.ModelContent = strings.ToValidUTF8(result.ModelContent, "\uFFFD")
	result.DisplayContent = strings.ToValidUTF8(result.DisplayContent, "\uFFFD")
	result.Artifacts = verifiedArtifacts(ctx, request, result.Artifacts)
	visibleBytes := len(result.ModelContent)
	originalBytes := max(visibleBytes, result.Metadata.OriginalModelBytes)
	result.Metadata.OriginalModelBytes = originalBytes

	artifact := recoverableArtifact(result.Artifacts)
	upstreamLoss := result.Metadata.ModelTruncated && artifact == nil
	if visibleBytes > limit && artifact == nil && !upstreamLoss {
		var failure string
		artifact, failure = materialize(ctx, request, result.ModelContent)
		if artifact != nil {
			result.Artifacts = appendArtifact(result.Artifacts, *artifact)
			result.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{Attempted: true, Complete: true}
		} else {
			result.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{
				Attempted: true, Complete: false, FailureReason: failure,
			}
			result.ModelContent = headTail(result.ModelContent, limit, "complete output unavailable; failure="+failure)
			result.Metadata.ModelTruncated = true
		}
	}
	if artifact != nil {
		result.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{Attempted: true, Complete: true}
		applyArtifactRecovery(&result, *artifact, originalBytes, descriptor.ResultRetention)
		if originalBytes > limit {
			result.ModelContent = headTail(result.ModelContent, limit, "complete=true; artifact="+artifact.ReadablePath)
			result.Metadata.ModelTruncated = true
		}
	} else {
		applyReplayRecovery(&result, request, originalBytes)
	}
	applyEagerNotice(&result, descriptor, originalBytes, limit, processor.policy)
	applyProtectedReceipt(&result, request, descriptor, limit)
	normalized, err := agent.NormalizeToolResult(result, descriptor)
	if err != nil {
		return result, fmt.Errorf("normalize processed tool result: %w", err)
	}
	if artifact == nil && normalized.Metadata.ModelTruncated && requiresLossless(descriptor, normalized) {
		failure := agent.ToolArtifactFailureStoreUnavailable
		if persistence := normalized.Metadata.ArtifactPersistence; persistence != nil && persistence.FailureReason != "" {
			failure = persistence.FailureReason
		} else {
			normalized.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{
				Attempted: true, Complete: false, FailureReason: failure,
			}
		}
		normalized.Status = agent.ToolResultError
		normalized.SyntheticReason = ""
		applyProtectedReceipt(&normalized, request, descriptor, limit)
		if failed, normalizeErr := agent.NormalizeToolResult(normalized, descriptor); normalizeErr == nil {
			normalized = failed
		}
		return normalized, agent.MarkToolControlError(fmt.Errorf("persist complete protected tool result: %s", failure))
	}
	return normalized, nil
}

func verifiedArtifacts(ctx context.Context, request agent.ToolResultProcessRequest, artifacts []agent.ToolArtifactRef) []agent.ToolArtifactRef {
	if len(artifacts) == 0 {
		return nil
	}
	verifier := agent.ToolArtifactVerifierFromContext(ctx)
	callID := effectiveCallID(request)
	result := make([]agent.ToolArtifactRef, len(artifacts))
	for index, artifact := range artifacts {
		artifact = canonicalArtifact(artifact)
		if recoverablePurpose(artifact.Purpose) {
			expected := agent.ToolArtifactRequest{ToolName: request.ToolName, ToolCallID: callID, Purpose: artifact.Purpose}
			if verifier == nil || verifier.VerifyToolArtifact(ctx, artifact, expected) != nil {
				artifact.Purpose = agent.ToolArtifactPurposeAttachment
			}
		}
		result[index] = artifact
	}
	return result
}

func materialize(ctx context.Context, request agent.ToolResultProcessRequest, content string) (*agent.ToolArtifactRef, string) {
	store := agent.ToolArtifactStoreFromContext(ctx)
	if store == nil {
		return nil, agent.ToolArtifactFailureStoreUnavailable
	}
	writer, err := store.BeginToolArtifact(ctx, agent.ToolArtifactRequest{
		ToolName: request.ToolName, ToolCallID: effectiveCallID(request),
		Purpose:  agent.ToolArtifactPurposeCompleteModelOutput,
		MIMEType: toolResultArtifactContentType, Extension: "log",
		Description: "Complete model-visible output from one tool call",
	})
	if err != nil {
		return nil, agent.ToolArtifactFailureBegin
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		_ = writer.Abort()
		return nil, agent.ToolArtifactFailureWrite
	}
	reference, err := writer.Commit()
	if err != nil {
		_ = writer.Abort()
		return nil, agent.ToolArtifactFailureCommit
	}
	reference = canonicalArtifact(reference)
	if reference.Purpose != agent.ToolArtifactPurposeCompleteModelOutput || !reference.Complete ||
		reference.ID == "" || reference.ReadablePath == "" || reference.ContentType == "" {
		return nil, agent.ToolArtifactFailureCommit
	}
	return &reference, ""
}

func effectiveCallID(request agent.ToolResultProcessRequest) string {
	if value := strings.TrimSpace(request.ExecutionID); value != "" {
		return value
	}
	return strings.TrimSpace(request.ProviderCallID)
}

func canonicalArtifact(artifact agent.ToolArtifactRef) agent.ToolArtifactRef {
	artifact.ID = strings.TrimSpace(artifact.ID)
	artifact.Purpose = agent.ToolArtifactPurpose(strings.TrimSpace(string(artifact.Purpose)))
	artifact.ReadablePath = strings.TrimSpace(strings.ToValidUTF8(artifact.ReadablePath, "\uFFFD"))
	artifact.ContentType = strings.TrimSpace(artifact.ContentType)
	if artifact.EstimatedTokens == 0 && artifact.EstimatedBytes > 0 {
		artifact.EstimatedTokens = estimatedTokens(artifact.EstimatedBytes)
	}
	return artifact
}

func recoverablePurpose(purpose agent.ToolArtifactPurpose) bool {
	return purpose == agent.ToolArtifactPurposeCompleteModelOutput || purpose == agent.ToolArtifactPurposeCompleteToolOutput
}

func recoverableArtifact(artifacts []agent.ToolArtifactRef) *agent.ToolArtifactRef {
	for _, artifact := range artifacts {
		artifact = canonicalArtifact(artifact)
		if artifact.Complete && artifact.ReadablePath != "" && artifact.ContentType != "" && recoverablePurpose(artifact.Purpose) {
			return &artifact
		}
	}
	return nil
}

func appendArtifact(artifacts []agent.ToolArtifactRef, artifact agent.ToolArtifactRef) []agent.ToolArtifactRef {
	for index := range artifacts {
		if artifacts[index].ID == artifact.ID || canonicalArtifact(artifacts[index]).ReadablePath == artifact.ReadablePath {
			artifacts[index] = artifact
			return artifacts
		}
	}
	return append(artifacts, artifact)
}

func applyArtifactRecovery(result *agent.ToolResult, artifact agent.ToolArtifactRef, originalBytes int, retention agent.ToolResultRetentionMode) {
	if result.ContextHints == nil {
		result.ContextHints = &agent.ToolResultContextHints{}
	}
	result.ContextHints.Recovery = agent.ToolResultRecoveryHint{
		Kind: agent.ToolResultRecoveryArtifact, ArtifactPath: artifact.ReadablePath,
		EstimatedBytes:  max(artifact.EstimatedBytes, int64(originalBytes)),
		EstimatedTokens: max(artifact.EstimatedTokens, estimatedTokens(int64(originalBytes))),
	}
	if result.ContextHints.ContextValue == "" {
		result.ContextHints.ContextValue = agent.ToolResultContextNormal
		if retention == agent.ToolResultEagerCandidate {
			result.ContextHints.ContextValue = agent.ToolResultContextDiscardable
		}
	}
}

func applyReplayRecovery(result *agent.ToolResult, request agent.ToolResultProcessRequest, originalBytes int) {
	if result.Status != agent.ToolResultSuccess {
		return
	}
	if result.ContextHints == nil {
		result.ContextHints = &agent.ToolResultContextHints{}
	}
	hints := result.ContextHints
	if hints.Recovery.Kind == "" && request.Definition.Descriptor.ResultRecoveryKind != "" {
		if reference := boundedArguments(request.Arguments); len(reference) > 0 {
			hints.Recovery = agent.ToolResultRecoveryHint{
				Kind: request.Definition.Descriptor.ResultRecoveryKind, Reference: reference,
				EstimatedBytes: int64(originalBytes), EstimatedTokens: estimatedTokens(int64(originalBytes)),
			}
		}
	}
	if hints.ContextValue == "" {
		hints.ContextValue = agent.ToolResultContextNormal
		if request.Definition.Descriptor.ResultRetention == agent.ToolResultEagerCandidate {
			hints.ContextValue = agent.ToolResultContextDiscardable
		}
	}
	if hints.SupersessionKey == "" && hints.Recovery.Kind != "" {
		hints.SupersessionKey = idempotencyKey(request.ToolName, request.Arguments)
	}
	if hints.Recovery.Kind == "" && hints.SupersessionKey == "" && hints.ContextValue == agent.ToolResultContextNormal {
		result.ContextHints = nil
	}
}

func applyEagerNotice(result *agent.ToolResult, descriptor agent.ToolDescriptor, originalBytes, limit int, policy Policy) {
	minimum := max(policy.EagerMinTokens, policy.ContextWindowTokens*15/100)
	if descriptor.ResultRetention != agent.ToolResultEagerCandidate || result.Status != agent.ToolResultSuccess ||
		result.SyntheticReason != "" || result.ContextHints == nil || result.ContextHints.Recovery.Kind == "" ||
		estimatedTokens(int64(originalBytes)) < minimum {
		return
	}
	notice := "\n\n" + eagerToolResultRetentionNotice
	if len(notice) >= limit {
		result.ModelContent = utf8Suffix(notice, limit)
		result.Metadata.ModelTruncated = true
		return
	}
	contentLimit := limit - len(notice)
	if len(result.ModelContent) > contentLimit {
		result.ModelContent = headTail(result.ModelContent, contentLimit, "space reserved for context retention notice")
		result.Metadata.ModelTruncated = true
	}
	result.ModelContent += notice
}

func applyProtectedReceipt(result *agent.ToolResult, request agent.ToolResultProcessRequest, descriptor agent.ToolDescriptor, limit int) {
	protected := descriptor.ResultRetention == agent.ToolResultProtected || result.Status != agent.ToolResultSuccess ||
		result.SyntheticReason != "" || descriptor.MutationScope != agent.ToolMutationNone ||
		result.Metadata.ArtifactPersistence != nil || len(result.Artifacts) > 0
	if !protected {
		result.ProtectedReceipt = nil
		return
	}
	arguments := sanitizedArguments(request.Arguments, min(ProtectedArgumentsMaxBytes, limit))
	outcome := protectedOutcome(request, *result, min(ProtectedOutcomeMaxBytes, limit))
	if arguments == "" && outcome == "" {
		result.ProtectedReceipt = nil
		return
	}
	result.ProtectedReceipt = &agent.ToolResultProtectedReceipt{SanitizedArguments: arguments, Outcome: outcome}
}

func protectedOutcome(request agent.ToolResultProcessRequest, result agent.ToolResult, limit int) string {
	type artifactReceipt struct {
		Purpose     agent.ToolArtifactPurpose `json:"purpose,omitempty"`
		Path        string                    `json:"path"`
		ContentType string                    `json:"content_type,omitempty"`
		Bytes       int64                     `json:"bytes,omitempty"`
	}
	artifacts := make([]artifactReceipt, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		artifact = canonicalArtifact(artifact)
		if !artifact.Complete || artifact.ReadablePath == "" || agent.ContainsSensitiveToolContextMaterial(artifact.ReadablePath) {
			continue
		}
		artifacts = append(artifacts, artifactReceipt{
			Purpose: artifact.Purpose, Path: artifact.ReadablePath, ContentType: artifact.ContentType, Bytes: artifact.EstimatedBytes,
		})
	}
	receipt := struct {
		Schema        string                    `json:"schema"`
		Tool          string                    `json:"tool"`
		Status        agent.ToolResultStatus    `json:"status"`
		Synthetic     agent.ToolSyntheticReason `json:"synthetic_reason,omitempty"`
		Source        agent.ToolSource          `json:"source"`
		Mutation      agent.ToolMutationScope   `json:"mutation_scope"`
		Recovery      agent.ToolRecoveryClass   `json:"recovery"`
		Target        string                    `json:"target,omitempty"`
		OriginalBytes int                       `json:"original_bytes,omitempty"`
		Truncated     bool                      `json:"truncated,omitempty"`
		Artifacts     []artifactReceipt         `json:"artifacts,omitempty"`
		Note          string                    `json:"note"`
	}{
		Schema: standardReceiptSchema, Tool: strings.TrimSpace(request.ToolName), Status: result.Status,
		Synthetic: result.SyntheticReason, Source: request.Definition.Descriptor.Source,
		Mutation: request.Definition.Descriptor.MutationScope, Recovery: request.Definition.Descriptor.Recovery,
		Target: safeTarget(request.Arguments), OriginalBytes: result.Metadata.OriginalModelBytes,
		Truncated: result.Metadata.ModelTruncated, Artifacts: artifacts,
		Note: "The rich result existed in the source turn; use the recovery reference or repeat the call if exact evidence is needed.",
	}
	encoded, err := json.Marshal(receipt)
	if err == nil && len(encoded) <= limit {
		return string(encoded)
	}
	fallback, _ := json.Marshal(map[string]any{
		"schema": standardReceiptSchema, "tool": receipt.Tool, "status": receipt.Status,
		"original_bytes": receipt.OriginalBytes,
	})
	if len(fallback) <= limit {
		return string(fallback)
	}
	return ""
}

func sanitizedArguments(arguments string, limit int) string {
	var value any
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(strings.ToValidUTF8(arguments, "\uFFFD"))))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	encoded, err := json.Marshal(sanitizeValue(value, 0))
	if err == nil && len(encoded) <= limit {
		return string(encoded)
	}
	fallback, _ := json.Marshal(map[string]any{
		"schema": "agent.tool_call.arguments_omitted.v1", "original_bytes": len(arguments),
	})
	if len(fallback) <= limit {
		return string(fallback)
	}
	return ""
}

func sanitizeValue(value any, depth int) any {
	if depth >= 10 {
		return "[nested value omitted]"
	}
	switch typed := value.(type) {
	case string:
		if agent.ContainsSensitiveToolContextMaterial(typed) {
			return redactedValue
		}
		if len(typed) > 4096 {
			return fmt.Sprintf("[string omitted: %d bytes]", len(typed))
		}
		return typed
	case []any:
		limit := min(len(typed), 64)
		result := make([]any, 0, limit+1)
		for _, item := range typed[:limit] {
			result = append(result, sanitizeValue(item, depth+1))
		}
		if limit < len(typed) {
			result = append(result, map[string]any{"_omitted_items": len(typed) - limit})
		}
		return result
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 64 {
			keys = keys[:64]
		}
		result := make(map[string]any, len(keys))
		for _, key := range keys {
			if agent.IsSensitiveToolContextKey(key) {
				result[key] = redactedValue
			} else {
				result[key] = sanitizeValue(typed[key], depth+1)
			}
		}
		return result
	default:
		return value
	}
}

func requiresLossless(descriptor agent.ToolDescriptor, result agent.ToolResult) bool {
	return descriptor.ResultRetention == agent.ToolResultProtected || result.Status != agent.ToolResultSuccess ||
		result.SyntheticReason == agent.ToolSyntheticEffectUnknown || descriptor.MutationScope != agent.ToolMutationNone ||
		descriptor.Recovery == agent.ToolRecoveryNonIdempotent
}

func boundedArguments(arguments string) map[string]any {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" || len(arguments) > 32*1024 {
		return nil
	}
	var result map[string]any
	if json.Unmarshal([]byte(arguments), &result) != nil {
		return nil
	}
	return result
}

func safeTarget(arguments string) string {
	values := boundedArguments(arguments)
	for _, key := range []string{"path", "file_path", "filename", "file", "pattern"} {
		value, _ := values[key].(string)
		value = strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD"))
		if value == "" {
			continue
		}
		if agent.ContainsSensitiveToolContextMaterial(value) {
			return redactedValue
		}
		if len(value) > 4096 {
			return utf8Prefix(value, 4080) + "...[truncated]"
		}
		return value
	}
	return ""
}

func idempotencyKey(toolName, arguments string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(arguments)))
	return strings.ToLower(strings.TrimSpace(toolName)) + ":" + hex.EncodeToString(hash[:8])
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultMaxBytes
	}
	return limit
}

func estimatedTokens(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	return int((bytes + 3) / 4)
}

func headTail(content string, limit int, status string) string {
	if limit <= 0 || len(content) <= limit {
		return content
	}
	note := fmt.Sprintf("\n[tool result truncated]\n[tool result preview: original_bytes=%d; %s]", len(content), strings.TrimSpace(status))
	if len(note) >= limit {
		return utf8Prefix(note, limit)
	}
	separator := "\n...[middle omitted]...\n"
	available := limit - len(note) - len(separator)
	if available <= 0 {
		return utf8Prefix(note, limit)
	}
	head := utf8Prefix(content, available/2)
	tail := utf8Suffix(content, available-len(head))
	return head + separator + tail + note
}

func utf8Prefix(value string, limit int) string {
	if limit >= len(value) {
		return value
	}
	limit = max(0, limit)
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

func utf8Suffix(value string, limit int) string {
	if limit >= len(value) {
		return value
	}
	start := len(value) - max(0, limit)
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	return value[start:]
}

var _ agent.ToolResultProcessor = (*standardProcessor)(nil)
