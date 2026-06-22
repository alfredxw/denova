package interactive

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type CreateStoryRequest struct {
	Title            string             `json:"title"`
	Origin           string             `json:"origin"`
	StoryTellerID    string             `json:"story_teller_id"`
	ReplyTargetChars int                `json:"reply_target_chars"`
	Opening          StoryOpeningConfig `json:"opening,omitempty"`
}

type AppendTurnRequest struct {
	BranchID      string         `json:"branch_id"`
	User          string         `json:"user"`
	Narrative     string         `json:"narrative"`
	Thinking      string         `json:"thinking,omitempty"`
	DisplayEvents []DisplayEvent `json:"display_events,omitempty"`
}

type AppendTurnWithStateRequest struct {
	BranchID      string         `json:"branch_id"`
	User          string         `json:"user"`
	Narrative     string         `json:"narrative"`
	Thinking      string         `json:"thinking,omitempty"`
	DisplayEvents []DisplayEvent `json:"display_events,omitempty"`
	Ops           []StateOp      `json:"ops,omitempty"`
	HotState      *HotState      `json:"hot_state,omitempty"`
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

type AppendStateDeltaRequest struct {
	ParentID string    `json:"parent_id"`
	BranchID string    `json:"branch_id"`
	Ops      []StateOp `json:"ops"`
}

type MarkStateFailedRequest struct {
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Error    string `json:"error"`
}

type UpdateStoryRequest struct {
	Title            string              `json:"title"`
	StoryTellerID    string              `json:"story_teller_id"`
	ReplyTargetChars *int                `json:"reply_target_chars,omitempty"`
	Opening          *StoryOpeningConfig `json:"opening,omitempty"`
}

type CreateBranchRequest struct {
	ParentEventID string `json:"parent_event_id"`
	Title         string `json:"title"`
}

type Index struct {
	CurrentStoryID string         `json:"current_story_id"`
	Stories        []StorySummary `json:"stories"`
}

type StorySummary struct {
	ID               string             `json:"id"`
	Title            string             `json:"title"`
	Origin           string             `json:"origin"`
	StoryTellerID    string             `json:"story_teller_id"`
	ReplyTargetChars int                `json:"reply_target_chars"`
	Opening          StoryOpeningConfig `json:"opening"`
	CreatedAt        string             `json:"created_at"`
	UpdatedAt        string             `json:"updated_at"`
	Branches         int                `json:"branches"`
	Events           int                `json:"events"`
}

type StoryOpeningConfig struct {
	Mode       string `json:"mode"`
	PresetID   string `json:"preset_id,omitempty"`
	PresetText string `json:"preset_text,omitempty"`
	CustomText string `json:"custom_text,omitempty"`
}

type BranchMeta struct {
	Head      string `json:"head"`
	CreatedAt string `json:"created_at"`
	From      string `json:"from,omitempty"`
	FromEvent string `json:"from_event,omitempty"`
	Title     string `json:"title,omitempty"`
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
	V                int                   `json:"v"`
	Type             string                `json:"type"`
	StoryID          string                `json:"story_id"`
	Title            string                `json:"title"`
	Origin           string                `json:"origin"`
	StoryTellerID    string                `json:"story_teller_id"`
	ReplyTargetChars int                   `json:"reply_target_chars"`
	Opening          StoryOpeningConfig    `json:"opening"`
	CurrentBranch    string                `json:"current_branch"`
	Branches         map[string]BranchMeta `json:"branches"`
	CreatedAt        string                `json:"created_at"`
	UpdatedAt        string                `json:"updated_at"`
}

type TurnEvent struct {
	V             int             `json:"v"`
	Type          string          `json:"type"`
	ID            string          `json:"id"`
	ParentID      any             `json:"parent_id"`
	BranchID      string          `json:"branch_id"`
	Ts            string          `json:"ts"`
	User          string          `json:"user"`
	Narrative     string          `json:"narrative"`
	Thinking      string          `json:"thinking,omitempty"`
	DisplayEvents []DisplayEvent  `json:"display_events,omitempty"`
	StateDelta    *StateDelta     `json:"state_delta,omitempty"`
	HotState      *HotState       `json:"hot_state,omitempty"`
	StateStatus   string          `json:"state_status,omitempty"`
	StateError    string          `json:"state_error,omitempty"`
	MemoryEntryID string          `json:"memory_entry_id,omitempty"`
	MemoryStatus  string          `json:"memory_status,omitempty"`
	MemoryError   string          `json:"memory_error,omitempty"`
	Alts          []TurnAlt       `json:"alts,omitempty"`
	AltIdx        int             `json:"alt_idx,omitempty"`
	Versions      []TurnVersion   `json:"versions,omitempty"`
	VersionIdx    int             `json:"version_idx,omitempty"`
	Flags         map[string]bool `json:"flags,omitempty"`
}

const TokenUsageEventType = "token_usage"

// DisplayEvent 表示互动回合中只用于前端展示的事件，例如思考过程和工具调用卡片。
// 它不进入下一轮 Agent 上下文；Args/Result 仅用于追溯当时的工具调用过程。
type DisplayEvent struct {
	ID        string `json:"id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Args      string `json:"args,omitempty"`
	Status    string `json:"status,omitempty"`
	Result    string `json:"result,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
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
	SchemaVersion int       `json:"schema_version,omitempty"`
	Ops           []StateOp `json:"ops"`
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
	V             int       `json:"v"`
	Type          string    `json:"type"`
	ID            string    `json:"id"`
	ParentID      string    `json:"parent_id"`
	BranchID      string    `json:"branch_id"`
	Ts            string    `json:"ts"`
	SchemaVersion int       `json:"schema_version,omitempty"`
	Ops           []StateOp `json:"ops"`
}

type ContextCompactionEvent struct {
	V                   int     `json:"v"`
	Type                string  `json:"type"`
	ID                  string  `json:"id"`
	ParentID            string  `json:"parent_id,omitempty"`
	BranchID            string  `json:"branch_id"`
	Ts                  string  `json:"ts"`
	AgentKind           string  `json:"agent_kind,omitempty"`
	Epoch               int     `json:"epoch"`
	Summary             string  `json:"summary"`
	SourceTurnCount     int     `json:"source_turn_count"`
	RetainedTurns       int     `json:"retained_turns"`
	TokensBefore        int     `json:"tokens_before"`
	TokensAfter         int     `json:"tokens_after"`
	TargetRatio         float64 `json:"target_ratio,omitempty"`
	ContextWindowTokens int     `json:"context_window_tokens"`
	Threshold           float64 `json:"threshold"`
	Reason              string  `json:"reason,omitempty"`
	Phase               string  `json:"phase,omitempty"`
}

type ContextCompactionRemovalEvent struct {
	V               int    `json:"v"`
	Type            string `json:"type"`
	ID              string `json:"id"`
	ParentID        string `json:"parent_id,omitempty"`
	BranchID        string `json:"branch_id"`
	Ts              string `json:"ts"`
	AgentKind       string `json:"agent_kind,omitempty"`
	CompactionID    string `json:"compaction_id,omitempty"`
	SourceTurnCount int    `json:"source_turn_count"`
	Reason          string `json:"reason,omitempty"`
}

type BranchEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	From     string `json:"from"`
	Ts       string `json:"ts"`
	Title    string `json:"title"`
}

type StateOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

type Snapshot struct {
	StoryID                  string                         `json:"story_id"`
	BranchID                 string                         `json:"branch_id"`
	Turns                    []TurnEvent                    `json:"turns"`
	CurrentTurn              *TurnEvent                     `json:"current_turn,omitempty"`
	TokenUsageEvents         []TokenUsageEvent              `json:"token_usage_events,omitempty"`
	ContextCompaction        *ContextCompactionEvent        `json:"context_compaction,omitempty"`
	ContextCompactionRemoval *ContextCompactionRemovalEvent `json:"context_compaction_removal,omitempty"`
	State                    map[string]any                 `json:"state"`
	Graph                    StoryGraph                     `json:"graph"`
}

type StoryGraph struct {
	Nodes    []PlotNode      `json:"nodes"`
	Branches []BranchSummary `json:"branches"`
}

type PlotNode struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	BranchID string `json:"branch_id"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Ts       string `json:"ts"`
	Current  bool   `json:"current"`
	Head     bool   `json:"head"`
}

type StoryContext struct {
	Meta     StoryMeta `json:"meta"`
	Snapshot Snapshot  `json:"snapshot"`
}

type InteractiveMemoryEntry struct {
	ID         string   `json:"id"`
	BranchID   string   `json:"branch_id"`
	TurnID     string   `json:"turn_id,omitempty"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Content    string   `json:"content"`
	People     []string `json:"people,omitempty"`
	Places     []string `json:"places,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Importance int      `json:"importance"`
	Archived   bool     `json:"archived"`
	Manual     bool     `json:"manual"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

type InteractiveMemoryRecall struct {
	BranchID  string   `json:"branch_id"`
	TurnID    string   `json:"turn_id,omitempty"`
	Query     string   `json:"query,omitempty"`
	MemoryIDs []string `json:"memory_ids"`
	CreatedAt string   `json:"created_at"`
}

type InteractiveMemoryState struct {
	StoryID      string                   `json:"story_id"`
	BranchID     string                   `json:"branch_id"`
	Entries      []InteractiveMemoryEntry `json:"entries"`
	RecentRecall *InteractiveMemoryRecall `json:"recent_recall,omitempty"`
	SyncStatus   string                   `json:"sync_status,omitempty"`
	SyncError    string                   `json:"sync_error,omitempty"`
}

type InteractiveMemoryCreateRequest struct {
	BranchID   string   `json:"branch_id"`
	TurnID     string   `json:"turn_id,omitempty"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Content    string   `json:"content"`
	People     []string `json:"people,omitempty"`
	Places     []string `json:"places,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Importance int      `json:"importance"`
}

type InteractiveMemoryUpdateRequest struct {
	Title      *string  `json:"title,omitempty"`
	Summary    *string  `json:"summary,omitempty"`
	Content    *string  `json:"content,omitempty"`
	People     []string `json:"people,omitempty"`
	Places     []string `json:"places,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Importance *int     `json:"importance,omitempty"`
}

type InteractiveMemoryArchiveRequest struct {
	Archived *bool `json:"archived,omitempty"`
}

type StoryMemorySettings struct {
	Enabled           bool `json:"enabled"`
	AutoIntervalTurns int  `json:"auto_interval_turns"`
}

type StoryMemoryField struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	GenerationInstruction string `json:"generation_instruction,omitempty"`
	Enabled               *bool  `json:"enabled,omitempty"`
	Required              bool   `json:"required,omitempty"`
	Order                 int    `json:"order"`
}

type StoryMemoryStructure struct {
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	Description           string             `json:"description,omitempty"`
	GenerationInstruction string             `json:"generation_instruction,omitempty"`
	Mode                  string             `json:"mode"`
	KeyFieldID            string             `json:"key_field_id,omitempty"`
	Fields                []StoryMemoryField `json:"fields"`
	Enabled               *bool              `json:"enabled,omitempty"`
	Order                 int                `json:"order"`
	BuiltIn               bool               `json:"built_in,omitempty"`
	CreatedAt             string             `json:"created_at,omitempty"`
	UpdatedAt             string             `json:"updated_at,omitempty"`
}

type StoryMemoryRecord struct {
	ID            string            `json:"id"`
	StructureID   string            `json:"structure_id"`
	BranchID      string            `json:"branch_id"`
	TurnID        string            `json:"turn_id,omitempty"`
	AnchorTurnID  string            `json:"anchor_turn_id,omitempty"`
	Key           string            `json:"key,omitempty"`
	Values        map[string]string `json:"values"`
	Archived      bool              `json:"archived,omitempty"`
	Manual        bool              `json:"manual,omitempty"`
	Source        string            `json:"source,omitempty"`
	InheritedFrom string            `json:"inherited_from,omitempty"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
}

type StoryMemoryState struct {
	StoryID         string                   `json:"story_id"`
	BranchID        string                   `json:"branch_id"`
	Settings        StoryMemorySettings      `json:"settings"`
	Structures      []StoryMemoryStructure   `json:"structures"`
	Records         []StoryMemoryRecord      `json:"records"`
	RecentRecall    *InteractiveMemoryRecall `json:"recent_recall,omitempty"`
	SyncStatus      string                   `json:"sync_status,omitempty"`
	SyncError       string                   `json:"sync_error,omitempty"`
	NextAutoInTurns int                      `json:"next_auto_in_turns,omitempty"`
}

type StoryMemorySettingsUpdateRequest struct {
	Enabled           *bool `json:"enabled,omitempty"`
	AutoIntervalTurns *int  `json:"auto_interval_turns,omitempty"`
}

type StoryMemoryStructureRequest struct {
	ID                    string             `json:"id,omitempty"`
	Name                  string             `json:"name"`
	Description           string             `json:"description,omitempty"`
	GenerationInstruction string             `json:"generation_instruction,omitempty"`
	Mode                  string             `json:"mode"`
	KeyFieldID            string             `json:"key_field_id,omitempty"`
	Fields                []StoryMemoryField `json:"fields"`
	Enabled               *bool              `json:"enabled,omitempty"`
	Order                 int                `json:"order"`
}

type StoryMemoryRecordRequest struct {
	ID          string            `json:"id,omitempty"`
	BranchID    string            `json:"branch_id,omitempty"`
	StructureID string            `json:"structure_id"`
	TurnID      string            `json:"turn_id,omitempty"`
	Key         string            `json:"key,omitempty"`
	Values      map[string]string `json:"values"`
	Manual      bool              `json:"manual,omitempty"`
}

type StoryMemoryRecordArchiveRequest struct {
	Archived *bool `json:"archived,omitempty"`
}

type StoryMemoryPatch struct {
	Op          string            `json:"op"`
	StructureID string            `json:"structure_id,omitempty"`
	RecordID    string            `json:"record_id,omitempty"`
	Key         string            `json:"key,omitempty"`
	Values      map[string]string `json:"values,omitempty"`
	Archived    *bool             `json:"archived,omitempty"`
}

func (p *StoryMemoryPatch) UnmarshalJSON(data []byte) error {
	var raw struct {
		Op          string         `json:"op"`
		StructureID string         `json:"structure_id,omitempty"`
		RecordID    string         `json:"record_id,omitempty"`
		Key         string         `json:"key,omitempty"`
		Values      map[string]any `json:"values,omitempty"`
		Archived    *bool          `json:"archived,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	*p = StoryMemoryPatch{
		Op:          raw.Op,
		StructureID: raw.StructureID,
		RecordID:    raw.RecordID,
		Key:         raw.Key,
		Archived:    raw.Archived,
	}
	if raw.Values != nil {
		p.Values = normalizeStoryMemoryPatchValues(raw.Values)
	}
	return nil
}

func normalizeStoryMemoryPatchValues(values map[string]any) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		switch typed := value.(type) {
		case nil:
			out[key] = ""
		case string:
			out[key] = typed
		case json.Number:
			out[key] = typed.String()
		case bool:
			out[key] = fmt.Sprintf("%t", typed)
		default:
			if data, err := json.Marshal(typed); err == nil {
				out[key] = string(data)
			} else {
				out[key] = fmt.Sprint(typed)
			}
		}
	}
	return out
}

type StoryMemoryGenerateRequest struct {
	BranchID string `json:"branch_id,omitempty"`
	Source   string `json:"source,omitempty"`
}
