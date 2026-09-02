package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"denova/internal/agents/sessionjournal"
	"denova/internal/localfs"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// OpenStoreAgentLog resolves a validated product Session ID below its owning
// Project Store before opening the embedded Agent stream.
func OpenStoreAgentLog(
	ctx context.Context,
	sessionsDir string,
	dataDir string,
	sessionID string,
	agentKind string,
	key agentsession.Key,
) (agentsession.Log, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
	return OpenAgentLog(ctx, filepath.Join(sessionsDir, sessionID+".jsonl"), dataDir, agentKind, key)
}

// OpenAgentLog binds public Agent lifecycle records to the same physical
// journal that owns this product Session's canonical messages.
func OpenAgentLog(
	ctx context.Context,
	filePath string,
	dataDir string,
	agentKind string,
	key agentsession.Key,
) (agentsession.Log, error) {
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(canonical))
	release, err := localfs.AcquireLease(ctx, filePath+".agent-"+hex.EncodeToString(digest[:16])+".lock")
	if err != nil {
		return nil, fmt.Errorf("acquire embedded Agent Session lease: %w", err)
	}
	sess, err := loadSession(filePath)
	if err != nil {
		_ = release()
		return nil, err
	}
	log, err := sessionjournal.NewLog(
		sess.journal, &sess.projection.AgentSessions, key, true,
		func() { _ = release() },
	)
	if err != nil {
		_ = sess.Close()
		_ = release()
		return nil, err
	}
	if err := migrateReleasedContextCompaction(ctx, sess, log, dataDir, agentKind); err != nil {
		_ = log.Close()
		return nil, err
	}
	return log, nil
}

// AgentSessionKeys returns the derived Agent streams currently embedded in a
// product Session journal. It is used only by bounded lifecycle deletion.
func AgentSessionKeys(filePath string) ([]agentsession.Key, error) {
	sess, err := loadSession(filePath)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	return sess.projection.AgentSessions.Keys(), nil
}
