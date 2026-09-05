package tools

import (
	"context"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// SkillLoader resolves complete Skill instructions by an exact name that the
// host has already advertised in the Agent's instructions.
type SkillLoader interface {
	Identity() agent.CapabilityIdentity
	Load(context.Context, string) (string, error)
}

type skillToolInput struct {
	Name string `json:"name" jsonschema:"minLength=1,maxLength=512" jsonschema_description:"Exact name from the available Skills catalog."`
}

// Skills exposes one exact-name loading operation. Catalog discovery belongs
// to the host so selecting and loading a Skill requires only one model call.
func Skills(loader SkillLoader) agent.Toolset {
	return defineToolset(func(context.Context) (agent.Toolset, error) {
		return buildSkills(loader)
	})
}

func buildSkills(loader SkillLoader) (agent.Toolset, error) {
	if loader == nil {
		return nil, errors.New("skills Toolset requires a SkillLoader")
	}
	identity := loader.Identity()
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return nil, errors.New("skills SkillLoader requires a stable Identity")
	}
	tool, err := agent.InferTool(
		"skill",
		"Load the complete instructions for one Skill by its exact name from the available Skills catalog.",
		func(ctx context.Context, input skillToolInput) (string, error) {
			name := strings.TrimSpace(input.Name)
			if name == "" {
				return "", errors.New("skill name is required")
			}
			return loader.Load(ctx, name)
		},
	)
	if err != nil {
		return nil, err
	}
	definition := agent.ToolDefinition{
		Tool: tool,
		Descriptor: readDescriptor(
			WithResultRecoveryKind(agent.ToolResultRecoveryRerun),
		),
	}
	return agent.StaticToolsIdentified(toolsetIdentity("tools.skills", identity), definition)
}
