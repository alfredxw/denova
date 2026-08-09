// Package toolresult owns provider-neutral result normalization and recovery
// metadata shared by execution and model-context maintenance.
package toolresult

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// RecoverableArtifactPurpose reports whether an artifact contains complete
// output that can safely replace rich inline model context.
func RecoverableArtifactPurpose(purpose agent.ToolArtifactPurpose) bool {
	return purpose == agent.ToolArtifactPurposeCompleteModelOutput || purpose == agent.ToolArtifactPurposeCompleteToolOutput
}

// CanonicalArtifact normalizes one recovery identity before verification,
// persistence, or context cleanup.
func CanonicalArtifact(artifact agent.ToolArtifactRef) agent.ToolArtifactRef {
	artifact.Purpose = agent.ToolArtifactPurpose(strings.TrimSpace(string(artifact.Purpose)))
	artifact.ReadablePath = strings.TrimSpace(strings.ToValidUTF8(artifact.ReadablePath, "\uFFFD"))
	artifact.ContentType = strings.TrimSpace(artifact.ContentType)
	if artifact.EstimatedTokens == 0 && artifact.EstimatedBytes > 0 {
		artifact.EstimatedTokens = EstimatedTokens(artifact.EstimatedBytes)
	}
	return artifact
}

// EstimatedTokens converts a byte count to the conservative result estimate
// used by retention policy and diagnostics.
func EstimatedTokens(byteSize int64) int {
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

// EagerMinimumTokens derives the minimum recoverable result size worth
// transitioning before general context pressure is reached.
func EagerMinimumTokens(configured, contextWindow int, ratio float64) int {
	configured = max(0, configured)
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.15
	}
	return max(configured, int(float64(max(0, contextWindow))*ratio))
}
