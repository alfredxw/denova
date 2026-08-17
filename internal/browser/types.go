// Package browser provides the isolated, stateful page-session boundary behind
// Denova's model-visible browser tool. The Session owns named tabs; Driver and
// Page keep the browser engine replaceable for tests and future runtimes.
package browser

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrUnavailable = errors.New("browser runtime is unavailable")

const (
	CommandObserve    = "observe"
	CommandGoto       = "goto"
	CommandWait       = "wait"
	CommandClick      = "click"
	CommandFill       = "fill"
	CommandType       = "type"
	CommandPress      = "press"
	CommandSelect     = "select"
	CommandEvaluate   = "evaluate"
	CommandScreenshot = "screenshot"
)

// Driver creates pages in one isolated browser context. Available must not
// launch a browser or download a runtime.
type Driver interface {
	Available(context.Context) error
	NewPage(context.Context) (Page, error)
	Close(context.Context) error
}

// Page is the smallest engine seam needed by the first browser contract.
// Implementations must bind every operation to the supplied Context.
type Page interface {
	Navigate(context.Context, string) error
	Observe(context.Context) (Observation, error)
	Wait(context.Context, WaitCondition) error
	Click(context.Context, string) error
	Fill(context.Context, string, string) error
	Type(context.Context, string, string) error
	Press(context.Context, string, string) error
	Select(context.Context, string, []string) error
	Evaluate(context.Context, string) (json.RawMessage, error)
	Screenshot(context.Context, bool) ([]byte, error)
	Close(context.Context) error
}

// Controller is the application-facing browser session seam used by tools.
type Controller interface {
	Open(context.Context, OpenRequest) (Result, error)
	Run(context.Context, RunRequest) (Result, error)
	Close(context.Context, CloseRequest) (Result, error)
}

type OpenRequest struct {
	Tab string
	URL string
}

type RunRequest struct {
	Tab        string
	Command    string
	URL        string
	Selector   string
	Text       string
	Key        string
	Values     []string
	Expression string
	FullPage   bool
	// TimeoutSeconds applies only to wait. Zero keeps the caller Context's
	// existing lifetime and therefore means no tool-imposed deadline.
	TimeoutSeconds int
}

// WaitCondition is satisfied when every non-empty field matches. Selector
// means a visible CSS match; Text means a substring of visible body text.
type WaitCondition struct {
	Selector string
	Text     string
}

type CloseRequest struct {
	Tab string
	All bool
}

type ElementSummary struct {
	Ref      string `json:"ref"`
	Role     string `json:"role"`
	Name     string `json:"name,omitempty"`
	Selector string `json:"selector"`
}

type Observation struct {
	URL       string           `json:"url"`
	Title     string           `json:"title,omitempty"`
	Text      string           `json:"text,omitempty"`
	Elements  []ElementSummary `json:"elements,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
}

type ScreenshotArtifact struct {
	Path     string `json:"path"`
	MIMEType string `json:"mime_type"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
}

type ExternalReceipt struct {
	Schema    string `json:"schema"`
	Boundary  string `json:"boundary"`
	Operation string `json:"operation"`
	Target    string `json:"target,omitempty"`
	Status    string `json:"status"`
}

type Result struct {
	Schema      string              `json:"schema"`
	Status      string              `json:"status"`
	Action      string              `json:"action"`
	Tab         string              `json:"tab,omitempty"`
	Command     string              `json:"command,omitempty"`
	Tabs        []string            `json:"tabs,omitempty"`
	Observation *Observation        `json:"observation,omitempty"`
	Value       json.RawMessage     `json:"value,omitempty"`
	Screenshot  *ScreenshotArtifact `json:"screenshot,omitempty"`
	Receipt     ExternalReceipt     `json:"receipt"`
}

// Options carry hard safety seams, not user preferences. ValidateURL is
// injectable so Session behavior can be tested without weakening production's
// public-network-only validator.
type Options struct {
	MaxTabs      int
	ArtifactRoot string
	ValidateURL  func(context.Context, string) (string, error)
}
