package interactive

type CreateStoryRequest struct {
	Title            string `json:"title"`
	Origin           string `json:"origin"`
	StoryTellerID    string `json:"story_teller_id"`
	ReplyTargetChars int    `json:"reply_target_chars"`
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
	Title            string `json:"title"`
	StoryTellerID    string `json:"story_teller_id"`
	ReplyTargetChars *int   `json:"reply_target_chars,omitempty"`
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
	ID               string `json:"id"`
	Title            string `json:"title"`
	Origin           string `json:"origin"`
	StoryTellerID    string `json:"story_teller_id"`
	ReplyTargetChars int    `json:"reply_target_chars"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	Branches         int    `json:"branches"`
	Events           int    `json:"events"`
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
	Alts          []TurnAlt       `json:"alts,omitempty"`
	AltIdx        int             `json:"alt_idx,omitempty"`
	Versions      []TurnVersion   `json:"versions,omitempty"`
	VersionIdx    int             `json:"version_idx,omitempty"`
	Flags         map[string]bool `json:"flags,omitempty"`
}

// DisplayEvent 表示互动回合中只用于前端展示的事件，例如工具调用卡片。
// 它不进入下一轮 Agent 上下文，并且不保存工具入参或返回正文。
type DisplayEvent struct {
	ID        string `json:"id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
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
	StoryID     string         `json:"story_id"`
	BranchID    string         `json:"branch_id"`
	Turns       []TurnEvent    `json:"turns"`
	CurrentTurn *TurnEvent     `json:"current_turn,omitempty"`
	State       map[string]any `json:"state"`
	Graph       StoryGraph     `json:"graph"`
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
