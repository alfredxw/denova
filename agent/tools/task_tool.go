package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

var (
	ErrTaskCapacityExceeded = errors.New("task capacity exceeded")
	ErrTaskInvalidInput     = errors.New("invalid task input")
)

// DefaultTaskAgentName is the built-in general delegated Agent selected when
// send delegate omits an explicit catalog name.
const DefaultTaskAgentName = "general-purpose"

type TaskRef struct {
	Agent   string `json:"agent,omitempty" jsonschema_description:"Delegated Agent name returned by send delegate."`
	Session string `json:"session,omitempty" jsonschema_description:"Child Session ID returned by send delegate."`
	Run     string `json:"run,omitempty" jsonschema_description:"Child Run ID returned by send delegate."`
}

type TaskRequest struct {
	Agent          string `json:"agent,omitempty" jsonschema_description:"Optional stable delegated Agent name from list_agents definitions. Omit it to use the built-in general-purpose Agent."`
	Prompt         string `json:"prompt,omitempty" jsonschema_description:"Self-contained goal, constraints, relevant references, expected output, and write scope."`
	IdempotencyKey string `json:"idempotency_key,omitempty" jsonschema_description:"Stable retry identity; omit to derive it from this tool execution."`
}

type Task struct {
	Ref     TaskRef               `json:"ref"`
	Status  string                `json:"status"`
	Reason  string                `json:"reason,omitempty"`
	Output  string                `json:"output,omitempty"`
	Receipt *agent.CommandReceipt `json:"receipt,omitempty"`
}

type TaskObservation struct {
	Task       Task        `json:"task"`
	Cursor     string      `json:"cursor,omitempty"`
	Output     string      `json:"output,omitempty"`
	Events     []TaskEvent `json:"events,omitempty"`
	Incomplete bool        `json:"incomplete,omitempty"`
}

// TaskWaitOutcome is one executor-owned synchronization snapshot returned
// after any target is ready. Task contains identity and status only; terminal
// payloads arrive through the parent completion mailbox or explicit observe.
// Err is per-target; a top-level Wait error is reserved for an interrupted wait
// or a failed Host interaction.
type TaskWaitOutcome struct {
	Task  *Task
	Ready bool
	Err   error
}

// TaskEvent is the bounded reconnect projection returned by observe. Live
// child events are also forwarded through the parent Agent invocation while
// await is active.
type TaskEvent struct {
	Cursor string      `json:"cursor,omitempty"`
	Type   string      `json:"type"`
	Run    string      `json:"run,omitempty"`
	Text   string      `json:"text,omitempty"`
	Tool   string      `json:"tool,omitempty"`
	Event  agent.Event `json:"event"`
}

// TaskExecutor owns exact-target authorization and durable command admission.
// New Session and Run IDs must be ASCII with at most 128 bytes so send can reserve
// its receipt budget before dispatch. Accepted commands must return their receipt
// even when a subsequent tracking or observation step fails.
type TaskExecutor interface {
	Identity() agent.CapabilityIdentity
	Start(context.Context, TaskRequest) (Task, error)
	FollowUp(context.Context, TaskRef, agent.Input) (Task, error)
	SendMessage(context.Context, TaskRef, agent.Input) (agent.CommandReceipt, error)
	Resume(context.Context, TaskRef, agent.ResumeRequest) (Task, error)
	Observe(context.Context, TaskRef, string) (TaskObservation, error)
	Wait(context.Context, []TaskRef) ([]TaskWaitOutcome, error)
	Steer(context.Context, TaskRef, agent.Input) (agent.CommandReceipt, error)
	Respond(context.Context, TaskRef, string, agent.InteractionResponse) error
	Abort(context.Context, TaskRef, agent.AbortRequest) (agent.CommandReceipt, error)
	Interrupt(context.Context, TaskRef, agent.SuspendRequest) (agent.CommandReceipt, error)
	ListAgents(context.Context, ListAgentsInput) (ListAgentsOutput, error)
}

type TaskAgentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func validateTaskRef(ref TaskRef) error {
	if err := validateTaskString("ref.agent", ref.Agent, 256); err != nil {
		return err
	}
	if err := validateTaskString("ref.session", ref.Session, 1024); err != nil {
		return err
	}
	return validateTaskString("ref.run", ref.Run, 1024)
}

func validateTaskString(name, value string, maxBytes int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is required", ErrTaskInvalidInput, name)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%w: %s exceeds %d bytes", ErrTaskInvalidInput, name, maxBytes)
	}
	return nil
}

func taskActionCommandID(ctx context.Context, action string, index int) string {
	executionID := agent.CurrentToolExecutionID(ctx)
	if executionID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%d", executionID, strings.TrimSpace(action), index)
}
