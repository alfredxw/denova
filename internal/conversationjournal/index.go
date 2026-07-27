package conversationjournal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/fsdurability"
)

type indexBody struct {
	Version             int             `json:"version"`
	Identity            Identity        `json:"identity"`
	VerifiedBytes       int64           `json:"verified_bytes"`
	Cursor              Cursor          `json:"cursor"`
	TransactionCount    int64           `json:"transaction_count"`
	Tail                Location        `json:"tail"`
	TailRecordSHA256    string          `json:"tail_record_sha256,omitempty"`
	InitialRecordSHA256 string          `json:"initial_record_sha256,omitempty"`
	NeedsNewline        bool            `json:"needs_newline,omitempty"`
	Sparse              []Location      `json:"sparse,omitempty"`
	Recent              []Location      `json:"recent,omitempty"`
	Projection          json.RawMessage `json:"projection"`
}

type indexFile struct {
	indexBody
	Checksum string `json:"checksum"`
}

func sidecarPath(journalPath string) string {
	if strings.EqualFold(filepath.Ext(journalPath), ".jsonl") {
		return strings.TrimSuffix(journalPath, filepath.Ext(journalPath)) + ".idx.json"
	}
	return journalPath + ".idx.json"
}

func encodeIndex(body indexBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.MarshalIndent(indexFile{indexBody: body, Checksum: hex.EncodeToString(digest[:])}, "", "  ")
}

func decodeIndex(data []byte) (indexBody, error) {
	var decoded indexFile
	if err := json.Unmarshal(data, &decoded); err != nil {
		return indexBody{}, err
	}
	bodyJSON, err := json.Marshal(decoded.indexBody)
	if err != nil {
		return indexBody{}, err
	}
	digest := sha256.Sum256(bodyJSON)
	if decoded.Version != indexVersion || decoded.Checksum != hex.EncodeToString(digest[:]) {
		return indexBody{}, fmt.Errorf("conversation journal index version or checksum mismatch")
	}
	return decoded.indexBody, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".conversation-index-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if err := writeAll(temporary, append(data, '\n')); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	cleanup = false
	return fsdurability.SyncDirectory(filepath.Dir(path))
}

func removeIndex(path string) error {
	err := os.Remove(sidecarPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (journal *Journal) restoreIndexLocked() (bool, error) {
	data, err := os.ReadFile(journal.indexPath)
	if err != nil {
		return false, err
	}
	body, err := decodeIndex(data)
	if err != nil {
		return false, err
	}
	if body.Identity != journal.identity {
		return false, fmt.Errorf("conversation journal index identity mismatch")
	}
	if body.VerifiedBytes < 0 || body.VerifiedBytes > journal.journalSize || body.Cursor == 0 {
		return false, fmt.Errorf("conversation journal index position is invalid")
	}
	if !json.Valid(body.Projection) {
		return false, fmt.Errorf("conversation journal index projection is invalid")
	}
	initialSHA, err := firstRecordSHA256(journal.path)
	if err != nil {
		return false, err
	}
	if initialSHA == "" || initialSHA != body.InitialRecordSHA256 {
		return false, fmt.Errorf("conversation journal index incarnation is stale")
	}
	tailSHA, err := locationSHA256(journal.path, body.Tail)
	if err != nil {
		return false, err
	}
	if tailSHA != body.TailRecordSHA256 {
		return false, fmt.Errorf("conversation journal index tail checksum is stale")
	}
	if err := journal.reducer.Restore(append(json.RawMessage(nil), body.Projection...)); err != nil {
		return false, fmt.Errorf("restore conversation projection checkpoint: %w", err)
	}
	journal.validOffset = body.VerifiedBytes
	journal.head = Head{
		Identity: journal.identity, Cursor: body.Cursor,
		RecordSHA256: body.TailRecordSHA256, VerifiedBytes: body.VerifiedBytes,
		TransactionCount: body.TransactionCount,
	}
	journal.initialRecordSHA256 = body.InitialRecordSHA256
	journal.needsNewline = body.NeedsNewline
	journal.sparse = append([]Location(nil), body.Sparse...)
	journal.recent = append([]Location(nil), body.Recent...)
	return true, nil
}

func (journal *Journal) persistIndexLocked() error {
	if journal.projectionInvalid {
		return fmt.Errorf("conversation projection is invalid; reopen before checkpoint")
	}
	projection, err := journal.reducer.Checkpoint()
	if err != nil {
		return fmt.Errorf("checkpoint conversation projection: %w", err)
	}
	if !json.Valid(projection) {
		return fmt.Errorf("conversation projection checkpoint is not valid JSON")
	}
	var tail Location
	if len(journal.recent) > 0 {
		tail = journal.recent[len(journal.recent)-1]
	} else if journal.head.Cursor > 0 {
		return fmt.Errorf("conversation journal index is missing its tail locator")
	}
	body := indexBody{
		Version: indexVersion, Identity: journal.identity,
		VerifiedBytes: journal.validOffset, Cursor: journal.head.Cursor,
		TransactionCount: journal.head.TransactionCount, Tail: tail,
		TailRecordSHA256:    journal.head.RecordSHA256,
		InitialRecordSHA256: journal.initialRecordSHA256,
		NeedsNewline:        journal.needsNewline,
		Sparse:              append([]Location(nil), journal.sparse...),
		Recent:              append([]Location(nil), journal.recent...),
		Projection:          append(json.RawMessage(nil), projection...),
	}
	data, err := encodeIndex(body)
	if err != nil {
		return err
	}
	if err := writeAtomic(journal.indexPath, data, 0o600); err != nil {
		return err
	}
	journal.dirtyTransactions = 0
	journal.indexDirty = false
	return nil
}

func firstRecordSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	line, readErr := bufio.NewReader(file).ReadBytes('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) && len(line) == 0 {
		return "", readErr
	}
	line = trimRecord(line)
	if len(line) == 0 || !json.Valid(line) {
		return "", fmt.Errorf("conversation journal first record is invalid")
	}
	return recordSHA256(line), nil
}

func locationSHA256(path string, location Location) (string, error) {
	if location.Cursor == 0 || location.Offset < 0 || location.Length <= 0 {
		return "", fmt.Errorf("conversation journal tail location is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data := make([]byte, location.Length)
	if _, err := file.ReadAt(data, location.Offset); err != nil {
		return "", err
	}
	return recordSHA256(data), nil
}
