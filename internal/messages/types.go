package messages

import "time"

const (
	MessageTypeChangelog        = "changelog"
	MessageTypeAutomation       = "automation"
	MessageTypeAutomationAction = "automation_action"
)

// Message describes one user-visible product notice. Message content is
// derived from bounded sources; per-user state only stores read metadata.
type Message struct {
	ID             string     `json:"id"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary,omitempty"`
	Body           string     `json:"body"`
	PublishedAt    string     `json:"published_at,omitempty"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	TaskID         string     `json:"task_id,omitempty"`
	RunID          string     `json:"run_id,omitempty"`
	InboxID        string     `json:"inbox_id,omitempty"`
	Workspace      string     `json:"workspace,omitempty"`
	Status         string     `json:"status,omitempty"`
	ActionRequired bool       `json:"action_required,omitempty"`
}

type ListResult struct {
	Items       []Message `json:"items"`
	UnreadCount int       `json:"unread_count"`
}
