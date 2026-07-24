package agents

import (
	"encoding/json"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
)

type idListInput struct {
	IDs []string `json:"ids" jsonschema:"description=要读取的资源 ID 列表"`
}

type configManagerToolBuilder struct {
	build      func() (agent.BaseTool, error)
	descriptor agenttools.Descriptor
}

func newConfigManagerTools(cfg *config.Config, settings config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	_ = settings
	novaDir := strings.TrimSpace(cfg.DataDir())
	workspace := strings.TrimSpace(cfg.Workspace)
	automationWorkspaces := append([]string(nil), cfg.AutomationWorkspaces...)
	read := func(capability string, build func() (agent.BaseTool, error)) configManagerToolBuilder {
		return configManagerToolBuilder{build: build, descriptor: boundedReadDescriptor(agenttools.SourceRead, capability)}
	}
	write := func(capability string, build func() (agent.BaseTool, error)) configManagerToolBuilder {
		return configManagerToolBuilder{build: build, descriptor: workspaceWriteDescriptor(agenttools.SourceWrite, capability, agenttools.RecoveryReconcilable)}
	}
	builders := []configManagerToolBuilder{
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newListStyleReferencesTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.BaseTool, error) { return newWriteStyleReferencesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newListTellersTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newReadTellersTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.BaseTool, error) { return newWriteTellersTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newListStoryDirectorsTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newReadStoryDirectorsTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.BaseTool, error) { return newWriteStoryDirectorsTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newListEventPackagesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newReadEventPackagesTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.BaseTool, error) { return newWriteEventPackagesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newListActorStatesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newReadActorStatesTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.BaseTool, error) { return newWriteActorStatesTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newListImagePresetsTool(novaDir) }),
		read(config.AgentToolLoreRead, func() (agent.BaseTool, error) { return newReadImagePresetsTool(novaDir) }),
		write(config.AgentToolLoreWrite, func() (agent.BaseTool, error) { return newWriteImagePresetsTool(novaDir) }),
		read(config.AgentToolTodo, func() (agent.BaseTool, error) {
			return newListAutomationsTool(novaDir, workspace, automationWorkspaces)
		}),
		read(config.AgentToolTodo, func() (agent.BaseTool, error) {
			return newReadAutomationsTool(novaDir, workspace, automationWorkspaces)
		}),
		write(config.AgentToolTodo, func() (agent.BaseTool, error) {
			return newWriteAutomationsTool(novaDir, workspace, automationWorkspaces)
		}),
		read(config.AgentToolSkills, func() (agent.BaseTool, error) { return newListSkillsTool(cfg) }),
		read(config.AgentToolSkills, func() (agent.BaseTool, error) { return newReadSkillsTool(cfg) }),
		write(config.AgentToolSkills, func() (agent.BaseTool, error) { return newWriteSkillsTool(cfg) }),
		read(config.AgentToolAgentConfigRead, func() (agent.BaseTool, error) { return newListAgentConfigsTool(cfg) }),
		write(config.AgentToolAgentConfigWrite, func() (agent.BaseTool, error) { return newWriteAgentConfigsTool(cfg) }),
	}
	tools := make([]agent.BaseTool, 0, len(builders)+2)
	for _, builder := range builders {
		t, err := builder.build()
		if err != nil {
			return nil, err
		}
		t, err = defineTool(t, builder.descriptor)
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
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
