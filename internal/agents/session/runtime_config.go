package session

import (
	"context"
	"fmt"
	"time"

	"denova/internal/agents/conversationconfig"
)

// RuntimeConfig returns the complete per-conversation Agent selection.
func (s *Session) RuntimeConfig() (conversationconfig.Snapshot, bool) {
	if s == nil {
		return conversationconfig.Snapshot{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimeConfigLocked()
}

// EnsureRuntimeConfig initializes a legacy session exactly once. Concurrent
// initializers converge through the canonical journal lease and return the
// winning durable snapshot.
func (s *Session) EnsureRuntimeConfig(seed conversationconfig.Config) (conversationconfig.Snapshot, error) {
	if err := conversationconfig.ValidateShape(seed, seed.AgentKind); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	var snapshot conversationconfig.Snapshot
	err := s.withCanonicalMutation(context.Background(), "initialize runtime config", func() error {
		if existing, ok := s.runtimeConfigLocked(); ok {
			if existing.AgentKind != seed.AgentKind {
				return fmt.Errorf("conversation Agent kind is immutable: have=%q want=%q", existing.AgentKind, seed.AgentKind)
			}
			snapshot = existing
			return nil
		}
		now := time.Now().UTC()
		revision := uint64(1)
		if err := s.appendJournalRecordLocked(sessionPatchRecord{
			Type: historyTypeSessionPatch, RuntimeConfig: &seed,
			RuntimeConfigRevision: revision, UpdatedAt: now,
		}); err != nil {
			return err
		}
		value := seed
		s.runtimeConfig = &value
		s.runtimeConfigRevision = revision
		advanceUpdatedAt(s, now)
		snapshot = conversationconfig.Snapshot{Config: value, Revision: revision}
		return nil
	})
	return snapshot, err
}

// SetRuntimeConfig replaces one complete snapshot with compare-and-swap
// protection so two open views cannot silently overwrite one another.
func (s *Session) SetRuntimeConfig(next conversationconfig.Config, expectedRevision uint64) (conversationconfig.Snapshot, error) {
	if err := conversationconfig.ValidateShape(next, next.AgentKind); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	var snapshot conversationconfig.Snapshot
	err := s.withCanonicalMutation(context.Background(), "update runtime config", func() error {
		if s.runtimeConfig == nil {
			return conversationconfig.ErrNotInitialized
		}
		if next.AgentKind != s.runtimeConfig.AgentKind {
			return fmt.Errorf("conversation Agent kind is immutable: have=%q want=%q", s.runtimeConfig.AgentKind, next.AgentKind)
		}
		if expectedRevision == 0 || s.runtimeConfigRevision != expectedRevision {
			return fmt.Errorf("%w: have=%d want=%d", conversationconfig.ErrRevisionConflict, s.runtimeConfigRevision, expectedRevision)
		}
		revision := s.runtimeConfigRevision + 1
		now := time.Now().UTC()
		if err := s.appendJournalRecordLocked(sessionPatchRecord{
			Type: historyTypeSessionPatch, RuntimeConfig: &next,
			RuntimeConfigRevision: revision, UpdatedAt: now,
		}); err != nil {
			return err
		}
		value := next
		s.runtimeConfig = &value
		s.runtimeConfigRevision = revision
		advanceUpdatedAt(s, now)
		snapshot = conversationconfig.Snapshot{Config: value, Revision: revision}
		return nil
	})
	return snapshot, err
}

func validateRuntimeConfigState(value *conversationconfig.Config, revision uint64, expectedAgentKind string) error {
	if value == nil {
		if revision != 0 {
			return fmt.Errorf("runtime config revision exists without a config")
		}
		return nil
	}
	if revision == 0 {
		return fmt.Errorf("runtime config revision is missing")
	}
	if err := conversationconfig.ValidateShape(*value, value.AgentKind); err != nil {
		return err
	}
	if expectedAgentKind != "" && value.AgentKind != expectedAgentKind {
		return fmt.Errorf("conversation Agent kind is immutable: have=%q want=%q", expectedAgentKind, value.AgentKind)
	}
	return nil
}

func (s *Session) runtimeConfigLocked() (conversationconfig.Snapshot, bool) {
	if s.runtimeConfig == nil || s.runtimeConfigRevision == 0 {
		return conversationconfig.Snapshot{}, false
	}
	return conversationconfig.Snapshot{Config: *s.runtimeConfig, Revision: s.runtimeConfigRevision}, true
}
