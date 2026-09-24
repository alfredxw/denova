// Package external owns product execution through non-Native Agent engines.
// Provider adapters receive scoped tools and content, never the application or
// a Native Agent run. Product acceptance and journal settlement belong to Host.
package external

import (
	"context"
	"encoding/json"

	"denova/config"
	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

// Adapter runs one engine turn or maintenance operation. Result is a candidate response;
// it does not acknowledge durable product completion.
type Adapter interface {
	// Version identifies the protocol/engine used to produce portable checkpoints.
	Version() string
	Run(context.Context, Input, Host) (Result, error)
}

// Input is prepared by the product boundary. Instructions contain only shared
// role/context settings; Native permissions, compaction and delegation are not
// part of this contract. History is rebuilt from the canonical product journal.
type Input struct {
	Selection    config.RuntimeSelection
	Instructions string
	History      []Message
	// HistoryBoundary identifies a canonical branch view when message positions
	// are relative to that view. Session journal messages already carry cursors.
	HistoryBoundary string
	Text            string
	Attachments     []agent.Attachment
	Tools           []Tool
	// Plan is the last canonical provider observation. It seeds event projection
	// on resume and supplies progress context when the disposable cache is gone.
	Plan []agent.TodoItem
	// These fields are host-local cache projections, never canonical facts.
	SessionID       string
	Directory       string
	Mode            OperationMode
	CumulativeUsage *agent.TokenUsage
}

type OperationMode string

const (
	OperationTurn      OperationMode = ""
	OperationSummarize OperationMode = "summarize"
	OperationCompact   OperationMode = "compact"
	OperationEvaluate  OperationMode = "evaluate"
)

// Message carries public conversation content only. Tool observations are
// rendered by the host as confirmed facts; private reasoning is never included.
type Message struct {
	Role        string
	Text        string
	Attachments []agent.Attachment
	ToolImages  []agent.Attachment
	// Cursor is host-only provenance and is never sent to the engine.
	Cursor uint64
}

type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// ToolCall.ID identifies one provider call within this attempt. Host maps it to
// a durable execution ID; identical arguments in a new call remain a new action.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ToolResult struct {
	Text    string
	Success bool
	Images  []agent.Attachment
}

type Result struct {
	Text      string
	SessionID string
	// Plan carries the final observed snapshot to a continuation of this lease.
	// Nil means no plan observation; an empty snapshot means the plan was cleared.
	Plan *agent.TodoState
	// Settled means the provider terminal response and all host callbacks were
	// drained. Only this acknowledgement permits cache reuse after interruption.
	Settled bool
	// Usage is the attempt's reported total, including failed attempts when
	// available. A missing value is not an estimate of zero consumption.
	Usage *agent.TokenUsage
	// CumulativeUsage is a host-only baseline for engines reporting thread totals.
	CumulativeUsage *agent.TokenUsage
}

// Host binds the Project, Session and accepted configuration revision. A tool
// result is returned only after its product outcome is durably recorded.
type Host interface {
	Emit(agentrun.Event) error
	CallTool(context.Context, ToolCall) (ToolResult, error)
}
