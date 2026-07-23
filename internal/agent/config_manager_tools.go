package agent

import (
	"encoding/json"
	"strings"

	adk "github.com/alfredxw/denova/adk"

	"denova/config"
)

type idListInput struct {
	IDs []string `json:"ids" jsonschema:"description=要读取的资源 ID 列表"`
}

type configManagerToolBuilder struct {
	build func() (adk.BaseTool, error)
}

func newConfigManagerTools(cfg *config.Config, settings config.ResolvedAgentToolSettings) ([]adk.BaseTool, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	_ = settings
	novaDir := strings.TrimSpace(cfg.DataDir())
	workspace := strings.TrimSpace(cfg.Workspace)
	automationWorkspaces := append([]string(nil), cfg.AutomationWorkspaces...)
	builders := []configManagerToolBuilder{
		{build: func() (adk.BaseTool, error) { return newListStyleReferencesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newWriteStyleReferencesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newListTellersTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newReadTellersTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newWriteTellersTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newListStoryDirectorsTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newReadStoryDirectorsTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newWriteStoryDirectorsTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newListEventPackagesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newReadEventPackagesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newWriteEventPackagesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newListActorStatesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newReadActorStatesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newWriteActorStatesTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newListImagePresetsTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newReadImagePresetsTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newWriteImagePresetsTool(novaDir) }},
		{build: func() (adk.BaseTool, error) { return newListAutomationsTool(novaDir, workspace, automationWorkspaces) }},
		{build: func() (adk.BaseTool, error) { return newReadAutomationsTool(novaDir, workspace, automationWorkspaces) }},
		{build: func() (adk.BaseTool, error) {
			return newWriteAutomationsTool(novaDir, workspace, automationWorkspaces)
		}},
		{build: func() (adk.BaseTool, error) { return newListSkillsTool(cfg) }},
		{build: func() (adk.BaseTool, error) { return newReadSkillsTool(cfg) }},
		{build: func() (adk.BaseTool, error) { return newWriteSkillsTool(cfg) }},
		{build: func() (adk.BaseTool, error) { return newListAgentConfigsTool(cfg) }},
		{build: func() (adk.BaseTool, error) { return newWriteAgentConfigsTool(cfg) }},
	}
	tools := make([]adk.BaseTool, 0, len(builders)+2)
	for _, builder := range builders {
		t, err := builder.build()
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
