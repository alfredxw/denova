package session

import (
	"context"
	"encoding/json"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// LoadCapability reads product-owned runtime state without creating an Agent.
func (s *Session) LoadCapability(ctx context.Context, key agentsession.Key, capability string) (json.RawMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return nil, false, err
	}
	return s.projection.AgentSessions.Capability(key, capability)
}

// UpdateCapability belongs to the selected application runtime. It must not
// race a Native Session owning the same capability; switching drains execution.
func (s *Session) UpdateCapability(ctx context.Context, key agentsession.Key, capability string, update func(json.RawMessage, bool) (json.RawMessage, error)) error {
	return s.withCanonicalMutation(ctx, "update runtime capability", func() error {
		record, err := s.projection.AgentSessions.CapabilityUpdate(key, capability, update)
		if err != nil || record == nil {
			return err
		}
		_, err = s.appendJournalRecordsWithUpgradeLocked("runtime-controls-v1", record)
		return err
	})
}
