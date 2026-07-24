package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	novaskills "denova/internal/agents/skills"
)

const generalSubAgentName = "general-purpose"

type skillToolInput struct {
	Name string `json:"name" jsonschema:"description=Name of the Skill to load"`
}

// newSkillTool exposes progressive Skill disclosure without coupling the Skill
// catalog to the Agent. The catalog summary is part of the stable tool
// description; full instructions are loaded only after an explicit call.
func newSkillTool(ctx context.Context, backend *novaskills.Backend, maxBytes int) (agent.BaseTool, error) {
	if backend == nil {
		return nil, nil
	}
	available, err := backend.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	if len(available) == 0 {
		return nil, nil
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
		content := strings.TrimSpace(skill.Content)
		if maxBytes > 0 && len(content) > maxBytes {
			content = truncateUTF8StringBytes(content, maxBytes)
			content += "\n\n[Skill instructions truncated at configured context fragment limit]"
		}
		return fmt.Sprintf("# Skill: %s\n\nDescription: %s\nContext mode: %s\n\n%s", skill.Name, skill.Description, skill.Context, content), nil
	})
	if err != nil {
		return nil, err
	}
	return defineTool(tool, boundedReadDescriptor(agenttools.SourceOther, config.AgentToolSkills))
}

type todoItem struct {
	Content    string `json:"content"`
	ActiveForm string `json:"activeForm"`
	Status     string `json:"status" jsonschema:"enum=pending,enum=in_progress,enum=completed"`
}

type writeTodosInput struct {
	Todos []todoItem `json:"todos"`
}

func newWriteTodosTool() (agent.BaseTool, error) {
	tool, err := agent.InferTool("write_todos", "Replace the current run's complete todo list. Send every item on each update; status must be pending, in_progress, or completed.", func(_ context.Context, input writeTodosInput) (string, error) {
		payload, err := json.Marshal(input.Todos)
		if err != nil {
			return "", err
		}
		return "Updated todo list to " + string(payload), nil
	})
	if err != nil {
		return nil, err
	}
	return defineTool(tool, agenttools.Descriptor{
		Source: agenttools.SourceOther, Capability: config.AgentToolTodo,
		Execution: agenttools.ExecutionWorkspaceExclusive, Recovery: agenttools.RecoveryIdempotent,
		ResultProjection: agenttools.ResultBoundedModelContext, MaxResultBytes: defaultToolResultMaxBytes,
	})
}

type taskToolInput struct {
	SubagentType string `json:"subagent_type" jsonschema:"description=Stable name of the delegated Agent"`
	Description  string `json:"description" jsonschema:"description=Complete self-contained task including goal context constraints and expected output"`
}

func newTaskTool(ctx context.Context, subAgents []agent.Runnable) (agent.BaseTool, error) {
	byName := make(map[string]agent.Runnable, len(subAgents))
	var description strings.Builder
	description.WriteString("Delegate a self-contained task to one Agent and return its final response. Available Agents:\n")
	for _, candidate := range subAgents {
		if candidate == nil {
			continue
		}
		name := strings.TrimSpace(candidate.Name(ctx))
		if name == "" {
			return nil, errors.New("task tool: subagent has no stable name")
		}
		if _, exists := byName[name]; exists {
			return nil, fmt.Errorf("task tool: duplicate subagent %q", name)
		}
		byName[name] = candidate
		description.WriteString("- ")
		description.WriteString(name)
		description.WriteString(": ")
		description.WriteString(strings.TrimSpace(candidate.Description(ctx)))
		description.WriteByte('\n')
	}
	if len(byName) == 0 {
		return nil, nil
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
		return nil, err
	}
	return defineTool(tool, agenttools.Descriptor{
		Source: agenttools.SourceOther, Execution: agenttools.ExecutionChild,
		Recovery: agenttools.RecoveryNonIdempotent, ResultProjection: agenttools.ResultBoundedModelContext,
		MaxResultBytes: defaultToolResultMaxBytes,
	})
}

func runSubAgent(ctx context.Context, child agent.Runnable, request string) (string, error) {
	events := child.Run(ctx, &agent.AgentInput{
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
			agent.EmitEvent(ctx, event)
			return "", event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			agent.EmitEvent(ctx, event)
			continue
		}
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		forwarded := *event
		forwarded.RunPath = append([]agent.RunStep(nil), event.RunPath...)
		forwardedOutput := *event.Output
		forwardedVariant := *event.Output.MessageOutput
		forwardedVariant.IsStreaming = false
		forwardedVariant.MessageStream = nil
		forwardedVariant.Message = message.Clone()
		forwardedOutput.MessageOutput = &forwardedVariant
		forwarded.Output = &forwardedOutput
		agent.EmitEvent(ctx, &forwarded)
		if message != nil && message.Role == agent.Assistant {
			final = message
		}
	}
	if final == nil {
		return "", errors.New("subagent completed without an assistant response")
	}
	return final.Content, nil
}
