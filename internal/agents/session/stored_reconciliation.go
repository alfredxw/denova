package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/filelease"
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
	return sess.CommitDomainMessageContext(ctx, intent)
}

// FindStoredContextCompaction reads an existing stable checkpoint by ID.
func FindStoredContextCompaction(dir, sessionID, id string) (ContextCompaction, bool, error) {
	sess, found, err := openStoredSession(context.Background(), dir, sessionID)
	if err != nil || !found {
		return ContextCompaction{}, false, err
	}
	record, found := sess.ContextCompactionByID(id)
	return record, found, nil
}

// FindStoredContextCompactionRemoval reads an existing stable removal by ID.
func FindStoredContextCompactionRemoval(dir, sessionID, id string) (ContextCompactionRemoval, bool, error) {
	sess, found, err := openStoredSession(context.Background(), dir, sessionID)
	if err != nil || !found {
		return ContextCompactionRemoval{}, false, err
	}
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
	return sess.CommitContextCompactionRemovalAtContext(ctx, ContextCursor{Revision: expectedRevision}, record)
}

func openStoredSession(ctx context.Context, dir, sessionID string) (_ *Session, found bool, resultErr error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, false, fmt.Errorf("session store directory is required")
	}
	if err := validateSessionID(sessionID); err != nil {
		return nil, false, err
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	release, err := filelease.Acquire(ctx, path+".domain.lock")
	if err != nil {
		return nil, false, fmt.Errorf("acquire stored session read lease: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, release()) }()
	sess, err := loadSession(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return sess, true, nil
}
