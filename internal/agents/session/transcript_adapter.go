package session

import (
	"fmt"
	"log"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// TranscriptSnapshot projects Denova's mixed product journal onto the stable
// Agent transcript contract. Display cards, interruptions, illustrations,
// compaction policy, and domain metadata intentionally remain product state.
func (s *Session) TranscriptSnapshot() (agentsession.Snapshot, error) {
	if s == nil {
		return agentsession.Snapshot{}, fmt.Errorf("session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transcriptSnapshotLocked()
}

func (s *Session) transcriptSnapshotLocked() (agentsession.Snapshot, error) {
	entries := make([]agentsession.Entry, 0, len(s.messages)+1)
	for _, record := range s.records {
		switch record.kind {
		case historyTypeMessage, historyTypeContextMessage:
			if record.message == nil {
				continue
			}
			entries = append(entries, agentsession.Entry{
				Revision: agentsession.Revision(len(entries) + 1),
				Type:     agentsession.EntryMessage,
				Message:  record.message,
			})
		case historyTypeClear:
			entries = append(entries, agentsession.Entry{
				Revision: agentsession.Revision(len(entries) + 1),
				Type:     agentsession.EntryClear,
			})
		}
	}
	revision := agentsession.Revision(s.contextRevision)
	if minimum := agentsession.Revision(len(entries)); revision < minimum {
		// Legacy journals did not persist context revisions. Restore still uses
		// one monotonic revision per projected transcript entry.
		revision = minimum
	}
	return agentsession.Restore(s.ID, revision, entries)
}

func (s *Session) effectiveTranscriptMessagesLocked() []*agent.Message {
	snapshot, err := s.transcriptSnapshotLocked()
	if err == nil {
		return snapshot.EffectiveMessages()
	}
	// The mixed journal predates the public contract, so corrupt legacy data
	// must remain readable. New writes are validated before reaching this path.
	log.Printf("internal/agents/session/transcript_adapter.go: projected transcript fallback session=%q error=%v", s.ID, err)
	result := make([]*agent.Message, len(s.messages)-s.clearAfterIndex)
	for index, message := range s.messages[s.clearAfterIndex:] {
		result[index] = agent.CloneMessage(message)
	}
	return result
}
