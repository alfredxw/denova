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
	displayStreamPersistBatchBytes = 4 * 1024
	maxTokenUsageDisplayEvents     = 10
	historyTypeMessage             = "message"
	historyTypeContextMessage      = "context_message"
	historyTypeDisplay             = "display"
	historyTypeClear               = "clear"
	historyTypeInterrupt           = "interrupt"
	historyTypeAsk                 = "ask"
	historyTypeContextBoundary     = "context_boundary"
	historyTypeCompaction          = "context_compaction"
	historyTypeCompactionRemoved   = "context_compaction_removed"
	historyTypeCompactionHealth    = "context_compaction_health"
	historyTypeToolResultCleanup   = "tool_result_cleanup"

	InterruptionPending  = "pending"
	InterruptionResolved = "resolved"
	AskPending           = "pending"
	AskAnswered          = "answered"
	AskCancelled         = "cancelled"
	AskKindQuestion      = "question"
	AskKindToolApproval  = "tool_approval"

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
	DisplaySegmentID string               `json:"display_segment_id,omitempty"`
	DisplayPhase     string               `json:"display_phase,omitempty"`
	Role             string               `json:"role,omitempty"`
	Content          string               `json:"content,omitempty"`
	Name             string               `json:"name,omitempty"`
	Args             string               `json:"args,omitempty"`
	Status           string               `json:"status,omitempty"`
	Result           string               `json:"result,omitempty"`
	Illustration     *ChapterIllustration `json:"illustration,omitempty"`
	Ask              *AskInteraction      `json:"ask,omitempty"`
	Message          *agent.Message       `json:"-"`
	CreatedAt        time.Time            `json:"created_at,omitempty"`

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
	SSEHiddenFields      []string                     `json:"sse_hidden_fields,omitempty"`
	SSEHiddenReason      string                       `json:"sse_hidden_reason,omitempty"`
	SSEDisplayNotice     string                       `json:"sse_display_notice,omitempty"`
	SSEGeneratedChars    int                          `json:"sse_generated_chars,omitempty"`
	UserReferences       []agentcontext.UserReference `json:"user_references,omitempty"`
	AgentCommandID       string                       `json:"agent_command_id,omitempty"`
	AgentOperationID     string                       `json:"agent_operation_id,omitempty"`
	AgentCycle           int                          `json:"agent_cycle,omitempty"`
	DomainCommitHash     string                       `json:"domain_commit_hash,omitempty"`
	ContextRevision      uint64                       `json:"context_revision,omitempty"`
}

type MessageMetadata struct {
	// MessageID and the coordinator identity form the stable cross-domain key
	// for one Agent cycle. They are model-invisible but survive journal replay.
	MessageID         string                       `json:"message_id,omitempty"`
	AgentCommandID    string                       `json:"agent_command_id,omitempty"`
	AgentOperationID  string                       `json:"agent_operation_id,omitempty"`
	AgentCycle        int                          `json:"agent_cycle,omitempty"`
	DomainCommitHash  string                       `json:"domain_commit_hash,omitempty"`
	ContextRevision   uint64                       `json:"context_revision,omitempty"`
	RunID             string                       `json:"run_id,omitempty"`
	AgentKind         string                       `json:"agent_kind,omitempty"`
	AgentName         string                       `json:"agent_name,omitempty"`
	RootAgentName     string                       `json:"root_agent_name,omitempty"`
	RunPath           []string                     `json:"run_path,omitempty"`
	SubAgent          bool                         `json:"subagent,omitempty"`
	SubAgentSessionID string                       `json:"subagent_session_id,omitempty"`
	SubAgentType      string                       `json:"subagent_type,omitempty"`
	UserReferences    []agentcontext.UserReference `json:"user_references,omitempty"`
	ContextOperations []ContextOperation           `json:"context_operations,omitempty"`
	// ProviderContinuation is process-local model-visible state copied onto the
	// canonical assistant Message. It is excluded from metadata serialization so
	// the protocol payload has exactly one durable source of truth.
	ProviderContinuation map[string]any `json:"-"`
}

const (
	ContextOperationCheckpoint = "checkpoint"
	ContextOperationRewind     = "rewind"
)

// ContextOperation is committed atomically with one assistant message. It is
// model-invisible metadata used to project future context without deleting the
// canonical/display transcript.
type ContextOperation struct {
	Kind             string                   `json:"kind"`
	AgentKind        string                   `json:"agent_kind"`
	CheckpointID     string                   `json:"checkpoint_id"`
	Purpose          string                   `json:"purpose,omitempty"`
	MessageCount     int                      `json:"message_count,omitempty"`
	BoundaryID       string                   `json:"boundary_id"`
	BoundaryLocator  ContextBoundaryLocator   `json:"boundary_locator"`
	Report           string                   `json:"report,omitempty"`
	MutationReceipts []ContextMutationReceipt `json:"mutation_receipts,omitempty"`

	// ResolvedBoundary is populated only after canonical journal lookup. It is
	// never part of message metadata or the rebuildable sidecar projection.
	ResolvedBoundary *ContextBoundarySnapshot `json:"-"`
}

// ContextBoundaryLocator identifies the one canonical journal record that owns
// a frozen projection. SHA256 covers that exact record payload.
type ContextBoundaryLocator struct {
	Cursor      conversationjournal.Cursor `json:"cursor"`
	RecordIndex int                        `json:"record_index,omitempty"`
	SHA256      string                     `json:"sha256"`
}

// ContextBoundarySnapshot is captured synchronously before a model call.
// EffectivePrefix resumes the current run; CanonicalPrefix excludes turn-only
// fragments so future turns can assemble fresh runtime context.
type ContextBoundarySnapshot struct {
	Cursor          ContextCursor    `json:"cursor"`
	LimitBytes      int              `json:"limit_bytes"`
	EffectiveSource string           `json:"effective_source"`
	CanonicalSource string           `json:"canonical_source"`
	EffectivePrefix []*agent.Message `json:"effective_prefix"`
	CanonicalPrefix []*agent.Message `json:"canonical_prefix"`
	EffectiveBytes  int              `json:"effective_bytes"`
	CanonicalBytes  int              `json:"canonical_bytes"`
	EffectiveSHA256 string           `json:"effective_sha256"`
	CanonicalSHA256 string           `json:"canonical_sha256"`
}

// ContextMutationReceipt preserves the fact of a committed side effect across
// rewind. Rewind changes model context only and never reverts external state.
type ContextMutationReceipt struct {
	Tool    string `json:"tool"`
	CallID  string `json:"call_id,omitempty"`
	Scope   string `json:"scope"`
	Summary string `json:"summary"`
}

type ContextWindowProjection struct {
	Checkpoint       ContextOperation `json:"checkpoint"`
	Rewind           ContextOperation `json:"rewind"`
	RewindAfterIndex int              `json:"rewind_after_index"`
	ContextRevision  uint64           `json:"context_revision"`
}

type historyRecord struct {
	journalID                    string
	kind                         string
	message                      *agent.Message
	messageMetadata              MessageMetadata
	display                      *DisplayEvent
	interruption                 *Interruption
	ask                          *AskInteraction
	compaction                   *ContextCompaction
	compactionRemoval            *ContextCompactionRemoval
	compactionHealth             *ContextCompactionHealth
	toolResultCleanup            *ToolResultCleanupRecord
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
	ID           string               `json:"id,omitempty"`
	Role         string               `json:"role"`
	DisplayPhase string               `json:"display_phase,omitempty"`
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

// AskInteraction is append-only session state for one blocking ask call. The
// ordinary tool call/result remains the model and display representation.
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
	Approval         *ToolApprovalPresentation `json:"approval,omitempty"`
	Answers          []AskAnswerResult         `json:"answers,omitempty"`
	CancelReason     string                    `json:"cancel_reason,omitempty"`
	CreatedAt        time.Time                 `json:"created_at"`
	ResolvedAt       *time.Time                `json:"resolved_at,omitempty"`
}

// AskCycleIdentity is the narrow runtime identity Session needs to reconcile a
// pending Ask after the process-local model continuation has been lost.
type AskCycleIdentity struct {
	CommandID   string
	OperationID string
	Cycle       int
}

// AskResolution is a normal structured result. Cancellation is not an error
// and must never be represented as an abnormal Agent interruption.
type AskResolution struct {
	Schema       string            `json:"schema"`
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	Answers      []AskAnswerResult `json:"answers,omitempty"`
	CancelReason string            `json:"cancel_reason,omitempty"`
}

// ContextCompaction records a model-visible summary epoch without modifying the
// raw user-facing transcript.
type ContextCompaction struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	agentcontext.CompactionCheckpoint
	SourceStartIndex int `json:"source_start_index"`
	SourceEndIndex   int `json:"source_end_index"`
	// Cursor fields are the stable v2 boundary. Index fields remain readable
	// for legacy journals and are populated during the transition.
	SourceStartCursor  conversationjournal.Cursor `json:"source_start_cursor,omitempty"`
	SourceEndCursor    conversationjournal.Cursor `json:"source_end_cursor,omitempty"`
	SourceMessageCount int                        `json:"source_message_count"`
	CreatedAt          time.Time                  `json:"created_at"`
	ContextRevision    uint64                     `json:"context_revision,omitempty"`
}

// ContextCompactionRemoval soft-disables the active model-visible compaction
// without deleting raw transcript or historical compaction records.
type ContextCompactionRemoval struct {
	Type              string                     `json:"type"`
	ID                string                     `json:"id"`
	AgentKind         string                     `json:"agent_kind,omitempty"`
	CompactionID      string                     `json:"compaction_id,omitempty"`
	SourceStartIndex  int                        `json:"source_start_index"`
	SourceEndIndex    int                        `json:"source_end_index"`
	SourceStartCursor conversationjournal.Cursor `json:"source_start_cursor,omitempty"`
	SourceEndCursor   conversationjournal.Cursor `json:"source_end_cursor,omitempty"`
	Reason            string                     `json:"reason,omitempty"`
	CreatedAt         time.Time                  `json:"created_at"`
	ContextRevision   uint64                     `json:"context_revision,omitempty"`
}

// ContextCompactionHealth durably records the failure fuse for one stable
// provider-neutral context structure. It is model-invisible and never replaces
// a successful ContextCompaction checkpoint.
type ContextCompactionHealth struct {
	Type                 string    `json:"type"`
	ID                   string    `json:"id"`
	AgentKind            string    `json:"agent_kind,omitempty"`
	BasisRevision        uint64    `json:"basis_revision"`
	StructureFingerprint string    `json:"structure_fingerprint"`
	Outcome              string    `json:"outcome"`
	FailureCode          string    `json:"failure_code,omitempty"`
	ConsecutiveFailures  int       `json:"consecutive_failures"`
	CreatedAt            time.Time `json:"created_at"`
}

// ToolResultReplacement shares the same frozen substitution contract across
// writing sessions and game journals.
type ToolResultReplacement = agentcontext.ToolResultReplacement

// ToolResultCleanupRecord records a model-visible projection without changing
// the canonical rich tool result or the user-facing transcript. SourceEnd is
// exclusive, matching the Session message range APIs.
type ToolResultCleanupRecord struct {
	Type             string                  `json:"type"`
	ID               string                  `json:"id"`
	AgentKind        string                  `json:"agent_kind,omitempty"`
	SourceStart      int64                   `json:"source_start"`
	SourceEnd        int64                   `json:"source_end"`
	Replacements     []ToolResultReplacement `json:"replacements"`
	ReclaimedTokens  int                     `json:"reclaimed_tokens"`
	TriggeredAtUsage int                     `json:"triggered_at_usage"`
	EarliestChanged  int64                   `json:"earliest_changed"`
	WarmSuffixTokens int                     `json:"warm_suffix_tokens"`
	RendererVersion  string                  `json:"renderer_version"`
	CreatedAt        time.Time               `json:"created_at"`
	ContextRevision  uint64                  `json:"context_revision,omitempty"`
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
	askWaiters             map[string]chan AskResolution
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
