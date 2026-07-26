package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/conversationjournal"
)

const maxContextBoundaryBytes = 64 * 1024 * 1024

const (
	contextBoundaryEffectiveSource = "agent.run-state.before-model"
	contextBoundaryCanonicalSource = "session.model-history.before-runtime-fragments"
)

// NewContextBoundarySnapshot freezes the exact effective and canonical
// projections. Each projection has an independent hard limit and digest.
func NewContextBoundarySnapshot(
	cursor ContextCursor,
	effectivePrefix, canonicalPrefix []*agent.Message,
	limitBytes int,
) (*ContextBoundarySnapshot, error) {
	if err := validateContextBoundaryCursor(cursor); err != nil {
		return nil, err
	}
	if limitBytes <= 0 || limitBytes > maxContextBoundaryBytes {
		return nil, fmt.Errorf("context boundary limit %d is invalid", limitBytes)
	}
	effective := copyBoundaryMessages(effectivePrefix)
	canonical := copyBoundaryMessages(canonicalPrefix)
	effectiveBytes, effectiveHash, err := contextProjectionDescriptor(effective, limitBytes)
	if err != nil {
		return nil, fmt.Errorf("effective context boundary projection: %w", err)
	}
	canonicalBytes, canonicalHash, err := contextProjectionDescriptor(canonical, limitBytes)
	if err != nil {
		return nil, fmt.Errorf("canonical context boundary projection: %w", err)
	}
	return &ContextBoundarySnapshot{
		Cursor: cursor, LimitBytes: limitBytes,
		EffectiveSource: contextBoundaryEffectiveSource,
		CanonicalSource: contextBoundaryCanonicalSource,
		EffectivePrefix: effective, CanonicalPrefix: canonical,
		EffectiveBytes: effectiveBytes, CanonicalBytes: canonicalBytes,
		EffectiveSHA256: effectiveHash, CanonicalSHA256: canonicalHash,
	}, nil
}

func validateContextBoundarySnapshot(boundary *ContextBoundarySnapshot) error {
	if boundary == nil {
		return fmt.Errorf("context boundary is missing")
	}
	if boundary.LimitBytes <= 0 || boundary.LimitBytes > maxContextBoundaryBytes {
		return fmt.Errorf("context boundary limit %d is invalid", boundary.LimitBytes)
	}
	if err := validateContextBoundaryCursor(boundary.Cursor); err != nil {
		return err
	}
	if boundary.EffectiveSource != contextBoundaryEffectiveSource || boundary.CanonicalSource != contextBoundaryCanonicalSource {
		return fmt.Errorf("context boundary source is invalid")
	}
	effectiveBytes, effectiveHash, err := contextProjectionDescriptor(boundary.EffectivePrefix, boundary.LimitBytes)
	if err != nil {
		return fmt.Errorf("effective context boundary projection: %w", err)
	}
	canonicalBytes, canonicalHash, err := contextProjectionDescriptor(boundary.CanonicalPrefix, boundary.LimitBytes)
	if err != nil {
		return fmt.Errorf("canonical context boundary projection: %w", err)
	}
	if effectiveBytes != boundary.EffectiveBytes || effectiveHash != boundary.EffectiveSHA256 ||
		canonicalBytes != boundary.CanonicalBytes || canonicalHash != boundary.CanonicalSHA256 {
		return fmt.Errorf("context boundary integrity check failed")
	}
	return nil
}

func validateContextBoundaryCursor(cursor ContextCursor) error {
	if cursor.MessageCount < 0 || cursor.ClearAfterIndex < 0 || cursor.ClearAfterIndex > cursor.MessageCount {
		return fmt.Errorf("context boundary cursor is invalid")
	}
	return nil
}

func copyBoundaryMessages(messages []*agent.Message) []*agent.Message {
	if messages == nil {
		return nil
	}
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = agent.CloneMessage(message)
	}
	return result
}

func contextProjectionDescriptor(messages []*agent.Message, limitBytes int) (int, string, error) {
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

// StoreContextBoundary appends the full projection once and returns the exact
// canonical locator later checkpoint and rewind records must reference.
func (s *Session) StoreContextBoundary(boundaryID string, boundary *ContextBoundarySnapshot) (ContextBoundaryLocator, error) {
	boundaryID = strings.TrimSpace(boundaryID)
	if boundaryID == "" || len(boundaryID) > maxContextLabelBytes {
		return ContextBoundaryLocator{}, fmt.Errorf("context boundary id is invalid")
	}
	if err := validateContextBoundarySnapshot(boundary); err != nil {
		return ContextBoundaryLocator{}, err
	}
	var locator ContextBoundaryLocator
	err := s.withCanonicalMutation(context.Background(), "store context boundary", func() error {
		record := contextBoundaryRecord{
			Type: historyTypeContextBoundary, BoundaryID: boundaryID,
			Boundary: *boundary, CreatedAt: time.Now().UTC(),
		}
		commit, err := s.appendJournalRecordsLocked(record)
		if err != nil {
			return err
		}
		if len(commit.Records) != 1 {
			return fmt.Errorf("context boundary append returned %d records", len(commit.Records))
		}
		location := commit.Records[0].Location
		locator = ContextBoundaryLocator{
			Cursor: location.Cursor, RecordIndex: location.RecordIndex,
			SHA256: contextBoundaryPayloadSHA256(commit.Records[0].Payload),
		}
		advanceUpdatedAt(s, record.CreatedAt)
		return nil
	})
	return locator, err
}

// LoadContextBoundary follows a canonical locator and validates both the exact
// journal payload and its effective/canonical projection digests.
func (s *Session) LoadContextBoundary(boundaryID string, locator ContextBoundaryLocator) (*ContextBoundarySnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadContextBoundaryLocked(strings.TrimSpace(boundaryID), locator)
}

func (s *Session) loadContextBoundaryLocked(boundaryID string, locator ContextBoundaryLocator) (*ContextBoundarySnapshot, error) {
	if boundaryID == "" || len(boundaryID) > maxContextLabelBytes || !validContextBoundaryLocator(locator) {
		return nil, fmt.Errorf("context boundary reference is invalid")
	}
	if s.journal == nil || locator.Cursor > s.journal.Head().Cursor {
		return nil, fmt.Errorf("context boundary locator is outside the canonical journal")
	}
	records, err := s.journal.ReadRange(context.Background(), conversationjournal.Range{
		After: locator.Cursor - 1, Through: locator.Cursor, Limit: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("read context boundary %q: %w", boundaryID, err)
	}
	for _, record := range records {
		if record.Location.Cursor != locator.Cursor || record.Location.RecordIndex != locator.RecordIndex {
			continue
		}
		if got := contextBoundaryPayloadSHA256(record.Payload); got != locator.SHA256 {
			return nil, fmt.Errorf("context boundary %q journal payload hash mismatch", boundaryID)
		}
		var stored contextBoundaryRecord
		if err := json.Unmarshal(record.Payload, &stored); err != nil {
			return nil, fmt.Errorf("decode context boundary %q: %w", boundaryID, err)
		}
		if stored.Type != historyTypeContextBoundary || stored.BoundaryID != boundaryID {
			return nil, fmt.Errorf("context boundary %q locator resolved to a different record", boundaryID)
		}
		if err := validateContextBoundarySnapshot(&stored.Boundary); err != nil {
			return nil, fmt.Errorf("context boundary %q integrity: %w", boundaryID, err)
		}
		return &stored.Boundary, nil
	}
	return nil, fmt.Errorf("context boundary %q record was not found", boundaryID)
}

func validContextBoundaryLocator(locator ContextBoundaryLocator) bool {
	if locator.Cursor == 0 || locator.RecordIndex < 0 || len(locator.SHA256) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(locator.SHA256)
	return err == nil
}

func contextBoundaryPayloadSHA256(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
