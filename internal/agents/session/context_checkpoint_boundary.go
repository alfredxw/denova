package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const maxContextCheckpointBoundaryBytes = 64 * 1024 * 1024

const (
	contextCheckpointEffectiveSource = "agent.run-state.before-model"
	contextCheckpointCanonicalSource = "session.model-history.before-runtime-fragments"
)

// NewContextCheckpointBoundary freezes both model-visible and canonical
// projections. Limits apply independently because both projections must be
// recoverable exactly; neither is silently truncated.
func NewContextCheckpointBoundary(
	cursor ContextCursor,
	effectivePrefix, canonicalPrefix []*agent.Message,
	limitBytes int,
) (*ContextCheckpointBoundary, error) {
	if err := validateContextCheckpointCursor(cursor); err != nil {
		return nil, err
	}
	if limitBytes <= 0 || limitBytes > maxContextCheckpointBoundaryBytes {
		return nil, fmt.Errorf("context checkpoint boundary limit %d is invalid", limitBytes)
	}
	effective := cloneCheckpointMessages(effectivePrefix)
	canonical := cloneCheckpointMessages(canonicalPrefix)
	effectiveBytes, effectiveHash, err := checkpointProjectionDescriptor(effective, limitBytes)
	if err != nil {
		return nil, fmt.Errorf("effective context checkpoint projection: %w", err)
	}
	canonicalBytes, canonicalHash, err := checkpointProjectionDescriptor(canonical, limitBytes)
	if err != nil {
		return nil, fmt.Errorf("canonical context checkpoint projection: %w", err)
	}
	return &ContextCheckpointBoundary{
		Schema:          ContextCheckpointBoundarySchema,
		Cursor:          cursor,
		LimitBytes:      limitBytes,
		EffectiveSource: contextCheckpointEffectiveSource,
		CanonicalSource: contextCheckpointCanonicalSource,
		EffectivePrefix: effective,
		CanonicalPrefix: canonical,
		EffectiveBytes:  effectiveBytes,
		CanonicalBytes:  canonicalBytes,
		EffectiveSHA256: effectiveHash,
		CanonicalSHA256: canonicalHash,
	}, nil
}

// CloneContextCheckpointBoundary returns a validated deep copy. Invalid or
// legacy boundaries fail closed instead of falling back to raw message counts.
func CloneContextCheckpointBoundary(boundary *ContextCheckpointBoundary) (*ContextCheckpointBoundary, error) {
	if boundary == nil {
		return nil, fmt.Errorf("context checkpoint boundary is missing")
	}
	if strings.TrimSpace(boundary.Schema) != ContextCheckpointBoundarySchema {
		return nil, fmt.Errorf("unsupported context checkpoint boundary schema %q", boundary.Schema)
	}
	if boundary.LimitBytes <= 0 || boundary.LimitBytes > maxContextCheckpointBoundaryBytes {
		return nil, fmt.Errorf("context checkpoint boundary limit %d is invalid", boundary.LimitBytes)
	}
	if err := validateContextCheckpointCursor(boundary.Cursor); err != nil {
		return nil, err
	}
	clone := *boundary
	clone.EffectiveSource = strings.TrimSpace(clone.EffectiveSource)
	clone.CanonicalSource = strings.TrimSpace(clone.CanonicalSource)
	if clone.EffectiveSource != contextCheckpointEffectiveSource || clone.CanonicalSource != contextCheckpointCanonicalSource {
		return nil, fmt.Errorf("context checkpoint boundary source is invalid")
	}
	clone.EffectivePrefix = cloneCheckpointMessages(boundary.EffectivePrefix)
	clone.CanonicalPrefix = cloneCheckpointMessages(boundary.CanonicalPrefix)
	effectiveBytes, effectiveHash, err := checkpointProjectionDescriptor(clone.EffectivePrefix, clone.LimitBytes)
	if err != nil {
		return nil, fmt.Errorf("effective context checkpoint projection: %w", err)
	}
	canonicalBytes, canonicalHash, err := checkpointProjectionDescriptor(clone.CanonicalPrefix, clone.LimitBytes)
	if err != nil {
		return nil, fmt.Errorf("canonical context checkpoint projection: %w", err)
	}
	if effectiveBytes != clone.EffectiveBytes || effectiveHash != clone.EffectiveSHA256 ||
		canonicalBytes != clone.CanonicalBytes || canonicalHash != clone.CanonicalSHA256 {
		return nil, fmt.Errorf("context checkpoint boundary integrity check failed")
	}
	return &clone, nil
}

func validateContextCheckpointCursor(cursor ContextCursor) error {
	if cursor.MessageCount < 0 || cursor.ClearAfterIndex < 0 || cursor.ClearAfterIndex > cursor.MessageCount {
		return fmt.Errorf("context checkpoint boundary cursor is invalid")
	}
	return nil
}

func cloneCheckpointMessages(messages []*agent.Message) []*agent.Message {
	if messages == nil {
		return nil
	}
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		cloned[index] = agent.CloneMessage(message)
	}
	return cloned
}

func checkpointProjectionDescriptor(messages []*agent.Message, limitBytes int) (int, string, error) {
	encoded, err := json.Marshal(messages)
	if err != nil {
		return 0, "", fmt.Errorf("encode projection: %w", err)
	}
	if len(encoded) > limitBytes {
		return 0, "", fmt.Errorf("projection is %d bytes, limit is %d", len(encoded), limitBytes)
	}
	digest := sha256.Sum256(encoded)
	return len(encoded), hex.EncodeToString(digest[:]), nil
}
