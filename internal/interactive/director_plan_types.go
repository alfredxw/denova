package interactive

import "strings"

const (
	DirectorPlanDocPlan        = "plan"
	DirectorPlanDocAgentBrief  = "agent_brief"
	DirectorPlanDocLoreContext = "lore_context"

	DirectorPlanStatusWaitingOpening = "waiting_opening"
	DirectorPlanStatusRunning        = "running"
	DirectorPlanStatusReady          = "ready"
	DirectorPlanStatusSkipped        = "skipped"
	DirectorPlanStatusFailed         = "failed"
	DirectorPlanStatusConflict       = "conflict"

	directorPlanFile         = "director.md"
	directorAgentBriefFile   = "agent-brief.md"
	directorPlanMetadataFile = "metadata.json"

	defaultBranchPlanningTurns = 5
)

// DirectorContextMaxBytes is the hard ceiling for a complete director-related
// context fragment. Total prompt assembly remains bounded by the model-aware
// context budget.
const DirectorContextMaxBytes = 256 * 1024

const (
	maxDirectorPlanDocBytes  = DirectorContextMaxBytes
	directorPlanVisibleBytes = DirectorContextMaxBytes
)

type StoryDirectorPlanningTemplates struct {
	Plan       string `json:"plan,omitempty"`
	AgentBrief string `json:"agent_brief,omitempty"`
}

type DirectorPlanSeed struct {
	Templates           StoryDirectorPlanningTemplates `json:"-"`
	BranchPlanningTurns int                            `json:"-"`
	Source              string                         `json:"-"`
	OpeningSummary      string                         `json:"-"`
	InitialStatus       string                         `json:"-"`
	InitialSummary      string                         `json:"-"`
	StartReady          bool                           `json:"-"`
}

type DirectorPlanDocs struct {
	Plan        string `json:"plan"`
	AgentBrief  string `json:"agent_brief"`
	LoreContext string `json:"lore_context"`
}

type DirectorPlanVisibleDocs struct {
	AgentBrief  string `json:"agent_brief,omitempty"`
	LoreContext string `json:"lore_context,omitempty"`
}

type DirectorPlanDocInfo struct {
	Path         string `json:"path"`
	Bytes        int    `json:"bytes"`
	Hash         string `json:"hash"`
	VisibleBytes int    `json:"visible_bytes,omitempty"`
}

type DirectorPlanRunStatus struct {
	Status           string                           `json:"status,omitempty"`
	Summary          string                           `json:"summary,omitempty"`
	Error            string                           `json:"error,omitempty"`
	SourceTurnID     string                           `json:"source_turn_id,omitempty"`
	UpdatedAt        string                           `json:"updated_at,omitempty"`
	PlannedDocs      int                              `json:"planned_docs,omitempty"`
	CompletedDocs    int                              `json:"completed_docs,omitempty"`
	StartReady       bool                             `json:"start_ready,omitempty"`
	Blocking         bool                             `json:"blocking,omitempty"`
	BaselineHashes   map[string]string                `json:"baseline_hashes,omitempty"`
	Decision         *PlanDecision                    `json:"decision,omitempty"`
	EventOpportunity EventOpportunity                 `json:"event_opportunity,omitempty"`
	DomainCommit     *DirectorPlanDomainCommitReceipt `json:"domain_commit,omitempty"`
}

type DirectorPlanMetadata struct {
	Version             int                            `json:"version"`
	StoryID             string                         `json:"story_id"`
	BranchID            string                         `json:"branch_id"`
	Revision            string                         `json:"revision"`
	BranchPlanningTurns int                            `json:"branch_planning_turns"`
	UpdatedAt           string                         `json:"updated_at"`
	Source              string                         `json:"source,omitempty"`
	SourceTurnID        string                         `json:"source_turn_id,omitempty"`
	Docs                map[string]DirectorPlanDocInfo `json:"docs,omitempty"`
	LastRun             *DirectorPlanRunStatus         `json:"last_run,omitempty"`
	// DerivedThroughTurnID is the durable receipt for the canonical
	// turn-to-Director projection. A committed Agent turn without this receipt
	// is an outbox item that must be drained before the next Game cycle reads
	// Director state.
	DerivedThroughTurnID string               `json:"derived_through_turn_id,omitempty"`
	DerivedAt            string               `json:"derived_at,omitempty"`
	EventRuntime         DirectorEventRuntime `json:"event_runtime,omitempty"`
	LoreRevision         string               `json:"lore_revision,omitempty"`
}

type DirectorPlan struct {
	StoryID     string                  `json:"story_id"`
	BranchID    string                  `json:"branch_id"`
	Docs        DirectorPlanDocs        `json:"docs"`
	VisibleDocs DirectorPlanVisibleDocs `json:"visible_docs,omitempty"`
	Metadata    DirectorPlanMetadata    `json:"metadata"`
}

type DirectorPlanStatus struct {
	StoryID          string               `json:"story_id"`
	BranchID         string               `json:"branch_id"`
	Status           string               `json:"status"`
	Summary          string               `json:"summary,omitempty"`
	Error            string               `json:"error,omitempty"`
	SourceTurnID     string               `json:"source_turn_id,omitempty"`
	UpdatedAt        string               `json:"updated_at,omitempty"`
	PlannedDocs      int                  `json:"planned_docs"`
	CompletedDocs    int                  `json:"completed_docs"`
	DocBytes         int                  `json:"doc_bytes"`
	VisibleBytes     int                  `json:"visible_bytes"`
	StartReady       bool                 `json:"start_ready"`
	Blocking         bool                 `json:"blocking"`
	Revision         string               `json:"revision,omitempty"`
	Decision         *PlanDecision        `json:"decision,omitempty"`
	EventRuntime     DirectorEventRuntime `json:"event_runtime,omitempty"`
	EventOpportunity EventOpportunity     `json:"event_opportunity,omitempty"`
}

type UpdateDirectorPlanRequest struct {
	BranchID     string           `json:"branch_id,omitempty"`
	Docs         DirectorPlanDocs `json:"docs"`
	BaseRevision string           `json:"base_revision,omitempty"`
	Source       string           `json:"source,omitempty"`
	Summary      string           `json:"summary,omitempty"`
}

type RebuildDirectorPlanRequest struct {
	BranchID    string `json:"branch_id,omitempty"`
	Source      string `json:"source,omitempty"`
	ResetEvents bool   `json:"reset_events,omitempty"`
}

type RunDirectorPlanRequest struct {
	BranchID             string `json:"branch_id,omitempty"`
	Source               string `json:"source,omitempty"`
	ForceEventEvaluation bool   `json:"force_event_evaluation,omitempty"`
}

type DirectorPlanRunToken struct {
	StoryID  string            `json:"story_id"`
	BranchID string            `json:"branch_id"`
	Revision string            `json:"revision"`
	Hashes   map[string]string `json:"hashes,omitempty"`
}

func NormalizeStoryDirectorPlanningTemplates(templates StoryDirectorPlanningTemplates) StoryDirectorPlanningTemplates {
	defaults := DefaultStoryDirectorPlanningTemplates()
	templates.Plan = normalizeDirectorPlanTemplate(templates.Plan, defaults.Plan)
	templates.AgentBrief = normalizeDirectorPlanTemplate(templates.AgentBrief, defaults.AgentBrief)
	return templates
}

func normalizeDirectorPlanTemplate(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return trimBytes(value, maxDirectorPlanDocBytes)
}

func NormalizeBranchPlanningTurns(value int) int {
	if value <= 0 {
		return defaultBranchPlanningTurns
	}
	if value < 1 {
		return 1
	}
	if value > 12 {
		return 12
	}
	return value
}
