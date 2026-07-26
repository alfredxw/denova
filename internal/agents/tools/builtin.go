package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	novaskills "denova/internal/agents/skills"
)

const GeneralSubAgentName = "general-purpose"

type skillToolInput struct {
	Name string `json:"name" jsonschema:"description=Name of the Skill to load"`
}

// newSkillTool exposes progressive Skill disclosure without coupling the Skill
// catalog to the Agent. The catalog summary is part of the stable tool
// description; full instructions are loaded only after an explicit call.
func newSkillTool(ctx context.Context, backend *novaskills.Backend, maxBytes int) (agent.ToolDefinition, error) {
	if backend == nil {
		return agent.ToolDefinition{}, nil
	}
	available, err := backend.List(ctx)
	if err != nil {
		return agent.ToolDefinition{}, fmt.Errorf("list skills: %w", err)
	}
	if len(available) == 0 {
		return agent.ToolDefinition{}, nil
	}
	sort.Slice(available, func(i, j int) bool { return available[i].Name < available[j].Name })
	var description strings.Builder
	description.WriteString("Load one available Skill's full instructions when its description matches the current task. Available Skills:\n")
	for _, item := range available {
		description.WriteString("- ")
		description.WriteString(item.Name)
		description.WriteString(": ")
		description.WriteString(item.Description)
		description.WriteByte('\n')
	}
	tool, err := agent.InferTool("skill", strings.TrimSpace(description.String()), func(callCtx context.Context, input skillToolInput) (string, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return "", errors.New("skill name is required")
		}
		skill, err := backend.Get(callCtx, name)
		if err != nil {
			return "", err
		}
		return novaskills.FormatForModel(skill, maxBytes), nil
	})
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	return defineTool(tool, boundedReadDescriptor(agent.ToolSourceOther, config.AgentToolSkills))
}

// NewSkill builds the progressive-disclosure Skill tool.
func NewSkill(ctx context.Context, backend *novaskills.Backend, maxBytes int) (agent.ToolDefinition, error) {
	return newSkillTool(ctx, backend, maxBytes)
}

const todoPlanSchema = "todo.plan.v1"

type todoPlanItem struct {
	Step   string `json:"step" jsonschema:"required" jsonschema_description:"Concise task step."`
	Status string `json:"status" jsonschema:"required,enum=pending,enum=in_progress,enum=completed" jsonschema_description:"Current step status: pending, in_progress, or completed."`
}

type todoInput struct {
	Plan []todoPlanItem `json:"plan" jsonschema:"required" jsonschema_description:"Complete replacement plan in display order; an empty array clears the plan."`
}

type todoPlanResult struct {
	Schema string         `json:"schema"`
	Plan   []todoPlanItem `json:"plan"`
}

func newTodoTool() (agent.ToolDefinition, error) {
	tool, err := agent.InferTool("todo", "Replace the current run's complete plan. Send every step on each update in display order. Use at most one in_progress step; an empty plan clears it.", func(_ context.Context, input todoInput) (agent.ToolResult, error) {
		inProgress := 0
		for index := range input.Plan {
			input.Plan[index].Step = strings.TrimSpace(input.Plan[index].Step)
			if input.Plan[index].Step == "" {
				return agent.ToolResult{}, fmt.Errorf("plan[%d].step is required", index)
			}
			if input.Plan[index].Status == "in_progress" {
				inProgress++
			}
		}
		if inProgress > 1 {
			return agent.ToolResult{}, errors.New("plan may contain at most one in_progress step")
		}
		if input.Plan == nil {
			input.Plan = []todoPlanItem{}
		}
		payload, err := json.Marshal(todoPlanResult{Schema: todoPlanSchema, Plan: input.Plan})
		if err != nil {
			return agent.ToolResult{}, err
		}
		result := agent.TextToolResult(string(payload))
		result.Details = json.RawMessage(payload)
		return result, nil
	})
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	return defineTool(tool, agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: config.AgentToolTodo,
		Execution:        agent.ToolExecutionSessionExclusive,
		MutationScope:    agent.ToolMutationSession,
		PostCheck:        agent.ToolPostCheckSessionState,
		Recovery:         agent.ToolRecoveryIdempotent,
		Steering:         agent.SteeringFinishCurrent,
		ResultProjection: agent.ToolResultBoundedModelContext, MaxResultBytes: defaultToolResultMaxBytes,
	})
}

// NewTodo builds the run-local complete-plan replacement tool.
func NewTodo() (agent.ToolDefinition, error) {
	return newTodoTool()
}

type taskToolInput struct {
	SubagentType string `json:"subagent_type" jsonschema:"description=Stable name of the delegated Agent"`
	Description  string `json:"description" jsonschema:"description=Complete self-contained task including goal context constraints and expected output"`
}

func newTaskTool(ctx context.Context, subAgents []agent.Runnable) (agent.ToolDefinition, error) {
	byName := make(map[string]agent.Runnable, len(subAgents))
	var description strings.Builder
	description.WriteString("Delegate a self-contained task to one Agent and return its final response. Available Agents:\n")
	for _, candidate := range subAgents {
		if candidate == nil {
			continue
		}
		name := strings.TrimSpace(candidate.Name(ctx))
		if name == "" {
			return agent.ToolDefinition{}, errors.New("task tool: subagent has no stable name")
		}
		if _, exists := byName[name]; exists {
			return agent.ToolDefinition{}, fmt.Errorf("task tool: duplicate subagent %q", name)
		}
		byName[name] = candidate
		description.WriteString("- ")
		description.WriteString(name)
		description.WriteString(": ")
		description.WriteString(strings.TrimSpace(candidate.Description(ctx)))
		description.WriteByte('\n')
	}
	if len(byName) == 0 {
		return agent.ToolDefinition{}, nil
	}
	tool, err := agent.InferTool("task", strings.TrimSpace(description.String()), func(callCtx context.Context, input taskToolInput) (string, error) {
		selected := byName[strings.TrimSpace(input.SubagentType)]
		if selected == nil {
			return "", fmt.Errorf("subagent type %q not found", input.SubagentType)
		}
		request := strings.TrimSpace(input.Description)
		if request == "" {
			return "", errors.New("task description is required")
		}
		return runSubAgent(callCtx, selected, request)
	})
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	return defineTool(tool, agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: config.AgentToolDelegation, Execution: agent.ToolExecutionChild,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryNonIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
		Steering:       agent.SteeringFinishCurrent,
		MaxResultBytes: defaultToolResultMaxBytes,
	})
}

// NewTask builds the child-Agent delegation tool. It returns nil when no
// delegated Agent is available.
func NewTask(ctx context.Context, subAgents []agent.Runnable) (agent.ToolDefinition, error) {
	return newTaskTool(ctx, subAgents)
}

func runSubAgent(ctx context.Context, child agent.Runnable, request string) (response string, returnErr error) {
	childName := strings.TrimSpace(child.Name(ctx))
	childCtx, finishInvocation, err := agent.BeginChildInvocation(ctx, childName)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := finishInvocation(); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	events := child.Run(childCtx, &agent.AgentInput{
		Messages:        []*agent.Message{agent.UserMessage(request)},
		EnableStreaming: true,
	})
	var final *agent.Message
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			agent.EmitEvent(childCtx, event)
			return "", event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			agent.EmitEvent(childCtx, event)
			continue
		}
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		forwarded := *event
		forwarded.RunPath = invocationRunPath(childCtx, event.RunPath)
		forwardedOutput := *event.Output
		forwardedVariant := *event.Output.MessageOutput
		forwardedVariant.IsStreaming = false
		forwardedVariant.MessageStream = nil
		forwardedVariant.Message = message.Clone()
		forwardedOutput.MessageOutput = &forwardedVariant
		forwarded.Output = &forwardedOutput
		agent.EmitEvent(childCtx, &forwarded)
		if message != nil && message.Role == agent.Assistant {
			final = message
		}
	}
	if final == nil {
		return "", errors.New("subagent completed without an assistant response")
	}
	return final.Content, nil
}

func invocationRunPath(ctx context.Context, local []agent.RunStep) []agent.RunStep {
	scope, ok := agent.InvocationScopeFromContext(ctx)
	if !ok || len(scope.RunPath) == 0 {
		return append([]agent.RunStep(nil), local...)
	}
	prefix := scope.RunPath
	if len(local) > 0 && local[0].String() == prefix[len(prefix)-1] {
		prefix = prefix[:len(prefix)-1]
	}
	path := make([]agent.RunStep, 0, len(prefix)+len(local))
	for _, name := range prefix {
		path = append(path, agent.NewRunStep(name))
	}
	path = append(path, local...)
	return path
}
