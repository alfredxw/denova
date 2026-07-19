package autosaveconflict

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	// DirectoryName is the append-only conflict journal under the configured
	// Denova data directory.
	DirectoryName = "conflicts"
	recordVersion = 1
)

// ErrInvalidInput reports a conflict record that cannot be durably restored.
var ErrInvalidInput = errors.New("invalid autosave conflict input")

// Snapshot is one revisioned JSON value involved in a three-way merge.
type Snapshot struct {
	Revision string          `json:"revision,omitempty"`
	Value    json.RawMessage `json:"value"`
}

// Input contains every value needed to recover a lossy autosave overlap.
type Input struct {
	Resource      string     `json:"resource"`
	Scope         string     `json:"scope"`
	ID            string     `json:"id"`
	Base          Snapshot   `json:"base"`
	Local         Snapshot   `json:"local"`
	External      Snapshot   `json:"external"`
	Merged        Snapshot   `json:"merged"`
	Strategy      string     `json:"strategy"`
	ConflictPaths [][]string `json:"conflict_paths"`
}

// Record is the immutable on-disk representation of one conflict.
type Record struct {
	Version       int        `json:"version"`
	ID            string     `json:"record_id"`
	CreatedAt     time.Time  `json:"created_at"`
	Resource      string     `json:"resource"`
	Scope         string     `json:"scope"`
	ResourceID    string     `json:"id"`
	Base          Snapshot   `json:"base"`
	Local         Snapshot   `json:"local"`
	External      Snapshot   `json:"external"`
	Merged        Snapshot   `json:"merged"`
	Strategy      string     `json:"strategy"`
	ConflictPaths [][]string `json:"conflict_paths"`
}

// AppendResult identifies the immutable record and its local recovery path.
type AppendResult struct {
	Record Record
	Path   string
}
