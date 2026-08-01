package filewatch

// ChangeType is the normalized filesystem operation exposed to callers.
// Native watcher details deliberately stay inside this package.
type ChangeType string

const (
	ChangeAdded   ChangeType = "added"
	ChangeUpdated ChangeType = "updated"
	ChangeDeleted ChangeType = "deleted"
)

// Change identifies one workspace-relative path changed on disk.
type Change struct {
	Path string     `json:"path"`
	Type ChangeType `json:"type"`
}

// Event is an ephemeral synchronization hint. Callers must re-read canonical
// workspace state; events are intentionally not a durable change journal.
type Event struct {
	Workspace string   `json:"workspace"`
	Source    string   `json:"source"`
	Changes   []Change `json:"changes,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	Resync    bool     `json:"resync,omitempty"`
}

type batch struct {
	changes []Change
	resync  bool
}
