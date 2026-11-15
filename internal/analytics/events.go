// Package analytics owns optional, content-free product usage statistics.
// Callers supply closed categories, never creative content or raw diagnostics.
package analytics

import (
	"sync"
	"time"
)

type Feature string

const (
	Writing    Feature = "writing"
	Game       Feature = "game"
	Chat       Feature = "chat"
	Image      Feature = "image"
	Library    Feature = "library"
	Versions   Feature = "versions"
	Automation Feature = "automation"
	Projects   Feature = "projects"
	Resources  Feature = "resources"
)

func (f Feature) Valid() bool {
	switch f {
	case Writing, Game, Chat, Image, Library, Versions, Automation, Projects, Resources:
		return true
	}
	return false
}

type Engine string

const (
	Native      Engine = "native"
	Codex       Engine = "codex"
	Claude      Engine = "claude"
	Grok        Engine = "grok"
	Antigravity Engine = "antigravity"
)

type Trigger string

const (
	User      Trigger = "user"
	Scheduled Trigger = "automation"
)

type Result string

const (
	Success   Result = "success"
	Failure   Result = "failure"
	Cancelled Result = "cancelled"
	Suspended Result = "suspended"
	Preempted Result = "preempted"
)

type Action string

const (
	BookCreated     Action = "book_created"
	ContentExported Action = "content_exported"
	VersionRestored Action = "version_restored"
)

// Run represents one admitted execution, not an internal model/tool round.
// Finish and Fail are mutually exclusive and idempotent. Opt-out invalidates
// existing handles, including after collection is enabled again.
type Run struct {
	service    *Service
	generation uint64
	feature    Feature
	engine     Engine
	trigger    Trigger
	started    time.Time
	id         string
	once       *sync.Once
}

func (r Run) Finish(result Result) {
	if r.service == nil {
		return
	}
	switch result {
	case Success, Failure, Cancelled, Suspended, Preempted:
		r.once.Do(func() { r.service.recordRun(r, result, "") })
	}
}

// Fail classifies a local error without serializing its message or stack.
func (r Run) Fail(err error) {
	if r.service == nil {
		return
	}
	r.once.Do(func() { r.service.recordRun(r, Failure, classifyError(err)) })
}

// event is PostHog's public capture protocol. No management credential is used.
type event struct {
	APIKey     string    `json:"api_key"`
	Event      string    `json:"event"`
	DistinctID string    `json:"distinct_id"`
	Timestamp  time.Time `json:"timestamp"`
	Properties eventData `json:"properties"`
}

// Only explicitly constructed properties cross the network. Settings values
// and error codes are validated enums; arbitrary objects are never accepted.
type eventData struct {
	AppVersion      string      `json:"app_version"`
	OS              string      `json:"os"`
	Source          string      `json:"source"`
	SchemaVersion   int         `json:"schema_version"`
	CohortDate      string      `json:"cohort_date,omitempty"`
	FirstSeenDate   string      `json:"first_seen_date,omitempty"`
	CohortOrigin    string      `json:"cohort_origin,omitempty"`
	FirstAppVersion string      `json:"first_app_version,omitempty"`
	Feature         Feature     `json:"feature_name,omitempty"`
	Operation       string      `json:"operation,omitempty"`
	Runtime         Engine      `json:"runtime,omitempty"`
	Trigger         Trigger     `json:"trigger,omitempty"`
	Outcome         Result      `json:"outcome,omitempty"`
	DurationMS      *int64      `json:"duration_ms,omitempty"`
	RunID           string      `json:"run_id,omitempty"`
	Action          Action      `json:"action,omitempty"`
	ErrorCode       ErrorCode   `json:"error_code,omitempty"`
	ErrorSource     ErrorSource `json:"error_source,omitempty"`
	StatusCode      int         `json:"status_code,omitempty"`
	SettingName     string      `json:"setting_name,omitempty"`
	SettingValue    string      `json:"value,omitempty"`
}
