package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// Agent operations reuse Session command receipts; they do not create a second
// task registry. A receipt acknowledges durable admission, never completion.
type agentReceipt struct {
	CommandID string `json:"command_id"`
	Cursor    string `json:"cursor"`
}
type agentToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
type sendItem struct {
	invalid        error
	Action         string   `json:"action" jsonschema:"enum=delegate,enum=message,enum=followup,enum=steer,enum=interrupt,enum=resume,enum=abort" jsonschema_description:"delegate, message, followup, steer, interrupt, resume, or abort. Each item is validated independently."`
	Agent          *string  `json:"agent,omitempty" jsonschema_description:"delegate only. Omit for general-purpose; use list_agents definitions for available types."`
	To             *TaskRef `json:"to,omitempty" jsonschema_description:"Existing target. message/followup require agent and session only. Run controls require agent, session and run."`
	Message        *string  `json:"message,omitempty" jsonschema_description:"Required for delegate, message, followup and steer. Self-contained instructions, at most 1 MiB UTF-8."`
	Reason         *string  `json:"reason,omitempty" jsonschema_description:"Required for interrupt and abort, at most 64 KiB UTF-8."`
	IdempotencyKey string   `json:"idempotency_key,omitempty" jsonschema_description:"Optional stable retry identity, at most 256 bytes. Normally derived from the tool execution and item index."`
}
type sendInput struct {
	Items []sendItem `json:"items" jsonschema:"minItems=1,maxItems=32"`
}
type sendResult struct {
	Index   int             `json:"index"`
	Outcome string          `json:"outcome"`
	Ref     *TaskRef        `json:"ref,omitempty"`
	Receipt *agentReceipt   `json:"receipt,omitempty"`
	Status  string          `json:"status,omitempty"`
	Error   *agentToolError `json:"error,omitempty"`
}

// Tasks exposes the complete delegation surface: send, await, and list_agents.
// The executor owns authorization and exact Run guards on every operation.
func Tasks(executor TaskExecutor) agent.Toolset {
	return defineToolset(func(context.Context) (agent.Toolset, error) {
		if executor == nil || executor.Identity().Kind == "" || executor.Identity().Version == 0 {
			return nil, errors.New("delegation requires an identified TaskExecutor")
		}
		schema, err := batchToolSchema[sendInput]("items")
		if err != nil {
			return nil, err
		}
		send, err := newSchemaTool("send", "Delegate independent work or communicate with existing child Agents. Use delegation only when explicitly requested. delegate creates a new isolated Session and Run and returns immediately; keep doing independent work. message adds context without waking idle or suspended work. followup creates a new Run queued behind existing work. steer adds priority instructions to the exact active or suspended Run without resuming it. interrupt pauses at a safe boundary; resume continues that same Run; abort terminates only the referenced Run. Never use a stale Run ref to control newer work. Accepted receipts acknowledge commands, not completion. Items succeed or fail independently. Discover types and existing instances with list_agents. Wait at real dependencies with await; completed results also arrive automatically.",
			schema,
			func(ctx context.Context, input sendInput) (agent.ToolResult, error) {
				if len(input.Items) < 1 || len(input.Items) > 32 {
					return agentFailure(fmt.Errorf("%w: send requires 1..32 items", ErrTaskInvalidInput))
				}
				if !sendReceiptsFit(ctx, input.Items, agentResultLimit(executor)) {
					return agentFailure(ErrAgentResultTooLarge)
				}
				results := make([]sendResult, len(input.Items))
				for index, item := range input.Items {
					results[index] = executeSend(ctx, executor, index, item)
				}
				return JSONResult(struct {
					Results []sendResult `json:"results"`
				}{results})
			})
		if err != nil {
			return nil, err
		}
		descriptor := writeDescriptor()
		descriptor.Source, descriptor.Capability = agent.ToolSourceOther, "delegation"
		descriptor.Execution, descriptor.MutationScope = agent.ToolExecutionChild, agent.ToolMutationNone
		descriptor.PostCheck, descriptor.Recovery = agent.ToolPostCheckNone, agent.ToolRecoveryReconcilable
		descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationDelegation)
		wait, err := newTaskWaitDefinition(executor)
		if err != nil {
			return nil, err
		}
		list, err := newListAgentsDefinition(executor)
		if err != nil {
			return nil, err
		}
		return agent.StaticToolsIdentified(toolsetIdentity("tools.tasks", executor.Identity()), agent.ToolDefinition{Tool: send, Descriptor: descriptor}, wait, list)
	})
}

func (item sendItem) validate() error {
	if item.invalid != nil {
		return fmt.Errorf("%w: %v", ErrTaskInvalidInput, item.invalid)
	}
	invalid := func(message string) error { return fmt.Errorf("%w: %s", ErrTaskInvalidInput, message) }
	if len(item.IdempotencyKey) > 256 {
		return invalid("idempotency_key exceeds 256 bytes")
	}
	switch item.Action {
	case "delegate":
		if item.To != nil || item.Reason != nil {
			return invalid("delegate accepts agent and message only")
		}
		if item.Agent != nil && validateTaskString("agent", *item.Agent, 256) != nil {
			return invalid("agent is invalid")
		}
	case "message", "followup", "steer", "interrupt", "resume", "abort":
		if item.Agent != nil || item.To == nil {
			return invalid("existing targets require to and do not accept agent")
		}
		if item.Action == "message" || item.Action == "followup" {
			if item.To.Run != "" {
				return invalid("message and followup target a Session, not a Run")
			}
			if err := validateTaskSessionRef(*item.To); err != nil {
				return err
			}
		} else if err := validateTaskRef(*item.To); err != nil {
			return err
		}
	default:
		return invalid("unsupported send action")
	}
	switch item.Action {
	case "delegate", "message", "followup", "steer":
		if item.Message == nil || item.Reason != nil {
			return invalid("this action requires message and does not accept reason")
		}
		return validateTaskString("message", *item.Message, 1<<20)
	case "interrupt", "abort":
		if item.Message != nil || item.Reason == nil {
			return invalid("this action requires reason and does not accept message")
		}
		return validateTaskString("reason", *item.Reason, 65536)
	case "resume":
		if item.Message != nil || item.Reason != nil {
			return invalid("resume accepts no message or reason")
		}
	}
	return nil
}

func executeSend(ctx context.Context, executor TaskExecutor, index int, item sendItem) sendResult {
	result := sendResult{Index: index, Outcome: "error"}
	if err := item.validate(); err != nil {
		result.Error = agentError(err)
		return result
	}
	id := strings.TrimSpace(item.IdempotencyKey)
	if id == "" {
		id = taskActionCommandID(ctx, item.Action, index)
	}
	var task Task
	var receipt agent.CommandReceipt
	var err error
	if item.To != nil {
		task.Ref = *item.To
	}
	switch item.Action {
	case "delegate":
		name := ""
		if item.Agent != nil {
			name = *item.Agent
		}
		task, err = executor.Start(ctx, TaskRequest{Agent: name, Prompt: *item.Message, IdempotencyKey: id})
	case "followup":
		task, err = executor.FollowUp(ctx, *item.To, agent.Input{Text: *item.Message, IdempotencyKey: id})
	case "message":
		receipt, err = executor.SendMessage(ctx, *item.To, agent.Input{Text: *item.Message, IdempotencyKey: id})
	case "steer":
		receipt, err = executor.Steer(ctx, *item.To, agent.Input{Text: *item.Message, IdempotencyKey: id})
	case "interrupt":
		receipt, err = executor.Interrupt(ctx, *item.To, agent.SuspendRequest{RunID: item.To.Run, Reason: *item.Reason, IdempotencyKey: id})
	case "resume":
		task, err = executor.Resume(ctx, *item.To, agent.ResumeRequest{RunID: item.To.Run, IdempotencyKey: id})
	case "abort":
		receipt, err = executor.Abort(ctx, *item.To, agent.AbortRequest{Reason: *item.Reason, IdempotencyKey: id})
	}
	if task.Receipt != nil {
		receipt = *task.Receipt
	}
	if receipt.CommandID != "" {
		result.Outcome, result.Ref, result.Status = "accepted", &task.Ref, task.Status
		result.Receipt = &agentReceipt{CommandID: receipt.CommandID, Cursor: strconv.FormatUint(uint64(receipt.Cursor), 10)}
	} else if err == nil {
		err = errors.New("executor did not return a command receipt")
	}
	result.Error = agentError(err)
	return result
}

var ErrTaskNotFound = errors.New("task target was not found")
var ErrTaskInvalidState = errors.New("task target is not in a valid state for this action")
var ErrAgentResultTooLarge = errors.New("agent result exceeds the configured context budget; request fewer targets or use a file artifact")

const maxAgentErrorBytes = 512

func agentResultLimit(executor TaskExecutor) int {
	if bounded, ok := executor.(interface{ ResultLimit() int }); ok && bounded.ResultLimit() > 0 {
		return min(defaultResultBytes, bounded.ResultLimit())
	}
	return defaultResultBytes
}

// Reserve identity, receipt and bounded diagnostics before any command is sent.
// New native IDs are ASCII and shorter than 128 bytes; existing refs are measured
// with their actual JSON escaping. Budget rejection has no side effects.
func sendReceiptsFit(ctx context.Context, items []sendItem, limit int) bool {
	reserved := len(`{"results":[]}`)
	for index, item := range items {
		ref := TaskRef{Agent: DefaultTaskAgentName, Session: strings.Repeat("x", 128), Run: strings.Repeat("x", 128)}
		if item.Agent != nil {
			ref.Agent = *item.Agent
		}
		if item.To != nil {
			ref = *item.To
			if item.Action == "followup" {
				ref.Run = strings.Repeat("x", 128)
			}
		}
		id := strings.TrimSpace(item.IdempotencyKey)
		if id == "" {
			id = taskActionCommandID(ctx, item.Action, index)
		}
		if id == "" {
			id = strings.Repeat("x", 128)
		}
		result := sendResult{Index: index, Outcome: "accepted", Ref: &ref, Status: "waiting_input",
			Receipt: &agentReceipt{CommandID: id, Cursor: "18446744073709551615"},
			Error:   &agentToolError{Code: "idempotency_conflict", Message: strings.Repeat("\x00", maxAgentErrorBytes)},
		}
		if item.validate() != nil {
			result.Ref, result.Receipt, result.Status, result.Outcome = nil, nil, "", "error"
		}
		encoded, _ := json.Marshal(result)
		reserved += len(encoded) + 1
		if reserved > limit {
			return false
		}
	}
	return true
}

func agentError(err error) *agentToolError {
	if err == nil {
		return nil
	}
	code, retryable := "execution_error", false
	switch {
	case errors.Is(err, ErrTaskInvalidInput), errors.Is(err, agent.ErrInvalidInput):
		code = "invalid_input"
	case errors.Is(err, ErrTaskCapacityExceeded), errors.Is(err, agent.ErrInputQueueFull):
		code, retryable = "capacity_exceeded", true
	case errors.Is(err, ErrTaskNotFound):
		code = "not_found"
	case errors.Is(err, agent.ErrPermissionDenied):
		code = "permission_denied"
	case errors.Is(err, agent.ErrIdempotencyConflict):
		code = "idempotency_conflict"
	case errors.Is(err, agent.ErrNoActiveRun):
		code = "target_changed"
	case errors.Is(err, agent.ErrRunSettled), errors.Is(err, agent.ErrSessionBusy), errors.Is(err, ErrTaskInvalidState):
		code = "invalid_state"
	case errors.Is(err, agent.ErrCursorExpired):
		code = "cursor_expired"
	case errors.Is(err, ErrAgentResultTooLarge):
		code = "result_too_large"
	case errors.Is(err, agent.ErrSessionClosed), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		retryable = true
	}
	message, truncated := boundAgentText(err.Error(), maxAgentErrorBytes-3)
	if truncated {
		message += "..."
	}
	return &agentToolError{Code: code, Message: message, Retryable: retryable}
}
func agentFailure(err error) (agent.ToolResult, error) {
	return JSONResult(struct {
		Error *agentToolError `json:"error"`
	}{agentError(err)})
}

func (item *sendItem) UnmarshalJSON(data []byte) error {
	type plain sendItem
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	item.invalid = decoder.Decode((*plain)(item))
	return nil
}
