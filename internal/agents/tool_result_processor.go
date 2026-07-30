package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

const (
	toolResultArtifactContentType  = "text/plain; charset=utf-8"
	eagerToolResultRetentionNotice = "[Context retention notice]\nThis is a very large, recoverable result and may be replaced by a compact receipt before the next user turn. Before completing the current run, preserve any conclusions needed later in the assistant context, domain state, or workspace state. Do not copy the entire result."
)

type toolResultProcessingPolicy struct {
	MaxBytes            int
	EagerMinTokens      int
	ContextWindowTokens int
}

type toolArtifactReferenceVerifier interface {
	VerifyToolArtifact(context.Context, agent.ToolArtifactRef, agent.ToolArtifactRequest) error
}

// processToolResult is the single post-execution seam for rich result
// normalization and complete-output materialization. Streaming tools may
// publish through the same store while they run; this processor recognizes
// that complete reference instead of writing a duplicate.
func processToolResult(
	ctx context.Context,
	decision ToolDecision,
	args string,
	result agent.ToolResult,
	policy toolResultProcessingPolicy,
) (agent.ToolResult, error) {
	limit := normalizeToolResultLimitBytes(firstPositive(policy.MaxBytes, decision.Descriptor.MaxResultBytes))
	result.ModelContent = strings.ToValidUTF8(result.ModelContent, "\uFFFD")
	result.Artifacts = normalizeUpstreamToolArtifacts(ctx, decision, result.Artifacts)
	visibleBytes := len(result.ModelContent)
	originalBytes := max(visibleBytes, result.Metadata.OriginalModelBytes)
	result.Metadata.OriginalModelBytes = max(result.Metadata.OriginalModelBytes, originalBytes)

	artifact := recoverableToolResultArtifact(result.Artifacts)
	// A streaming endpoint may already have discarded bytes before returning its
	// bounded preview. Never persist that preview as though it were the complete
	// output; only a complete upstream artifact can make the result lossless.
	upstreamLoss := result.Metadata.ModelTruncated && artifact == nil
	if visibleBytes > limit && artifact == nil && !upstreamLoss {
		var failure string
		artifact, failure = materializeToolResult(ctx, decision, result.ModelContent)
		if artifact != nil {
			result.Artifacts = appendToolResultArtifact(result.Artifacts, *artifact)
			result.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{Attempted: true, Complete: true}
		} else {
			result.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{
				Attempted: true, Complete: false, FailureReason: failure,
			}
			result.ModelContent = toolResultHeadTailPreview(result.ModelContent, limit,
				fmt.Sprintf("complete output unavailable; failure=%s", failure))
			result.Metadata.ModelTruncated = true
			if toolResultRequiresLosslessMaterialization(decision.Descriptor, result) {
				normalized, normalizeErr := normalizeProcessedToolResult(decision, args, result, limit)
				if normalizeErr != nil {
					return result, normalizeErr
				}
				return normalized, agent.MarkToolControlError(
					fmt.Errorf("persist complete protected tool result: %s", failure),
				)
			}
		}
	}
	if artifact != nil {
		result.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{Attempted: true, Complete: true}
		applyArtifactRecoveryHint(&result, *artifact, originalBytes, decision.Descriptor.EffectiveResultRetention())
		if originalBytes > limit {
			result.ModelContent = toolResultHeadTailPreview(result.ModelContent, limit,
				fmt.Sprintf("complete=true; artifact=%s", artifact.ReadablePath))
			result.Metadata.ModelTruncated = true
		}
	} else {
		applyReplayRecoveryHint(&result, decision, args, originalBytes)
	}
	applyEagerToolResultRetentionNotice(&result, decision.Descriptor, originalBytes, limit, policy)
	normalized, err := normalizeProcessedToolResult(decision, args, result, limit)
	if err != nil {
		return result, err
	}
	if artifact == nil && normalized.Metadata.ModelTruncated &&
		toolResultRequiresLosslessMaterialization(decision.Descriptor, normalized) {
		failure := agent.ToolArtifactFailureStoreUnavailable
		if persistence := normalized.Metadata.ArtifactPersistence; persistence != nil &&
			persistence.Attempted && !persistence.Complete && persistence.FailureReason != "" {
			failure = persistence.FailureReason
		} else {
			normalized.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{
				Attempted: true, Complete: false, FailureReason: failure,
			}
		}
		return normalized, agent.MarkToolControlError(fmt.Errorf("persist complete protected tool result: %s", failure))
	}
	return normalized, nil
}

// applyEagerToolResultRetentionNotice asks the existing post-tool model step
// to retain durable conclusions before an eligible result can leave the rich
// context. It never starts another summarizer request and never exceeds the
// ordinary tool-result inline budget.
func applyEagerToolResultRetentionNotice(
	result *agent.ToolResult,
	descriptor agent.ToolDescriptor,
	originalBytes, limit int,
	policy toolResultProcessingPolicy,
) {
	minimumTokens := eagerToolResultMinimumTokens(policy.EagerMinTokens, policy.ContextWindowTokens, 0.15)
	if result == nil || descriptor.EffectiveResultRetention() != agent.ToolResultEagerCandidate ||
		result.Status != agent.ToolResultSuccess || result.SyntheticReason != "" ||
		result.ContextHints == nil || result.ContextHints.Recovery.Kind == "" ||
		estimatedToolResultTokens(int64(originalBytes)) < minimumTokens {
		return
	}

	notice := "\n\n" + eagerToolResultRetentionNotice
	if len(notice) >= limit {
		result.ModelContent = utf8Suffix(notice, limit)
		result.Metadata.ModelTruncated = true
		return
	}
	contentBudget := limit - len(notice)
	if len(result.ModelContent) > contentBudget {
		result.ModelContent = toolResultHeadTailPreview(result.ModelContent, contentBudget, "space reserved for context retention notice")
		result.Metadata.ModelTruncated = true
	}
	result.ModelContent += notice
}

func materializeToolResult(ctx context.Context, decision ToolDecision, content string) (*agent.ToolArtifactRef, string) {
	store := agent.ToolArtifactStoreFromContext(ctx)
	if store == nil {
		return nil, agent.ToolArtifactFailureStoreUnavailable
	}
	callID := strings.TrimSpace(decision.ExecutionID)
	if callID == "" {
		callID = strings.TrimSpace(decision.ProviderCallID)
	}
	writer, err := store.BeginToolArtifact(ctx, agent.ToolArtifactRequest{
		ToolName: decision.ToolName, ToolCallID: callID, Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
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
	reference = canonicalToolResultArtifact(reference)
	if reference.Purpose != agent.ToolArtifactPurposeCompleteModelOutput ||
		!reference.Complete || reference.ReadablePath == "" || reference.ContentType == "" {
		return nil, agent.ToolArtifactFailureCommit
	}
	return &reference, ""
}

func normalizeProcessedToolResult(decision ToolDecision, args string, result agent.ToolResult, limit int) (agent.ToolResult, error) {
	descriptor := decision.Descriptor
	descriptor.MaxResultBytes = limit
	manifest := manifestForDefinition(decision.ToolName, descriptor)
	prepareToolResultProjectionMetadata(manifest, args, &result)
	normalized, err := agent.NormalizeToolResult(result, descriptor)
	if err != nil {
		return result, fmt.Errorf("Invalid structured tool result: %w", err)
	}
	return projectProtectedToolResultReceipt(manifest, args, normalized), nil
}

func toolResultRequiresLosslessMaterialization(descriptor agent.ToolDescriptor, result agent.ToolResult) bool {
	if descriptor.EffectiveResultRetention() == agent.ToolResultProtected || result.Status != agent.ToolResultSuccess ||
		result.SyntheticReason == agent.ToolSyntheticEffectUnknown {
		return true
	}
	return descriptor.MutationScope != agent.ToolMutationNone || descriptor.Recovery == agent.ToolRecoveryNonIdempotent
}

func recoverableToolResultArtifact(artifacts []agent.ToolArtifactRef) *agent.ToolArtifactRef {
	for index := range artifacts {
		artifact := canonicalToolResultArtifact(artifacts[index])
		if !artifact.Complete || artifact.ReadablePath == "" || artifact.ContentType == "" {
			continue
		}
		if recoverableToolArtifactPurpose(artifact.Purpose) {
			return &artifact
		}
	}
	return nil
}

func recoverableToolArtifactPurpose(purpose agent.ToolArtifactPurpose) bool {
	return purpose == agent.ToolArtifactPurposeCompleteModelOutput || purpose == agent.ToolArtifactPurposeCompleteToolOutput
}

// normalizeUpstreamToolArtifacts reserves lossless recovery claims for
// references proven by the run-scoped host store. An extension can still
// return useful attachments, but a forged complete-output label can never
// authorize deletion of the rich result.
func normalizeUpstreamToolArtifacts(
	ctx context.Context,
	decision ToolDecision,
	artifacts []agent.ToolArtifactRef,
) []agent.ToolArtifactRef {
	if len(artifacts) == 0 {
		return nil
	}
	store := agent.ToolArtifactStoreFromContext(ctx)
	verifier, canVerify := store.(toolArtifactReferenceVerifier)
	callID := strings.TrimSpace(decision.ExecutionID)
	if callID == "" {
		callID = strings.TrimSpace(decision.ProviderCallID)
	}
	normalized := make([]agent.ToolArtifactRef, len(artifacts))
	for index, candidate := range artifacts {
		candidate = canonicalToolResultArtifact(candidate)
		if recoverableToolArtifactPurpose(candidate.Purpose) {
			expected := agent.ToolArtifactRequest{ToolName: decision.ToolName, ToolCallID: callID, Purpose: candidate.Purpose}
			if !canVerify || verifier.VerifyToolArtifact(ctx, candidate, expected) != nil {
				candidate.Purpose = agent.ToolArtifactPurposeAttachment
			}
		}
		normalized[index] = candidate
	}
	return normalized
}

func canonicalToolResultArtifact(artifact agent.ToolArtifactRef) agent.ToolArtifactRef {
	artifact.Purpose = agent.ToolArtifactPurpose(strings.TrimSpace(string(artifact.Purpose)))
	artifact.ReadablePath = strings.TrimSpace(strings.ToValidUTF8(artifact.ReadablePath, "\uFFFD"))
	if artifact.ReadablePath == "" {
		artifact.ReadablePath = strings.TrimSpace(strings.ToValidUTF8(artifact.URI, "\uFFFD"))
	}
	artifact.URI = artifact.ReadablePath
	artifact.ContentType = strings.TrimSpace(artifact.ContentType)
	if artifact.ContentType == "" {
		artifact.ContentType = strings.TrimSpace(artifact.MIMEType)
	}
	artifact.MIMEType = artifact.ContentType
	if artifact.EstimatedBytes == 0 && artifact.ByteSize > 0 {
		artifact.EstimatedBytes = artifact.ByteSize
	}
	artifact.ByteSize = artifact.EstimatedBytes
	if artifact.EstimatedTokens == 0 && artifact.EstimatedBytes > 0 {
		artifact.EstimatedTokens = estimatedToolResultTokens(artifact.EstimatedBytes)
	}
	return artifact
}

func appendToolResultArtifact(artifacts []agent.ToolArtifactRef, artifact agent.ToolArtifactRef) []agent.ToolArtifactRef {
	for index := range artifacts {
		if artifacts[index].ID == artifact.ID ||
			canonicalToolResultArtifact(artifacts[index]).ReadablePath == artifact.ReadablePath {
			artifacts[index] = artifact
			return artifacts
		}
	}
	return append(artifacts, artifact)
}

func applyArtifactRecoveryHint(
	result *agent.ToolResult,
	artifact agent.ToolArtifactRef,
	originalBytes int,
	retention agent.ToolResultRetentionMode,
) {
	if result.ContextHints == nil {
		result.ContextHints = &agent.ToolResultContextHints{}
	}
	result.ContextHints.Recovery = agent.ToolResultRecoveryHint{
		Kind: agent.ToolResultRecoveryArtifact, ArtifactPath: artifact.ReadablePath,
		EstimatedBytes:  max(artifact.EstimatedBytes, int64(originalBytes)),
		EstimatedTokens: max(artifact.EstimatedTokens, estimatedToolResultTokens(int64(originalBytes))),
	}
	if result.ContextHints.ContextValue == "" {
		result.ContextHints.ContextValue = agent.ToolResultContextNormal
		if retention == agent.ToolResultEagerCandidate {
			result.ContextHints.ContextValue = agent.ToolResultContextDiscardable
		}
	}
}

func applyReplayRecoveryHint(result *agent.ToolResult, decision ToolDecision, args string, originalBytes int) {
	if result.Status != agent.ToolResultSuccess {
		return
	}
	if result.ContextHints == nil {
		result.ContextHints = &agent.ToolResultContextHints{}
	}
	hints := result.ContextHints
	if hints.Recovery.Kind == "" {
		kind := decision.Descriptor.ResultRecoveryKind
		// Only replay-era descriptors may fall back to broad source inference.
		// New descriptors must distinguish read from grep and fetch from search so
		// every cleanup placeholder names an executable recovery operation.
		if kind == "" && decision.Descriptor.ContextRetention != "" {
			kind = recoveryKindForToolSource(decision.Descriptor.Source)
		}
		if kind != "" {
			reference := boundedRecoveryArguments(args)
			if len(reference) == 0 && strings.TrimSpace(decision.Target) != "" {
				reference = map[string]any{"path": decision.Target}
			}
			if len(reference) != 0 {
				hints.Recovery = agent.ToolResultRecoveryHint{
					Kind: kind, Reference: reference, EstimatedBytes: int64(originalBytes),
					EstimatedTokens: estimatedToolResultTokens(int64(originalBytes)),
				}
			}
		}
	}
	if hints.ContextValue == "" {
		hints.ContextValue = agent.ToolResultContextNormal
		if decision.Descriptor.EffectiveResultRetention() == agent.ToolResultEagerCandidate {
			hints.ContextValue = agent.ToolResultContextDiscardable
		}
	}
	if hints.SupersessionKey == "" && hints.Recovery.Kind != "" {
		hints.SupersessionKey = toolIdempotencyKey(decision.ToolName, args)
	}
	if hints.Recovery.Kind == "" && hints.SupersessionKey == "" && hints.ContextValue == agent.ToolResultContextNormal {
		result.ContextHints = nil
	}
}

func recoveryKindForToolSource(source agent.ToolSource) agent.ToolResultRecoveryKind {
	switch source {
	case agent.ToolSourceRead:
		return agent.ToolResultRecoveryRead
	case agent.ToolSourceWeb:
		return agent.ToolResultRecoveryRefetch
	case agent.ToolSourceLore, agent.ToolSourceHistory:
		return agent.ToolResultRecoveryRerun
	default:
		return ""
	}
}

func boundedRecoveryArguments(args string) map[string]any {
	args = strings.TrimSpace(args)
	if args == "" || len(args) > 32*1024 {
		return nil
	}
	var reference map[string]any
	if err := json.Unmarshal([]byte(args), &reference); err != nil {
		return nil
	}
	return reference
}

func toolResultHeadTailPreview(content string, limit int, status string) string {
	if limit <= 0 || len(content) <= limit {
		return content
	}
	status = strings.TrimSpace(strings.ToValidUTF8(status, "\uFFFD"))
	note := fmt.Sprintf("\n[tool result truncated]\n[tool result preview: original_bytes=%d; %s]", len(content), status)
	if len(note) > max(32, limit*2/3) {
		note, _ = truncateUTF8Bytes(note, max(32, limit*2/3))
	}
	separator := "\n...[middle omitted]...\n"
	available := limit - len(note) - len(separator)
	if available <= 0 {
		preview, _ := truncateUTF8Bytes(note, limit)
		return preview
	}
	headBytes := available / 2
	tailBytes := available - headBytes
	head := utf8Prefix(content, headBytes)
	tail := utf8Suffix(content, tailBytes)
	omitted := max(0, len(content)-len(head)-len(tail))
	separator = fmt.Sprintf("\n...[omitted %d bytes]...\n", omitted)
	available = limit - len(note) - len(separator)
	if available <= 0 {
		preview, _ := truncateUTF8Bytes(note, limit)
		return preview
	}
	headBytes = available / 2
	tailBytes = available - headBytes
	return utf8Prefix(content, headBytes) + separator + utf8Suffix(content, tailBytes) + note
}

func utf8Prefix(content string, limit int) string {
	if limit >= len(content) {
		return content
	}
	limit = min(limit, len(content))
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit]
}

func utf8Suffix(content string, limit int) string {
	if limit >= len(content) {
		return content
	}
	start := len(content) - max(0, limit)
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++
	}
	return content[start:]
}

func estimatedToolResultTokens(byteSize int64) int {
	if byteSize <= 0 {
		return 0
	}
	estimate := byteSize / 4
	if byteSize%4 != 0 {
		estimate++
	}
	maxInt := int64(^uint(0) >> 1)
	if estimate > maxInt {
		return int(maxInt)
	}
	return int(estimate)
}
