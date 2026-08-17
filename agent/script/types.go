// Package script executes small, synchronous JavaScript function bodies over
// a deliberately narrow host API. It knows nothing about Agent registries,
// permissions, sessions, or persisted user state.
package script

import (
	"context"
	"encoding/json"

	"github.com/dop251/goja"
)

// ContractVersion changes only when the observable JavaScript or host contract
// changes. Saved Script Tool implementation identities include this value.
const ContractVersion = 1

// Config contains limits enforced by the in-process JavaScript engine.
type Config struct {
	MaxSourceBytes   int
	MaxOutputBytes   int
	MaxCallStackSize int
}

// Source is a JavaScript function body. Name is diagnostic-only and must not
// contain an absolute host path when it can reach a user-visible error.
type Source struct {
	Name string
	Code string
}

// Program is an immutable compiled function body. Its implementation remains
// private so callers cannot couple themselves to Goja.
type Program struct {
	compiled *goja.Program
	name     string
}

// Call is one tool request crossing the script host boundary.
type Call struct {
	Name      string
	Arguments json.RawMessage
}

// Outcome is the complete script-visible projection of one tool result.
type Outcome struct {
	Tool      string          `json:"tool"`
	OK        bool            `json:"ok"`
	Status    string          `json:"status"`
	Output    json.RawMessage `json:"output"`
	Truncated bool            `json:"truncated"`
	Artifacts []Artifact      `json:"artifacts"`
	Reason    string          `json:"reason"`
}

// Artifact is the bounded script-visible subset of an Agent artifact receipt.
type Artifact struct {
	ID           string `json:"id"`
	ReadablePath string `json:"readable_path,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	Complete     bool   `json:"complete"`
}

// Host performs a source-ordered batch through the owning Agent execution
// pipeline. Ordinary tool failures belong in Outcomes; error is reserved for
// cancellation or a broken host contract.
type Host interface {
	CallTools(context.Context, []Call) ([]Outcome, error)
}

// Diagnostic is a stable compile diagnostic suitable for editor markers.
type Diagnostic struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

// Failure is a user-program failure. Engine and Host infrastructure failures
// are returned as Go errors instead.
type Failure struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

// RunResult contains exactly one JSON value plus bounded diagnostic logs.
type RunResult struct {
	Value   json.RawMessage `json:"value,omitempty"`
	Logs    []string        `json:"logs,omitempty"`
	Failure *Failure        `json:"failure,omitempty"`
}
