package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// ContextCompactionFailureState is the small provider-neutral state persisted
// by each conversation domain. The fingerprint excludes the ordinary growing
// transcript tail, so repeated failures remain consecutive until a real
// context/configuration structure change or an explicit compaction succeeds.
type ContextCompactionFailureState struct {
	StructureFingerprint string
	ConsecutiveFailures  int
}

type ContextCompactionHealthOutcome string

const (
	ContextCompactionHealthFailure     ContextCompactionHealthOutcome = "failure"
	ContextCompactionHealthSuccess     ContextCompactionHealthOutcome = "success"
	ContextCompactionHealthManualRetry ContextCompactionHealthOutcome = "manual_retry"
)

// Blocks reports whether automatic compaction is fused for the unchanged
// structure. Explicit/manual compaction always bypasses the fuse.
func (state ContextCompactionFailureState) Blocks(structureFingerprint string, maximum int, automatic bool) bool {
	return automatic && maximum > 0 && state.ConsecutiveFailures >= maximum &&
		strings.TrimSpace(state.StructureFingerprint) != "" &&
		state.StructureFingerprint == strings.TrimSpace(structureFingerprint)
}

// NextFailure advances the failure count for one structure. A changed
// fingerprint starts a fresh sequence instead of inheriting an obsolete fuse.
func (state ContextCompactionFailureState) NextFailure(structureFingerprint string) ContextCompactionFailureState {
	structureFingerprint = strings.TrimSpace(structureFingerprint)
	next := 1
	if structureFingerprint != "" && state.StructureFingerprint == structureFingerprint && state.ConsecutiveFailures > 0 {
		next = state.ConsecutiveFailures + 1
	}
	return ContextCompactionFailureState{StructureFingerprint: structureFingerprint, ConsecutiveFailures: next}
}

// ContextCompactionStructureFingerprint hashes only deterministic structure
// selected by a storage-domain adapter: stable runtime messages, exact tool
// schemas, model/config anchors, and active structural record identities.
func ContextCompactionStructureFingerprint(messages []*agent.Message, tools []*agent.ToolInfo, anchors ...string) string {
	payload := struct {
		Messages []*agent.Message  `json:"messages,omitempty"`
		Tools    []*agent.ToolInfo `json:"tools,omitempty"`
		Anchors  []string          `json:"anchors,omitempty"`
	}{messages, tools, append([]string(nil), anchors...)}
	data, err := json.Marshal(payload)
	if err == nil {
		sum := sha256.Sum256(data)
		return "sha256:" + hex.EncodeToString(sum[:])
	}

	// Invalid opaque multimodal JSON must not disable the safety fuse. The
	// fallback still covers roles/content, tool identities, and anchors.
	hash := sha256.New()
	for _, anchor := range anchors {
		_, _ = hash.Write([]byte(strings.TrimSpace(anchor)))
		_, _ = hash.Write([]byte{0})
	}
	for _, message := range messages {
		if message == nil {
			continue
		}
		_, _ = hash.Write([]byte(message.Role))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(message.Content))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(message.Name))
		for _, call := range message.ToolCalls {
			_, _ = hash.Write([]byte(call.ID))
			_, _ = hash.Write([]byte(call.Function.Name))
		}
	}
	for _, tool := range tools {
		if tool != nil {
			_, _ = hash.Write([]byte(tool.Name))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(tool.Desc))
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// ContextCompactionNoProgressLatched prevents a degraded but publishable
// checkpoint from immediately retriggering without meaningful new context or
// a changed cleanup candidate set.
func ContextCompactionNoProgressLatched(
	previousTokens, contextWindow int,
	threshold, recoveryBand float64,
	newSourceTokens, minimumChangeTokens int,
	previousCandidateFingerprint string, previousCandidateGeneration uint64,
	currentCandidateFingerprint string, currentCandidateGeneration uint64,
) bool {
	if previousTokens <= 0 || contextWindow <= 0 || minimumChangeTokens <= 0 || newSourceTokens >= minimumChangeTokens {
		return false
	}
	previousCandidateFingerprint = strings.TrimSpace(previousCandidateFingerprint)
	currentCandidateFingerprint = strings.TrimSpace(currentCandidateFingerprint)
	// Legacy checkpoints had no candidate identity. Allow one re-evaluation so
	// the next durable checkpoint can establish the new latch generation.
	if previousCandidateFingerprint == "" {
		return false
	}
	if previousCandidateFingerprint != currentCandidateFingerprint || previousCandidateGeneration != currentCandidateGeneration {
		return false
	}
	threshold = effectiveContextCompactionThreshold(threshold)
	if recoveryBand <= 0 || recoveryBand > 1 {
		recoveryBand = config.DefaultContextCompactionRecoveryBand
	}
	recoveryTarget := int(float64(contextWindow) * threshold * recoveryBand)
	hardPublishBand := ContextCompactionPublishLimit(contextWindow, threshold)
	return previousTokens > recoveryTarget && previousTokens < hardPublishBand
}

// ContextCompactionCandidateIdentity hashes bounded tool-result locators and
// sizes, never raw results. Generation counts model-visible tool-result rows;
// the fingerprint detects replacement within the same generation.
func ContextCompactionCandidateIdentity(messages []*agent.Message, _ int) (string, uint64) {
	type candidate struct {
		Index      int    `json:"index"`
		CallID     string `json:"call_id,omitempty"`
		Tool       string `json:"tool,omitempty"`
		Bytes      int    `json:"bytes"`
		HasSummary bool   `json:"has_summary,omitempty"`
	}
	candidates := make([]candidate, 0)
	for index, message := range messages {
		if message == nil || message.Role != agent.ToolRole {
			continue
		}
		candidates = append(candidates, candidate{
			Index: index, CallID: strings.TrimSpace(message.ToolCallID), Tool: strings.TrimSpace(message.ToolName),
			Bytes: len([]byte(message.Content)), HasSummary: message.ToolResult != nil,
		})
	}
	payload := struct {
		Candidates []candidate `json:"candidates,omitempty"`
	}{Candidates: candidates}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), uint64(len(candidates))
}

func contextCompactionFailureReason(result ContextCompactionResult) string {
	if reason := strings.TrimSpace(result.FallbackReason); reason != "" {
		return reason
	}
	if reason := strings.TrimSpace(result.SkippedReason); reason != "" {
		return reason
	}
	return "compaction_failed"
}
