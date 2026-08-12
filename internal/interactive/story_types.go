package interactive

import (
	interactivestate "denova/internal/interactive/state"
	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/conversationconfig"
	"denova/internal/interactive/director"
)

type CreateStoryRequest struct {
	Title                     string                            `json:"title"`
	Origin                    string                            `json:"origin"`
	StoryTellerID             string                            `json:"story_teller_id"`
	StoryDirectorID           string                            `json:"story_director_id,omitempty"`
	DirectorRunPolicy         *director.RunPolicy               `json:"director_run_policy,omitempty"`
	ModuleRefs                *StoryDirectorModuleRefs          `json:"module_refs,omitempty"`
	ReplyTargetChars          int                               `json:"reply_target_chars"`
	ChoiceCount               int                               `json:"choice_count"`
	Opening                   StoryOpeningConfig                `json:"opening,omitempty"`
	ImageSettings             StoryImageSettings                `json:"image_settings,omitempty"`
	InitialTraitRolls         []InitialActorTraitRoll           `json:"initial_trait_rolls,omitempty"`
	StateSchemaPolicy         *StoryStateSchemaPolicy           `json:"state_schema_policy,omitempty"`
	ActorState                *StoryDirectorActorStateSystem    `json:"-"`
	TRPGSystem                *StoryDirectorTRPGSystem          `json:"-"`
	ActorStateAdaptation      *ActorStateSchemaAdaptationRecord `json:"-"`
	InitialStateOps           []interactivestate.Op             `json:"-"`
	DirectorPlanSeed          *DirectorPlanSeed                 `json:"-"`
	StateSchemaInitialization *StateSchemaInitializationStatus  `json:"-"`
	RuntimeConfig             *conversationconfig.Config        `json:"-"`
}

type AppendTurnRequest struct {
	BranchID             string                `json:"branch_id"`
	User                 string                `json:"user"`
	Narrative            string                `json:"narrative"`
	Thinking             string                `json:"thinking,omitempty"`
	DisplayEvents        []DisplayEvent        `json:"display_events,omitempty"`
	ModelContextMessages []ModelContextMessage `json:"model_context_messages,omitempty"`
}

type AppendTurnWithStateRequest struct {
	BranchID             string                    `json:"branch_id"`
	ExpectedParentID     *string                   `json:"expected_parent_id,omitempty"`
	ReplaceTurnID        string                    `json:"replace_turn_id,omitempty"`
	User                 string                    `json:"user"`
	Narrative            string                    `json:"narrative"`
	Thinking             string                    `json:"thinking,omitempty"`
	RunID                string                    `json:"run_id,omitempty"`
	AgentKind            string                    `json:"agent_kind,omitempty"`
	AgentCommandID       string                    `json:"agent_command_id,omitempty"`
	AgentOperationID     string                    `json:"agent_operation_id,omitempty"`
	AgentCycle           int                       `json:"agent_cycle,omitempty"`
	AgentCanonicalHash   string                    `json:"agent_canonical_hash,omitempty"`
	DisplayEvents        []DisplayEvent            `json:"display_events,omitempty"`
	ModelContextMessages []ModelContextMessage     `json:"model_context_messages,omitempty"`
	Ops                  []interactivestate.Op     `json:"ops,omitempty"`
	ActorOps             []ActorStateOp            `json:"actor_ops,omitempty"`
	RuleResolution       *RuleResolution           `json:"rule_resolution,omitempty"`
	TurnResult           *TurnResult               `json:"turn_result,omitempty"`
	TerminalOutcome      *TerminalOutcome          `json:"terminal_outcome,omitempty"`
	StateSchemaProposal  *ActorStateSchemaProposal `json:"-"`
}

type RuleResolutionRerollRequest struct {
	BranchID string `json:"branch_id,omitempty"`
	TurnID   string `json:"turn_id,omitempty"`
}

type RewindTurnRequest struct {
	BranchID string `json:"branch_id"`
	TurnID   string `json:"turn_id"`
}

type SwitchTurnVersionRequest struct {
	BranchID      string `json:"branch_id"`
	TurnID        string `json:"turn_id"`
	VersionTurnID string `json:"version_turn_id"`
}

// UpdateTurnNarrativeRequest updates only the creator-visible narrative of an
// existing turn. ExpectedNarrative provides compare-and-swap protection for
// editors opened from an older snapshot.
type UpdateTurnNarrativeRequest struct {
	BranchID          string  `json:"branch_id"`
	TurnID            string  `json:"-"`
	Narrative         string  `json:"narrative"`
	ExpectedNarrative *string `json:"expected_narrative,omitempty"`
}

// UpdateTurnNarrativeResult reports the durable edited turn. Public Agent
// transcript synchronization observes ContextRevision and atomically rebuilds
// its derived history before the next turn.
type UpdateTurnNarrativeResult struct {
	Turn TurnEvent `json:"turn"`
}

type InteractiveImageGenerateRequest struct {
	CommandID string `json:"command_id"`
	BranchID  string `json:"branch_id,omitempty"`
	TurnID    string `json:"turn_id"`
	Source    string `json:"source,omitempty"`
	Force     bool   `json:"force,omitempty"`
}

type AppendStateDeltaRequest struct {
	ParentID string                `json:"parent_id"`
	BranchID string                `json:"branch_id"`
	Ops      []interactivestate.Op `json:"ops"`
	ActorOps []ActorStateOp        `json:"actor_ops,omitempty"`
}

type MarkStateFailedRequest struct {
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Error    string `json:"error"`
}

type UpdateStoryRequest struct {
	Title                     string                           `json:"title"`
	Origin                    *string                          `json:"origin,omitempty"`
	StoryTellerID             string                           `json:"story_teller_id"`
	StoryDirectorID           string                           `json:"story_director_id,omitempty"`
	DirectorRunPolicy         *director.RunPolicy              `json:"director_run_policy,omitempty"`
	ModuleRefs                *StoryDirectorModuleRefs         `json:"module_refs,omitempty"`
	ReplyTargetChars          *int                             `json:"reply_target_chars,omitempty"`
	ChoiceCount               *int                             `json:"choice_count,omitempty"`
	Opening                   *StoryOpeningConfig              `json:"opening,omitempty"`
	ImageSettings             *StoryImageSettings              `json:"image_settings,omitempty"`
	StateSchemaPolicy         *StoryStateSchemaPolicy          `json:"state_schema_policy,omitempty"`
	ActorState                *StoryDirectorActorStateSystem   `json:"-"`
	TRPGSystem                *StoryDirectorTRPGSystem         `json:"-"`
	StateSchemaInitialization *StateSchemaInitializationStatus `json:"-"`
}

type CreateBranchRequest struct {
	ParentEventID string `json:"parent_event_id"`
	Title         string `json:"title"`
}

type Index struct {
	Version        int            `json:"version"`
	CurrentStoryID string         `json:"current_story_id"`
	Stories        []StorySummary `json:"stories"`
}

type StorySummary struct {
	ID                string                   `json:"id"`
	Title             string                   `json:"title"`
	Origin            string                   `json:"origin"`
	StoryTellerID     string                   `json:"story_teller_id"`
	StoryDirectorID   string                   `json:"story_director_id"`
	DirectorRunPolicy *director.RunPolicy      `json:"director_run_policy,omitempty"`
	ModuleRefs        *StoryDirectorModuleRefs `json:"module_refs,omitempty"`
	ReplyTargetChars  int                      `json:"reply_target_chars"`
	ChoiceCount       int                      `json:"choice_count"`
	Opening           StoryOpeningConfig       `json:"opening"`
	ImageSettings     StoryImageSettings       `json:"image_settings"`
	StateSchemaPolicy *StoryStateSchemaPolicy  `json:"state_schema_policy,omitempty"`
	CreatedAt         string                   `json:"created_at"`
	UpdatedAt         string                   `json:"updated_at"`
	Branches          int                      `json:"branches"`
	Events            int                      `json:"events"`
	// TurnCount is the canonical depth of the story's current branch. Journal
	// side events and turns that only exist on another branch are excluded.
	TurnCount int `json:"turn_count"`
}

type StoryOpeningConfig struct {
	Mode       string `json:"mode"`
	PresetID   string `json:"preset_id,omitempty"`
	PresetText string `json:"preset_text,omitempty"`
	CustomText string `json:"custom_text,omitempty"`
}

type StoryImageSettings struct {
	Mode          string `json:"mode"`
	IntervalTurns int    `json:"interval_turns,omitempty"`
	PresetID      string `json:"preset_id,omitempty"`
}

type BranchMeta struct {
	Head                  string                     `json:"head"`
	CreatedAt             string                     `json:"created_at"`
	From                  string                     `json:"from,omitempty"`
	FromEvent             string                     `json:"from_event,omitempty"`
	Title                 string                     `json:"title,omitempty"`
	RuntimeConfig         *conversationconfig.Config `json:"runtime_config,omitempty"`
	RuntimeConfigRevision uint64                     `json:"runtime_config_revision,omitempty"`
}

type BranchSummary struct {
	ID        string `json:"id"`
	Head      string `json:"head"`
	From      string `json:"from,omitempty"`
	FromEvent string `json:"from_event,omitempty"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at"`
	Current   bool   `json:"current"`
}

type StoryMeta struct {
	V                         int                              `json:"v"`
	Type                      string                           `json:"type"`
	StoryID                   string                           `json:"story_id"`
	Title                     string                           `json:"title"`
	Origin                    string                           `json:"origin"`
	StoryTellerID             string                           `json:"story_teller_id"`
	StoryDirectorID           string                           `json:"story_director_id,omitempty"`
	DirectorRunPolicy         *director.RunPolicy              `json:"director_run_policy,omitempty"`
	ModuleRefs                *StoryDirectorModuleRefs         `json:"module_refs,omitempty"`
	ReplyTargetChars          int                              `json:"reply_target_chars"`
	ChoiceCount               int                              `json:"choice_count"`
	Opening                   StoryOpeningConfig               `json:"opening"`
	ImageSettings             StoryImageSettings               `json:"image_settings"`
	StateSchemaPolicy         *StoryStateSchemaPolicy          `json:"state_schema_policy,omitempty"`
	InitialTraitRolls         []InitialActorTraitRoll          `json:"initial_trait_rolls,omitempty"`
	ActorStateSchema          *ActorStateSchemaSnapshot        `json:"actor_state_schema,omitempty"`
	StateSchemaInitialization *StateSchemaInitializationStatus `json:"state_schema_initialization,omitempty"`
	CurrentBranch             string                           `json:"current_branch"`
	Branches                  map[string]BranchMeta            `json:"branches"`
	CreatedAt                 string                           `json:"created_at"`
	UpdatedAt                 string                           `json:"updated_at"`
}

type TurnEvent struct {
	V                  int    `json:"v"`
	Type               string `json:"type"`
	ID                 string `json:"id"`
	ParentID           any    `json:"parent_id"`
	BranchID           string `json:"branch_id"`
	Ts                 string `json:"ts"`
	User               string `json:"user"`
	Narrative          string `json:"narrative"`
	Thinking           string `json:"thinking,omitempty"`
	RunID              string `json:"run_id,omitempty"`
	AgentKind          string `json:"agent_kind,omitempty"`
	AgentCommandID     string `json:"agent_command_id,omitempty"`
	AgentOperationID   string `json:"agent_operation_id,omitempty"`
	AgentCycle         int    `json:"agent_cycle,omitempty"`
	AgentCommitHash    string `json:"agent_commit_hash,omitempty"`
	AgentCanonicalHash string `json:"agent_canonical_hash,omitempty"`
	PlayerInputID      string `json:"player_input_id,omitempty"`
	PlayerInputHash    string `json:"player_input_hash,omitempty"`
	// ConsumedPlayerInputIDs closes every reachable accepted input whose intent
	// was visible to this successful cycle. PlayerInputID remains the exact
	// current command identity; older interrupted inputs are resolved without
	// deleting their append-only audit events.
	ConsumedPlayerInputIDs []string `json:"consumed_player_input_ids,omitempty"`
	// ResolvedPlayerInputContexts preserves interrupted inputs at their original
	// acceptance boundary after this Turn closes them. The current Turn's input
	// remains represented by User and ModelContextMessages; only older pending
	// inputs belong here, so their model evidence cannot be reordered into this
	// Turn during a cold model-history projection.
	ResolvedPlayerInputContexts []ResolvedPlayerInputContext `json:"resolved_player_input_contexts,omitempty"`
	DisplayEvents               []DisplayEvent               `json:"display_events,omitempty"`
	ModelContextMessages        []ModelContextMessage        `json:"model_context_messages,omitempty"`
	StateDelta                  *StateDelta                  `json:"state_delta,omitempty"`
	HotState                    *HotState                    `json:"hot_state,omitempty"`
	RuleResolution              *RuleResolution              `json:"rule_resolution,omitempty"`
	TurnResult                  *TurnResult                  `json:"turn_result,omitempty"`
	TerminalOutcome             *TerminalOutcome             `json:"terminal_outcome,omitempty"`
	StateStatus                 string                       `json:"state_status,omitempty"`
	StateError                  string                       `json:"state_error,omitempty"`
	Alts                        []TurnAlt                    `json:"alts,omitempty"`
	AltIdx                      int                          `json:"alt_idx,omitempty"`
	Versions                    []TurnVersion                `json:"versions,omitempty"`
	VersionIdx                  int                          `json:"version_idx,omitempty"`
	Flags                       map[string]bool              `json:"flags,omitempty"`
}

const TokenUsageEventType = "token_usage"

// DisplayEventRoleNarrative marks the position where the turn narrative was
// streamed relative to thinking/tool events. It carries no content; the UI
// renders turn.narrative at this anchor so submission tool cards stay after
// the prose instead of being folded into the thinking trace group.
const DisplayEventRoleNarrative = "narrative"

// DisplayEvent 表示互动回合中只用于前端展示的事件，例如思考过程和工具调用卡片。
// 它不进入下一轮 Agent 上下文；Args/Result 仅用于追溯当时的工具调用过程。
// Role 为 narrative 的事件是正文位置锚点：正文本身不进入 DisplayEvents，
// 锚点只标记正文在事件流中的相对位置，供前端按真实顺序穿插渲染。
type DisplayEvent struct {
	ID                string   `json:"id,omitempty"`
	Role              string   `json:"role"`
	Content           string   `json:"content,omitempty"`
	Name              string   `json:"name,omitempty"`
	Args              string   `json:"args,omitempty"`
	Status            string   `json:"status,omitempty"`
	Result            string   `json:"result,omitempty"`
	CreatedAt         string   `json:"created_at,omitempty"`
	AgentKind         string   `json:"agent_kind,omitempty"`
	AgentName         string   `json:"agent_name,omitempty"`
	RootAgentName     string   `json:"root_agent_name,omitempty"`
	RunPath           []string `json:"run_path,omitempty"`
	SubAgent          bool     `json:"subagent,omitempty"`
	RunID             string   `json:"run_id,omitempty"`
	SubAgentSessionID string   `json:"subagent_session_id,omitempty"`
	SubAgentType      string   `json:"subagent_type,omitempty"`
	SSEHiddenFields   []string `json:"sse_hidden_fields,omitempty"`
	SSEHiddenReason   string   `json:"sse_hidden_reason,omitempty"`
	SSEDisplayNotice  string   `json:"sse_display_notice,omitempty"`
	SSEGeneratedChars int      `json:"sse_generated_chars,omitempty"`
}

// ModelContextMessage is model-visible turn evidence hidden from the chat UI.
// It stores only assistant tool calls and tool results, never raw thinking.
type ModelContextMessage struct {
	Role       string                   `json:"role"`
	Content    string                   `json:"content,omitempty"`
	Name       string                   `json:"name,omitempty"`
	ToolCalls  []ModelContextToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
	ToolName   string                   `json:"tool_name,omitempty"`
	ToolResult *agent.ToolResultSummary `json:"tool_result,omitempty"`
}

type ModelContextToolCall struct {
	Index    *int                     `json:"index,omitempty"`
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function ModelContextFunctionCall `json:"function"`
	Extra    map[string]any           `json:"extra,omitempty"`
}

type ModelContextFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type TokenUsageEvent struct {
	V                    int              `json:"v"`
	Type                 string           `json:"type"`
	ID                   string           `json:"id"`
	StoryID              string           `json:"story_id,omitempty"`
	BranchID             string           `json:"branch_id"`
	CreatedAt            string           `json:"created_at"`
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

type TurnAlt struct {
	Narrative string `json:"narrative"`
	Ts        string `json:"ts"`
}

type TurnVersion struct {
	TurnID  string `json:"turn_id"`
	Ts      string `json:"ts"`
	Current bool   `json:"current"`
}

type StateDelta struct {
	SchemaVersion int                   `json:"schema_version,omitempty"`
	Ops           []interactivestate.Op `json:"ops"`
	ActorOps      []ActorStateOp        `json:"actor_ops,omitempty"`
}

type HotState struct {
	Choices []string `json:"choices"`
}

type HotChoicesEvent struct {
	V        int      `json:"v"`
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	ParentID string   `json:"parent_id"`
	BranchID string   `json:"branch_id"`
	Ts       string   `json:"ts"`
	Choices  []string `json:"choices"`
}

type StateDeltaEvent struct {
	V             int                   `json:"v"`
	Type          string                `json:"type"`
	ID            string                `json:"id"`
	ParentID      string                `json:"parent_id"`
	BranchID      string                `json:"branch_id"`
	Ts            string                `json:"ts"`
	SchemaVersion int                   `json:"schema_version,omitempty"`
	Ops           []interactivestate.Op `json:"ops"`
	ActorOps      []ActorStateOp        `json:"actor_ops,omitempty"`
}

// ContextCompactionProjection is the read-only product projection of the
// public Agent checkpoint currently bound to a Story branch. Story Store never
// persists this value and therefore cannot become a second maintenance
// authority.
type ContextCompactionProjection struct {
	ID       string `json:"id"`
	BranchID string `json:"branch_id"`
	agentcontext.CompactionCheckpoint
	SourceTurnCount int `json:"source_turn_count"`
}

// TurnVersionProjection records one immutable event copied from the previous
// canonical suffix. The source event remains byte-for-byte auditable while the
// projected event receives a new ID and parent on the selected version path.
type TurnVersionProjection struct {
	SourceID    string `json:"source_id"`
	ProjectedID string `json:"projected_id"`
	EventType   string `json:"event_type"`
}

// TurnVersionSelectionEvent is an append-only audit record for a canonical
// version choice. It deliberately stays off the active ancestry: ProjectedHeadID
// is the branch head, while ParentID only links the audit record to that result.
type TurnVersionSelectionEvent struct {
	V               int                     `json:"v"`
	Type            string                  `json:"type"`
	ID              string                  `json:"id"`
	ParentID        string                  `json:"parent_id,omitempty"`
	BranchID        string                  `json:"branch_id"`
	Ts              string                  `json:"ts"`
	ReplacedTurnID  string                  `json:"replaced_turn_id"`
	SelectedTurnID  string                  `json:"selected_turn_id"`
	PreviousHeadID  string                  `json:"previous_head_id,omitempty"`
	ProjectedHeadID string                  `json:"projected_head_id,omitempty"`
	ProjectedEvents []TurnVersionProjection `json:"projected_events,omitempty"`
	CurrentState    map[string]any          `json:"current_state,omitempty"`
	CurrentTurnID   string                  `json:"current_turn_id,omitempty"`
	CurrentDepth    int                     `json:"current_depth,omitempty"`
}

type BranchEvent struct {
	V               int            `json:"v"`
	Type            string         `json:"type"`
	ID              string         `json:"id"`
	ParentID        string         `json:"parent_id"`
	BranchID        string         `json:"branch_id"`
	From            string         `json:"from"`
	Ts              string         `json:"ts"`
	Title           string         `json:"title"`
	StateCheckpoint map[string]any `json:"state_checkpoint,omitempty"`
	LatestTurnID    string         `json:"latest_turn_id,omitempty"`
	Depth           int            `json:"depth,omitempty"`
}

type Snapshot struct {
	StoryID                    string                           `json:"story_id"`
	BranchID                   string                           `json:"branch_id"`
	ContextRevision            uint64                           `json:"context_revision,omitempty"`
	Turns                      []TurnEvent                      `json:"turns"`
	PendingPlayerInputs        []PlayerInputAcceptedEvent       `json:"pending_player_inputs,omitempty"`
	PendingModelContextBatches []ModelContextBatchEvent         `json:"pending_model_context_batches,omitempty"`
	CurrentTurn                *TurnEvent                       `json:"current_turn,omitempty"`
	TokenUsageEvents           []TokenUsageEvent                `json:"token_usage_events,omitempty"`
	ContextCompaction          *ContextCompactionProjection     `json:"context_compaction,omitempty"`
	DirectorPlan               *DirectorPlan                    `json:"-"`
	DirectorPlanStatus         *DirectorPlanStatus              `json:"director_plan_status,omitempty"`
	State                      map[string]any                   `json:"state"`
	ActorStateSchema           *ActorStateSchemaSnapshot        `json:"actor_state_schema,omitempty"`
	StateSchemaInitialization  *StateSchemaInitializationStatus `json:"state_schema_initialization,omitempty"`
	Graph                      StoryGraph                       `json:"graph"`
	TurnCount                  int                              `json:"turn_count"`
	TurnStart                  int                              `json:"turn_start"`
	HistoryBeforeCursor        string                           `json:"history_before_cursor,omitempty"`
	HasEarlierTurns            bool                             `json:"has_earlier_turns"`
}

// StoryHistoryPage is the bounded UI history projection. BeforeCursor is
// opaque to callers and remains tied to one story generation and branch.
type StoryHistoryPage struct {
	StoryID      string      `json:"story_id"`
	BranchID     string      `json:"branch_id"`
	Turns        []TurnEvent `json:"turns"`
	BeforeCursor string      `json:"before_cursor,omitempty"`
	HasMore      bool        `json:"has_more"`
}

type StoryGraph struct {
	Nodes    []PlotNode      `json:"nodes"`
	Branches []BranchSummary `json:"branches"`
}

type PlotNode struct {
	ID           string `json:"id"`
	ParentID     string `json:"parent_id,omitempty"`
	BranchID     string `json:"branch_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Ts           string `json:"ts"`
	Current      bool   `json:"current"`
	Head         bool   `json:"head"`
	Terminal     bool   `json:"terminal,omitempty"`
	TerminalType string `json:"terminal_type,omitempty"`
}

type StoryContext struct {
	Meta     StoryMeta `json:"meta"`
	Snapshot Snapshot  `json:"snapshot"`
}
