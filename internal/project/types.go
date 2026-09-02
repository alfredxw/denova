// Package project owns Denova's user-level Project identity and storage layout.
//
// A Project is deliberately independent from its content directory. The ID is
// stable across display-name changes and directory relinks, while Type selects
// the product behavior (for example the Writing, General, or Agents Project).
package project

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrNotFound means no durable Project owns the requested stable identity.
	ErrNotFound = errors.New("project not found")
	// ErrArchived means the Project still owns a durable Project Store but is
	// hidden from active use until explicitly restored or relinked.
	ErrArchived = errors.New("project is archived")
	// ErrUnavailable means the Project record exists but its content directory
	// cannot currently be opened.
	ErrUnavailable = errors.New("project directory is unavailable")
)

type Type string

const (
	TypeBook    Type = "book"
	TypeGeneral Type = "general"
	TypeAgents  Type = "agents"
)

func (kind Type) Valid() bool {
	return kind == TypeBook || kind == TypeGeneral || kind == TypeAgents
}

type Status string

const (
	StatusAvailable Status = "available"
	StatusMissing   Status = "missing"
	StatusArchived  Status = "archived"
)

type SortMode string

const (
	SortRecent SortMode = "recent"
	SortManual SortMode = "manual"
)

// LocationKind identifies how Project content is resolved. Managed content is
// relative to the current Denova data root; external content is an opaque host
// path and may be unavailable after moving the data root to another host.
type LocationKind string

const (
	LocationManaged  LocationKind = "managed"
	LocationExternal LocationKind = "external"
)

func (kind LocationKind) Valid() bool {
	return kind == LocationManaged || kind == LocationExternal
}

// ProjectLocation is the durable content locator. Managed paths always use
// canonical slash-relative syntax. External paths are never rewritten merely
// because the registry is opened on another operating system.
type ProjectLocation struct {
	Kind LocationKind `json:"kind"`
	Path string       `json:"path"`
}

// Record is the durable user-owned Project definition. WorkspacePath and
// Status are runtime/API projections and are cleared before registry storage.
type Record struct {
	ID   string `json:"id"`
	Type Type   `json:"type"`
	Name string `json:"name"`
	// StoreDirName is the immutable, human-readable directory segment below
	// stores. It is deliberately separate from both stable identity and
	// the mutable display name.
	StoreDirName string          `json:"store_dir"`
	Location     ProjectLocation `json:"location"`
	// WorkspacePath is resolved from Location for the current host. It remains
	// in API responses for existing clients but is never authoritative on disk.
	WorkspacePath string     `json:"path,omitempty"`
	Status        Status     `json:"status,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastOpenedAt  time.Time  `json:"last_opened_at,omitempty"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
}

// UnmarshalJSON accepts the unreleased state_dir field only long enough to
// rewrite the current Project Registry using the Project Store terminology.
func (record *Record) UnmarshalJSON(data []byte) error {
	type wireRecord Record
	decoded := struct {
		*wireRecord
		LegacyStateDirName string `json:"state_dir"`
	}{wireRecord: (*wireRecord)(record)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if record.StoreDirName == "" {
		record.StoreDirName = decoded.LegacyStateDirName
	}
	return nil
}

// Layout separates content owned by a Project from its Denova Project Store.
// Interactive story content intentionally remains beneath ContentRoot.
type Layout struct {
	ProjectID   string
	Type        Type
	ContentRoot string
	StoreRoot   string
}

func (layout Layout) SessionsDir() string    { return joinStore(layout.StoreRoot, "sessions") }
func (layout Layout) ConfigPath() string     { return joinStore(layout.StoreRoot, "config.toml") }
func (layout Layout) ChangesDir() string     { return joinStore(layout.StoreRoot, "changes") }
func (layout Layout) ReviewsDir() string     { return joinStore(layout.StoreRoot, "reviews") }
func (layout Layout) RunsDir() string        { return joinStore(layout.StoreRoot, "runs") }
func (layout Layout) ArtifactsDir() string   { return joinStore(layout.StoreRoot, "artifacts") }
func (layout Layout) AutomationsDir() string { return joinStore(layout.StoreRoot, "automations") }
func (layout Layout) VersionRepositoryDir() string {
	return joinStore(layout.StoreRoot, "versions", "repository")
}
