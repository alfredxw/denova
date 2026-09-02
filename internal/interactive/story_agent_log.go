package interactive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"denova/internal/agents/sessionjournal"
	"denova/internal/localfs"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// OpenAgentLog binds one game branch Agent Session to the Story JSONL that
// already owns its canonical model history.
func OpenAgentLog(
	ctx context.Context,
	contentRoot string,
	novaDir string,
	storyID string,
	key agentsession.Key,
) (agentsession.Log, error) {
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	store := NewStoreWithNovaDir(contentRoot, novaDir)
	path := store.storyPath(storyID)
	digest := sha256.Sum256([]byte(canonical))
	release, err := localfs.AcquireLease(ctx, path+".agent-"+hex.EncodeToString(digest[:16])+".lock")
	if err != nil {
		return nil, fmt.Errorf("acquire embedded game Agent Session lease: %w", err)
	}
	store.mu.Lock()
	handle, err := store.openStoryJournalLocked(storyID)
	store.mu.Unlock()
	if err != nil {
		_ = release()
		return nil, err
	}
	log, err := sessionjournal.NewLog(
		handle.journal, &handle.projection.AgentSessions, key, true,
		func() { _ = release() },
	)
	if err != nil {
		_ = handle.journal.Close()
		_ = release()
		return nil, err
	}
	return log, nil
}

// AgentSessionKeys returns the derived Agent streams embedded in one Story.
func AgentSessionKeys(contentRoot, novaDir, storyID string) ([]agentsession.Key, error) {
	store := NewStoreWithNovaDir(contentRoot, novaDir)
	store.mu.Lock()
	handle, err := store.openStoryJournalLocked(storyID)
	store.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer handle.journal.Close()
	return handle.projection.AgentSessions.Keys(), nil
}
