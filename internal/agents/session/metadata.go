package session

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/conversationconfig"
	"denova/internal/conversationjournal"
)

// metadataLocked returns a current session summary without materializing its
// transcript. The caller holds Store.mu, so a resident Session can be sampled
// directly while cold sessions use only the bounded journal projection.
func (s *Store) metadataLocked(id, filePath, activeID string) (SessionMeta, error) {
	if sess := s.cache[id]; sess != nil {
		return sess.metadata(activeID), nil
	}
	return loadSessionMetadata(filePath, activeID)
}

func (s *Session) metadata(activeID string) SessionMeta {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta := SessionMeta{
		ID: s.ID, Title: s.titleLocked(), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		Active: s.ID == activeID, MessageCount: s.visibleMessageCountLocked(),
	}
	if snapshot, ok := s.runtimeConfigLocked(); ok {
		meta.RuntimeConfig = &snapshot
	}
	return meta
}

// loadSessionMetadata restores the derived projection and scans only an
// unindexed canonical tail. It deliberately avoids ReadRange and therefore
// does not decode the recent transcript window needed by an opened chat.
func loadSessionMetadata(filePath, activeID string) (SessionMeta, error) {
	id := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	generation, err := readSessionJournalIncarnation(filePath)
	if err != nil {
		return SessionMeta{}, err
	}
	projection := newSessionJournalProjection(id, generation)
	journal, err := conversationjournal.Open(
		context.Background(),
		filePath,
		conversationjournal.Identity{ID: id, Generation: generation},
		projection,
		conversationjournal.Options{},
	)
	if err != nil {
		return SessionMeta{}, fmt.Errorf("打开会话元数据 journal 失败 %s: %w", filePath, err)
	}
	// Close first so a concurrently appended tail is reflected in projection.
	// Clean indexes are not rewritten by Close.
	if err := journal.Close(); err != nil {
		// The sidecar is rebuildable. Match Store.Close semantics by preserving
		// readable canonical metadata while making the checkpoint failure visible.
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agents/session/metadata.go] closing metadata journal failed path=%q err=%v", filePath, err))
	}

	now := time.Now().UTC()
	createdAt := projection.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := projection.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	meta := SessionMeta{
		ID: id, Title: projection.Title, CreatedAt: createdAt, UpdatedAt: updatedAt,
		Active: id == activeID, MessageCount: projection.VisibleMessageCount,
	}
	if projection.RuntimeConfig != nil && projection.RuntimeConfigRevision > 0 {
		snapshot := conversationconfig.Snapshot{Config: *projection.RuntimeConfig, Revision: projection.RuntimeConfigRevision}
		meta.RuntimeConfig = &snapshot
	}
	return meta, nil
}
