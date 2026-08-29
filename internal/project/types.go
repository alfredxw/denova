// Package project owns Denova's user-level Project identity and storage layout.
//
// A Project is deliberately independent from its content directory. The ID is
// stable across display-name changes and directory relinks, while Type selects
// the product behavior (for example the Writing, General, or Harness Agent).
package project

import (
	"errors"
	"time"
)

var (
	// ErrNotFound means no durable Project owns the requested stable identity.
	ErrNotFound = errors.New("project not found")
	// ErrArchived means the Project still owns durable user state but is hidden
	// from active use until explicitly restored or relinked.
	ErrArchived = errors.New("project is archived")
	// ErrUnavailable means the Project record exists but its content directory
	// cannot currently be opened.
	ErrUnavailable = errors.New("project directory is unavailable")
)

type Type string

const (
	TypeBook    Type = "book"
	TypeGeneral Type = "general"
	TypeHarness Type = "harness"
)

func (kind Type) Valid() bool {
	return kind == TypeBook || kind == TypeGeneral || kind == TypeHarness
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

// Record is the durable user-owned Project definition. WorkspacePath points
// at project content; it is not the Project identity and is safe to relink.
type Record struct {
	ID   string `json:"id"`
	Type Type   `json:"type"`
	Name string `json:"name"`
	// StateDirName is the immutable, human-readable directory segment below
	// project-state. It is deliberately separate from both stable identity and
	// the mutable display name.
	StateDirName  string     `json:"state_dir"`
	WorkspacePath string     `json:"path"`
	Status        Status     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastOpenedAt  time.Time  `json:"last_opened_at,omitempty"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
}

// Layout separates content owned by a Project from state owned by the user.
// Interactive story content intentionally remains beneath ContentRoot.
type Layout struct {
	ProjectID   string
	Type        Type
	ContentRoot string
	StateRoot   string
}

func (layout Layout) SessionsDir() string    { return joinState(layout.StateRoot, "sessions") }
func (layout Layout) ConfigPath() string     { return joinState(layout.StateRoot, "config.toml") }
func (layout Layout) ChangesDir() string     { return joinState(layout.StateRoot, "changes") }
func (layout Layout) ReviewsDir() string     { return joinState(layout.StateRoot, "reviews") }
func (layout Layout) RunsDir() string        { return joinState(layout.StateRoot, "runs") }
func (layout Layout) ArtifactsDir() string   { return joinState(layout.StateRoot, "artifacts") }
func (layout Layout) AutomationsDir() string { return joinState(layout.StateRoot, "automations") }
func (layout Layout) VersionRepositoryDir() string {
	return joinState(layout.StateRoot, "versions", "repository")
}
