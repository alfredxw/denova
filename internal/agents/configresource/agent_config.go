package configresource

import (
	"fmt"
	"strings"

	"denova/config"
)

type agentConfigSnapshot struct {
	Paths            config.SettingsPaths          `json:"paths"`
	Agents           []agentConfigAgentDefinition  `json:"agents"`
	SubAgentParents  []string                      `json:"subagent_parents"`
	ToolCapabilities []agentConfigToolCapability   `json:"tool_capabilities"`
	Layers           agentConfigLayeredSnapshot    `json:"layers"`
	SubAgentIndex    []agentConfigSubAgentIndexRow `json:"sub_agent_index"`
	Notes            []string                      `json:"notes,omitempty"`
}

type agentConfigAgentDefinition struct {
	Kind           string `json:"kind"`
	SessionID      string `json:"session_id,omitempty"`
	SubAgentParent bool   `json:"subagent_parent"`
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
	DefaultImageAPI  string                              `json:"default_image_api_profile_id,omitempty"`
	ImageAPIProfiles []safeImageAPIProfileSettings       `json:"image_api_profiles,omitempty"`
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
	Provider            string   `json:"provider,omitempty"`
	Protocol            string   `json:"protocol,omitempty"`
	BaseURL             string   `json:"base_url,omitempty"`
	Model               string   `json:"model,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
	ContextWindowTokens *int     `json:"context_window_tokens,omitempty"`
}

type safeImageAPIProfileSettings struct {
	ID                  string                      `json:"id,omitempty"`
	Name                string                      `json:"name,omitempty"`
	Provider            string                      `json:"provider,omitempty"`
	Protocol            string                      `json:"protocol,omitempty"`
	BaseURL             string                      `json:"base_url,omitempty"`
	Model               string                      `json:"model,omitempty"`
	PromptGuide         string                      `json:"prompt_guide,omitempty"`
	DefaultSize         string                      `json:"default_size,omitempty"`
	DefaultAspectRatio  string                      `json:"default_aspect_ratio,omitempty"`
	DefaultResolution   string                      `json:"default_resolution,omitempty"`
	DefaultQuality      string                      `json:"default_quality,omitempty"`
	DefaultOutputFormat string                      `json:"default_output_format,omitempty"`
	ComfyUI             *safeComfyUIProfileSettings `json:"comfyui,omitempty"`
}

type safeComfyUIProfileSettings struct {
	WorkflowMode string `json:"workflow_mode,omitempty"`
	WorkflowName string `json:"workflow_name,omitempty"`
}

type agentConfigSubAgentIndexRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name,omitempty"`
	Enabled     bool     `json:"enabled"`
	Parents     []string `json:"parents,omitempty"`
	Description string   `json:"description,omitempty"`
	Layer       string   `json:"layer"`
}

func loadAgentConfigLayered(cfg *config.Config) (config.LayeredSettings, error) {
	novaDir := ""
	workspace := ""
	projectConfigPath := ""
	if cfg != nil {
		novaDir = cfg.DataDir()
		workspace = cfg.Workspace
		projectConfigPath = config.ProjectConfigPath(cfg.ProjectStateDir)
	}
	layered, err := config.LoadLayeredWithStartupConfigAt(novaDir, workspace, projectConfigPath)
	if err != nil {
		return config.LayeredSettings{}, fmt.Errorf("read Agent configuration: %w", err)
	}
	return layered, nil
}

func writableAgentConfigPath(cfg *config.Config, scope string) (string, error) {
	novaDir := ""
	workspace := ""
	if cfg != nil {
		novaDir = cfg.DataDir()
		workspace = cfg.Workspace
	}
	switch scope {
	case "user":
		return config.UserConfigPath(novaDir), nil
	case "workspace":
		if cfg != nil && strings.TrimSpace(cfg.ProjectStateDir) != "" {
			return config.ProjectConfigPath(cfg.ProjectStateDir), nil
		}
		if strings.TrimSpace(workspace) == "" {
			return "", fmt.Errorf("cannot write workspace configuration because no workspace is open")
		}
		return config.WorkspaceConfigPath(workspace), nil
	default:
		return "", fmt.Errorf("unsupported configuration scope: %s", scope)
	}
}

func agentConfigDefinitions() []agentConfigAgentDefinition {
	definitions := config.AgentKindDefinitions()
	out := make([]agentConfigAgentDefinition, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, agentConfigAgentDefinition{
			Kind:           definition.Kind,
			SessionID:      definition.SessionID,
			SubAgentParent: config.IsSubAgentParentKind(definition.Kind),
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
		DefaultImageAPI:  settings.DefaultImageAPIProfileID,
		ImageAPIProfiles: safeImageAPIProfiles(settings.ImageAPIProfiles),
		AgentModels:      settings.AgentModels,
		AgentTools:       settings.AgentTools,
		AgentPrompts:     settings.AgentPrompts,
		AgentSkills:      settings.AgentSkills,
		AgentContext:     settings.AgentContexts,
		GeneralSubAgents: settings.GeneralSubAgents,
		SubAgents:        settings.SubAgents,
	}
}

func safeImageAPIProfiles(profiles []config.ImageAPIProfileSettings) []safeImageAPIProfileSettings {
	if len(profiles) == 0 {
		return nil
	}
	out := make([]safeImageAPIProfileSettings, 0, len(profiles))
	for _, profile := range profiles {
		var comfy *safeComfyUIProfileSettings
		if profile.ComfyUI != nil {
			comfy = &safeComfyUIProfileSettings{
				WorkflowMode: profile.ComfyUI.WorkflowMode,
				WorkflowName: profile.ComfyUI.WorkflowName,
			}
		}
		out = append(out, safeImageAPIProfileSettings{
			ID:                  profile.ID,
			Name:                profile.Name,
			Provider:            profile.Provider,
			Protocol:            profile.Protocol,
			BaseURL:             profile.BaseURL,
			Model:               profile.Model,
			PromptGuide:         profile.PromptGuide,
			DefaultSize:         profile.DefaultSize,
			DefaultAspectRatio:  profile.DefaultAspectRatio,
			DefaultResolution:   profile.DefaultResolution,
			DefaultQuality:      profile.DefaultQuality,
			DefaultOutputFormat: profile.DefaultOutputFormat,
			ComfyUI:             comfy,
		})
	}
	return out
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
			Provider:            profile.Provider,
			Protocol:            profile.Protocol,
			BaseURL:             profile.BaseURL,
			Model:               profile.Model,
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
	return config.IsSubAgentParentKind(agent)
}

func setAgentModelOverride(settings *config.Settings, agent string, value config.AgentModelOverride) {
	switch agent {
	case "default":
		settings.AgentModels.Default = value
	case config.AgentKindGeneral:
		settings.AgentModels.General = value
	case config.AgentKindIDE:
		settings.AgentModels.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentModels.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentModels.ConfigManager = value
	case config.AgentKindInteractiveDirector:
		settings.AgentModels.InteractiveDirector = value
	case config.AgentKindVersionSummary:
		settings.AgentModels.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentModels.ToolAgent = value
	case config.AgentKindImage:
		settings.AgentModels.Image = value
	}
}

func setAgentToolOverride(settings *config.Settings, agent string, value config.AgentToolOverride) {
	switch agent {
	case "default":
		settings.AgentTools.Default = value
	case config.AgentKindGeneral:
		settings.AgentTools.General = value
	case config.AgentKindIDE:
		settings.AgentTools.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentTools.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentTools.ConfigManager = value
	case config.AgentKindInteractiveDirector:
		settings.AgentTools.InteractiveDirector = value
	case config.AgentKindVersionSummary:
		settings.AgentTools.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentTools.ToolAgent = value
	case config.AgentKindImage:
		settings.AgentTools.Image = value
	}
}

func setAgentPromptOverride(settings *config.Settings, agent string, value config.AgentPromptOverride) {
	switch agent {
	case "default":
		settings.AgentPrompts.Default = value
	case config.AgentKindGeneral:
		settings.AgentPrompts.General = value
	case config.AgentKindIDE:
		settings.AgentPrompts.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentPrompts.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentPrompts.ConfigManager = value
	case config.AgentKindInteractiveDirector:
		settings.AgentPrompts.InteractiveDirector = value
	case config.AgentKindVersionSummary:
		settings.AgentPrompts.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentPrompts.ToolAgent = value
	case config.AgentKindImage:
		settings.AgentPrompts.Image = value
	}
}

func setAgentSkillOverride(settings *config.Settings, agent string, value config.AgentSkillOverride) {
	switch agent {
	case "default":
		settings.AgentSkills.Default = value
	case config.AgentKindGeneral:
		settings.AgentSkills.General = value
	case config.AgentKindIDE:
		settings.AgentSkills.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentSkills.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentSkills.ConfigManager = value
	case config.AgentKindInteractiveDirector:
		settings.AgentSkills.InteractiveDirector = value
	case config.AgentKindVersionSummary:
		settings.AgentSkills.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentSkills.ToolAgent = value
	case config.AgentKindImage:
		settings.AgentSkills.Image = value
	}
}

func setAgentContextOverride(settings *config.Settings, agent string, value config.AgentContextOverride) {
	switch agent {
	case "default":
		settings.AgentContexts.Default = value
	case config.AgentKindGeneral:
		settings.AgentContexts.General = value
	case config.AgentKindIDE:
		settings.AgentContexts.IDE = value
	case config.AgentKindInteractiveStory:
		settings.AgentContexts.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.AgentContexts.ConfigManager = value
	case config.AgentKindInteractiveDirector:
		settings.AgentContexts.InteractiveDirector = value
	case config.AgentKindVersionSummary:
		settings.AgentContexts.VersionSummary = value
	case config.AgentKindToolAgent:
		settings.AgentContexts.ToolAgent = value
	case config.AgentKindImage:
		settings.AgentContexts.Image = value
	}
}

func setGeneralSubAgentOverride(settings *config.Settings, agent string, value *bool) {
	switch agent {
	case "default":
		settings.GeneralSubAgents.Default = value
	case config.AgentKindGeneral:
		settings.GeneralSubAgents.General = value
	case config.AgentKindIDE:
		settings.GeneralSubAgents.IDE = value
	case config.AgentKindInteractiveStory:
		settings.GeneralSubAgents.InteractiveStory = value
	case config.AgentKindConfigManager:
		settings.GeneralSubAgents.ConfigManager = value
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
