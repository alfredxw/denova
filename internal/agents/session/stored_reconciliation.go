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

// FindStoredContextCompaction reads an existing stable checkpoint by ID.
func FindStoredContextCompaction(dir, sessionID, id string) (ContextCompaction, bool, error) {
	sess, found, err := openStoredSession(context.Background(), dir, sessionID)
	if err != nil || !found {
		return ContextCompaction{}, false, err
	}
	defer sess.Close()
	record, found := sess.ContextCompactionByID(id)
	return record, found, nil
}

// FindStoredContextCompactionRemoval reads an existing stable removal by ID.
func FindStoredContextCompactionRemoval(dir, sessionID, id string) (ContextCompactionRemoval, bool, error) {
	sess, found, err := openStoredSession(context.Background(), dir, sessionID)
	if err != nil || !found {
		return ContextCompactionRemoval{}, false, err
	}
	defer sess.Close()
	record, found := sess.ContextCompactionRemovalByID(id)
	return record, found, nil
}

// CommitStoredContextCompaction publishes one frozen structural mutation for
// a durable binding that is not necessarily open in the UI process.
func CommitStoredContextCompaction(
	ctx context.Context,
	dir string,
	sessionID string,
	expectedRevision uint64,
	record ContextCompaction,
) (ContextCompaction, error) {
	sess, found, err := openStoredSession(ctx, dir, sessionID)
	if err != nil {
		return ContextCompaction{}, err
	}
	if !found {
		return ContextCompaction{}, fmt.Errorf("%w: %s", ErrStoredSessionNotFound, sessionID)
	}
	defer sess.Close()
	return sess.AppendContextCompactionAtContext(ctx, ContextCursor{Revision: expectedRevision}, record)
}

// CommitStoredContextCompactionRemoval is the removal counterpart used by
// cold structural recovery.
func CommitStoredContextCompactionRemoval(
	ctx context.Context,
	dir string,
	sessionID string,
	expectedRevision uint64,
	record ContextCompactionRemoval,
) (ContextCompactionRemoval, bool, error) {
	sess, found, err := openStoredSession(ctx, dir, sessionID)
	if err != nil {
		return ContextCompactionRemoval{}, false, err
	}
	if !found {
		return ContextCompactionRemoval{}, false, fmt.Errorf("%w: %s", ErrStoredSessionNotFound, sessionID)
	}
	defer sess.Close()
	return sess.CommitContextCompactionRemovalAtContext(ctx, ContextCursor{Revision: expectedRevision}, record)
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
