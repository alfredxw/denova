package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Store 管理会话的 JSONL 文件持久化。
type Store struct {
	dir   string
	mu    sync.Mutex
	cache map[string]*Session
}

// NewStore 创建会话存储，目录不存在则自动创建。
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建会话目录失败: %w", err)
	}
	return &Store{
		dir:   dir,
		cache: make(map[string]*Session),
	}, nil
}

// GetOrCreate 获取指定 ID 的会话，不存在则创建。
func (s *Store) GetOrCreate(id string) (*Session, error) {
	if err := validateSessionID(id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateLocked(id)
}

// Get 获取指定 ID 的已存在会话。
func (s *Store) Get(id string) (*Session, error) {
	if err := validateSessionID(id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.exists(id) {
		return nil, fmt.Errorf("会话不存在: %s", id)
	}
	return s.loadLocked(id)
}

// Create 创建一个新的会话。
func (s *Store) Create(title string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 0; i < 5; i++ {
		id := newSessionID()
		filePath := s.sessionPath(id)
		if _, err := os.Stat(filePath); err == nil {
			continue
		}
		sess, err := createSession(id, filePath, title)
		if err != nil {
			return nil, err
		}
		s.cache[id] = sess
		return sess, nil
	}
	return nil, fmt.Errorf("生成会话 ID 失败")
}

// GetActiveOrCreate 返回最近激活会话，不存在时创建默认会话。
func (s *Store) GetActiveOrCreate() (*Session, error) {
	activeID, _ := s.ActiveID()
	if activeID == "" || !s.exists(activeID) {
		activeID = defaultSessionID
	}
	sess, err := s.GetOrCreate(activeID)
	if err != nil {
		return nil, err
	}
	if err := s.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	return sess, nil
}

// List 返回当前存储目录下的所有会话摘要。
func (s *Store) List(activeID string) ([]SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	result := make([]SessionMeta, 0, len(files))
	for _, file := range files {
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		sess, err := s.loadLocked(id)
		if err != nil {
			return nil, err
		}
		result = append(result, SessionMeta{
			ID:           sess.ID,
			Title:        sess.Title(),
			CreatedAt:    sess.CreatedAt,
			UpdatedAt:    sess.UpdatedAt,
			Active:       sess.ID == activeID,
			MessageCount: sess.MessageCount(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

// ListByPrefix 返回 ID 匹配指定前缀的会话摘要，用于游戏模式按子模式筛选会话。
func (s *Store) ListByPrefix(prefix string) ([]SessionMeta, error) {
	if err := validateSessionID(prefix); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	activeID, _ := s.ActiveID()
	files, err := filepath.Glob(filepath.Join(s.dir, prefix+"*.jsonl"))
	if err != nil {
		return nil, err
	}
	result := make([]SessionMeta, 0, len(files))
	for _, file := range files {
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		sess, err := s.loadLocked(id)
		if err != nil {
			return nil, err
		}
		result = append(result, SessionMeta{
			ID:           sess.ID,
			Title:        sess.Title(),
			CreatedAt:    sess.CreatedAt,
			UpdatedAt:    sess.UpdatedAt,
			Active:       sess.ID == activeID,
			MessageCount: sess.MessageCount(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

// Rename 修改指定会话标题。
func (s *Store) Rename(id, title string) error {
	sess, err := s.GetOrCreate(id)
	if err != nil {
		return err
	}
	return sess.Rename(title)
}

// Delete 删除指定会话文件。
func (s *Store) Delete(id string) error {
	if err := validateSessionID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	count, err := s.countLocked()
	if err != nil {
		return err
	}
	if count <= 1 {
		return fmt.Errorf("不能删除当前唯一会话")
	}
	delete(s.cache, id)
	if err := os.Remove(s.sessionPath(id)); err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	return nil
}

// DeleteByPrefix 删除 ID 匹配指定前缀的会话文件，用于删除互动故事线时级联清理会话。
func (s *Store) DeleteByPrefix(prefix string) error {
	if err := validateSessionID(prefix); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := filepath.Glob(filepath.Join(s.dir, prefix+"*.jsonl"))
	if err != nil {
		return err
	}
	for _, file := range files {
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		delete(s.cache, id)
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除会话失败: %w", err)
		}
	}
	return nil
}

// ActiveID 返回最近激活会话 ID。
func (s *Store) ActiveID() (string, error) {
	data, err := os.ReadFile(s.activePath())
	if err != nil {
		return "", err
	}
	var state activeSessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}
	return state.ActiveID, nil
}

// SetActiveID 持久化最近激活会话 ID。
func (s *Store) SetActiveID(id string) error {
	if err := validateSessionID(id); err != nil {
		return err
	}
	data, err := json.MarshalIndent(activeSessionState{ActiveID: id}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.activePath(), data, 0o644)
}

func (s *Store) getOrCreateLocked(id string) (*Session, error) {
	if sess, ok := s.cache[id]; ok {
		return sess, nil
	}

	filePath := s.sessionPath(id)
	var (
		sess *Session
		err  error
	)
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		sess, err = createSession(id, filePath, defaultSessionTitle)
	} else {
		sess, err = loadSession(filePath)
	}
	if err != nil {
		return nil, err
	}

	s.cache[id] = sess
	return sess, nil
}

func (s *Store) loadLocked(id string) (*Session, error) {
	if sess, ok := s.cache[id]; ok {
		return sess, nil
	}
	sess, err := loadSession(s.sessionPath(id))
	if err != nil {
		return nil, err
	}
	s.cache[id] = sess
	return sess, nil
}

func (s *Store) exists(id string) bool {
	if err := validateSessionID(id); err != nil {
		return false
	}
	_, err := os.Stat(s.sessionPath(id))
	return err == nil
}

func (s *Store) countLocked() (int, error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

func (s *Store) sessionPath(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

func (s *Store) activePath() string {
	return filepath.Join(s.dir, "active.json")
}

type activeSessionState struct {
	ActiveID string `json:"active_id"`
}
