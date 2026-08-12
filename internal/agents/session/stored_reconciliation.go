package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

var ErrStoredSessionNotFound = errors.New("stored session not found")

// FindStoredDomainCommit opens an existing session journal read-only and
// queries one exact canonical receipt. Missing stores and sessions are a
// definite not-found result and are never created during recovery.
func FindStoredDomainCommit(
	dir string,
	sessionID string,
	identity DomainCommitIdentity,
	role agent.RoleType,
	hash string,
) (DomainCommitReceipt, bool, error) {
	sess, found, err := openStoredSession(context.Background(), dir, sessionID)
	if err != nil || !found {
		return DomainCommitReceipt{}, false, err
	}
	defer sess.Close()
	return sess.FindDomainCommit(identity, role, hash)
}

// FindStoredAgentCanonicalCommit proves the exact public Agent stage hash
// without creating or mutating the product Session during cold recovery.
func FindStoredAgentCanonicalCommit(
	dir string,
	sessionID string,
	identity DomainCommitIdentity,
	role agent.RoleType,
	hash string,
) (DomainCommitReceipt, bool, error) {
	sess, found, err := openStoredSession(context.Background(), dir, sessionID)
	if err != nil || !found {
		return DomainCommitReceipt{}, false, err
	}
	defer sess.Close()
	return sess.FindAgentCanonicalCommit(identity, role, hash)
}

// CommitStoredDomainMessage opens an existing canonical Session without
// mutating active-session UI state and publishes one exact idempotent message.
// Missing sessions are retryable errors: recovery must never silently invent a
// different conversation for an already accepted binding.
func CommitStoredDomainMessage(
	ctx context.Context,
	dir string,
	sessionID string,
	intent DomainCommitIntent,
) (DomainCommitReceipt, error) {
	sess, found, err := openStoredSession(ctx, dir, sessionID)
	if err != nil {
		return DomainCommitReceipt{}, err
	}
	if !found {
		return DomainCommitReceipt{}, fmt.Errorf("%w: %s", ErrStoredSessionNotFound, sessionID)
	}
	defer sess.Close()
	return sess.CommitDomainMessageContext(ctx, intent)
}

func openStoredSession(_ context.Context, dir, sessionID string) (*Session, bool, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, false, fmt.Errorf("session store directory is required")
	}
	if err := validateSessionID(sessionID); err != nil {
		return nil, false, err
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	// loadSession opens the shared Conversation Journal, which owns the exact
	// cross-process lease. Taking the same non-reentrant lease here would
	// deadlock cold recovery before the journal can validate its incarnation.
	sess, err := loadSession(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return sess, true, nil
}
