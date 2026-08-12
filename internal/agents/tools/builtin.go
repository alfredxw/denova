package tools

import (
	"context"
	"errors"
	"fmt"
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
	return defineTool(tool, boundedReadDescriptor(agent.ToolSourceOther, config.AgentToolSkills, agent.ToolResultRecoveryRerun))
}

// NewSkill builds the progressive-disclosure Skill tool.
func NewSkill(ctx context.Context, backend *novaskills.Backend, maxBytes int) (agent.ToolDefinition, error) {
	return newSkillTool(ctx, backend, maxBytes)
}
