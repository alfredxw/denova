package interactive

// TurnNarrativeRevisedEvent replaces only the creator-visible prose of the
// latest logical turn. State, choices, images, trace, and turn identity remain
// attached to the immutable original turn.
type TurnNarrativeRevisedEvent struct {
	V         int    `json:"v"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	ParentID  string `json:"parent_id"`
	BranchID  string `json:"branch_id"`
	Ts        string `json:"ts"`
	TurnID    string `json:"turn_id"`
	Narrative string `json:"narrative"`
}

type TurnDisplayAppendedEvent struct {
	V        int          `json:"v"`
	Type     string       `json:"type"`
	ID       string       `json:"id"`
	ParentID string       `json:"parent_id"`
	BranchID string       `json:"branch_id"`
	Ts       string       `json:"ts"`
	TurnID   string       `json:"turn_id"`
	Display  DisplayEvent `json:"display"`
}

// TurnStateRevisedEvent is the single append-only settlement seam for async
// state generation, failure reporting, and rule rerolls.
type TurnStateRevisedEvent struct {
	V                    int              `json:"v"`
	Type                 string           `json:"type"`
	ID                   string           `json:"id"`
	ParentID             string           `json:"parent_id"`
	BranchID             string           `json:"branch_id"`
	Ts                   string           `json:"ts"`
	TurnID               string           `json:"turn_id"`
	StateDelta           *StateDelta      `json:"state_delta,omitempty"`
	ClearStateDelta      bool             `json:"clear_state_delta,omitempty"`
	StateStatus          string           `json:"state_status"`
	StateError           string           `json:"state_error,omitempty"`
	RuleResolution       *RuleResolution  `json:"rule_resolution,omitempty"`
	ClearRuleResolution  bool             `json:"clear_rule_resolution,omitempty"`
	TerminalOutcome      *TerminalOutcome `json:"terminal_outcome,omitempty"`
	ClearTerminalOutcome bool             `json:"clear_terminal_outcome,omitempty"`
	Reason               string           `json:"reason,omitempty"`
}

type StoryConfigUpdatedEvent struct {
	V        int      `json:"v"`
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	ParentID string   `json:"parent_id,omitempty"`
	BranchID string   `json:"branch_id"`
	Ts       string   `json:"ts"`
	Fields   []string `json:"fields,omitempty"`
}

type BranchSwitchedEvent struct {
	V          int    `json:"v"`
	Type       string `json:"type"`
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	BranchID   string `json:"branch_id"`
	Ts         string `json:"ts"`
	FromBranch string `json:"from_branch,omitempty"`
	ToBranch   string `json:"to_branch"`
}

type BranchArchivedEvent struct {
	V                     int    `json:"v"`
	Type                  string `json:"type"`
	ID                    string `json:"id"`
	ParentID              string `json:"parent_id,omitempty"`
	BranchID              string `json:"branch_id"`
	Ts                    string `json:"ts"`
	PreviousCurrentBranch string `json:"previous_current_branch,omitempty"`
	NextCurrentBranch     string `json:"next_current_branch,omitempty"`
}

type BranchHeadMovedEvent struct {
	V                int            `json:"v"`
	Type             string         `json:"type"`
	ID               string         `json:"id"`
	ParentID         string         `json:"parent_id,omitempty"`
	BranchID         string         `json:"branch_id"`
	Ts               string         `json:"ts"`
	PreviousHead     string         `json:"previous_head"`
	NextHead         string         `json:"next_head,omitempty"`
	NextLatestTurnID string         `json:"next_latest_turn_id,omitempty"`
	NextDepth        int            `json:"next_depth,omitempty"`
	StateCheckpoint  map[string]any `json:"state_checkpoint"`
	PlanCheckpoint   *BranchPlan    `json:"plan_checkpoint,omitempty"`
	Reason           string         `json:"reason"`
}
