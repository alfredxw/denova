package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"denova/internal/agents/conversationjournal"
)

const searchCacheBytes = 16 << 20
const searchDocumentBytes = 2 << 20

type searchDocument struct {
	stamp sessionFileStamp
	texts []string // Newest first, only creator-visible user/assistant content.
	bytes int
	used  uint64
}

// IsTranscriptMessage identifies creator-visible messages consistently for
// full-text search and export, excluding tool traffic and discarded outcomes.
func IsTranscriptMessage(entry HistoryEntry) bool {
	return entry.Type == "message" && !entry.SubAgent && entry.Status != "discarded" &&
		(entry.Content != "" || len(entry.Attachments) > 0) &&
		(entry.Role == "user" || entry.Role == "assistant")
}

// SearchHistoryContent searches all durable display pages, not the resident
// execution window. The bounded, memory-only cache is validated against the
// canonical file on every lookup, including writes from another Store/process.
// Oversized conversations are streamed in full instead of truncated or cached.
func (s *Store) SearchHistoryContent(ctx context.Context, id, query string) (string, error) {
	if err := validateSessionID(id); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path := s.sessionPath(id)
	if err := ensureSessionJournalV3(path); err != nil {
		return "", fmt.Errorf("migrate searchable history: %w", err)
	}
	stamp, err := currentSessionFileStamp(path)
	if err != nil {
		return "", err
	}
	s.searchMu.Lock()
	s.searchClock++
	if cached, ok := s.searchCache[id]; ok {
		if cached.stamp == stamp {
			cached.used = s.searchClock
			s.searchCache[id] = cached
			s.searchMu.Unlock()
			for _, text := range cached.texts {
				if excerpt := transcriptExcerpt(text, query); excerpt != "" {
					return excerpt, nil
				}
			}
			return "", nil
		}
		s.searchBytes -= cached.bytes
		delete(s.searchCache, id)
	}
	s.searchMu.Unlock()
	generation, err := readSessionJournalIncarnation(path)
	if err != nil {
		return "", err
	}
	projection := newSessionJournalProjection(id, generation)
	journal, err := conversationjournal.Open(ctx, path, conversationjournal.Identity{ID: id, Generation: generation}, projection, conversationjournal.Options{})
	if err != nil {
		return "", err
	}
	defer func() {
		if err := journal.Close(); err != nil {
			slog.WarnContext(ctx, "Failed to checkpoint history search projection", "session_id", id, "err", err)
		}
	}()
	snapshot := &Session{ID: id, journal: journal, projection: projection, journalIncarnation: generation}
	document := searchDocument{stamp: stamp}
	excerpt := ""
	for before := -1; ; {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		page, err := snapshot.readHistoryPageLocked(ctx, before, 100)
		if err != nil {
			return "", fmt.Errorf("read searchable history: %w", err)
		}
		for index := len(page.Entries) - 1; index >= 0; index-- {
			entry := page.Entries[index]
			if !IsTranscriptMessage(entry) {
				continue
			}
			text := strings.Join(strings.Fields(entry.Content), " ")
			if excerpt == "" {
				excerpt = transcriptExcerpt(text, query)
			}
			document.bytes += len(text) + 32
			if document.bytes <= searchDocumentBytes {
				document.texts = append(document.texts, text)
			} else {
				document.texts = nil
				if excerpt != "" {
					return excerpt, nil
				}
			}
		}
		if !page.HasMore {
			break
		}
		before = page.NextBefore
	}
	// Never label a pre-append snapshot with a later writer's file stamp.
	if after, err := currentSessionFileStamp(path); err == nil && after == stamp && document.bytes <= searchDocumentBytes {
		s.searchMu.Lock()
		defer s.searchMu.Unlock()
		s.searchClock++
		document.used = s.searchClock
		if old, ok := s.searchCache[id]; ok {
			s.searchBytes -= old.bytes
			delete(s.searchCache, id)
		}
		if s.searchCache == nil {
			s.searchCache = make(map[string]searchDocument)
		}
		for len(s.searchCache) >= 1024 || s.searchBytes+document.bytes > searchCacheBytes {
			oldestID, oldest := "", s.searchClock
			for key, cached := range s.searchCache {
				if cached.used <= oldest {
					oldestID, oldest = key, cached.used
				}
			}
			s.searchBytes -= s.searchCache[oldestID].bytes
			delete(s.searchCache, oldestID)
		}
		s.searchCache[id] = document
		s.searchBytes += document.bytes
	}
	return excerpt, nil
}

func transcriptExcerpt(text, query string) string {
	lower := strings.ToLower(text)
	match := strings.Index(lower, query)
	if match < 0 {
		return ""
	}
	runes := []rune(text)
	position := utf8.RuneCountInString(lower[:match])
	start, end := max(0, position-8), min(len(runes), position+utf8.RuneCountInString(query)+80)
	excerpt := string(runes[start:end])
	if start > 0 {
		excerpt = "…" + excerpt
	}
	if end < len(runes) {
		excerpt += "…"
	}
	return excerpt
}
