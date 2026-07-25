package tools

import (
	"encoding/json"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

type idListInput struct {
	IDs []string `json:"ids" jsonschema:"description=要读取的资源 ID 列表"`
}

type configManagerToolBuilder struct {
	build      func() (agent.Tool, error)
	descriptor agent.ToolDescriptor
}

func newConfigManagerTools(cfg *config.Config, settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	_ = settings
	novaDir := strings.TrimSpace(cfg.DataDir())
	workspace := strings.TrimSpace(cfg.Workspace)
	automationWorkspaces := append([]string(nil), cfg.AutomationWorkspaces...)
	read := func(capability string, build func() (agent.Tool, error)) configManagerToolBuilder {
		return configManagerToolBuilder{build: build, descriptor: boundedReadDescriptor(agent.ToolSourceRead, capability)}
	}
	write := func(capability string, build func() (agent.Tool, error)) configManagerToolBuilder {
		return configManagerToolBuilder{build: build, descriptor: workspaceWriteDescriptor(agent.ToolSourceWrite, capability, agent.ToolRecoveryReconcilable)}
	}
	builders := []configManagerToolBuilder{
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newListStyleReferencesTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.Tool, error) { return newWriteStyleReferencesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newListTellersTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newReadTellersTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.Tool, error) { return newWriteTellersTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newListStoryDirectorsTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newReadStoryDirectorsTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.Tool, error) { return newWriteStoryDirectorsTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newListEventPackagesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newReadEventPackagesTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.Tool, error) { return newWriteEventPackagesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newListActorStatesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newReadActorStatesTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.Tool, error) { return newWriteActorStatesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newListImagePresetsTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.Tool, error) { return newReadImagePresetsTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.Tool, error) { return newWriteImagePresetsTool(novaDir) }),
		read(config.AgentToolTodo, func() (agent.Tool, error) {
			return newListAutomationsTool(novaDir, workspace, automationWorkspaces)
		}),
		read(config.AgentToolTodo, func() (agent.Tool, error) {
			return newReadAutomationsTool(novaDir, workspace, automationWorkspaces)
		}),
		write(config.AgentToolTodo, func() (agent.Tool, error) {
			return newWriteAutomationsTool(novaDir, workspace, automationWorkspaces)
		}),
		read(config.AgentToolSkills, func() (agent.Tool, error) { return newListSkillsTool(cfg) }),
		read(config.AgentToolSkills, func() (agent.Tool, error) { return newReadSkillsTool(cfg) }),
		write(config.AgentToolSkills, func() (agent.Tool, error) { return newWriteSkillsTool(cfg) }),
		read(config.AgentToolAgentConfigRead, func() (agent.Tool, error) { return newListAgentConfigsTool(cfg) }),
		write(config.AgentToolAgentConfigWrite, func() (agent.Tool, error) { return newWriteAgentConfigsTool(cfg) }),
	}
	tools := make([]agent.ToolDefinition, 0, len(builders)+2)
	for _, builder := range builders {
		tool, err := builder.build()
		if err != nil {
			return nil, err
		}
		definition, err := defineTool(tool, builder.descriptor)
		if err != nil {
			return nil, err
		}
		tools = append(tools, definition)
	}
	loreTools, err := newLoreTools(workspace, true)
	if err != nil {
		return nil, err
	}
	tools = append(tools, loreTools...)
	return tools, nil
}

func marshalToolJSON(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func formatBatchResult(message string, result map[string][]string) string {
	data, _ := json.Marshal(result)
	return strings.TrimSpace(message) + "\n" + string(data)
}

func firstConfigNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolLabel(value bool, trueLabel, falseLabel string) string {
	if value {
		return trueLabel
	}
	return falseLabel
}
