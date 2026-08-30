package tools

import (
	"context"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

type taskWaitTarget struct {
	Ref TaskRef `json:"ref,omitempty" jsonschema_description:"Previously started delegated task."`
}

type taskWaitInput struct {
	Targets []taskWaitTarget `json:"targets" jsonschema:"required,minItems=1,maxItems=32" jsonschema_description:"Tasks to wait for. The wait returns when any valid target is ready."`
}

func newTaskWaitDefinition(executor TaskExecutor) (agent.ToolDefinition, error) {
	schema, err := reflectedToolSchema[taskWaitInput]()
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	tool, err := newSchemaTool(
		"task_wait",
		"Wait until any previously started delegated task is ready. Call only at a real dependency point, and pass all tasks whose results can unblock the current work. User steering interrupts this wait without aborting the tasks.",
		schema,
		func(ctx context.Context, input taskWaitInput) (agent.ToolResult, error) {
			if len(input.Targets) == 0 {
				return agent.ToolResult{}, errors.New("task_wait requires at least one target")
			}
			results := make([]taskItemResult, len(input.Targets))
			refs := make([]TaskRef, 0, len(input.Targets))
			indices := make([]int, 0, len(input.Targets))
			for index, target := range input.Targets {
				results[index].Index = index
				if itemErr := validateTaskRef(target.Ref); itemErr != nil {
					setTaskItemError(&results[index], itemErr)
					continue
				}
				refs = append(refs, target.Ref)
				indices = append(indices, index)
			}
			if len(refs) == 0 {
				return JSONResult(struct {
					Results []taskItemResult `json:"results"`
				}{Results: results})
			}
			outcomes, waitErr := executor.Wait(ctx, refs)
			if waitErr != nil {
				return agent.ToolResult{}, waitErr
			}
			if len(outcomes) != len(refs) {
				return agent.ToolResult{}, errors.New("task executor returned an incomplete wait result")
			}
			for index, outcome := range outcomes {
				resultIndex := indices[index]
				results[resultIndex].Task = outcome.Task
				results[resultIndex].Ready = outcome.Ready
				setTaskItemError(&results[resultIndex], outcome.Err)
			}
			return JSONResult(struct {
				Results []taskItemResult `json:"results"`
			}{Results: results})
		},
	)
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	return agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: "delegation", Execution: agent.ToolExecutionInteractiveWait,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringInterruptibleWait,
		MaxResultBytes: defaultResultBytes,
		Presentation:   agent.UniformToolPresentation(agent.ToolPresentationDelegation),
	}}, nil
}
