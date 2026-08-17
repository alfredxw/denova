package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxSessionJournalHeaderBytes = 128 * 1024

func sessionHeaderIncarnation(header sessionHeader) string {
	if incarnationID := strings.TrimSpace(header.IncarnationID); incarnationID != "" {
		return incarnationID
	}
	return fmt.Sprintf(
		"legacy-header:%s:%s",
		strings.TrimSpace(header.ID),
		header.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
}

func legacyMessageJournalIncarnation(id string) string {
	return "legacy-message:" + strings.TrimSpace(id)
}

// readSessionJournalIncarnation reads only the immutable first record. Every
// mutation performs this check while holding the canonical journal lease, so a
// stale Session handle can never append into a file deleted and recreated with
// the same public session ID.
func readSessionJournalIncarnation(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4*1024), maxSessionJournalHeaderBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &typed); err != nil {
			return "", fmt.Errorf("parse session journal identity %s: %w", filePath, err)
		}
		if typed.Type == "" {
			id := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
			return legacyMessageJournalIncarnation(id), nil
		}
		if typed.Type != "session" {
			return "", fmt.Errorf("session journal identity record has type %q: %s", typed.Type, filePath)
		}
		var header sessionHeader
		if err := json.Unmarshal(line, &header); err != nil {
			return "", fmt.Errorf("parse session journal header identity %s: %w", filePath, err)
		}
		if strings.TrimSpace(header.ID) == "" {
			header.ID = strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
		}
		return sessionHeaderIncarnation(header), nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read session journal identity %s: %w", filePath, err)
	}
	return "", fmt.Errorf("session journal has no identity record: %s", filePath)
}
