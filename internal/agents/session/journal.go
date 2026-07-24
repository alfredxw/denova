package session

import (
	"time"
)

const (
	journalFormatVersion         = 2
	historyTypeSessionPatch      = "session_patch"
	historyTypeDisplayPatch      = "display_patch"
	historyTypeInterruptionPatch = "interruption_patch"
)

// sessionHeader is immutable journal metadata. Mutable session attributes are
// persisted as patch records so an update never rewrites prior history.
type sessionHeader struct {
	Type          string    `json:"type"`
	Version       int       `json:"version,omitempty"`
	ID            string    `json:"id"`
	IncarnationID string    `json:"incarnation_id,omitempty"`
	Title         string    `json:"title,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
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
	Type      string    `json:"type"`
	Title     *string   `json:"title,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// displayPatchRecord stores only the mutation. Pointer fields distinguish an
// explicit empty value from an omitted field.
type displayPatchRecord struct {
	Type           string               `json:"type"`
	TargetRecordID string               `json:"target_record_id"`
	CreatedAt      time.Time            `json:"created_at"`
	Status         *string              `json:"status,omitempty"`
	Result         *string              `json:"result,omitempty"`
	ArgsAppend     string               `json:"args_append,omitempty"`
	ContentAppend  string               `json:"content_append,omitempty"`
	Illustration   *ChapterIllustration `json:"illustration,omitempty"`
}

type interruptionPatchRecord struct {
	Type       string     `json:"type"`
	TargetID   string     `json:"target_id"`
	Status     string     `json:"status"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
