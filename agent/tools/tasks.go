package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type TaskRef struct {
	Executor string `json:"executor"`
	ID       string `json:"id"`
}

type TaskRequest struct {
	Prompt   string            `json:"prompt"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Task struct {
	Ref    TaskRef `json:"ref"`
	Status string  `json:"status"`
}

type TaskObservation struct {
	Task       Task   `json:"task"`
	Cursor     string `json:"cursor,omitempty"`
	Output     string `json:"output,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"`
}

type TaskExecutor interface {
	Identity() agent.CapabilityIdentity
	Start(context.Context, TaskRequest) (Task, error)
	Observe(context.Context, TaskRef, string) (TaskObservation, error)
	Steer(context.Context, TaskRef, agent.Input) error
	Abort(context.Context, TaskRef, string) error
}

type taskToolInput struct {
	Action string        `json:"action" jsonschema:"enum=start,enum=observe,enum=steer,enum=abort"`
	Starts []TaskRequest `json:"starts,omitempty" jsonschema:"maxItems=32"`
	Refs   []TaskRef     `json:"refs,omitempty" jsonschema:"maxItems=32"`
	Cursor string        `json:"cursor,omitempty"`
	Input  string        `json:"input,omitempty" jsonschema:"maxLength=1048576"`
	Reason string        `json:"reason,omitempty" jsonschema:"maxLength=65536"`
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
	tool, err := agent.InferTool("task", "Start, observe, steer, or abort delegated tasks. Batch operations return per-item outcomes.\n\n启动、观察、引导或终止委派任务；批量操作逐项返回结果。", func(ctx context.Context, input taskToolInput) (agent.ToolResult, error) {
		results := make([]taskItemResult, 0)
		switch strings.TrimSpace(input.Action) {
		case "start":
			if len(input.Starts) == 0 {
				return agent.ToolResult{}, errors.New("task start requires at least one request")
			}
			for index, request := range input.Starts {
				item := taskItemResult{Index: index}
				task, itemErr := executor.Start(ctx, request)
				if itemErr != nil {
					item.Error = itemErr.Error()
				} else {
					item.Task = &task
				}
				results = append(results, item)
			}
		case "observe", "steer", "abort":
			if len(input.Refs) == 0 {
				return agent.ToolResult{}, fmt.Errorf("task %s requires at least one ref", input.Action)
			}
			for index, ref := range input.Refs {
				item := taskItemResult{Index: index}
				switch input.Action {
				case "observe":
					observation, itemErr := executor.Observe(ctx, ref, input.Cursor)
					if itemErr != nil {
						item.Error = itemErr.Error()
					} else {
						item.Observation = &observation
					}
				case "steer":
					if itemErr := executor.Steer(ctx, ref, agent.Text(input.Input)); itemErr != nil {
						item.Error = itemErr.Error()
					}
				case "abort":
					if itemErr := executor.Abort(ctx, ref, input.Reason); itemErr != nil {
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
	descriptor.MutationScope = agent.ToolMutationExternal
	descriptor.Execution = agent.ToolExecutionSessionExclusive
	descriptor.Recovery = agent.ToolRecoveryReconcilable
	return agent.StaticToolsIdentified(toolsetIdentity("tools.tasks", identity), agent.ToolDefinition{Tool: tool, Descriptor: descriptor}), nil
}
