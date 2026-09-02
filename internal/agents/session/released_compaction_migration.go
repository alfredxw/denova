package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/agents/sessionjournal"
	"denova/internal/localfs"

	agent "github.com/alfredxw/denova/agent"
)

const releasedProductCompactionBackupDirectory = "product-session-v0.3.3-compaction"

type releasedContextCompactionMigration struct {
	State   json.RawMessage
	Deleted bool
}

func migrateReleasedContextCompaction(
	ctx context.Context,
	sess *Session,
	log *sessionjournal.Log,
	dataDir string,
	agentKind string,
) error {
	migration, found, err := sess.releasedContextCompactionMigration(agentKind)
	if err != nil || !found {
		return err
	}
	migrated, err := log.HasCapabilityRecord(agent.CompactionCapability)
	if err != nil {
		return err
	}
	if migrated {
		return nil
	}
	backupPath, err := backupReleasedContextCompactionJournal(
		sess.filePath, dataDir, sess.journal.Head().VerifiedBytes,
	)
	if err != nil {
		return fmt.Errorf("backup released Product Session Compaction: %w", err)
	}
	changed, err := log.ImportCapabilityIfAbsent(
		ctx, agent.CompactionCapability, migration.State, migration.Deleted,
	)
	if err != nil {
		return fmt.Errorf("convert released Product Session Compaction: %w", err)
	}
	if changed {
		slog.InfoContext(ctx, fmt.Sprintf(
			"[agent-session] converted released Product Session Compaction session_id=%s agent_kind=%s deleted=%t backup=%s",
			sess.ID, strings.TrimSpace(agentKind), migration.Deleted, backupPath,
		))
	}
	return nil
}

func (s *Session) releasedContextCompactionMigration(agentKind string) (releasedContextCompactionMigration, bool, error) {
	if s == nil || s.projection == nil {
		return releasedContextCompactionMigration{}, false, nil
	}
	legacy, found := s.projection.latestReleasedContextCompaction(agentKind)
	if !found {
		return releasedContextCompactionMigration{}, false, nil
	}
	if legacy.Removal != nil {
		return releasedContextCompactionMigration{Deleted: true}, true, nil
	}
	record := legacy.Compaction
	clearAfter := s.projection.ClearAfter
	replacementFrom := record.SourceStartIndex - clearAfter
	replacementTo := record.SourceEndIndex - clearAfter
	effectiveCount := s.projection.MessageCount - clearAfter
	// v0.3.3 allowed an empty source interval when only an earlier checkpoint
	// was re-summarized. The current Agent state deliberately requires a real
	// raw replacement range. Falling back to the still-complete raw transcript
	// is safer than inventing coordinates or refusing to open user data.
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.Summary) == "" ||
		replacementFrom < 0 || replacementTo <= replacementFrom || replacementTo > effectiveCount {
		return releasedContextCompactionMigration{Deleted: true}, true, nil
	}
	revision := uint64(max(1, record.Epoch))
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.UpdatedAt
	}
	state := agent.CompactionState{
		ID: record.ID, Revision: revision,
		SourceRevision: fmt.Sprintf("product-session-v0.3.3:%s", record.ID),
		Summary:        record.Summary, TokenEstimate: record.TokensAfter,
		Metrics: agent.CompactionMetrics{
			EstimatedTokensBefore: record.TokensBefore,
			EstimatedTokensAfter:  record.TokensAfter,
			ContextWindowTokens:   record.ContextWindowTokens,
			Threshold:             record.Threshold,
			SourceMessageCount:    record.SourceMessageCount,
		},
		ReplacementFrom: replacementFrom, ReplacementTo: replacementTo,
		CreatedAt: createdAt,
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return releasedContextCompactionMigration{}, false, err
	}
	return releasedContextCompactionMigration{State: encoded}, true, nil
}

func (projection *sessionJournalProjection) latestReleasedContextCompaction(
	agentKind string,
) (releasedContextCompactionProjection, bool) {
	agentKind = strings.TrimSpace(agentKind)
	var selected releasedContextCompactionProjection
	found := false
	for kind, candidate := range projection.ReleasedContextCompactions {
		if agentKind != "" && kind != "" && kind != agentKind {
			continue
		}
		if !found || releasedCompactionAfter(candidate, selected) {
			selected, found = candidate, true
		}
	}
	return selected, found
}

func releasedCompactionAfter(left, right releasedContextCompactionProjection) bool {
	return left.Cursor > right.Cursor || left.Cursor == right.Cursor && left.RecordIndex > right.RecordIndex
}

func backupReleasedContextCompactionJournal(sourcePath, dataDir string, length int64) (string, error) {
	if strings.TrimSpace(dataDir) == "" || length <= 0 {
		return "", fmt.Errorf("released Product Session backup path or length is invalid")
	}
	backupRoot := filepath.Join(dataDir, "backups", releasedProductCompactionBackupDirectory)
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return "", err
	}
	input, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(backupRoot, ".session-*.jsonl")
	if err != nil {
		_ = input.Close()
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = input.Close()
		_ = temp.Close()
		return "", err
	}
	digest := sha256.New()
	copied, copyErr := io.CopyN(io.MultiWriter(temp, digest), input, length)
	inputCloseErr := input.Close()
	syncErr := temp.Sync()
	tempCloseErr := temp.Close()
	if err := errors.Join(copyErr, inputCloseErr, syncErr, tempCloseErr); err != nil {
		return "", err
	}
	if copied != length {
		return "", fmt.Errorf("released Product Session backup is incomplete")
	}
	ext := hex.EncodeToString(digest.Sum(nil))
	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	backupPath := filepath.Join(backupRoot, base+"-"+ext+".jsonl")
	if info, statErr := os.Stat(backupPath); statErr == nil {
		if !info.Mode().IsRegular() || info.Size() != length {
			return "", fmt.Errorf("released Product Session backup conflicts with %s", backupPath)
		}
		existingDigest, hashErr := hashFilePrefix(backupPath, length)
		if hashErr != nil {
			return "", fmt.Errorf("verify released Product Session backup: %w", hashErr)
		}
		if existingDigest != ext {
			return "", fmt.Errorf("released Product Session backup checksum mismatch: %s", backupPath)
		}
		return backupPath, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	if err := os.Rename(tempPath, backupPath); err != nil {
		return "", err
	}
	if err := localfs.SyncDirectory(backupRoot); err != nil {
		return "", err
	}
	return backupPath, nil
}

func hashFilePrefix(path string, length int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, copyErr := io.CopyN(digest, file, length)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
