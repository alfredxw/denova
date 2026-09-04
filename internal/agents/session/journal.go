package session

import (
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationconfig"
)

const (
	journalFormatVersion         = 2
	historyTypeSessionPatch      = "session_patch"
	historyTypeDisplayPatch      = "display_patch"
	historyTypeInterruptionPatch = "interruption_patch"
	historyTypeContextBatch      = "context_batch"

	retiredHistoryTypeAsk               = "ask"
	retiredHistoryTypeAskPatch          = "ask_patch"
	retiredHistoryTypeCompaction        = "context_compaction"
	retiredHistoryTypeCompactionRemoved = "context_compaction_removed"
	retiredHistoryTypeCompactionHealth  = "context_compaction_health"
	retiredHistoryTypeToolResultCleanup = "tool_result_cleanup"
	retiredHistoryTypeGoalChanged       = "goal_changed"
)

// isRetiredSessionJournalRecordType identifies records whose runtime feature
// no longer exists and whose payload never owned canonical conversation text.
// They remain explicit so genuinely unknown or corrupt records still fail.
func isRetiredSessionJournalRecordType(recordType string) bool {
	switch recordType {
	case retiredHistoryTypeAsk,
		retiredHistoryTypeAskPatch,
		retiredHistoryTypeCompaction,
		retiredHistoryTypeCompactionRemoved,
		retiredHistoryTypeCompactionHealth,
		retiredHistoryTypeToolResultCleanup,
		retiredHistoryTypeGoalChanged:
		return true
	default:
		return false
	}
}

// sessionHeader is immutable journal metadata. Mutable session attributes are
// persisted as patch records so an update never rewrites prior history.
type sessionHeader struct {
	Type                  string                     `json:"type"`
	Version               int                        `json:"version,omitempty"`
	ID                    string                     `json:"id"`
	IncarnationID         string                     `json:"incarnation_id,omitempty"`
	Title                 string                     `json:"title,omitempty"`
	CreatedAt             time.Time                  `json:"created_at"`
	UpdatedAt             time.Time                  `json:"updated_at,omitempty"`
	RuntimeConfig         *conversationconfig.Config `json:"runtime_config,omitempty"`
	RuntimeConfigRevision uint64                     `json:"runtime_config_revision,omitempty"`
}

type clearRecord struct {
	Type            string    `json:"type"`
	CreatedAt       time.Time `json:"created_at"`
	ContextRevision uint64    `json:"context_revision,omitempty"`
}

type interruptionRecord struct {
	Type string `json:"type"`
	Interruption
}

// displayRecord carries an internal immutable identifier. DisplayEvent.ID is a
// provider/tool identifier and is not guaranteed to exist or be unique.
type displayRecord struct {
	Type     string `json:"type"`
	RecordID string `json:"record_id,omitempty"`
	DisplayEvent
}

type sessionPatchRecord struct {
	Type                  string                     `json:"type"`
	Title                 *string                    `json:"title,omitempty"`
	RuntimeConfig         *conversationconfig.Config `json:"runtime_config,omitempty"`
	RuntimeConfigRevision uint64                     `json:"runtime_config_revision,omitempty"`
	UpdatedAt             time.Time                  `json:"updated_at"`
}

// displayPatchRecord stores only the mutation. Pointer fields distinguish an
// explicit empty value from an omitted field.
type displayPatchRecord struct {
	Type             string                  `json:"type"`
	TargetRecordID   string                  `json:"target_record_id"`
	CreatedAt        time.Time               `json:"created_at"`
	DisplayPhase     *string                 `json:"display_phase,omitempty"`
	Status           *string                 `json:"status,omitempty"`
	Result           *string                 `json:"result,omitempty"`
	ToolPresentation *agent.ToolPresentation `json:"tool_presentation,omitempty"`
	ArgsAppend       string                  `json:"args_append,omitempty"`
	ContentAppend    string                  `json:"content_append,omitempty"`
	Illustration     *ChapterIllustration    `json:"illustration,omitempty"`
	Ask              *AskInteraction         `json:"ask,omitempty"`
}

type interruptionPatchRecord struct {
	Type       string     `json:"type"`
	TargetID   string     `json:"target_id"`
	Status     string     `json:"status"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type contextBatchRecord struct {
	Type            string               `json:"type"`
	CreatedAt       time.Time            `json:"created_at"`
	Identity        DomainCommitIdentity `json:"identity"`
	Sequence        int                  `json:"sequence"`
	ContextRevision uint64               `json:"context_revision"`
	Messages        []agent.Message      `json:"messages"`
}

// releasedContextCompactionRecord is the exact v0.3.3 Product Session
// checkpoint schema. It is retained only long enough to project the released
// record into Agent's versioned capability lane in this same journal.
type releasedContextCompactionRecord struct {
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

type releasedContextCompactionRemovalRecord struct {
	Type             string    `json:"type"`
	ID               string    `json:"id"`
	AgentKind        string    `json:"agent_kind,omitempty"`
	CompactionID     string    `json:"compaction_id,omitempty"`
	SourceStartIndex int       `json:"source_start_index"`
	SourceEndIndex   int       `json:"source_end_index"`
	Reason           string    `json:"reason,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}
