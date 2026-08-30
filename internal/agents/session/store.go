package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	agentattachment "denova/internal/agents/attachment"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
	"denova/internal/localfs"
)

const maxResidentSessions = 32

// Store 管理会话的 JSONL 文件持久化。
type Store struct {
	dir      string
	mu       sync.Mutex
	cache    map[string]*Session
	used     map[string]uint64
	sequence uint64
	metadata map[string]cachedSessionMetadata
}

// NewStore 创建会话存储，目录不存在则自动创建。
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建会话目录失败: %w", err)
	}
	return &Store{
		dir:      dir,
		cache:    make(map[string]*Session),
		used:     make(map[string]uint64),
		metadata: make(map[string]cachedSessionMetadata),
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

// GetOrCreateWithRuntimeConfig creates a new session with an atomic initial
// snapshot, or durably initializes a legacy session that predates this field.
func (s *Store) GetOrCreateWithRuntimeConfig(id string, seed conversationconfig.Config, channel Channel) (*Session, error) {
	if err := validateSessionID(id); err != nil {
		return nil, err
	}
	if err := conversationconfig.ValidateShape(seed, seed.AgentKind); err != nil {
		return nil, err
	}
	resolvedChannel, err := ParseChannel(string(channel))
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	sess, err := s.getOrCreateLockedWithRuntimeConfig(id, &seed, resolvedChannel)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if sess.Channel != resolvedChannel {
		return nil, fmt.Errorf("session channel mismatch: have=%q want=%q", sess.Channel, resolvedChannel)
	}
	if _, err := sess.EnsureRuntimeConfig(seed); err != nil {
		return nil, err
	}
	return sess, nil
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

// Exists reports whether a durable session identity is already present.
func (s *Store) Exists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exists(id)
}

// Create 创建一个新的会话。
func (s *Store) Create(title string) (*Session, error) {
	return s.create(title, nil, ChannelAgent)
}

// CreateWithRuntimeConfig persists the initial runtime snapshot in the
// immutable header, so a newly returned session is never temporarily global.
func (s *Store) CreateWithRuntimeConfig(title string, seed conversationconfig.Config, channel Channel) (*Session, error) {
	if err := conversationconfig.ValidateShape(seed, seed.AgentKind); err != nil {
		return nil, err
	}
	resolvedChannel, err := ParseChannel(string(channel))
	if err != nil {
		return nil, err
	}
	return s.create(title, &seed, resolvedChannel)
}

func (s *Store) create(title string, seed *conversationconfig.Config, channel Channel) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 0; i < 5; i++ {
		id := newSessionID()
		filePath := s.sessionPath(id)
		if _, err := os.Stat(filePath); err == nil {
			continue
		}
		sess, err := createSessionWithRuntimeConfig(id, filePath, title, seed, channel)
		if err != nil {
			return nil, err
		}
		s.cacheSessionLocked(id, sess)
		return sess, nil
	}
	return nil, fmt.Errorf("生成会话 ID 失败")
}

// RecentRuntimeConfig returns the latest snapshot for one Agent kind. Session
// metadata is projection-backed, so this does not materialize old transcripts.
func (s *Store) RecentRuntimeConfig(agentKind, excludeID string) (conversationconfig.Config, bool, error) {
	metas, err := s.List("")
	if err != nil {
		return conversationconfig.Config{}, false, err
	}
	for _, meta := range metas {
		if meta.ID == excludeID || meta.RuntimeConfig == nil {
			continue
		}
		if meta.RuntimeConfig.AgentKind == agentKind {
			return meta.RuntimeConfig.Config, true, nil
		}
	}
	return conversationconfig.Config{}, false, nil
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
	visible := make(map[string]struct{}, len(files))
	for _, file := range files {
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		visible[id] = struct{}{}
		meta, err := s.metadataLocked(id, file, activeID)
		if err != nil {
			// One obsolete or damaged Session must not hide every healthy
			// conversation. The canonical journal remains untouched and the
			// exact failure stays observable for manual repair.
			slog.Error("[session.Store.List] skipped unreadable Session metadata", "session_id", id, "path", file, "error", err)
			continue
		}
		result = append(result, meta)
	}
	s.discardMissingMetadataLocked(visible)
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
		meta, err := s.metadataLocked(id, file, activeID)
		if err != nil {
			slog.Error("[session.Store.ListByPrefix] skipped unreadable Session metadata", "session_id", id, "path", file, "error", err)
			continue
		}
		result = append(result, meta)
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
	if err := s.closeCachedLocked(id); err != nil {
		return fmt.Errorf("关闭待删除会话失败: %w", err)
	}
	if err := removeSessionJournal(s.sessionPath(id)); err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	delete(s.metadata, id)
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
		if err := s.closeCachedLocked(id); err != nil {
			return fmt.Errorf("关闭待删除会话失败: %w", err)
		}
		if err := removeSessionJournal(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("删除会话失败: %w", err)
		}
		delete(s.metadata, id)
	}
	return nil
}

// Close flushes every cached derived index. Stores remain reopenable because
// workspace-transition failure recovery may keep the same Store value alive.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var result error
	for id := range s.cache {
		result = errors.Join(result, s.closeCachedLocked(id))
	}
	return result
}

func (s *Store) closeCachedLocked(id string) error {
	sess := s.cache[strings.TrimSpace(id)]
	delete(s.cache, strings.TrimSpace(id))
	delete(s.used, strings.TrimSpace(id))
	if sess == nil {
		return nil
	}
	return sess.Close()
}

// removeSessionJournal shares the exact append/domain-commit lease. Holding
// Store.mu keeps this Store's cache transition atomic; independent stores and
// processes rendezvous on the file lease before unlinking the canonical inode.
func removeSessionJournal(path string) (resultErr error) {
	release, err := localfs.AcquireLease(context.Background(), path+".domain.lock")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, release()) }()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除会话索引失败: %w", err)
	}
	if err := os.RemoveAll(sessionToolArtifactDirectory(path)); err != nil {
		return fmt.Errorf("删除会话工具产物失败: %w", err)
	}
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	stateRoot := filepath.Dir(filepath.Dir(path))
	if err := agentattachment.RemoveScope(stateRoot, agentattachment.SessionScope(sessionID)); err != nil {
		return fmt.Errorf("delete Session attachments: %w", err)
	}
	if err := syncParentDirectory(path); err != nil {
		return fmt.Errorf("同步会话删除目录失败: %w", err)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	previousID, _ := s.activeIDUnlocked()
	data, err := json.MarshalIndent(activeSessionState{ActiveID: id}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.activePath(), data, 0o644); err != nil {
		return err
	}
	if previousID != "" && previousID != id {
		return s.closeCachedLocked(previousID)
	}
	return nil
}

func (s *Store) activeIDUnlocked() (string, error) {
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

func (s *Store) getOrCreateLocked(id string) (*Session, error) {
	return s.getOrCreateLockedWithRuntimeConfig(id, nil, ChannelAgent)
}

func (s *Store) getOrCreateLockedWithRuntimeConfig(id string, seed *conversationconfig.Config, channel Channel) (*Session, error) {
	if sess, ok := s.cache[id]; ok {
		s.touchSessionLocked(id)
		return sess, nil
	}

	filePath := s.sessionPath(id)
	var (
		sess *Session
		err  error
	)
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		sess, err = createSessionWithRuntimeConfig(id, filePath, defaultSessionTitle, seed, channel)
		if errors.Is(err, os.ErrExist) {
			// Another Store/process won deterministic session creation after our
			// stat. Load its canonical header; EnsureRuntimeConfig below will
			// converge legacy initialization through the journal lease.
			sess, err = loadSession(filePath)
		}
	} else {
		sess, err = loadSession(filePath)
	}
	if err != nil {
		return nil, err
	}

	s.cacheSessionLocked(id, sess)
	return sess, nil
}

func (s *Store) loadLocked(id string) (*Session, error) {
	if sess, ok := s.cache[id]; ok {
		s.touchSessionLocked(id)
		return sess, nil
	}
	sess, err := loadSession(s.sessionPath(id))
	if err != nil {
		return nil, err
	}
	s.cacheSessionLocked(id, sess)
	return sess, nil
}

func (s *Store) cacheSessionLocked(id string, sess *Session) {
	s.cache[id] = sess
	s.touchSessionLocked(id)
	if len(s.cache) <= maxResidentSessions {
		return
	}
	victim := ""
	var oldest uint64
	for candidate := range s.cache {
		if candidate == id {
			continue
		}
		used := s.used[candidate]
		if victim == "" || used < oldest {
			victim, oldest = candidate, used
		}
	}
	// Capacity eviction only releases Store ownership. An in-flight caller may
	// safely finish with its Session; canonical mutations rendezvous through the
	// journal lease if the same identity is opened again.
	delete(s.cache, victim)
	delete(s.used, victim)
}

func (s *Store) touchSessionLocked(id string) {
	s.sequence++
	s.used[id] = s.sequence
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
