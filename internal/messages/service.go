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

func (s *Service) List() (ListResult, error) {
	return s.ListForLocale("")
}

func (s *Service) ListForLocale(locale string) (ListResult, error) {
	items, err := s.changelogMessages(locale)
	if err != nil {
		return ListResult{}, err
	}
	stateMu.Lock()
	readState, err := s.readState()
	stateMu.Unlock()
	if err != nil {
		return ListResult{}, err
	}
	unread := applyReadState(items, readState)
	return ListResult{Items: items, UnreadCount: unread}, nil
}

func (s *Service) MarkRead(id string) (Message, error) {
	return s.MarkReadForLocale(id, "")
}

func (s *Service) MarkReadForLocale(id, locale string) (Message, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Message{}, fmt.Errorf("message id is required")
	}
	items, err := s.changelogMessages(locale)
	if err != nil {
		return Message{}, err
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	state, err := s.readState()
	if err != nil {
		return Message{}, err
	}
	applyReadState(items, state)
	var found *Message
	for i := range items {
		if items[i].ID == id {
			found = &items[i]
			break
		}
	}
	if found == nil {
		return Message{}, fmt.Errorf("message %s not found", id)
	}
	if found.ReadAt != nil {
		return *found, nil
	}
	now := time.Now().UTC()
	state[id] = now
	if err := s.writeState(state); err != nil {
		return Message{}, err
	}
	found.ReadAt = &now
	return *found, nil
}

func (s *Service) MarkAllRead() (ListResult, error) {
	return s.MarkAllReadForLocale("")
}

func (s *Service) MarkAllReadForLocale(locale string) (ListResult, error) {
	items, err := s.changelogMessages(locale)
	if err != nil {
		return ListResult{}, err
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	state, err := s.readState()
	if err != nil {
		return ListResult{}, err
	}
	now := time.Now().UTC()
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, ok := state[item.ID]; !ok {
			state[item.ID] = now
		}
	}
	if err := s.writeState(state); err != nil {
		return ListResult{}, err
	}
	applyReadState(items, state)
	return ListResult{Items: items, UnreadCount: 0}, nil
}

func applyReadState(items []Message, state map[string]time.Time) int {
	unread := 0
	for i := range items {
		if readAt, ok := state[items[i].ID]; ok {
			t := readAt
			items[i].ReadAt = &t
			continue
		}
		unread++
	}
	return unread
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
