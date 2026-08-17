package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type TaskRef struct {
	Agent   string `json:"agent" jsonschema:"minLength=1,maxLength=256" jsonschema_description:"Delegated Agent name returned by start."`
	Session string `json:"session" jsonschema:"minLength=1,maxLength=1024" jsonschema_description:"Child Session ID returned by start."`
	Run     string `json:"run" jsonschema:"minLength=1,maxLength=1024" jsonschema_description:"Process-local child Run ID returned by start."`
}

type TaskRequest struct {
	Agent          string `json:"agent" jsonschema:"minLength=1,maxLength=256" jsonschema_description:"Stable delegated Agent name from the catalog in this tool description."`
	Prompt         string `json:"prompt" jsonschema:"minLength=1,maxLength=1048576" jsonschema_description:"Self-contained goal, constraints, relevant references, expected output, and write scope."`
	Detached       bool   `json:"detached,omitempty" jsonschema_description:"Return immediately so the task can be controlled with later task calls."`
	IdempotencyKey string `json:"idempotency_key,omitempty" jsonschema:"maxLength=65536" jsonschema_description:"Stable retry identity; omit to derive it from this tool execution."`
}

type Task struct {
	Ref    TaskRef `json:"ref"`
	Status string  `json:"status"`
	Output string  `json:"output,omitempty"`
}

type TaskObservation struct {
	Task         Task                       `json:"task"`
	Cursor       string                     `json:"cursor,omitempty"`
	Output       string                     `json:"output,omitempty"`
	Events       []TaskEvent                `json:"events,omitempty"`
	Interactions []agent.InteractionRequest `json:"interactions,omitempty"`
	Incomplete   bool                       `json:"incomplete,omitempty"`
}

// TaskInteractionResponse targets one live child Run interaction. The Run and
// its interaction waiter must still exist in the current process.
type TaskInteractionResponse struct {
	Ref           TaskRef                   `json:"ref" jsonschema_description:"Child task waiting for this response."`
	InteractionID string                    `json:"interaction_id" jsonschema:"minLength=1,maxLength=1024" jsonschema_description:"Interaction ID returned by observe."`
	Response      agent.InteractionResponse `json:"response" jsonschema_description:"Answer, permission choice, or cancellation for the child interaction."`
}

// TaskEvent is the bounded reconnect projection returned by observe. Live
// child events are also forwarded through the parent Agent invocation while a
// non-detached start is waiting.
type TaskEvent struct {
	Cursor string      `json:"cursor,omitempty"`
	Type   string      `json:"type"`
	Run    string      `json:"run,omitempty"`
	Text   string      `json:"text,omitempty"`
	Tool   string      `json:"tool,omitempty"`
	Event  agent.Event `json:"event"`
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

type taskStartInput struct {
	Action string        `json:"action" jsonschema:"required,enum=start" jsonschema_description:"Start delegated tasks."`
	Starts []TaskRequest `json:"starts" jsonschema:"required,minItems=1,maxItems=32" jsonschema_description:"Independent task requests; every item returns its own outcome."`
}

type taskObserveInput struct {
	Action string    `json:"action" jsonschema:"required,enum=observe" jsonschema_description:"Read current task output, events, and interactions."`
	Refs   []TaskRef `json:"refs" jsonschema:"required,minItems=1,maxItems=32" jsonschema_description:"Tasks to observe; every reference returns its own outcome."`
	Cursor string    `json:"cursor,omitempty" jsonschema:"maxLength=65536" jsonschema_description:"Opaque event cursor returned by a previous observation."`
}

type taskSteerInput struct {
	Action string    `json:"action" jsonschema:"required,enum=steer" jsonschema_description:"Add instructions to running tasks."`
	Refs   []TaskRef `json:"refs" jsonschema:"required,minItems=1,maxItems=32" jsonschema_description:"Tasks that receive the same steering input."`
	Input  string    `json:"input" jsonschema:"required,minLength=1,maxLength=1048576" jsonschema_description:"Additional instruction for the referenced tasks."`
}

type taskRespondInput struct {
	Action    string                    `json:"action" jsonschema:"required,enum=respond" jsonschema_description:"Resolve child task interactions."`
	Responses []TaskInteractionResponse `json:"responses" jsonschema:"required,minItems=1,maxItems=32" jsonschema_description:"Independent interaction responses; every item returns its own outcome."`
}

type taskAbortInput struct {
	Action string    `json:"action" jsonschema:"required,enum=abort" jsonschema_description:"Abort running tasks."`
	Refs   []TaskRef `json:"refs" jsonschema:"required,minItems=1,maxItems=32" jsonschema_description:"Tasks to abort; every reference returns its own outcome."`
	Reason string    `json:"reason" jsonschema:"required,minLength=1,maxLength=65536" jsonschema_description:"Reason recorded with the abort request."`
}

type taskItemResult struct {
	Index       int              `json:"index"`
	Task        *Task            `json:"task,omitempty"`
	Observation *TaskObservation `json:"observation,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// Tasks connects the common task tool to a local subagent, remote worker, or
// host task system. Every batch item is attempted independently.
func Tasks(executor TaskExecutor) agent.Toolset {
	return defineToolset(func(context.Context) (agent.Toolset, error) {
		return buildTasks(executor)
	})
}

func buildTasks(executor TaskExecutor) (agent.Toolset, error) {
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
	invoke := func(ctx context.Context, input taskToolInput) (agent.ToolResult, error) {
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
	}
	tool, err := newUnionTool(
		"task", description, invoke,
		toolSchemaFor[taskStartInput](), toolSchemaFor[taskObserveInput](), toolSchemaFor[taskSteerInput](),
		toolSchemaFor[taskRespondInput](), toolSchemaFor[taskAbortInput](),
	)
	if err != nil {
		return nil, err
	}
	descriptor := writeDescriptor()
	descriptor.Source = agent.ToolSourceOther
	descriptor.Capability = "delegation"
	descriptor.Execution = agent.ToolExecutionChild
	descriptor.MutationScope = agent.ToolMutationNone
	descriptor.PostCheck = agent.ToolPostCheckNone
	descriptor.Recovery = agent.ToolRecoveryReconcilable
	descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationDelegation)
	return agent.StaticToolsIdentified(toolsetIdentity("tools.tasks", identity), agent.ToolDefinition{Tool: tool, Descriptor: descriptor})
}

func taskActionCommandID(ctx context.Context, action string, index int) string {
	executionID := agent.CurrentToolExecutionID(ctx)
	if executionID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%d", executionID, strings.TrimSpace(action), index)
}
