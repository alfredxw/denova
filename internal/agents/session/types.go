package session

import (
	"sync"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
)

const (
	defaultSessionID               = "default"
	defaultSessionTitle            = "新会话"
	displayStreamPersistBatchBytes = 64 * 1024
	maxTokenUsageDisplayEvents     = 10
	historyTypeMessage             = "message"
	historyTypeContextMessage      = "context_message"
	historyTypeDisplay             = "display"
	historyTypeClear               = "clear"
	historyTypeInterrupt           = "interrupt"

	InterruptionPending  = "pending"
	InterruptionResolved = "resolved"
	AskPending           = "pending"
	AskAnswered          = "answered"
	AskCancelled         = "cancelled"
	AskKindQuestion      = "question"
	AskKindToolApproval  = "tool_approval"
	// Tool approval option IDs are stable transport values used by the current
	// web UI. Public Agent PermissionChoice remains the lifecycle vocabulary.
	ToolApprovalAllowOnceOptionID      = "allow-once"
	ToolApprovalAllowWorkspaceOptionID = "allow-workspace"
	ToolApprovalDenyOptionID           = "deny"

	// Display phases classify root assistant prose without changing the
	// canonical transcript that is assembled for the model.
	DisplayPhaseCandidate = "candidate"
	DisplayPhaseProgress  = "progress"
	DisplayPhaseFinal     = "final"
	DisplayPhasePartial   = "partial"
)

// HistoryEntry 表示用于前端展示的会话历史记录。
type HistoryEntry struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	// DisplaySegmentID distinguishes display transcript identity from a
	// canonical message ID when both are projected as ordinary history rows.
	DisplaySegmentID string                  `json:"display_segment_id,omitempty"`
	DisplayPhase     string                  `json:"display_phase,omitempty"`
	Role             string                  `json:"role,omitempty"`
	Content          string                  `json:"content,omitempty"`
	Name             string                  `json:"name,omitempty"`
	Args             string                  `json:"args,omitempty"`
	Status           string                  `json:"status,omitempty"`
	Result           string                  `json:"result,omitempty"`
	ToolPresentation *agent.ToolPresentation `json:"tool_presentation,omitempty"`
	Illustration     *ChapterIllustration    `json:"illustration,omitempty"`
	Ask              *AskInteraction         `json:"ask,omitempty"`
	Message          *agent.Message          `json:"-"`
	CreatedAt        time.Time               `json:"created_at,omitempty"`

	RunID                string                       `json:"run_id,omitempty"`
	AgentKind            string                       `json:"agent_kind,omitempty"`
	AgentName            string                       `json:"agent_name,omitempty"`
	RootAgentName        string                       `json:"root_agent_name,omitempty"`
	RunPath              []string                     `json:"run_path,omitempty"`
	SubAgent             bool                         `json:"subagent,omitempty"`
	SubAgentSessionID    string                       `json:"subagent_session_id,omitempty"`
	SubAgentType         string                       `json:"subagent_type,omitempty"`
	PromptTokens         int                          `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int                          `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int                          `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64                      `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int                          `json:"completion_tokens,omitempty"`
	ReasoningTokens      int                          `json:"reasoning_tokens,omitempty"`
	TotalTokens          int                          `json:"total_tokens,omitempty"`
	ModelCalls           int                          `json:"model_calls,omitempty"`
	GeneratedBytes       int                          `json:"generated_bytes,omitempty"`
	UsageCalls           []TokenUsageCall             `json:"usage_calls,omitempty"`
	RunStartedAt         string                       `json:"run_started_at,omitempty"`
	RunFinishedAt        string                       `json:"run_finished_at,omitempty"`
	DurationMS           int64                        `json:"duration_ms,omitempty"`
	RunStatus            string                       `json:"run_status,omitempty"`
	UserReferences       []agentcontext.UserReference `json:"user_references,omitempty"`
	AgentCommandID       string                       `json:"agent_command_id,omitempty"`
	AgentOperationID     string                       `json:"agent_operation_id,omitempty"`
	AgentCycle           int                          `json:"agent_cycle,omitempty"`
	DomainCommitHash     string                       `json:"domain_commit_hash,omitempty"`
	AgentCanonicalHash   string                       `json:"agent_canonical_hash,omitempty"`
	ContextRevision      uint64                       `json:"context_revision,omitempty"`
}

type MessageMetadata struct {
	// MessageID and the coordinator identity form the stable cross-domain key
	// for one Agent cycle. They are model-invisible but survive journal replay.
	MessageID        string `json:"message_id,omitempty"`
	AgentCommandID   string `json:"agent_command_id,omitempty"`
	AgentOperationID string `json:"agent_operation_id,omitempty"`
	AgentCycle       int    `json:"agent_cycle,omitempty"`
	DomainCommitHash string `json:"domain_commit_hash,omitempty"`
	// AgentCanonicalHash is the public Agent package's exact stage hash. It is
	// stored atomically with the product message, while DomainCommitHash guards
	// the richer Denova payload. Recovery must query this value, never infer a
	// successful Agent commit from cycle identity alone.
	AgentCanonicalHash string `json:"agent_canonical_hash,omitempty"`
	// ResolveInterruptionID is committed in the same journal transaction as
	// the canonical assistant output. It closes the crash window where output
	// was visible but its recovery marker remained pending.
	ResolveInterruptionID string                       `json:"resolve_interruption_id,omitempty"`
	ContextRevision       uint64                       `json:"context_revision,omitempty"`
	RunID                 string                       `json:"run_id,omitempty"`
	AgentKind             string                       `json:"agent_kind,omitempty"`
	AgentName             string                       `json:"agent_name,omitempty"`
	RootAgentName         string                       `json:"root_agent_name,omitempty"`
	RunPath               []string                     `json:"run_path,omitempty"`
	SubAgent              bool                         `json:"subagent,omitempty"`
	SubAgentSessionID     string                       `json:"subagent_session_id,omitempty"`
	SubAgentType          string                       `json:"subagent_type,omitempty"`
	UserReferences        []agentcontext.UserReference `json:"user_references,omitempty"`
	// ContextOnly keeps host-owned continuation instructions in model history
	// without projecting them as user-authored chat messages.
	ContextOnly bool `json:"context_only,omitempty"`
	// ProviderContinuation is process-local model-visible state copied onto the
	// canonical assistant Message. It is excluded from metadata serialization so
	// the protocol payload has exactly one durable source of truth.
	ProviderContinuation map[string]any `json:"-"`
}

type historyRecord struct {
	journalID                    string
	kind                         string
	message                      *agent.Message
	messageMetadata              MessageMetadata
	display                      *DisplayEvent
	interruption                 *Interruption
	createdAt                    time.Time
	displayArgsPersistedBytes    int
	displayContentPersistedBytes int
}

type messageRecord struct {
	Type      string        `json:"type"`
	CreatedAt time.Time     `json:"created_at,omitempty"`
	Message   agent.Message `json:"message"`
	MessageMetadata
}

// DisplayEvent 表示只用于前端展示的非上下文事件，例如 thinking 和工具卡片。
type DisplayEvent struct {
	ID               string                  `json:"id,omitempty"`
	Role             string                  `json:"role"`
	DisplayPhase     string                  `json:"display_phase,omitempty"`
	Content          string                  `json:"content,omitempty"`
	Name             string                  `json:"name,omitempty"`
	Args             string                  `json:"args,omitempty"`
	Status           string                  `json:"status,omitempty"`
	Result           string                  `json:"result,omitempty"`
	ToolPresentation *agent.ToolPresentation `json:"tool_presentation,omitempty"`
	Illustration     *ChapterIllustration    `json:"illustration,omitempty"`
	Ask              *AskInteraction         `json:"ask,omitempty"`
	CreatedAt        time.Time               `json:"created_at,omitempty"`

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
	RunStartedAt         string           `json:"run_started_at,omitempty"`
	RunFinishedAt        string           `json:"run_finished_at,omitempty"`
	DurationMS           int64            `json:"duration_ms,omitempty"`
	RunStatus            string           `json:"run_status,omitempty"`
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

// AskOption is one model-provided choice. "Other" is deliberately absent:
// every interactive host adds that localized choice itself.
type AskOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskQuestion is a stable, recoverable question shown by an interactive host.
// Empty Options means free text. MultiSelect is meaningful only with options.
type AskQuestion struct {
	ID                  string      `json:"id"`
	Question            string      `json:"question"`
	Options             []AskOption `json:"options,omitempty"`
	MultiSelect         bool        `json:"multi_select,omitempty"`
	RecommendedOptionID string      `json:"recommended_option_id,omitempty"`
}

// AskAnswer is accepted from the UI. Option IDs are checked against the
// persisted question before the answer becomes a model-visible result.
type AskAnswer struct {
	QuestionID        string   `json:"question_id"`
	SelectedOptionIDs []string `json:"selected_option_ids,omitempty"`
	CustomInput       string   `json:"custom_input,omitempty"`
}

type AskSelectedOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type AskAnswerResult struct {
	QuestionID      string              `json:"question_id"`
	Question        string              `json:"question"`
	SelectedOptions []AskSelectedOption `json:"selected_options,omitempty"`
	CustomInput     string              `json:"custom_input,omitempty"`
}

// ToolApprovalPresentation is host-generated display metadata. It is never
// accepted from a model tool schema and never enters model context.
type ToolApprovalPresentation struct {
	Mode               string `json:"mode"`
	ToolName           string `json:"tool_name"`
	Command            string `json:"command,omitempty"`
	Details            string `json:"details,omitempty"`
	Cwd                string `json:"cwd,omitempty"`
	Risk               string `json:"risk"`
	RuleID             string `json:"rule_id"`
	ArgsHash           string `json:"args_hash"`
	CanRemember        bool   `json:"can_remember,omitempty"`
	RuleMatcherVersion int    `json:"rule_matcher_version,omitempty"`
	RuleCommandKey     string `json:"rule_command_key,omitempty"`
	RuleCommandPattern string `json:"rule_command_pattern,omitempty"`
}

// AskInteraction is the read-only transport and display projection of one
// public Agent Interaction. Product Sessions may persist it only in the
// display journal; it never enters canonical messages or model context.
type AskInteraction struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	Kind   string `json:"kind,omitempty"`
	// ToolCallID is the durable execution ID used by lifecycle and display
	// correlation. ProviderCallID is transcript-only diagnostic metadata.
	ToolCallID     string `json:"tool_call_id"`
	ProviderCallID string `json:"provider_call_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	AgentKind      string `json:"agent_kind"`
	// AgentCommandID, AgentOperationID, and AgentCycle bind the blocking
	// interaction to the durable coordinator cycle that owned its tool call.
	// They are optional only for journals written before this correlation was
	// introduced.
	AgentCommandID   string                    `json:"agent_command_id,omitempty"`
	AgentOperationID string                    `json:"agent_operation_id,omitempty"`
	AgentCycle       int                       `json:"agent_cycle,omitempty"`
	Status           string                    `json:"status"`
	Questions        []AskQuestion             `json:"questions,omitempty"`
	AllowOther       bool                      `json:"allow_other,omitempty"`
	Approval         *ToolApprovalPresentation `json:"approval,omitempty"`
	Answers          []AskAnswerResult         `json:"answers,omitempty"`
	CancelReason     string                    `json:"cancel_reason,omitempty"`
	CreatedAt        time.Time                 `json:"created_at"`
	ResolvedAt       *time.Time                `json:"resolved_at,omitempty"`
}

// Session 保存单个会话的内存状态。
type Session struct {
	ID                    string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	runtimeConfig         *conversationconfig.Config
	runtimeConfigRevision uint64

	filePath               string
	title                  string
	clearAfterIndex        int
	contextRevision        uint64
	journalSize            int64
	journalOffset          int64
	journalIncarnation     string
	journalNeedsLF         bool
	journalLineCount       int
	lastReplayBytes        int64
	lastReplayRecords      int
	journal                *conversationjournal.Journal
	projection             *sessionJournalProjection
	materializedCursor     conversationjournal.Cursor
	messageBaseIndex       int
	messageCount           int
	historyBaseIndex       int
	partialMaterialization bool
	mu                     sync.Mutex
	messages               []*agent.Message
	records                []historyRecord
}

// SessionMeta 是会话列表摘要。
type SessionMeta struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Active       bool      `json:"active"`
	MessageCount int       `json:"message_count"`
	// RuntimeConfig is projection metadata used to seed new conversations. The
	// dedicated conversation-config API is the sole client mutation/read seam.
	RuntimeConfig *conversationconfig.Snapshot `json:"-"`
}
