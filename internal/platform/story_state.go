package platform

import "encoding/json"

// StoryState is a read-only committed projection. Historical queries never
// move a branch head and must not be used as a second state reducer.
type StoryState struct {
	StoryID        string            `json:"storyId"`
	BranchID       string            `json:"branchId"`
	TurnID         string            `json:"turnId,omitempty"`
	SourceRevision string            `json:"sourceRevision,omitempty"`
	State          map[string]any    `json:"state"`
	StateSchema    *StoryStateSchema `json:"stateSchema,omitempty"`
}

type StoryStateSchema struct {
	Version   int                  `json:"version"`
	Revision  int                  `json:"revision"`
	Templates []StoryStateTemplate `json:"templates"`
}

type StoryStateTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Fields      []StoryStateField `json:"fields"`
}

// Name is the exact canonical field identity, including localized punctuation.
// No dotted-path alias or display label may replace it in rules or lookups.
type StoryStateField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Default     any      `json:"default,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Options     []string `json:"options,omitempty"`
	Description string   `json:"description,omitempty"`
	Group       string   `json:"group,omitempty"`
	Display     string   `json:"display,omitempty"`
}

// StoryStateChange preserves native operation provenance. Path is present for
// legacy/general operations; ActorID and FieldID identify modern field changes.
type StoryStateChange struct {
	Op           string `json:"op"`
	ActorID      string `json:"actorId,omitempty"`
	FieldID      string `json:"fieldId,omitempty"`
	Path         string `json:"path,omitempty"`
	Value        any    `json:"value,omitempty"`
	Reason       string `json:"reason,omitempty"`
	SourceTurnID string `json:"sourceTurnId,omitempty"`
	SourceKind   string `json:"sourceKind,omitempty"`
	SourceID     string `json:"sourceId,omitempty"`
}

// StoryConfiguration adapts the existing native opening configuration. PUT is
// allowed only before the first turn and without a running operation. It never
// modifies reusable presets. InitialActors are frozen into this Story only.
type StoryConfiguration struct {
	Origin             string              `json:"origin"`
	Protagonist        *StoryProtagonist   `json:"protagonist,omitempty"`
	ModuleRefs         *StoryModuleRefs    `json:"moduleRefs,omitempty"`
	PlanningTemplateID string              `json:"planningTemplateId,omitempty"`
	PlanningMode       string              `json:"planningMode,omitempty"`
	StateSchemaMode    string              `json:"stateSchemaMode,omitempty"`
	ReplyTargetChars   int                 `json:"replyTargetChars,omitempty"`
	ChoiceCount        int                 `json:"choiceCount,omitempty"`
	InitialActors      []StoryInitialActor `json:"initialActors,omitempty"`
	StatePreset        json.RawMessage     `json:"statePreset,omitempty"`
	RulePreset         json.RawMessage     `json:"rulePreset,omitempty"`
}

type StoryProtagonist struct {
	Mode                string `json:"mode"`
	Name                string `json:"name,omitempty"`
	Profile             string `json:"profile,omitempty"`
	SourceLoreItemID    string `json:"sourceLoreItemId,omitempty"`
	SourceLoreUpdatedAt string `json:"sourceLoreUpdatedAt,omitempty"`
}

type StoryInitialActor struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	TemplateID  string         `json:"templateId"`
	Role        string         `json:"role,omitempty"`
	Description string         `json:"description,omitempty"`
	State       map[string]any `json:"state,omitempty"`
}

type StoryModuleRefs struct {
	NarrativeStyleID       string   `json:"narrativeStyleId,omitempty"`
	NarrativeStyleDisabled bool     `json:"narrativeStyleDisabled,omitempty"`
	EventPackageIDs        []string `json:"eventPackageIds,omitempty"`
	EventPackagesDisabled  bool     `json:"eventPackagesDisabled,omitempty"`
	RuleSystemID           string   `json:"ruleSystemId,omitempty"`
	RuleSystemDisabled     bool     `json:"ruleSystemDisabled,omitempty"`
	ActorStateID           string   `json:"actorStateId,omitempty"`
	ActorStateDisabled     bool     `json:"actorStateDisabled,omitempty"`
	ImagePresetID          string   `json:"imagePresetId,omitempty"`
	ImagePresetDisabled    bool     `json:"imagePresetDisabled,omitempty"`
}

// StoryPreset deliberately excludes filesystem paths, loader diagnostics and
// private execution data. Content is the selected preset's portable content.
type StoryPreset struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Revision    string `json:"revision,omitempty"`
	Content     any    `json:"content"`
}

// StoryPresetImport accepts portable content only. Its identity is assigned
// from normalized content by the host, never supplied by an extension.
type StoryPresetImport struct {
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Content     json.RawMessage `json:"content"`
}
