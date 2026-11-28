package session

import (
	"bufio"
	"bytes"
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
	"time"

	"denova/internal/agents/conversationjournal"
	"denova/internal/localfs"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

const (
	v3MigrationBackupSuffix = ".v0.8.0.bak"
	v3MigrationMaxLineBytes = 64 << 20
)

type v3MigrationTransaction struct {
	Journal              string            `json:"journal"`
	Version              int               `json:"version"`
	Identity             json.RawMessage   `json:"identity"`
	Cursor               uint64            `json:"cursor"`
	PreviousRecordSHA256 string            `json:"previous_record_sha256,omitempty"`
	CommittedAt          time.Time         `json:"committed_at"`
	Records              []json.RawMessage `json:"records"`
	Checksum             string            `json:"checksum"`
}

// ensureSessionJournalV3 lazily upgrades v0.8.0 (version < 3) session journals
// that contain legacy streaming display patches or duplicated assistant display
// rows into single-write v3 records. A verified backup is written before
// atomically replacing the journal, and stale sidecar indexes are removed so
// they rebuild cleanly without SHA-256 deduplication state.
func ensureSessionJournalV3(filePath string) error {
	headerVersion, needsInspect, err := peekSessionJournalVersion(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !needsInspect || headerVersion >= journalFormatVersion {
		return nil
	}
	needsMigration, err := sessionJournalNeedsV3Migration(filePath)
	if err != nil || !needsMigration {
		return err
	}

	release, err := localfs.AcquireLease(context.Background(), filePath+".domain.lock")
	if err != nil {
		return fmt.Errorf("acquire session migration lease %s: %w", filePath, err)
	}
	defer release()

	headerVersion, needsInspect, err = peekSessionJournalVersion(filePath)
	if err != nil || !needsInspect || headerVersion >= journalFormatVersion {
		return err
	}
	needsMigration, err = sessionJournalNeedsV3Migration(filePath)
	if err != nil || !needsMigration {
		return err
	}

	if err := ensureV3MigrationBackup(filePath); err != nil {
		return err
	}

	records, err := readAllSessionRecordsForV3Migration(filePath)
	if err != nil {
		return err
	}
	normalized, err := normalizeSessionRecordsToV3(filePath, records)
	if err != nil {
		return err
	}
	if err := writeNormalizedV3Session(filePath, normalized); err != nil {
		return err
	}
	sidecar := conversationjournal.SidecarPath(filePath)
	_ = os.Remove(sidecar)
	_ = os.Remove(sidecar + ".current")
	slog.Info("[agents/session] migrated legacy session journal to v3", "path", filePath, "records_before", len(records), "records_after", len(normalized))
	return nil
}

func peekSessionJournalVersion(filePath string) (int, bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4*1024), maxSessionJournalHeaderBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var header struct {
			Type    string `json:"type"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(line, &header); err != nil {
			// Let conversationjournal.Open handle corrupt/truncated files uniformly.
			return 0, false, nil
		}
		if header.Type == "session" {
			return header.Version, header.Version < journalFormatVersion, nil
		}
		if header.Type == "" {
			return 0, true, nil
		}
		return 0, false, nil
	}
	return 0, false, nil
}

func sessionJournalNeedsV3Migration(filePath string) (bool, error) {
	records, err := readAllSessionRecordsForV3Migration(filePath)
	if err != nil {
		// Defer format/corruption errors to the normal journal loader.
		return false, nil
	}
	hasAssistantDisplay := false
	hasAssistantMessage := false
	for _, raw := range records {
		var typed struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Status  string `json:"status"`
			SubAgent bool  `json:"subagent"`
			Message struct {
				Role agentschema.RoleType `json:"role"`
			} `json:"message"`
		}
		if err := json.Unmarshal(raw, &typed); err != nil {
			continue
		}
		switch typed.Type {
		case historyTypeDisplayPatch:
			var patch displayPatchRecord
			if err := json.Unmarshal(raw, &patch); err == nil {
				if patch.ContentAppend != "" || patch.ArgsAppend != "" || patch.DisplayPhase != nil ||
					patch.Result != nil || patch.ToolPresentation != nil || patch.Illustration != nil ||
					(patch.Status != nil && patch.Ask == nil) {
					return true, nil
				}
			}
		case historyTypeDisplay:
			if typed.Status == "discarded" {
				return true, nil
			}
			if typed.Role == "assistant" && !typed.SubAgent {
				hasAssistantDisplay = true
			}
		case historyTypeMessage:
			if typed.Message.Role == agentschema.Assistant {
				hasAssistantMessage = true
			}
		}
	}
	return hasAssistantDisplay && hasAssistantMessage, nil
}

func ensureV3MigrationBackup(filePath string) error {
	backup := filePath + v3MigrationBackupSuffix
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		source, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer source.Close()
		temp, err := os.CreateTemp(filepath.Dir(filePath), ".session-v3-backup-*")
		if err != nil {
			return err
		}
		defer os.Remove(temp.Name())
		_, copyErr := io.Copy(temp, source)
		if err := errors.Join(copyErr, source.Close(), temp.Sync(), temp.Close()); err != nil {
			return err
		}
		if err := os.Rename(temp.Name(), backup); err != nil {
			return err
		}
		return syncParentDirectory(backup)
	} else if err != nil {
		return err
	}
	// If an earlier migration attempt crashed after creating the backup, verify
	// that the backup matches the source before touching the canonical journal.
	var digests [2][sha256.Size]byte
	for i, name := range []string{filePath, backup} {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		sum := sha256.New()
		_, copyErr := io.Copy(sum, file)
		if err := errors.Join(copyErr, file.Close()); err != nil {
			return err
		}
		copy(digests[i][:], sum.Sum(nil))
	}
	if digests[0] != digests[1] {
		return fmt.Errorf("session v3 migration backup %s differs from source %s", backup, filePath)
	}
	return nil
}

func readAllSessionRecordsForV3Migration(filePath string) ([]json.RawMessage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var result []json.RawMessage
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			var discriminator struct {
				Journal string `json:"journal"`
			}
			if err := json.Unmarshal(trimmed, &discriminator); err != nil {
				if readErr == io.EOF {
					// Incomplete trailing line matches conversationjournal crash recovery.
					break
				}
				return nil, fmt.Errorf("decode session migration record: %w", err)
			}
			if discriminator.Journal == "denova.conversation.append" {
				var tx v3MigrationTransaction
				if err := json.Unmarshal(trimmed, &tx); err != nil {
					return nil, fmt.Errorf("decode session migration transaction: %w", err)
				}
				body, err := json.Marshal(struct {
					Journal              string            `json:"journal"`
					Version              int               `json:"version"`
					Identity             json.RawMessage   `json:"identity"`
					Cursor               uint64            `json:"cursor"`
					PreviousRecordSHA256 string            `json:"previous_record_sha256,omitempty"`
					CommittedAt          time.Time         `json:"committed_at"`
					Records              []json.RawMessage `json:"records"`
				}{
					Journal: tx.Journal, Version: tx.Version, Identity: tx.Identity,
					Cursor: tx.Cursor, PreviousRecordSHA256: tx.PreviousRecordSHA256,
					CommittedAt: tx.CommittedAt, Records: tx.Records,
				})
				if err != nil {
					return nil, err
				}
				digest := sha256.Sum256(body)
				if hex.EncodeToString(digest[:]) != tx.Checksum {
					return nil, fmt.Errorf("session migration transaction checksum mismatch at cursor %d", tx.Cursor)
				}
				for _, payload := range tx.Records {
					result = append(result, append(json.RawMessage(nil), payload...))
				}
			} else {
				result = append(result, append(json.RawMessage(nil), trimmed...))
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	return result, nil
}

type v3MigrationEntry struct {
	raw        json.RawMessage
	recordType string
	display    *displayRecord
	message    *messageRecord
	dropped    bool
}

func normalizeSessionRecordsToV3(filePath string, records []json.RawMessage) ([]json.RawMessage, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("empty session journal %s", filePath)
	}
	id := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	entries := make([]v3MigrationEntry, 0, len(records)+1)
	displayByID := make(map[string]int)

	var firstTyped struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(records[0], &firstTyped); err != nil {
		return nil, err
	}
	startIdx := 0
	if firstTyped.Type == "session" {
		var header sessionHeader
		if err := json.Unmarshal(records[0], &header); err != nil {
			return nil, err
		}
		if strings.TrimSpace(header.ID) == "" {
			header.ID = id
		}
		header.Version = journalFormatVersion
		header.IncarnationID = newSessionJournalIncarnationID()
		encoded, err := json.Marshal(header)
		if err != nil {
			return nil, err
		}
		entries = append(entries, v3MigrationEntry{raw: encoded, recordType: "session"})
		startIdx = 1
	} else {
		header := sessionHeader{
			Type:          "session",
			Version:       journalFormatVersion,
			ID:            id,
			IncarnationID: newSessionJournalIncarnationID(),
			CreatedAt:     time.Now().UTC(),
		}
		encoded, err := json.Marshal(header)
		if err != nil {
			return nil, err
		}
		entries = append(entries, v3MigrationEntry{raw: encoded, recordType: "session"})
	}

	// Pass 1: fold display_patch mutations directly into their target display records.
	for i := startIdx; i < len(records); i++ {
		raw := records[i]
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &typed); err != nil {
			return nil, err
		}
		switch typed.Type {
		case historyTypeDisplay:
			var rec displayRecord
			if err := json.Unmarshal(raw, &rec); err != nil {
				return nil, err
			}
			recID := strings.TrimSpace(rec.RecordID)
			if recID == "" {
				recID = legacyJournalRecordID("display", i+1)
				rec.RecordID = recID
			}
			idx := len(entries)
			entries = append(entries, v3MigrationEntry{recordType: historyTypeDisplay, display: &rec})
			displayByID[recID] = idx
		case historyTypeDisplayPatch:
			var patch displayPatchRecord
			if err := json.Unmarshal(raw, &patch); err != nil {
				return nil, err
			}
			targetIdx, ok := displayByID[strings.TrimSpace(patch.TargetRecordID)]
			if !ok || entries[targetIdx].display == nil {
				continue
			}
			target := entries[targetIdx].display
			if patch.DisplayPhase != nil {
				target.DisplayPhase = *patch.DisplayPhase
			}
			if patch.Status != nil {
				target.Status = *patch.Status
			}
			if patch.Result != nil {
				target.Result = *patch.Result
			}
			if normalized := cloneSessionToolPresentation(patch.ToolPresentation); normalized != nil {
				target.ToolPresentation = normalized
			}
			if patch.ArgsAppend != "" {
				target.Args += patch.ArgsAppend
			}
			if patch.ContentAppend != "" {
				target.Content += patch.ContentAppend
			}
			if patch.Illustration != nil {
				target.Illustration = cloneChapterIllustration(patch.Illustration)
			}
			if patch.Ask != nil {
				target.Ask = cloneAskInteraction(patch.Ask)
			}
		case historyTypeMessage:
			var msg messageRecord
			if err := json.Unmarshal(raw, &msg); err != nil {
				return nil, err
			}
			entries = append(entries, v3MigrationEntry{raw: raw, recordType: historyTypeMessage, message: &msg})
		default:
			entries = append(entries, v3MigrationEntry{raw: raw, recordType: typed.Type})
		}
	}

	// Pass 2: drop discarded display previews and fold duplicate root assistant
	// display copies into the canonical assistant message once.
	assistantDisplayByRun := make(map[string][]int)
	for i := range entries {
		entry := &entries[i]
		switch entry.recordType {
		case historyTypeClear:
			clear(assistantDisplayByRun)
		case historyTypeDisplay:
			if entry.display == nil || entry.display.Status == "discarded" {
				entry.dropped = true
				continue
			}
			if entry.display.Role == "assistant" && !entry.display.SubAgent {
				if runID := strings.TrimSpace(entry.display.RunID); runID != "" {
					assistantDisplayByRun[runID] = append(assistantDisplayByRun[runID], i)
				}
			}
		case historyTypeMessage:
			if entry.message == nil {
				continue
			}
			if entry.message.Message.Role == agentschema.User {
				clear(assistantDisplayByRun)
				continue
			}
			if entry.message.Message.Role != agentschema.Assistant || entry.message.SubAgent {
				continue
			}
			runID := strings.TrimSpace(entry.message.RunID)
			if runID == "" {
				continue
			}
			indices := assistantDisplayByRun[runID]
			if len(indices) == 0 {
				continue
			}
			delete(assistantDisplayByRun, runID)
			canonicalText := entry.message.Message.Content
			var combined strings.Builder
			matchedSingle := -1
			for _, idx := range indices {
				seg := entries[idx].display
				if seg == nil {
					continue
				}
				combined.WriteString(seg.Content)
				if seg.Content == canonicalText {
					matchedSingle = idx
				}
			}
			if matchedSingle >= 0 {
				if phase := entries[matchedSingle].display.DisplayPhase; phase != "" {
					entry.message.DisplayPhase = phase
				} else if entry.message.DisplayPhase == "" {
					entry.message.DisplayPhase = DisplayPhaseFinal
				}
				entries[matchedSingle].dropped = true
				if encoded, err := json.Marshal(entry.message); err == nil {
					entry.raw = encoded
				}
			} else if combined.String() == canonicalText {
				lastPhase := entries[indices[len(indices)-1]].display.DisplayPhase
				if lastPhase != "" {
					entry.message.DisplayPhase = lastPhase
				} else if entry.message.DisplayPhase == "" {
					entry.message.DisplayPhase = DisplayPhaseFinal
				}
				for _, idx := range indices {
					entries[idx].dropped = true
				}
				if encoded, err := json.Marshal(entry.message); err == nil {
					entry.raw = encoded
				}
			}
		}
	}

	out := make([]json.RawMessage, 0, len(entries))
	for i := range entries {
		entry := &entries[i]
		if entry.dropped {
			continue
		}
		if entry.recordType == historyTypeDisplay && entry.display != nil {
			encoded, err := json.Marshal(entry.display)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded)
			continue
		}
		out = append(out, entry.raw)
	}
	return out, nil
}

func writeNormalizedV3Session(filePath string, records []json.RawMessage) error {
	temp, err := os.CreateTemp(filepath.Dir(filePath), ".session-v3-migrate-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	writer := bufio.NewWriterSize(temp, 64*1024)
	for _, record := range records {
		if _, err := writer.Write(record); err != nil {
			_ = temp.Close()
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			_ = temp.Close()
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := errors.Join(temp.Sync(), temp.Close()); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), filePath); err != nil {
		return err
	}
	return syncParentDirectory(filePath)
}
