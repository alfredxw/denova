package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrStoredSessionNotFound = errors.New("stored session not found")

// CommitStoredDomainMessage opens an existing Session without mutating an
// active Session object's UI state and publishes one idempotent message.
func CommitStoredDomainMessage(
	ctx context.Context,
	dir string,
	sessionID string,
	intent DomainCommitIntent,
) (DomainCommitReceipt, error) {
	sess, found, err := openStoredSession(dir, sessionID)
	if err != nil {
		return DomainCommitReceipt{}, err
	}
	if !found {
		return DomainCommitReceipt{}, fmt.Errorf("%w: %s", ErrStoredSessionNotFound, sessionID)
	}
	defer sess.Close()
	return sess.CommitDomainMessageContext(ctx, intent)
}

func openStoredSession(dir, sessionID string) (*Session, bool, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, false, errors.New("session store directory is required")
	}
	if err := validateSessionID(sessionID); err != nil {
		return nil, false, err
	}
	sess, err := loadSession(filepath.Join(dir, sessionID+".jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return sess, true, nil
}
