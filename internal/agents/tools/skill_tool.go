package tools

import (
	"context"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	novaskills "denova/internal/agents/skills"
)

const GeneralSubAgentName = "general-purpose"

type skillToolInput struct {
	Name string `json:"name" jsonschema:"description=Exact name of the Skill to load from the available Skills catalog"`
}

const skillToolDescription = "Load the complete instructions for one Skill from the available Skills catalog. Call this tool with the exact Skill name after the user names a Skill or the current task clearly matches its catalog description. Do not call it for a Skill whose full instructions are already loaded in context."

// newSkillTool exposes progressive Skill disclosure. The available catalog is
// injected into the system instruction so this provider-visible schema remains
// stable when Skills are added, removed, or edited.
func newSkillTool(_ context.Context, backend *novaskills.Backend, maxBytes int) (agent.ToolDefinition, error) {
	if backend == nil {
		return agent.ToolDefinition{}, nil
	}
	tool, err := agent.InferTool("skill", skillToolDescription, func(callCtx context.Context, input skillToolInput) (string, error) {
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
