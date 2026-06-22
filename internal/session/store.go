package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

const (
	defaultSessionID             = "default"
	defaultSessionTitle          = "新会话"
	historyTypeMessage           = "message"
	historyTypeDisplay           = "display"
	historyTypeClear             = "clear"
	historyTypeInterrupt         = "interrupt"
	historyTypeCompaction        = "context_compaction"
	historyTypeCompactionRemoved = "context_compaction_removed"

	InterruptionPending  = "pending"
	InterruptionResolved = "resolved"
)

// HistoryEntry 表示用于前端展示的会话历史记录。
type HistoryEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	Name      string          `json:"name,omitempty"`
	Args      string          `json:"args,omitempty"`
	Status    string          `json:"status,omitempty"`
	Result    string          `json:"result,omitempty"`
	Message   *schema.Message `json:"-"`
	CreatedAt time.Time       `json:"created_at,omitempty"`

	RunID                string           `json:"run_id,omitempty"`
	AgentKind            string           `json:"agent_kind,omitempty"`
	PromptTokens         int              `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int              `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int              `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64          `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int              `json:"completion_tokens,omitempty"`
	ReasoningTokens      int              `json:"reasoning_tokens,omitempty"`
	TotalTokens          int              `json:"total_tokens,omitempty"`
	ModelCalls           int              `json:"model_calls,omitempty"`
	GeneratedBytes       int              `json:"generated_bytes,omitempty"`
	UsageCalls           []TokenUsageCall `json:"usage_calls,omitempty"`
}

type historyRecord struct {
	kind              string
	message           *schema.Message
	display           *DisplayEvent
	interruption      *Interruption
	compaction        *ContextCompaction
	compactionRemoval *ContextCompactionRemoval
	createdAt         time.Time
}

// DisplayEvent 表示只用于前端展示的非上下文事件，例如 thinking 和工具卡片。
type DisplayEvent struct {
	ID        string    `json:"id,omitempty"`
	Role      string    `json:"role"`
	Content   string    `json:"content,omitempty"`
	Name      string    `json:"name,omitempty"`
	Args      string    `json:"args,omitempty"`
	Status    string    `json:"status,omitempty"`
	Result    string    `json:"result,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`

	RunID                string           `json:"run_id,omitempty"`
	AgentKind            string           `json:"agent_kind,omitempty"`
	PromptTokens         int              `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int              `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int              `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64          `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int              `json:"completion_tokens,omitempty"`
	ReasoningTokens      int              `json:"reasoning_tokens,omitempty"`
	TotalTokens          int              `json:"total_tokens,omitempty"`
	ModelCalls           int              `json:"model_calls,omitempty"`
	GeneratedBytes       int              `json:"generated_bytes,omitempty"`
	UsageCalls           []TokenUsageCall `json:"usage_calls,omitempty"`
}

type TokenUsageCall struct {
	Index                int      `json:"index,omitempty"`
	CreatedAt            string   `json:"created_at,omitempty"`
	FinishReason         string   `json:"finish_reason,omitempty"`
	RequestedTools       []string `json:"requested_tools,omitempty"`
	AfterTools           []string `json:"after_tools,omitempty"`
	PromptTokens         int      `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int      `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int      `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64  `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int      `json:"completion_tokens,omitempty"`
	ReasoningTokens      int      `json:"reasoning_tokens,omitempty"`
	TotalTokens          int      `json:"total_tokens,omitempty"`
}

// Interruption 表示一次异常中断后可恢复的对话轮次。
type Interruption struct {
	ID               string     `json:"id"`
	Status           string     `json:"status"`
	UserMessage      string     `json:"user_message"`
	AssistantContent string     `json:"assistant_content,omitempty"`
	Reason           string     `json:"reason,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
}

// ContextCompaction records a model-visible summary epoch without modifying the
// raw user-facing transcript.
type ContextCompaction struct {
	Type                string    `json:"type"`
	ID                  string    `json:"id"`
	AgentKind           string    `json:"agent_kind,omitempty"`
	Epoch               int       `json:"epoch"`
	Summary             string    `json:"summary"`
	SourceStartIndex    int       `json:"source_start_index"`
	SourceEndIndex      int       `json:"source_end_index"`
	SourceMessageCount  int       `json:"source_message_count"`
	RetainedTurns       int       `json:"retained_turns"`
	TokensBefore        int       `json:"tokens_before"`
	TokensAfter         int       `json:"tokens_after"`
	TargetRatio         float64   `json:"target_ratio,omitempty"`
	ContextWindowTokens int       `json:"context_window_tokens"`
	Threshold           float64   `json:"threshold"`
	Reason              string    `json:"reason,omitempty"`
	Phase               string    `json:"phase,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// ContextCompactionRemoval soft-disables the active model-visible compaction
// without deleting raw transcript or historical compaction records.
type ContextCompactionRemoval struct {
	Type             string    `json:"type"`
	ID               string    `json:"id"`
	AgentKind        string    `json:"agent_kind,omitempty"`
	CompactionID     string    `json:"compaction_id,omitempty"`
	SourceStartIndex int       `json:"source_start_index"`
	SourceEndIndex   int       `json:"source_end_index"`
	Reason           string    `json:"reason,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Session 保存单个会话的内存状态。
type Session struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time

	filePath        string
	title           string
	clearAfterIndex int
	mu              sync.Mutex
	messages        []*schema.Message
	records         []historyRecord
}

// Append 追加消息并持久化到磁盘。
func (s *Session) Append(msg *schema.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, msg)
	s.records = append(s.records, historyRecord{kind: historyTypeMessage, message: msg})
	s.touchLocked()
	if s.title == defaultSessionTitle && msg.Role == schema.User && strings.TrimSpace(msg.Content) != "" {
		s.title = deriveTitle(msg.Content)
	}

	return s.persistLocked()
}

// AppendDisplayEvent 追加仅用于前端展示的事件，不进入 Agent 有效上下文。
func (s *Session) AppendDisplayEvent(event DisplayEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(event.Role) == "" {
		return fmt.Errorf("展示事件 role 不能为空")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	s.records = append(s.records, historyRecord{kind: historyTypeDisplay, display: &event, createdAt: event.CreatedAt})
	s.UpdatedAt = event.CreatedAt
	return s.persistLocked()
}

// UpdateDisplayToolStatus 更新已持久化工具卡片的执行状态，不保存工具参数或输出。
func (s *Session) UpdateDisplayToolStatus(id, name, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].kind != historyTypeDisplay || s.records[i].display == nil || s.records[i].display.Role != "tool_call" {
			continue
		}
		if id != "" && s.records[i].display.ID != id {
			continue
		}
		if id == "" && name != "" && s.records[i].display.Name != name {
			continue
		}
		s.records[i].display.Status = status
		s.UpdatedAt = time.Now().UTC()
		return s.persistLocked()
	}
	return nil
}

// AppendDisplayToolArgs appends streamed tool arguments to a persisted tool card.
func (s *Session) AppendDisplayToolArgs(id, name, delta string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if delta == "" {
		return nil
	}
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].kind != historyTypeDisplay || s.records[i].display == nil || s.records[i].display.Role != "tool_call" {
			continue
		}
		if id != "" && s.records[i].display.ID != id {
			continue
		}
		if id == "" && name != "" && s.records[i].display.Name != name {
			continue
		}
		s.records[i].display.Args += delta
		s.UpdatedAt = time.Now().UTC()
		return s.persistLocked()
	}
	return nil
}

// UpdateDisplayToolResult stores the result preview for a persisted tool card.
func (s *Session) UpdateDisplayToolResult(id, name, status, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].kind != historyTypeDisplay || s.records[i].display == nil || s.records[i].display.Role != "tool_call" {
			continue
		}
		if id != "" && s.records[i].display.ID != id {
			continue
		}
		if id == "" && name != "" && s.records[i].display.Name != name {
			continue
		}
		s.records[i].display.Status = status
		s.records[i].display.Result = result
		s.UpdatedAt = time.Now().UTC()
		return s.persistLocked()
	}
	return nil
}

// AppendClearMarker 追加上下文清理标记，不删除历史消息。
func (s *Session) AppendClearMarker() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.clearAfterIndex = len(s.messages)
	s.records = append(s.records, historyRecord{kind: historyTypeClear, createdAt: now})
	s.UpdatedAt = now
	return s.persistLocked()
}

// AppendContextCompaction persists a compaction epoch. It intentionally does
// not append to messages, so user-visible history stays uncompressed.
func (s *Session) AppendContextCompaction(record ContextCompaction) (ContextCompaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	record.Type = historyTypeCompaction
	if strings.TrimSpace(record.ID) == "" {
		record.ID = newContextCompactionID()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.Epoch <= 0 {
		record.Epoch = s.nextCompactionEpochLocked(record.AgentKind)
	}
	if record.SourceEndIndex <= 0 || record.SourceEndIndex > len(s.messages) {
		record.SourceEndIndex = len(s.messages)
	}
	if record.SourceStartIndex < s.clearAfterIndex {
		record.SourceStartIndex = s.clearAfterIndex
	}
	if record.SourceStartIndex > record.SourceEndIndex {
		record.SourceStartIndex = record.SourceEndIndex
	}
	if record.SourceMessageCount <= 0 {
		record.SourceMessageCount = record.SourceEndIndex - record.SourceStartIndex
	}
	s.records = append(s.records, historyRecord{kind: historyTypeCompaction, compaction: &record, createdAt: record.CreatedAt})
	s.UpdatedAt = record.CreatedAt
	return record, s.persistLocked()
}

// RemoveLatestContextCompaction soft-disables the latest active compaction for
// an agent. Raw messages remain untouched so context can reconnect to history.
func (s *Session) RemoveLatestContextCompaction(agentKind, reason string) (ContextCompactionRemoval, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	compaction, ok := s.latestActiveContextCompactionLocked(agentKind)
	if !ok {
		return ContextCompactionRemoval{}, false, nil
	}
	now := time.Now().UTC()
	record := ContextCompactionRemoval{
		Type:             historyTypeCompactionRemoved,
		ID:               newContextCompactionRemovalID(),
		AgentKind:        compaction.AgentKind,
		CompactionID:     compaction.ID,
		SourceStartIndex: compaction.SourceStartIndex,
		SourceEndIndex:   compaction.SourceEndIndex,
		Reason:           strings.TrimSpace(reason),
		CreatedAt:        now,
	}
	if strings.TrimSpace(record.AgentKind) == "" {
		record.AgentKind = strings.TrimSpace(agentKind)
	}
	s.records = append(s.records, historyRecord{kind: historyTypeCompactionRemoved, compactionRemoval: &record, createdAt: record.CreatedAt})
	s.UpdatedAt = record.CreatedAt
	return record, true, s.persistLocked()
}

// MarkInterrupted 记录一次异常中断，供用户后续明确要求继续时恢复。
func (s *Session) MarkInterrupted(userMessage, assistantContent, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	record := &Interruption{
		ID:               newInterruptionID(),
		Status:           InterruptionPending,
		UserMessage:      strings.TrimSpace(userMessage),
		AssistantContent: assistantContent,
		Reason:           strings.TrimSpace(reason),
		CreatedAt:        now,
	}
	s.records = append(s.records, historyRecord{kind: historyTypeInterrupt, interruption: record, createdAt: now})
	s.UpdatedAt = now
	return s.persistLocked()
}

// PendingInterruption 返回最近一条待恢复的异常中断记录。
func (s *Session) PendingInterruption() *Interruption {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.kind != historyTypeInterrupt || record.interruption == nil {
			continue
		}
		if record.interruption.Status == InterruptionPending {
			copied := *record.interruption
			return &copied
		}
	}
	return nil
}

// ResolveInterruption 标记异常中断已被恢复处理。
func (s *Session) ResolveInterruption(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	for _, record := range s.records {
		if record.kind != historyTypeInterrupt || record.interruption == nil {
			continue
		}
		if record.interruption.ID == id {
			record.interruption.Status = InterruptionResolved
			record.interruption.ResolvedAt = &now
			s.UpdatedAt = now
			return s.persistLocked()
		}
	}
	return fmt.Errorf("异常中断记录不存在: %s", id)
}

// GetMessages 返回所有消息的快照。
func (s *Session) GetMessages() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*schema.Message, len(s.messages))
	copy(result, s.messages)
	return result
}

// GetEffectiveMessages 返回最后一个清理标记之后的 Agent 有效上下文。
func (s *Session) GetEffectiveMessages() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*schema.Message, len(s.messages)-s.clearAfterIndex)
	copy(result, s.messages[s.clearAfterIndex:])
	return result
}

// LatestContextCompaction returns the newest compaction epoch after the latest
// clear marker for the given agent kind.
func (s *Session) LatestContextCompaction(agentKind string) (ContextCompaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.latestActiveContextCompactionLocked(agentKind)
}

// LatestContextCompactionRemoval returns the newest removal marker after the
// latest clear marker for the given agent kind.
func (s *Session) LatestContextCompactionRemoval(agentKind string) (ContextCompactionRemoval, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.kind != historyTypeCompactionRemoved || record.compactionRemoval == nil {
			continue
		}
		removal := *record.compactionRemoval
		if removal.SourceEndIndex <= s.clearAfterIndex {
			continue
		}
		if strings.TrimSpace(agentKind) != "" && strings.TrimSpace(removal.AgentKind) != "" && removal.AgentKind != agentKind {
			continue
		}
		return removal, true
	}
	return ContextCompactionRemoval{}, false
}

func (s *Session) NextContextCompactionEpoch(agentKind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextCompactionEpochLocked(agentKind)
}

func (s *Session) latestActiveContextCompactionLocked(agentKind string) (ContextCompaction, bool) {
	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.kind == historyTypeCompactionRemoved && record.compactionRemoval != nil {
			removal := *record.compactionRemoval
			if removal.SourceEndIndex <= s.clearAfterIndex {
				continue
			}
			if strings.TrimSpace(agentKind) == "" || strings.TrimSpace(removal.AgentKind) == "" || removal.AgentKind == agentKind {
				return ContextCompaction{}, false
			}
			continue
		}
		if record.kind != historyTypeCompaction || record.compaction == nil {
			continue
		}
		compaction := *record.compaction
		if compaction.SourceEndIndex <= s.clearAfterIndex {
			continue
		}
		if strings.TrimSpace(agentKind) != "" && strings.TrimSpace(compaction.AgentKind) != "" && compaction.AgentKind != agentKind {
			continue
		}
		return compaction, true
	}
	return ContextCompaction{}, false
}

// MessageCountSinceClear returns the number of effective raw transcript
// messages after the latest clear marker.
func (s *Session) MessageCountSinceClear() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages) - s.clearAfterIndex
}

// MessageCountTotal returns the raw persisted message count.
func (s *Session) MessageCountTotal() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

// History 返回包含 clear 标记的完整会话历史。
func (s *Session) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]HistoryEntry, 0, len(s.records))
	for _, record := range s.records {
		switch record.kind {
		case historyTypeClear:
			result = append(result, HistoryEntry{Type: historyTypeClear, CreatedAt: record.createdAt})
		case historyTypeMessage:
			if record.message == nil {
				continue
			}
			result = append(result, HistoryEntry{
				Type:    historyTypeMessage,
				Role:    string(record.message.Role),
				Content: record.message.Content,
				Message: record.message,
			})
		case historyTypeDisplay:
			if record.display == nil {
				continue
			}
			result = append(result, HistoryEntry{
				Type:                 historyTypeMessage,
				ID:                   record.display.ID,
				Role:                 record.display.Role,
				Content:              record.display.Content,
				Name:                 record.display.Name,
				Args:                 record.display.Args,
				Status:               record.display.Status,
				Result:               record.display.Result,
				CreatedAt:            record.display.CreatedAt,
				RunID:                record.display.RunID,
				AgentKind:            record.display.AgentKind,
				PromptTokens:         record.display.PromptTokens,
				CachedPromptTokens:   record.display.CachedPromptTokens,
				UncachedPromptTokens: record.display.UncachedPromptTokens,
				CacheHitRate:         record.display.CacheHitRate,
				CompletionTokens:     record.display.CompletionTokens,
				ReasoningTokens:      record.display.ReasoningTokens,
				TotalTokens:          record.display.TotalTokens,
				ModelCalls:           record.display.ModelCalls,
				GeneratedBytes:       record.display.GeneratedBytes,
				UsageCalls:           cloneTokenUsageCalls(record.display.UsageCalls),
			})
		}
	}
	return result
}

func cloneTokenUsageCalls(calls []TokenUsageCall) []TokenUsageCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]TokenUsageCall, len(calls))
	copy(result, calls)
	for i := range result {
		result[i].RequestedTools = append([]string(nil), result[i].RequestedTools...)
		result[i].AfterTools = append([]string(nil), result[i].AfterTools...)
	}
	return result
}

// Clear 兼容旧调用语义：追加 clear 标记，不物理删除消息。
func (s *Session) Clear() error {
	return s.AppendClearMarker()
}

// Rename 更新会话标题并持久化。
func (s *Session) Rename(title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("会话标题不能为空")
	}
	s.title = title
	s.touchLocked()
	return s.persistLocked()
}

// Title 返回持久化会话标题。
func (s *Session) Title() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.titleLocked()
}

// MessageCount 返回消息数量。
func (s *Session) MessageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func (s *Session) titleLocked() string {
	if strings.TrimSpace(s.title) != "" {
		return s.title
	}
	return defaultSessionTitle
}

func (s *Session) touchLocked() {
	s.UpdatedAt = time.Now().UTC()
}

func (s *Session) persistLocked() error {
	header := sessionHeader{
		Type:      "session",
		ID:        s.ID,
		Title:     s.titleLocked(),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}

	var sb strings.Builder
	if err := writeJSONLine(&sb, header); err != nil {
		return err
	}
	for _, record := range s.records {
		switch record.kind {
		case historyTypeClear:
			if err := writeJSONLine(&sb, clearRecord{Type: historyTypeClear, CreatedAt: record.createdAt}); err != nil {
				return err
			}
		case historyTypeInterrupt:
			if record.interruption == nil {
				continue
			}
			if err := writeJSONLine(&sb, interruptionRecord{Type: historyTypeInterrupt, Interruption: *record.interruption}); err != nil {
				return err
			}
		case historyTypeCompaction:
			if record.compaction == nil {
				continue
			}
			if err := writeJSONLine(&sb, *record.compaction); err != nil {
				return err
			}
		case historyTypeCompactionRemoved:
			if record.compactionRemoval == nil {
				continue
			}
			if err := writeJSONLine(&sb, *record.compactionRemoval); err != nil {
				return err
			}
		case historyTypeDisplay:
			if record.display == nil {
				continue
			}
			if err := writeJSONLine(&sb, displayRecord{Type: historyTypeDisplay, DisplayEvent: *record.display}); err != nil {
				return err
			}
		case historyTypeMessage:
			if record.message == nil {
				continue
			}
			if err := writeJSONLine(&sb, record.message); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(s.filePath, []byte(sb.String()), 0o644)
}

// SessionMeta 是会话列表摘要。
type SessionMeta struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Active       bool      `json:"active"`
	MessageCount int       `json:"message_count"`
}

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

// ListByPrefix 返回 ID 匹配指定前缀的会话摘要，用于互动模式按子模式筛选会话。
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

// sessionHeader JSONL 文件首行的元数据。
type sessionHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type clearRecord struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

type interruptionRecord struct {
	Type string `json:"type"`
	Interruption
}

type displayRecord struct {
	Type string `json:"type"`
	DisplayEvent
}

type activeSessionState struct {
	ActiveID string `json:"active_id"`
}

func createSession(id, filePath, title string) (*Session, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(title) == "" {
		title = defaultSessionTitle
	}
	header := sessionHeader{
		Type:      "session",
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	var sb strings.Builder
	if err := writeJSONLine(&sb, header); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, []byte(sb.String()), 0o644); err != nil {
		return nil, err
	}
	return &Session{
		ID:              id,
		CreatedAt:       now,
		UpdatedAt:       now,
		filePath:        filePath,
		title:           title,
		clearAfterIndex: 0,
		messages:        make([]*schema.Message, 0),
		records:         make([]historyRecord, 0),
	}, nil
}

func loadSession(filePath string) (*Session, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if !scanner.Scan() {
		return nil, fmt.Errorf("会话文件为空: %s", filePath)
	}

	id := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	now := time.Now().UTC()
	sess := &Session{
		ID:              id,
		CreatedAt:       now,
		UpdatedAt:       now,
		filePath:        filePath,
		title:           defaultSessionTitle,
		clearAfterIndex: 0,
		messages:        make([]*schema.Message, 0),
		records:         make([]historyRecord, 0),
	}

	firstLine := strings.TrimSpace(scanner.Text())
	var header sessionHeader
	if err := json.Unmarshal([]byte(firstLine), &header); err == nil && header.Type == "session" {
		sess.ID = firstNonEmpty(header.ID, id)
		sess.CreatedAt = header.CreatedAt
		if sess.CreatedAt.IsZero() {
			sess.CreatedAt = now
		}
		sess.UpdatedAt = header.UpdatedAt
		if sess.UpdatedAt.IsZero() {
			sess.UpdatedAt = sess.CreatedAt
		}
		if strings.TrimSpace(header.Title) != "" {
			sess.title = header.Title
		}
	} else if err := appendMessageLine(sess, firstLine); err != nil {
		return nil, fmt.Errorf("会话头部解析失败 %s: %w", filePath, err)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		_ = appendRecordLine(sess, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if sess.title == defaultSessionTitle {
		for _, msg := range sess.messages {
			if msg.Role == schema.User && strings.TrimSpace(msg.Content) != "" {
				sess.title = deriveTitle(msg.Content)
				break
			}
		}
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = sess.CreatedAt
	}
	return sess, nil
}

func appendRecordLine(sess *Session, line string) error {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &typed); err == nil && typed.Type == historyTypeClear {
		var marker clearRecord
		if err := json.Unmarshal([]byte(line), &marker); err != nil {
			return err
		}
		sess.clearAfterIndex = len(sess.messages)
		sess.records = append(sess.records, historyRecord{kind: historyTypeClear, createdAt: marker.CreatedAt})
		if marker.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = marker.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeInterrupt {
		var marker interruptionRecord
		if err := json.Unmarshal([]byte(line), &marker); err != nil {
			return err
		}
		interruption := marker.Interruption
		if strings.TrimSpace(interruption.ID) == "" {
			interruption.ID = newInterruptionID()
		}
		if strings.TrimSpace(interruption.Status) == "" {
			interruption.Status = InterruptionPending
		}
		sess.records = append(sess.records, historyRecord{kind: historyTypeInterrupt, interruption: &interruption, createdAt: interruption.CreatedAt})
		if interruption.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = interruption.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeCompaction {
		var record ContextCompaction
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return err
		}
		if strings.TrimSpace(record.ID) == "" {
			record.ID = newContextCompactionID()
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = sess.UpdatedAt
		}
		if record.Type == "" {
			record.Type = historyTypeCompaction
		}
		sess.records = append(sess.records, historyRecord{kind: historyTypeCompaction, compaction: &record, createdAt: record.CreatedAt})
		if record.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = record.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeCompactionRemoved {
		var record ContextCompactionRemoval
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return err
		}
		if strings.TrimSpace(record.ID) == "" {
			record.ID = newContextCompactionRemovalID()
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = sess.UpdatedAt
		}
		if record.Type == "" {
			record.Type = historyTypeCompactionRemoved
		}
		sess.records = append(sess.records, historyRecord{kind: historyTypeCompactionRemoved, compactionRemoval: &record, createdAt: record.CreatedAt})
		if record.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = record.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeDisplay {
		var marker displayRecord
		if err := json.Unmarshal([]byte(line), &marker); err != nil {
			return err
		}
		event := marker.DisplayEvent
		if event.CreatedAt.IsZero() {
			event.CreatedAt = sess.UpdatedAt
		}
		sess.records = append(sess.records, historyRecord{kind: historyTypeDisplay, display: &event, createdAt: event.CreatedAt})
		if event.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = event.CreatedAt
		}
		return nil
	}
	return appendMessageLine(sess, line)
}

func appendMessageLine(sess *Session, line string) error {
	var msg schema.Message
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return err
	}
	if msg.Role == "" && msg.Content == "" {
		return nil
	}
	sess.messages = append(sess.messages, &msg)
	sess.records = append(sess.records, historyRecord{kind: historyTypeMessage, message: &msg})
	return nil
}

func (s *Session) nextCompactionEpochLocked(agentKind string) int {
	epoch := 0
	for _, record := range s.records {
		if record.kind != historyTypeCompaction || record.compaction == nil {
			continue
		}
		if strings.TrimSpace(agentKind) != "" && strings.TrimSpace(record.compaction.AgentKind) != "" && record.compaction.AgentKind != agentKind {
			continue
		}
		if record.compaction.Epoch > epoch {
			epoch = record.compaction.Epoch
		}
	}
	return epoch + 1
}

func writeJSONLine(sb *strings.Builder, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sb.Write(data)
	sb.WriteByte('\n')
	return nil
}

func validateSessionID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("会话 ID 不能为空")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("会话 ID 包含非法字符: %s", id)
	}
	return nil
}

func newSessionID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "s-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("s-%d", time.Now().UTC().UnixNano())
}

func newInterruptionID() string {
	return strings.TrimPrefix(newSessionID(), "s-")
}

func newContextCompactionID() string {
	return "cc-" + strings.TrimPrefix(newSessionID(), "s-")
}

func newContextCompactionRemovalID() string {
	return "ccr-" + strings.TrimPrefix(newSessionID(), "s-")
}

func deriveTitle(content string) string {
	title := strings.TrimSpace(content)
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60]) + "..."
	}
	if title == "" {
		return defaultSessionTitle
	}
	return title
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
