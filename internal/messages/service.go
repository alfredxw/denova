package messages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const stateFileName = "state.json"

var stateMu sync.Mutex

// Service owns changelog parsing and read-state persistence. It does not merge
// dynamic messages — cross-source merging lives in the app layer
// (MessagesAppService), which composes changelog + automation notices before
// applying read state.
type Service struct {
	novaDir       string
	changelogPath string
}

type stateFile struct {
	Read map[string]string `json:"read,omitempty"`
}

func NewService(novaDir string) *Service {
	return &Service{novaDir: novaDir}
}

func NewServiceWithChangelog(novaDir, changelogPath string) *Service {
	return &Service{novaDir: novaDir, changelogPath: changelogPath}
}

// ChangelogForLocale returns parsed changelog messages for the locale. Read
// state is not applied here; the caller applies it to the merged list.
func (s *Service) ChangelogForLocale(locale string) ([]Message, error) {
	return s.changelogMessages(locale)
}

// ReadState returns the persisted read state (message id → read time).
func (s *Service) ReadState() (map[string]time.Time, error) {
	stateMu.Lock()
	defer stateMu.Unlock()
	return s.readState()
}

// MarkRead marks a single message id as read and persists the state. It works
// for any id (changelog or dynamic) since read state is keyed by id. Returns
// the read time that was stored.
func (s *Service) MarkRead(id string) (time.Time, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return time.Time{}, fmt.Errorf("message id is required")
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	state, err := s.readState()
	if err != nil {
		return time.Time{}, err
	}
	if t, ok := state[id]; ok {
		return t, nil
	}
	now := time.Now().UTC()
	state[id] = now
	if err := s.writeState(state); err != nil {
		return time.Time{}, err
	}
	return now, nil
}

// MarkAllRead marks all given message ids as read and persists the state.
func (s *Service) MarkAllRead(ids []string) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	state, err := s.readState()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := state[id]; ok {
			continue
		}
		state[id] = now
	}
	return s.writeState(state)
}

func (s *Service) changelogMessages(locale string) ([]Message, error) {
	path := s.resolveChangelogPath()
	if path == "" {
		return []Message{}, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Message{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read changelog failed: %w", err)
	}
	return parseChangelogMessagesForLocale(string(data), locale), nil
}

func (s *Service) resolveChangelogPath() string {
	candidates := []string{}
	if strings.TrimSpace(s.changelogPath) != "" {
		candidates = append(candidates, s.changelogPath)
	}
	if env := strings.TrimSpace(os.Getenv("DENOVA_CHANGELOG_PATH")); env != "" {
		candidates = append(candidates, env)
	} else if env := strings.TrimSpace(os.Getenv("NOVA_CHANGELOG_PATH")); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, "CHANGELOG.md")
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "CHANGELOG.md"),
			filepath.Join(exeDir, "..", "CHANGELOG.md"),
			filepath.Join(exeDir, "..", "..", "CHANGELOG.md"),
		)
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (s *Service) readState() (map[string]time.Time, error) {
	state := map[string]time.Time{}
	path, err := s.statePath()
	if err != nil {
		return state, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read message state failed: %w", err)
	}
	var file stateFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse message state failed: %w", err)
	}
	for id, value := range file.Read {
		t, err := time.Parse(time.RFC3339Nano, value)
		if err != nil || strings.TrimSpace(id) == "" {
			continue
		}
		state[id] = t
	}
	return state, nil
}

func (s *Service) writeState(state map[string]time.Time) error {
	path, err := s.statePath()
	if err != nil {
		return err
	}
	file := stateFile{Read: map[string]string{}}
	for id, value := range state {
		if strings.TrimSpace(id) == "" || value.IsZero() {
			continue
		}
		file.Read[id] = value.UTC().Format(time.RFC3339Nano)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (s *Service) statePath() (string, error) {
	if strings.TrimSpace(s.novaDir) == "" {
		return "", fmt.Errorf("nova dir is required")
	}
	return filepath.Join(s.novaDir, "messages", stateFileName), nil
}
