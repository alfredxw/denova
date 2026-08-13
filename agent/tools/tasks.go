package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type TaskRef struct {
	Agent   string `json:"agent"`
	Session string `json:"session"`
	Run     string `json:"run"`
}

type TaskRequest struct {
	Agent          string `json:"agent" jsonschema:"description=Stable name of the delegated Agent"`
	Prompt         string `json:"prompt" jsonschema:"description=Complete self-contained task including context constraints and expected output,maxLength=1048576"`
	Detached       bool   `json:"detached,omitempty" jsonschema:"description=Return immediately and use observe/steer/respond/abort to control the task"`
	IdempotencyKey string `json:"idempotency_key,omitempty" jsonschema:"description=Optional stable retry identity,maxLength=65536"`
}

type Task struct {
	Ref    TaskRef `json:"ref"`
	Status string  `json:"status"`
	Output string  `json:"output,omitempty"`
}

type TaskObservation struct {
	Task            Task                       `json:"task"`
	Cursor          string                     `json:"cursor,omitempty"`
	Output          string                     `json:"output,omitempty"`
	Events          []TaskEvent                `json:"events,omitempty"`
	Interactions    []agent.InteractionRequest `json:"interactions,omitempty"`
	RecoveryActions []agent.RecoveryAction     `json:"recovery_actions,omitempty"`
	Incomplete      bool                       `json:"incomplete,omitempty"`
}

// TaskInteractionResponse targets one durable child Run interaction. Ref and
// InteractionID are sufficient after a process restart; no executor-local
// waiter or task map participates in resolution.
type TaskInteractionResponse struct {
	Ref           TaskRef                   `json:"ref"`
	InteractionID string                    `json:"interaction_id"`
	Response      agent.InteractionResponse `json:"response"`
}

// TaskEvent is the bounded reconnect projection returned by observe. Live
// child events are also forwarded through the parent Agent invocation while a
// non-detached start is waiting.
type TaskEvent struct {
	Cursor     string                `json:"cursor,omitempty"`
	Type       string                `json:"type"`
	Durability agent.EventDurability `json:"durability,omitempty"`
	Run        string                `json:"run,omitempty"`
	Text       string                `json:"text,omitempty"`
	Tool       string                `json:"tool,omitempty"`
	Event      agent.Event           `json:"event"`
}

type TaskExecutor interface {
	Identity() agent.CapabilityIdentity
	Start(context.Context, TaskRequest) (Task, error)
	Observe(context.Context, TaskRef, string) (TaskObservation, error)
	Steer(context.Context, TaskRef, agent.Input) error
	Respond(context.Context, TaskRef, string, agent.InteractionResponse) error
	Abort(context.Context, TaskRef, agent.AbortRequest) error
}

type TaskAgentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type taskAgentCatalog interface {
	TaskAgents() []TaskAgentInfo
}

type taskToolInput struct {
	Action    string                    `json:"action" jsonschema:"enum=start,enum=observe,enum=steer,enum=respond,enum=abort"`
	Starts    []TaskRequest             `json:"starts,omitempty" jsonschema:"maxItems=32"`
	Refs      []TaskRef                 `json:"refs,omitempty" jsonschema:"maxItems=32"`
	Responses []TaskInteractionResponse `json:"responses,omitempty" jsonschema:"maxItems=32"`
	Cursor    string                    `json:"cursor,omitempty"`
	Input     string                    `json:"input,omitempty" jsonschema:"maxLength=1048576"`
	Reason    string                    `json:"reason,omitempty" jsonschema:"maxLength=65536"`
}

type taskItemResult struct {
	Index       int              `json:"index"`
	Task        *Task            `json:"task,omitempty"`
	Observation *TaskObservation `json:"observation,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// Tasks connects the common task tool to a local subagent, remote worker, or
// host task system. Every batch item is attempted independently.
func Tasks(executor TaskExecutor) (agent.Toolset, error) {
	if executor == nil {
		return nil, errors.New("tasks Toolset requires a TaskExecutor")
	}
	identity := executor.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return nil, errors.New("tasks TaskExecutor requires a stable Identity")
	}
	description := "Start, observe, steer, respond to, or abort delegated tasks. Batch operations return per-item outcomes."
	if catalog, ok := executor.(taskAgentCatalog); ok {
		for _, candidate := range catalog.TaskAgents() {
			description += fmt.Sprintf("\n- %s: %s", candidate.Name, candidate.Description)
		}
	}
	tool, err := agent.InferTool("task", description, func(ctx context.Context, input taskToolInput) (agent.ToolResult, error) {
		results := make([]taskItemResult, 0)
		switch strings.TrimSpace(input.Action) {
		case "start":
			if len(input.Starts) == 0 {
				return agent.ToolResult{}, errors.New("task start requires at least one request")
			}
			for index, request := range input.Starts {
				item := taskItemResult{Index: index}
				if strings.TrimSpace(request.IdempotencyKey) == "" {
					if executionID := agent.CurrentToolExecutionID(ctx); executionID != "" {
						request.IdempotencyKey = fmt.Sprintf("%s:%d", executionID, index)
					}
				}
				task, itemErr := executor.Start(ctx, request)
				if itemErr != nil {
					item.Error = itemErr.Error()
				} else {
					item.Task = &task
				}
				results = append(results, item)
			}
		case "respond":
			if len(input.Responses) == 0 {
				return agent.ToolResult{}, errors.New("task respond requires at least one response")
			}
			for index, response := range input.Responses {
				item := taskItemResult{Index: index}
				if response.Response.Cancelled && response.Response.Permission == "" && len(response.Response.Answers) == 0 {
					response.Response.Cancelled = true
				}
				if itemErr := executor.Respond(ctx, response.Ref, response.InteractionID, response.Response); itemErr != nil {
					item.Error = itemErr.Error()
				}
				results = append(results, item)
			}
		case "observe", "steer", "abort":
			if len(input.Refs) == 0 {
				return agent.ToolResult{}, fmt.Errorf("task %s requires at least one ref", input.Action)
			}
			for index, ref := range input.Refs {
				item := taskItemResult{Index: index}
				commandID := taskActionCommandID(ctx, input.Action, index)
				switch input.Action {
				case "observe":
					observation, itemErr := executor.Observe(ctx, ref, input.Cursor)
					if itemErr != nil {
						item.Error = itemErr.Error()
					} else {
						item.Observation = &observation
					}
				case "steer":
					steer := agent.Text(input.Input)
					steer.IdempotencyKey = commandID
					if itemErr := executor.Steer(ctx, ref, steer); itemErr != nil {
						item.Error = itemErr.Error()
					}
				case "abort":
					if itemErr := executor.Abort(ctx, ref, agent.AbortRequest{Reason: input.Reason, IdempotencyKey: commandID}); itemErr != nil {
						item.Error = itemErr.Error()
					}
				}
				results = append(results, item)
			}
		default:
			return agent.ToolResult{}, fmt.Errorf("unsupported task action %q", input.Action)
		}
		return JSONResult(struct {
			Results []taskItemResult `json:"results"`
		}{Results: results})
	})
	if err != nil {
		return nil, err
	}
	descriptor := writeDescriptor()
	descriptor.Source = agent.ToolSourceOther
	descriptor.Capability = "task"
	descriptor.Execution = agent.ToolExecutionChild
	descriptor.MutationScope = agent.ToolMutationNone
	descriptor.PostCheck = agent.ToolPostCheckNone
	descriptor.Recovery = agent.ToolRecoveryReconcilable
	return agent.StaticToolsIdentified(toolsetIdentity("tools.tasks", identity), agent.ToolDefinition{Tool: tool, Descriptor: descriptor})
}

func taskActionCommandID(ctx context.Context, action string, index int) string {
	executionID := agent.CurrentToolExecutionID(ctx)
	if executionID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%d", executionID, strings.TrimSpace(action), index)
}
