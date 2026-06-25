package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"nova/config"
)

type agentConfigWriteInput struct {
	Scope      string                      `json:"scope" jsonschema:"description=写入层级：user 或 workspace；必须显式指定"`
	Message    string                      `json:"message" jsonschema:"description=本次 Agent 配置变更说明"`
	Operations []agentConfigWriteOperation `json:"operations" jsonschema:"description=批量 Agent 配置操作"`
}

type agentConfigWriteOperation struct {
	Op       string                       `json:"op" jsonschema:"description=操作类型：set_agent_override/set_general_sub_agent/upsert_sub_agent/delete_sub_agent"`
	Agent    string                       `json:"agent,omitempty" jsonschema:"description=目标 Agent kind；set_agent_override 可用 default 或具体 Agent kind；set_general_sub_agent 可用 default/ide/interactive_story/config_manager/automation"`
	ID       string                       `json:"id,omitempty" jsonschema:"description=目标 SubAgent ID；delete_sub_agent 必填，upsert_sub_agent 可作为 sub_agent.id 的补充"`
	Model    *config.AgentModelOverride   `json:"model,omitempty" jsonschema:"description=set_agent_override 时替换目标 Agent 在该层的模型覆盖配置"`
	Tools    *config.AgentToolOverride    `json:"tools,omitempty" jsonschema:"description=set_agent_override 时替换目标 Agent 在该层的工具权限覆盖配置"`
	Prompt   *config.AgentPromptOverride  `json:"prompt,omitempty" jsonschema:"description=set_agent_override 时替换目标 Agent 在该层的提示词覆盖配置"`
	Skills   *config.AgentSkillOverride   `json:"skills,omitempty" jsonschema:"description=set_agent_override 时替换目标 Agent 在该层的 Skill 可用性覆盖配置"`
	Context  *config.AgentContextOverride `json:"context,omitempty" jsonschema:"description=set_agent_override 时替换目标 Agent 在该层的上下文压缩覆盖配置"`
	Enabled  *bool                        `json:"enabled,omitempty" jsonschema:"description=set_general_sub_agent 时设置通用 SubAgent 开关；省略或 null 表示继承"`
	SubAgent config.SubAgentConfig        `json:"sub_agent,omitempty" jsonschema:"description=upsert_sub_agent 使用的完整 SubAgent 配置；屏蔽继承项时可提供相同 id 且 enabled=false"`
}

type agentConfigSnapshot struct {
	Paths            config.SettingsPaths          `json:"paths"`
	Agents           []agentConfigAgentDefinition  `json:"agents"`
	DeepAgentParents []string                      `json:"deep_agent_parents"`
	ToolCapabilities []agentConfigToolCapability   `json:"tool_capabilities"`
	Layers           agentConfigLayeredSnapshot    `json:"layers"`
	SubAgentIndex    []agentConfigSubAgentIndexRow `json:"sub_agent_index"`
	Notes            []string                      `json:"notes,omitempty"`
}

type agentConfigAgentDefinition struct {
	Kind            string `json:"kind"`
	SessionID       string `json:"session_id,omitempty"`
	DeepAgentParent bool   `json:"deep_agent_parent"`
}

type agentConfigToolCapability struct {
	Source string `json:"source"`
}

type agentConfigLayeredSnapshot struct {
	User      agentConfigLayerSnapshot `json:"user"`
	Workspace agentConfigLayerSnapshot `json:"workspace"`
	Effective agentConfigLayerSnapshot `json:"effective"`
}

type agentConfigLayerSnapshot struct {
	DefaultModel     string                              `json:"default_model,omitempty"`
	ModelProfiles    []safeModelProfileSettings          `json:"model_profiles,omitempty"`
	AgentModels      config.AgentModelSettings           `json:"agent_models,omitempty"`
	AgentTools       config.AgentToolSettings            `json:"agent_tools,omitempty"`
	AgentPrompts     config.AgentPromptSettings          `json:"agent_prompts,omitempty"`
	AgentSkills      config.AgentSkillSettings           `json:"agent_skills,omitempty"`
	AgentContext     config.AgentContextSettings         `json:"agent_context,omitempty"`
	GeneralSubAgents config.AgentGeneralSubAgentSettings `json:"general_sub_agents,omitempty"`
	SubAgents        []config.SubAgentConfig             `json:"sub_agents,omitempty"`
}

type safeModelProfileSettings struct {
	ID                  string   `json:"id,omitempty"`
	Name                string   `json:"name,omitempty"`
	OpenAIBaseURL       string   `json:"openai_base_url,omitempty"`
	OpenAIModel         string   `json:"openai_model,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
	ContextWindowTokens *int     `json:"context_window_tokens,omitempty"`
}

type agentConfigSubAgentIndexRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name,omitempty"`
	Enabled     bool     `json:"enabled"`
	Parents     []string `json:"parents,omitempty"`
	Description string   `json:"description,omitempty"`
	Layer       string   `json:"layer"`
}

func newListAgentConfigsTool(cfg *config.Config) (tool.BaseTool, error) {
	return utils.InferTool("list_agent_configs", "一次性读取 Agent 页相关配置：Agent kind、工具能力、user/workspace/effective 三层配置、自定义 SubAgent 索引和配置文件路径；不会返回 API key。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		layered, err := loadAgentConfigLayered(cfg)
		if err != nil {
			return "", err
		}
		snapshot := agentConfigSnapshot{
			Paths:            layered.Paths,
			Agents:           agentConfigDefinitions(),
			DeepAgentParents: config.DeepAgentParentKinds(),
			ToolCapabilities: agentConfigToolCapabilities(),
			Layers: agentConfigLayeredSnapshot{
				User:      agentConfigLayer(layered.User),
				Workspace: agentConfigLayer(layered.Workspace),
				Effective: agentConfigLayer(layered.Effective),
			},
			SubAgentIndex: agentConfigSubAgentIndex(layered),
			Notes: []string{
				"write_agent_configs 必须显式指定 scope=user 或 scope=workspace。",
				"model_profiles 已脱敏，不包含模型密钥；本工具不负责创建或编辑模型配置。",
				"delete_sub_agent 只删除目标层配置；如需屏蔽继承来的 SubAgent，请 upsert 同 ID 且 enabled=false 的覆盖项。",
			},
		}
		return marshalToolJSON(snapshot)
	})
}

func newWriteAgentConfigsTool(cfg *config.Config) (tool.BaseTool, error) {
	return utils.InferTool("write_agent_configs", "批量写入 Agent 页配置。必须显式指定 scope=user 或 scope=workspace；只修改 agent_models、agent_tools、agent_prompts、agent_skills、agent_context、general_sub_agents 和 sub_agents。", func(ctx context.Context, input agentConfigWriteInput) (string, error) {
		_ = ctx
		scope := strings.TrimSpace(input.Scope)
		if scope != "user" && scope != "workspace" {
			return "", fmt.Errorf("scope 必须显式指定为 user 或 workspace")
		}
		layered, err := loadAgentConfigLayered(cfg)
		if err != nil {
			return "", err
		}
		path, settings, err := loadWritableAgentConfigSettings(cfg, scope)
		if err != nil {
			return "", err
		}
		result := map[string][]string{
			"agent_overrides":     {},
			"general_sub_agents":  {},
			"upserted_sub_agents": {},
			"deleted_sub_agents":  {},
		}
		for index, op := range input.Operations {
			if err := applyAgentConfigWriteOperation(&settings, layered, op, result); err != nil {
				return "", fmt.Errorf("Agent 配置操作 #%d 失败: %w", index+1, err)
			}
		}
		if err := config.WriteSettingsFile(path, settings); err != nil {
			return "", err
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "Agent 配置已更新"), result), nil
	})
}

func loadAgentConfigLayered(cfg *config.Config) (config.LayeredSettings, error) {
	novaDir := ""
	workspace := ""
	if cfg != nil {
		novaDir = cfg.NovaDir
		workspace = cfg.Workspace
	}
	layered, err := config.LoadLayered(novaDir, workspace)
	if err != nil {
		return config.LayeredSettings{}, fmt.Errorf("读取 Agent 配置失败: %w", err)
	}
	return layered, nil
}

func loadWritableAgentConfigSettings(cfg *config.Config, scope string) (string, config.Settings, error) {
	novaDir := ""
	workspace := ""
	if cfg != nil {
		novaDir = cfg.NovaDir
		workspace = cfg.Workspace
	}
	switch scope {
	case "user":
		path := config.UserConfigPath(novaDir)
		settings, err := config.ReadSettingsFile(path)
		return path, settings, err
	case "workspace":
		if strings.TrimSpace(workspace) == "" {
			return "", config.Settings{}, fmt.Errorf("当前没有打开的工作区，无法写入 workspace 配置")
		}
		path := config.WorkspaceConfigPath(workspace)
		settings, err := config.ReadSettingsFile(path)
		return path, settings, err
	default:
		return "", config.Settings{}, fmt.Errorf("不支持的配置层级: %s", scope)
	}
}

func applyAgentConfigWriteOperation(settings *config.Settings, layered config.LayeredSettings, op agentConfigWriteOperation, result map[string][]string) error {
	switch strings.TrimSpace(op.Op) {
	case "set_agent_override":
		agent := strings.TrimSpace(op.Agent)
		if !validAgentConfigKey(agent) {
			return fmt.Errorf("无效 Agent kind: %s", op.Agent)
		}
		changed := false
		if op.Model != nil {
			setAgentModelOverride(settings, agent, *op.Model)
			changed = true
		}
		if op.Tools != nil {
			setAgentToolOverride(settings, agent, *op.Tools)
			changed = true
		}
		if op.Prompt != nil {
			setAgentPromptOverride(settings, agent, *op.Prompt)
			changed = true
		}
		if op.Skills != nil {
			setAgentSkillOverride(settings, agent, *op.Skills)
			changed = true
		}
		if op.Context != nil {
			setAgentContextOverride(settings, agent, *op.Context)
			changed = true
		}
		if !changed {
			return fmt.Errorf("set_agent_override 至少需要 model/tools/prompt/skills/context 之一")
		}
		result["agent_overrides"] = append(result["agent_overrides"], agent)
		return nil
	case "set_general_sub_agent":
		agent := strings.TrimSpace(op.Agent)
		if !validGeneralSubAgentKey(agent) {
			return fmt.Errorf("无效通用 SubAgent 父 Agent: %s", op.Agent)
		}
		setGeneralSubAgentOverride(settings, agent, op.Enabled)
		result["general_sub_agents"] = append(result["general_sub_agents"], agent)
		return nil
	case "upsert_sub_agent":
		sub := op.SubAgent
		if strings.TrimSpace(sub.ID) == "" {
			sub.ID = op.ID
		}
		sub = fillSubAgentRequiredFields(sub, settings.SubAgents, layered.Effective.SubAgents)
		sanitized := config.SanitizeSubAgents([]config.SubAgentConfig{sub})
		if len(sanitized) != 1 {
			return fmt.Errorf("SubAgent 配置无效：id、description 和 system_prompt 必须有效")
		}
		settings.SubAgents = upsertSubAgent(settings.SubAgents, sanitized[0])
		result["upserted_sub_agents"] = append(result["upserted_sub_agents"], sanitized[0].ID)
		return nil
	case "delete_sub_agent":
		id := config.NormalizeSubAgentID(op.ID)
		if id == "" {
			return fmt.Errorf("delete_sub_agent 需要 id")
		}
		settings.SubAgents = deleteSubAgent(settings.SubAgents, id)
		result["deleted_sub_agents"] = append(result["deleted_sub_agents"], id)
		return nil
	default:
		return fmt.Errorf("不支持的 op: %s", op.Op)
	}
}

func agentConfigDefinitions() []agentConfigAgentDefinition {
	definitions := config.AgentKindDefinitions()
	out := make([]agentConfigAgentDefinition, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, agentConfigAgentDefinition{
			Kind:            definition.Kind,
			SessionID:       definition.SessionID,
			DeepAgentParent: config.IsDeepAgentParentKind(definition.Kind),
		})
	}
	return out
}

func agentConfigToolCapabilities() []agentConfigToolCapability {
	capabilities := config.AgentToolCapabilities()
	out := make([]agentConfigToolCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, agentConfigToolCapability{Source: capability.Source})
	}
	return out
}

func agentConfigLayer(settings config.Settings) agentConfigLayerSnapshot {
	return agentConfigLayerSnapshot{
		DefaultModel:     settings.OpenAIModel,
		ModelProfiles:    safeModelProfiles(settings.ModelProfiles),
		AgentModels:      settings.AgentModels,
		AgentTools:       settings.AgentTools,
		AgentPrompts:     settings.AgentPrompts,
		AgentSkills:      settings.AgentSkills,
		AgentContext:     settings.AgentContexts,
		GeneralSubAgents: settings.GeneralSubAgents,
		SubAgents:        settings.SubAgents,
	}
}

func safeModelProfiles(profiles []config.ModelProfileSettings) []safeModelProfileSettings {
	if len(profiles) == 0 {
		return nil
	}
	out := make([]safeModelProfileSettings, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, safeModelProfileSettings{
			ID:                  profile.ID,
			Name:                profile.Name,
			OpenAIBaseURL:       profile.OpenAIBaseURL,
			OpenAIModel:         profile.OpenAIModel,
			Temperature:         profile.Temperature,
			ContextWindowTokens: profile.ContextWindowTokens,
		})
	}
	return out
}

func agentConfigSubAgentIndex(layered config.LayeredSettings) []agentConfigSubAgentIndexRow {
	var rows []agentConfigSubAgentIndexRow
	appendRows := func(layer string, subAgents []config.SubAgentConfig) {
		for _, sub := range subAgents {
			rows = append(rows, agentConfigSubAgentIndexRow{
				ID:          sub.ID,
				Name:        sub.Name,
				Enabled:     config.SubAgentEnabled(sub),
				Parents:     sub.Parents,
				Description: sub.Description,
				Layer:       layer,
			})
		}
	}
	appendRows("user", layered.User.SubAgents)
	appendRows("workspace", layered.Workspace.SubAgents)
	appendRows("effective", layered.Effective.SubAgents)
	return rows
}

func validAgentConfigKey(agent string) bool {
	if agent == "default" {
		return true
	}
	_, ok := config.LookupAgentKind(agent)
	return ok
}

func validGeneralSubAgentKey(agent string) bool {
	if agent == "default" {
		return true
	}
	return config.IsDeepAgentParentKind(agent)
}

func setAgentModelOverride(settings *config.Settings, agent string, value config.AgentModelOverride) {
	switch agent {
	case "default":
		settings.AgentModels.Default = value
	case config.AgentKindIDE:
		settings.AgentModels.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentModels.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentModels.ConfigManager = value
	case config.AgentKindInteractiveState:
		settings.AgentModels.InteractiveState = value
	case config.AgentKindInteractiveHotChoices:
		settings.AgentModels.InteractiveHotChoices = value
	case config.AgentKindVersionSummary:
		settings.AgentModels.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentModels.ToolAgent = value
	case config.AgentKindAutomation:
		settings.AgentModels.Automation = value
	case config.AgentKindContextCompaction:
		settings.AgentModels.ContextCompaction = value
	}
}

func setAgentToolOverride(settings *config.Settings, agent string, value config.AgentToolOverride) {
	switch agent {
	case "default":
		settings.AgentTools.Default = value
	case config.AgentKindIDE:
		settings.AgentTools.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentTools.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentTools.ConfigManager = value
	case config.AgentKindInteractiveState:
		settings.AgentTools.InteractiveState = value
	case config.AgentKindInteractiveHotChoices:
		settings.AgentTools.InteractiveHotChoices = value
	case config.AgentKindVersionSummary:
		settings.AgentTools.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentTools.ToolAgent = value
	case config.AgentKindAutomation:
		settings.AgentTools.Automation = value
	case config.AgentKindContextCompaction:
		settings.AgentTools.ContextCompaction = value
	}
}

func setAgentPromptOverride(settings *config.Settings, agent string, value config.AgentPromptOverride) {
	switch agent {
	case "default":
		settings.AgentPrompts.Default = value
	case config.AgentKindIDE:
		settings.AgentPrompts.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentPrompts.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentPrompts.ConfigManager = value
	case config.AgentKindInteractiveState:
		settings.AgentPrompts.InteractiveState = value
	case config.AgentKindInteractiveHotChoices:
		settings.AgentPrompts.InteractiveHotChoices = value
	case config.AgentKindVersionSummary:
		settings.AgentPrompts.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentPrompts.ToolAgent = value
	case config.AgentKindAutomation:
		settings.AgentPrompts.Automation = value
	case config.AgentKindContextCompaction:
		settings.AgentPrompts.ContextCompaction = value
	}
}

func setAgentSkillOverride(settings *config.Settings, agent string, value config.AgentSkillOverride) {
	switch agent {
	case "default":
		settings.AgentSkills.Default = value
	case config.AgentKindIDE:
		settings.AgentSkills.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentSkills.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentSkills.ConfigManager = value
	case config.AgentKindInteractiveState:
		settings.AgentSkills.InteractiveState = value
	case config.AgentKindInteractiveHotChoices:
		settings.AgentSkills.InteractiveHotChoices = value
	case config.AgentKindVersionSummary:
		settings.AgentSkills.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentSkills.ToolAgent = value
	case config.AgentKindAutomation:
		settings.AgentSkills.Automation = value
	case config.AgentKindContextCompaction:
		settings.AgentSkills.ContextCompaction = value
	}
}

func setAgentContextOverride(settings *config.Settings, agent string, value config.AgentContextOverride) {
	switch agent {
	case "default":
		settings.AgentContexts.Default = value
	case config.AgentKindIDE:
		settings.AgentContexts.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentContexts.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentContexts.ConfigManager = value
	case config.AgentKindInteractiveState:
		settings.AgentContexts.InteractiveState = value
	case config.AgentKindInteractiveHotChoices:
		settings.AgentContexts.InteractiveHotChoices = value
	case config.AgentKindVersionSummary:
		settings.AgentContexts.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentContexts.ToolAgent = value
	case config.AgentKindAutomation:
		settings.AgentContexts.Automation = value
	case config.AgentKindContextCompaction:
		settings.AgentContexts.ContextCompaction = value
	}
}

func setGeneralSubAgentOverride(settings *config.Settings, agent string, value *bool) {
	switch agent {
	case "default":
		settings.GeneralSubAgents.Default = value
	case config.AgentKindIDE:
		settings.GeneralSubAgents.IDE = value
	case config.AgentKindInteractiveStory:
		settings.GeneralSubAgents.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.GeneralSubAgents.ConfigManager = value
	case config.AgentKindAutomation:
		settings.GeneralSubAgents.Automation = value
	}
}

func fillSubAgentRequiredFields(sub config.SubAgentConfig, targetLayer, effective []config.SubAgentConfig) config.SubAgentConfig {
	id := config.NormalizeSubAgentID(sub.ID)
	if id == "" {
		return sub
	}
	base, ok := findSubAgentByID(targetLayer, id)
	if !ok {
		base, ok = findSubAgentByID(effective, id)
	}
	if !ok {
		return sub
	}
	if strings.TrimSpace(sub.Name) == "" {
		sub.Name = base.Name
	}
	if strings.TrimSpace(sub.Description) == "" {
		sub.Description = base.Description
	}
	if strings.TrimSpace(sub.SystemPrompt) == "" {
		sub.SystemPrompt = base.SystemPrompt
	}
	return sub
}

func findSubAgentByID(subAgents []config.SubAgentConfig, id string) (config.SubAgentConfig, bool) {
	for _, sub := range subAgents {
		if config.NormalizeSubAgentID(sub.ID) == id {
			return sub, true
		}
	}
	return config.SubAgentConfig{}, false
}

func upsertSubAgent(current []config.SubAgentConfig, sub config.SubAgentConfig) []config.SubAgentConfig {
	id := config.NormalizeSubAgentID(sub.ID)
	out := append([]config.SubAgentConfig{}, current...)
	for index := range out {
		if config.NormalizeSubAgentID(out[index].ID) == id {
			out[index] = sub
			return out
		}
	}
	return append(out, sub)
}

func deleteSubAgent(current []config.SubAgentConfig, id string) []config.SubAgentConfig {
	out := make([]config.SubAgentConfig, 0, len(current))
	for _, sub := range current {
		if config.NormalizeSubAgentID(sub.ID) != id {
			out = append(out, sub)
		}
	}
	return out
}
