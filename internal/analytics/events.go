// Package analytics owns optional, content-free product usage statistics. It
// accepts only bounded product categories, never domain records or model data.
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

// Run captures the collection generation at admission. Disabling statistics
// invalidates this handle, including when collection is enabled again later.
type Run struct {
	service    *Service
	generation uint64
	feature    Feature
	engine     Engine
	trigger    Trigger
	started    time.Time
	once       *sync.Once
}

func (r Run) Finish(result Result) {
	if r.service == nil {
		return
	}
	switch result {
	case Success, Failure, Cancelled, Suspended, Preempted:
	default:
		return
	}
	r.once.Do(func() { r.service.recordRun(r, result) })
}

type event struct {
	Type    string  `json:"type"`
	Payload payload `json:"payload"`
}

// Only this private, explicitly constructed schema crosses the network.
type payload struct {
	Website   string    `json:"website"`
	Hostname  string    `json:"hostname"`
	Tag       string    `json:"tag"`
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Name      string    `json:"name"`
	Timestamp int64     `json:"timestamp"`
	Data      eventData `json:"data"`
}

type eventData struct {
	Version    string  `json:"version,omitempty"`
	Platform   string  `json:"platform,omitempty"`
	CohortDate string  `json:"cohort_date,omitempty"`
	Feature    Feature `json:"feature,omitempty"`
	Runtime    Engine  `json:"runtime,omitempty"`
	Trigger    Trigger `json:"trigger,omitempty"`
	Outcome    Result  `json:"outcome,omitempty"`
	// Non-execution events omit duration; executions retain a measured zero.
	DurationMS *int64 `json:"duration_ms,omitempty"`
	Action     Action `json:"action,omitempty"`
}
