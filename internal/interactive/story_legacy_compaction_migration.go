package interactive

import (
	"bufio"
	"bytes"
	"context"
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
)

const (
	legacyStoryEventTypeContextCompaction        = "context_compaction"
	legacyStoryEventTypeContextCompactionRemoved = "context_compaction_removed"
	legacyStoryCompactionMigrationName           = "interactive-story-v0.3.3-compaction"
)

type legacyStoryCompactionInspection struct {
	parentsByEventID map[string]string
	hasTransactions  bool
}

// migrateReleasedStoryCompactionJournal removes the two context-maintenance
// events written by v0.3.3. They never owned narrative or state, but they did
// advance branch ancestry, so every surviving reference is reconnected before
// the current projection sees the journal. Detection is the migration receipt:
// a successful rewrite contains no released event and is therefore idempotent.
func (s *Store) migrateReleasedStoryCompactionJournal(storyID string) (bool, error) {
	required, err := s.releasedStoryCompactionMigrationRequired()
	if err != nil {
		return false, err
	}
	if !required {
		return false, nil
	}
	path := s.storyPath(storyID)
	inspection, err := inspectReleasedStoryCompactionJournal(path)
	if err != nil {
		return false, err
	}
	if len(inspection.parentsByEventID) == 0 {
		return false, nil
	}
	if inspection.hasTransactions {
		return false, fmt.Errorf("released story compaction migration does not accept a mixed transaction journal: %s", path)
	}
	resolvedParents, err := resolveReleasedStoryCompactionParents(inspection.parentsByEventID)
	if err != nil {
		return false, fmt.Errorf("resolve released story compaction ancestry: %w", err)
	}
	backupPath, err := s.backupReleasedStoryCompactionJournal(path)
	if err != nil {
		return false, fmt.Errorf("backup released story before compaction migration: %w", err)
	}
	// The sidecar is derived. Removing it before the atomic canonical rewrite is
	// safe even if the rewrite fails, and guarantees the next open rebuilds from
	// the migrated parent chain.
	if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove released story projection before migration: %w", err)
	}
	if err := rewriteReleasedStoryCompactionJournal(path, inspection.parentsByEventID, resolvedParents); err != nil {
		return false, fmt.Errorf("rewrite released story compaction journal: %w", err)
	}
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[interactive-story] migrated released compaction events story_id=%s removed=%d backup=%s",
		storyID, len(inspection.parentsByEventID), backupPath,
	))
	return true, nil
}

func (s *Store) releasedStoryCompactionMigrationRequired() (bool, error) {
	data, err := os.ReadFile(s.indexPath())
	if errors.Is(err, os.ErrNotExist) {
		// A released story may be recovered independently from its catalog. Scan it
		// once rather than making the missing derived index a data-loss boundary.
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read story index before released compaction migration: %w", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return false, fmt.Errorf("decode story index before released compaction migration: %w", err)
	}
	return header.Version < storyIndexSchemaVersion, nil
}

func inspectReleasedStoryCompactionJournal(path string) (legacyStoryCompactionInspection, error) {
	inspection := legacyStoryCompactionInspection{parentsByEventID: make(map[string]string)}
	err := scanStoryJSONL(path, func(lineNumber int, line []byte) error {
		var envelope struct {
			Journal  string          `json:"journal"`
			Type     string          `json:"type"`
			ID       string          `json:"id"`
			ParentID json.RawMessage `json:"parent_id"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			return fmt.Errorf("decode story journal line %d: %w", lineNumber, err)
		}
		if strings.TrimSpace(envelope.Journal) != "" {
			inspection.hasTransactions = true
			return nil
		}
		if !isReleasedStoryCompactionEvent(envelope.Type) {
			return nil
		}
		eventID := strings.TrimSpace(envelope.ID)
		if eventID == "" {
			return fmt.Errorf("released story compaction event on line %d has no id", lineNumber)
		}
		if _, duplicate := inspection.parentsByEventID[eventID]; duplicate {
			return fmt.Errorf("released story compaction event id %q is duplicated", eventID)
		}
		parentID, err := releasedStoryParentID(envelope.ParentID)
		if err != nil {
			return fmt.Errorf("decode released story compaction parent on line %d: %w", lineNumber, err)
		}
		inspection.parentsByEventID[eventID] = parentID
		return nil
	})
	return inspection, err
}

func resolveReleasedStoryCompactionParents(parentsByEventID map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(parentsByEventID))
	for eventID := range parentsByEventID {
		current := eventID
		visited := make(map[string]bool)
		for {
			if visited[current] {
				return nil, fmt.Errorf("released story compaction ancestry contains a cycle at %q", current)
			}
			visited[current] = true
			parentID, removed := parentsByEventID[current]
			if !removed {
				resolved[eventID] = current
				break
			}
			if parentID == "" {
				resolved[eventID] = ""
				break
			}
			current = parentID
		}
	}
	return resolved, nil
}

func rewriteReleasedStoryCompactionJournal(path string, removed, resolved map[string]string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".migration-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		_ = temp.Close()
		return err
	}

	writer := bufio.NewWriterSize(temp, 64*1024)
	lineNumber := 0
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxStoryLineBytes)
	for scanner.Scan() {
		lineNumber++
		line := append([]byte(nil), scanner.Bytes()...)
		var envelope struct {
			Type     string          `json:"type"`
			ParentID json.RawMessage `json:"parent_id"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			_ = temp.Close()
			return fmt.Errorf("decode story journal line %d: %w", lineNumber, err)
		}
		if isReleasedStoryCompactionEvent(envelope.Type) {
			continue
		}
		changed := false
		var object map[string]any
		if envelope.Type == StoryEventTypeMeta {
			object, err = decodeStoryMigrationObject(line)
			if err != nil {
				_ = temp.Close()
				return fmt.Errorf("decode story metadata on line %d: %w", lineNumber, err)
			}
			changed = reconnectReleasedStoryBranches(object, removed, resolved)
		} else if len(envelope.ParentID) > 0 {
			parentID, parentErr := releasedStoryParentID(envelope.ParentID)
			if parentErr != nil {
				_ = temp.Close()
				return fmt.Errorf("decode story event parent on line %d: %w", lineNumber, parentErr)
			}
			if next, ok := resolved[parentID]; ok {
				object, err = decodeStoryMigrationObject(line)
				if err != nil {
					_ = temp.Close()
					return fmt.Errorf("decode story event on line %d: %w", lineNumber, err)
				}
				object["parent_id"] = releasedStoryParentValue(next)
				changed = true
			}
		}
		if changed {
			line, err = json.Marshal(object)
			if err != nil {
				_ = temp.Close()
				return fmt.Errorf("encode migrated story journal line %d: %w", lineNumber, err)
			}
		}
		if _, err := writer.Write(line); err != nil {
			_ = temp.Close()
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			_ = temp.Close()
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = temp.Close()
		return err
	}
	// Close the source before replacing it so the atomic rename also works on
	// platforms that do not allow an open file to be replaced.
	if err := input.Close(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := writer.Flush(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return localfs.SyncDirectory(filepath.Dir(path))
}

func reconnectReleasedStoryBranches(meta map[string]any, removed, resolved map[string]string) bool {
	branches, ok := meta["branches"].(map[string]any)
	if !ok {
		return false
	}
	changed := false
	for _, value := range branches {
		branch, ok := value.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"head", "from_event"} {
			current, ok := branch[field].(string)
			if !ok {
				continue
			}
			current = strings.TrimSpace(current)
			if _, wasRemoved := removed[current]; !wasRemoved {
				continue
			}
			branch[field] = resolved[current]
			changed = true
		}
	}
	return changed
}

func (s *Store) backupReleasedStoryCompactionJournal(sourcePath string) (string, error) {
	dataRoot := strings.TrimSpace(s.novaDir)
	if dataRoot == "" {
		dataRoot = filepath.Join(s.root, ".denova")
	}
	backupRoot := filepath.Join(dataRoot, "backups", legacyStoryCompactionMigrationName)
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return "", err
	}
	backupDir, err := os.MkdirTemp(backupRoot, time.Now().UTC().Format("20060102T150405.000000000Z")+"-*")
	if err != nil {
		return "", err
	}
	backupPath := filepath.Join(backupDir, filepath.Base(sourcePath))
	input, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	output, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = input.Close()
		return "", err
	}
	_, copyErr := io.Copy(output, input)
	inputCloseErr := input.Close()
	syncErr := output.Sync()
	outputCloseErr := output.Close()
	if err := errors.Join(copyErr, inputCloseErr, syncErr, outputCloseErr); err != nil {
		return "", err
	}
	if err := localfs.SyncDirectory(backupDir); err != nil {
		return "", err
	}
	if err := localfs.SyncDirectory(backupRoot); err != nil {
		return "", err
	}
	return backupPath, nil
}

func scanStoryJSONL(path string, visit func(int, []byte) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxStoryLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			return fmt.Errorf("story journal contains an empty record on line %d", lineNumber)
		}
		if err := visit(lineNumber, line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func decodeStoryMigrationObject(line []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	return object, nil
}

func releasedStoryParentID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var parentID string
	if err := json.Unmarshal(raw, &parentID); err != nil {
		return "", fmt.Errorf("parent_id must be a string or null: %w", err)
	}
	return strings.TrimSpace(parentID), nil
}

func releasedStoryParentValue(parentID string) any {
	if parentID == "" {
		return nil
	}
	return parentID
}

func isReleasedStoryCompactionEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case legacyStoryEventTypeContextCompaction, legacyStoryEventTypeContextCompactionRemoved:
		return true
	default:
		return false
	}
}
