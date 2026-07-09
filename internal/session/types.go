package session

import (
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

const (
	defaultSessionID             = "default"
	defaultSessionTitle          = "新会话"
	displayToolArgsPersistBytes  = 4 * 1024
	displayToolArgsPreviewBytes  = 32 * 1024
	displayToolArgsTruncatedHint = "\n...\n[session preview truncated / 会话预览已截断]"
	maxTokenUsageDisplayEvents   = 10
	historyTypeMessage           = "message"
	historyTypeContextMessage    = "context_message"
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
	Type         string               `json:"type"`
	ID           string               `json:"id,omitempty"`
	Role         string               `json:"role,omitempty"`
	Content      string               `json:"content,omitempty"`
	Name         string               `json:"name,omitempty"`
	Args         string               `json:"args,omitempty"`
	Status       string               `json:"status,omitempty"`
	Result       string               `json:"result,omitempty"`
	Illustration *ChapterIllustration `json:"illustration,omitempty"`
	Message      *schema.Message      `json:"-"`
	CreatedAt    time.Time            `json:"created_at,omitempty"`

	RunID                string           `json:"run_id,omitempty"`
	AgentKind            string           `json:"agent_kind,omitempty"`
	AgentName            string           `json:"agent_name,omitempty"`
	RootAgentName        string           `json:"root_agent_name,omitempty"`
	RunPath              []string         `json:"run_path,omitempty"`
	SubAgent             bool             `json:"subagent,omitempty"`
	SubAgentSessionID    string           `json:"subagent_session_id,omitempty"`
	SubAgentType         string           `json:"subagent_type,omitempty"`
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
	SSEHiddenFields      []string         `json:"sse_hidden_fields,omitempty"`
	SSEHiddenReason      string           `json:"sse_hidden_reason,omitempty"`
	SSEDisplayNotice     string           `json:"sse_display_notice,omitempty"`
	SSEGeneratedChars    int              `json:"sse_generated_chars,omitempty"`
}

type MessageMetadata struct {
	RunID             string   `json:"run_id,omitempty"`
	AgentKind         string   `json:"agent_kind,omitempty"`
	AgentName         string   `json:"agent_name,omitempty"`
	RootAgentName     string   `json:"root_agent_name,omitempty"`
	RunPath           []string `json:"run_path,omitempty"`
	SubAgent          bool     `json:"subagent,omitempty"`
	SubAgentSessionID string   `json:"subagent_session_id,omitempty"`
	SubAgentType      string   `json:"subagent_type,omitempty"`
}

type historyRecord struct {
	kind              string
	message           *schema.Message
	messageMetadata   MessageMetadata
	display           *DisplayEvent
	interruption      *Interruption
	compaction        *ContextCompaction
	compactionRemoval *ContextCompactionRemoval
	createdAt         time.Time

	displayArgsPersistedBytes int
}

type messageRecord struct {
	Type      string         `json:"type"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
	Message   schema.Message `json:"message"`
	MessageMetadata
}

// DisplayEvent 表示只用于前端展示的非上下文事件，例如 thinking 和工具卡片。
type DisplayEvent struct {
	ID           string               `json:"id,omitempty"`
	Role         string               `json:"role"`
	Content      string               `json:"content,omitempty"`
	Name         string               `json:"name,omitempty"`
	Args         string               `json:"args,omitempty"`
	Status       string               `json:"status,omitempty"`
	Result       string               `json:"result,omitempty"`
	Illustration *ChapterIllustration `json:"illustration,omitempty"`
	CreatedAt    time.Time            `json:"created_at,omitempty"`

	RunID                string           `json:"run_id,omitempty"`
	AgentKind            string           `json:"agent_kind,omitempty"`
	AgentName            string           `json:"agent_name,omitempty"`
	RootAgentName        string           `json:"root_agent_name,omitempty"`
	RunPath              []string         `json:"run_path,omitempty"`
	SubAgent             bool             `json:"subagent,omitempty"`
	SubAgentSessionID    string           `json:"subagent_session_id,omitempty"`
	SubAgentType         string           `json:"subagent_type,omitempty"`
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
	SSEHiddenFields      []string         `json:"sse_hidden_fields,omitempty"`
	SSEHiddenReason      string           `json:"sse_hidden_reason,omitempty"`
	SSEDisplayNotice     string           `json:"sse_display_notice,omitempty"`
	SSEGeneratedChars    int              `json:"sse_generated_chars,omitempty"`
}

type ChapterIllustration struct {
	Schema        string `json:"schema"`
	ChapterPath   string `json:"chapter_path"`
	ImagePath     string `json:"image_path"`
	MetaPath      string `json:"meta_path"`
	Markdown      string `json:"markdown"`
	AltText       string `json:"alt_text"`
	ProfileID     string `json:"profile_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
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
	Strategy            string    `json:"strategy,omitempty"`
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

// SessionMeta 是会话列表摘要。
type SessionMeta struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Active       bool      `json:"active"`
	MessageCount int       `json:"message_count"`
}
