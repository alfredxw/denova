package config

import (
	"fmt"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const agentProfileSchemaVersion = 1

const agentProfileDefaultsFilename = "defaults.toml"

type agentProfileDefaultsDocument struct {
	SchemaVersion   int                  `toml:"schema_version"`
	Kind            string               `toml:"kind"`
	Model           AgentModelOverride   `toml:"model,omitempty"`
	Tools           AgentToolOverride    `toml:"tools,omitempty"`
	Prompt          AgentPromptOverride  `toml:"prompt,omitempty"`
	Skills          AgentSkillOverride   `toml:"skills,omitempty"`
	Context         AgentContextOverride `toml:"context,omitempty"`
	GeneralSubAgent *bool                `toml:"general_subagent_enabled,omitempty"`
}

type mainAgentProfileDocument struct {
	SchemaVersion       int                  `toml:"schema_version"`
	Kind                string               `toml:"kind"`
	Model               AgentModelOverride   `toml:"model,omitempty"`
	Tools               AgentToolOverride    `toml:"tools,omitempty"`
	Prompt              AgentPromptOverride  `toml:"prompt,omitempty"`
	Skills              AgentSkillOverride   `toml:"skills,omitempty"`
	Context             AgentContextOverride `toml:"context,omitempty"`
	GeneralSubAgent     *bool                `toml:"general_subagent_enabled,omitempty"`
	ToolParallelism     *int                 `toml:"tool_parallelism,omitempty"`
	SubAgentParallelism *int                 `toml:"subagent_parallelism,omitempty"`
	ImageAPIProfileID   string               `toml:"image_api_profile_id,omitempty"`
	DefaultImageAgentID *string              `toml:"default_image_agent_id,omitempty"`
}

type customAgentProfileDocument struct {
	SchemaVersion int               `toml:"schema_version"`
	Kind          string            `toml:"kind"`
	Agent         CustomAgentConfig `toml:"agent"`
}

type subAgentProfileDocument struct {
	SchemaVersion int            `toml:"schema_version"`
	Kind          string         `toml:"kind"`
	Agent         SubAgentConfig `toml:"agent"`
}

type fixedAgentProfile struct {
	Kind     string
	Filename string
}

var fixedAgentProfiles = []fixedAgentProfile{
	{Kind: AgentKindGeneral, Filename: "general.toml"},
	{Kind: AgentKindIDE, Filename: "writing.toml"},
	{Kind: AgentKindInteractiveStory, Filename: "game.toml"},
	{Kind: AgentKindImage, Filename: "image.toml"},
	{Kind: AgentKindVersionSummary, Filename: "version-summary.toml"},
	{Kind: AgentKindToolAgent, Filename: "tool-agent.toml"},
}

func encodeAgentProfileDefaults(settings Settings) ([]byte, error) {
	return toml.Marshal(agentProfileDefaultsDocument{
		SchemaVersion:   agentProfileSchemaVersion,
		Kind:            "defaults",
		Model:           settings.AgentModels.Default,
		Tools:           settings.AgentTools.Default,
		Prompt:          settings.AgentPrompts.Default,
		Skills:          settings.AgentSkills.Default,
		Context:         settings.AgentContexts.Default,
		GeneralSubAgent: settings.GeneralSubAgents.Default,
	})
}

func decodeAgentProfileDefaults(path string, content []byte) (agentProfileDefaultsDocument, error) {
	var document agentProfileDefaultsDocument
	if err := toml.Unmarshal(content, &document); err != nil {
		return agentProfileDefaultsDocument{}, fmt.Errorf("decode Agent Profile defaults %s: %w", path, err)
	}
	if document.SchemaVersion != agentProfileSchemaVersion || strings.TrimSpace(document.Kind) != "defaults" {
		return agentProfileDefaultsDocument{}, fmt.Errorf("invalid Agent Profile defaults header in %s", path)
	}
	return document, nil
}

func applyAgentProfileDefaults(settings *Settings, document agentProfileDefaultsDocument) {
	settings.AgentModels.Default = document.Model
	settings.AgentTools.Default = document.Tools
	settings.AgentPrompts.Default = document.Prompt
	settings.AgentSkills.Default = document.Skills
	settings.AgentContexts.Default = document.Context
	settings.GeneralSubAgents.Default = document.GeneralSubAgent
}

func mainAgentProfileForSettings(settings Settings, kind string) (mainAgentProfileDocument, error) {
	definition, ok := LookupAgentKind(kind)
	if !ok {
		return mainAgentProfileDocument{}, fmt.Errorf("unknown main Agent kind %q", kind)
	}
	document := mainAgentProfileDocument{SchemaVersion: agentProfileSchemaVersion, Kind: kind}
	if definition.ModelOverride != nil {
		document.Model = definition.ModelOverride(settings.AgentModels)
	}
	if definition.ToolOverride != nil {
		document.Tools = definition.ToolOverride(settings.AgentTools)
	}
	if definition.PromptOverride != nil {
		document.Prompt = definition.PromptOverride(settings.AgentPrompts)
	}
	if definition.SkillOverride != nil {
		document.Skills = definition.SkillOverride(settings.AgentSkills)
	}
	if definition.ContextOverride != nil {
		document.Context = definition.ContextOverride(settings.AgentContexts)
	}
	document.GeneralSubAgent = generalSubAgentOverrideFor(settings.GeneralSubAgents, kind)
	switch kind {
	case AgentKindGeneral:
		document.ToolParallelism = settings.AgentToolParallelism
		document.SubAgentParallelism = settings.AgentSubAgentParallelism
	case AgentKindImage:
		document.ImageAPIProfileID = strings.TrimSpace(settings.DefaultImageAPIProfileID)
		document.DefaultImageAgentID = settings.DefaultImageAgentID
	}
	return document, nil
}

func applyMainAgentProfile(settings *Settings, document mainAgentProfileDocument, expectedKind string) error {
	if settings == nil {
		return fmt.Errorf("Agent Profile settings target is nil")
	}
	if document.SchemaVersion != agentProfileSchemaVersion {
		return fmt.Errorf("unsupported Agent Profile schema_version %d", document.SchemaVersion)
	}
	document.Kind = strings.TrimSpace(document.Kind)
	if document.Kind != expectedKind {
		return fmt.Errorf("Agent Profile kind %q does not match file kind %q", document.Kind, expectedKind)
	}
	definition, ok := LookupAgentKind(expectedKind)
	if !ok {
		return fmt.Errorf("unknown main Agent kind %q", expectedKind)
	}
	if definition.SetModelOverride != nil {
		definition.SetModelOverride(&settings.AgentModels, document.Model)
	}
	if definition.SetToolOverride != nil {
		definition.SetToolOverride(&settings.AgentTools, document.Tools)
	}
	if definition.SetPromptOverride != nil {
		definition.SetPromptOverride(&settings.AgentPrompts, document.Prompt)
	}
	if definition.SetSkillOverride != nil {
		definition.SetSkillOverride(&settings.AgentSkills, document.Skills)
	}
	if definition.SetContextOverride != nil {
		definition.SetContextOverride(&settings.AgentContexts, document.Context)
	}
	setGeneralSubAgentProfileOverride(&settings.GeneralSubAgents, expectedKind, document.GeneralSubAgent)
	switch expectedKind {
	case AgentKindGeneral:
		settings.AgentToolParallelism = document.ToolParallelism
		settings.AgentSubAgentParallelism = document.SubAgentParallelism
	case AgentKindImage:
		settings.DefaultImageAPIProfileID = strings.TrimSpace(document.ImageAPIProfileID)
		settings.DefaultImageAgentID = document.DefaultImageAgentID
	}
	return nil
}

func setGeneralSubAgentProfileOverride(settings *AgentGeneralSubAgentSettings, kind string, enabled *bool) {
	if settings == nil {
		return
	}
	switch kind {
	case AgentKindGeneral:
		settings.General = enabled
	case AgentKindIDE:
		settings.IDE = enabled
	case AgentKindInteractiveStory:
		settings.InteractiveStory = enabled
	}
}

func encodeMainAgentProfile(settings Settings, profile fixedAgentProfile) ([]byte, error) {
	document, err := mainAgentProfileForSettings(settings, profile.Kind)
	if err != nil {
		return nil, err
	}
	return toml.Marshal(document)
}

func decodeMainAgentProfile(path string, content []byte, expectedKind string) (mainAgentProfileDocument, error) {
	var document mainAgentProfileDocument
	if err := toml.Unmarshal(content, &document); err != nil {
		return mainAgentProfileDocument{}, fmt.Errorf("decode Agent Profile %s: %w", path, err)
	}
	probe := Settings{}
	if err := applyMainAgentProfile(&probe, document, expectedKind); err != nil {
		return mainAgentProfileDocument{}, fmt.Errorf("validate Agent Profile %s: %w", path, err)
	}
	return document, nil
}

func encodeCustomAgentProfile(agent CustomAgentConfig) ([]byte, error) {
	return toml.Marshal(customAgentProfileDocument{
		SchemaVersion: agentProfileSchemaVersion,
		Kind:          "custom_main_agent",
		Agent:         agent,
	})
}

func decodeCustomAgentProfile(path string, content []byte) (CustomAgentConfig, error) {
	var document customAgentProfileDocument
	if err := toml.Unmarshal(content, &document); err != nil {
		return CustomAgentConfig{}, fmt.Errorf("decode Custom Main Agent Profile %s: %w", path, err)
	}
	if document.SchemaVersion != agentProfileSchemaVersion || strings.TrimSpace(document.Kind) != "custom_main_agent" {
		return CustomAgentConfig{}, fmt.Errorf("invalid Custom Main Agent Profile header in %s", path)
	}
	agents := SanitizeCustomAgents([]CustomAgentConfig{document.Agent})
	if len(agents) != 1 || strings.TrimSpace(agents[0].Name) == "" || CustomAgentRuntimeKind(agents[0]) == "" {
		return CustomAgentConfig{}, fmt.Errorf("invalid Custom Main Agent Profile in %s", path)
	}
	if agents[0].ID != strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) {
		return CustomAgentConfig{}, fmt.Errorf("Custom Main Agent ID %q does not match filename %q", agents[0].ID, filepath.Base(path))
	}
	return agents[0], nil
}

func encodeSubAgentProfile(agent SubAgentConfig) ([]byte, error) {
	return toml.Marshal(subAgentProfileDocument{
		SchemaVersion: agentProfileSchemaVersion,
		Kind:          "subagent",
		Agent:         agent,
	})
}

func decodeSubAgentProfile(path string, content []byte) (SubAgentConfig, error) {
	var document subAgentProfileDocument
	if err := toml.Unmarshal(content, &document); err != nil {
		return SubAgentConfig{}, fmt.Errorf("decode SubAgent Profile %s: %w", path, err)
	}
	if document.SchemaVersion != agentProfileSchemaVersion || strings.TrimSpace(document.Kind) != "subagent" {
		return SubAgentConfig{}, fmt.Errorf("invalid SubAgent Profile header in %s", path)
	}
	agents := SanitizeSubAgents([]SubAgentConfig{document.Agent})
	if len(agents) != 1 {
		return SubAgentConfig{}, fmt.Errorf("invalid SubAgent Profile in %s", path)
	}
	if agents[0].ID != strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) {
		return SubAgentConfig{}, fmt.Errorf("SubAgent ID %q does not match filename %q", agents[0].ID, filepath.Base(path))
	}
	return agents[0], nil
}

func agentProfileSettings(settings Settings) Settings {
	return Settings{
		DefaultImageAPIProfileID: settings.DefaultImageAPIProfileID,
		AgentModels: AgentModelSettings{
			Default: settings.AgentModels.Default, General: settings.AgentModels.General,
			IDE: settings.AgentModels.IDE, InteractiveStory: settings.AgentModels.InteractiveStory,
			VersionSummary: settings.AgentModels.VersionSummary, ToolAgent: settings.AgentModels.ToolAgent, Image: settings.AgentModels.Image,
		},
		AgentTools: AgentToolSettings{
			Default: settings.AgentTools.Default, General: settings.AgentTools.General,
			IDE: settings.AgentTools.IDE, InteractiveStory: settings.AgentTools.InteractiveStory,
			VersionSummary: settings.AgentTools.VersionSummary, ToolAgent: settings.AgentTools.ToolAgent, Image: settings.AgentTools.Image,
		},
		AgentPrompts: AgentPromptSettings{
			Default: settings.AgentPrompts.Default, General: settings.AgentPrompts.General,
			IDE: settings.AgentPrompts.IDE, InteractiveStory: settings.AgentPrompts.InteractiveStory,
			VersionSummary: settings.AgentPrompts.VersionSummary, ToolAgent: settings.AgentPrompts.ToolAgent, Image: settings.AgentPrompts.Image,
		},
		AgentSkills: AgentSkillSettings{
			Default: settings.AgentSkills.Default, General: settings.AgentSkills.General,
			IDE: settings.AgentSkills.IDE, InteractiveStory: settings.AgentSkills.InteractiveStory,
			VersionSummary: settings.AgentSkills.VersionSummary, ToolAgent: settings.AgentSkills.ToolAgent, Image: settings.AgentSkills.Image,
		},
		AgentContexts: AgentContextSettings{
			Default: settings.AgentContexts.Default, General: settings.AgentContexts.General,
			IDE: settings.AgentContexts.IDE, InteractiveStory: settings.AgentContexts.InteractiveStory,
			VersionSummary: settings.AgentContexts.VersionSummary, ToolAgent: settings.AgentContexts.ToolAgent, Image: settings.AgentContexts.Image,
		},
		GeneralSubAgents: AgentGeneralSubAgentSettings{
			Default: settings.GeneralSubAgents.Default, General: settings.GeneralSubAgents.General,
			IDE: settings.GeneralSubAgents.IDE, InteractiveStory: settings.GeneralSubAgents.InteractiveStory,
		},
		SubAgents:                append([]SubAgentConfig(nil), settings.SubAgents...),
		CustomAgents:             append([]CustomAgentConfig(nil), settings.CustomAgents...),
		DefaultImageAgentID:      settings.DefaultImageAgentID,
		AgentToolParallelism:     settings.AgentToolParallelism,
		AgentSubAgentParallelism: settings.AgentSubAgentParallelism,
	}
}

func clearAgentProfileSettings(settings *Settings) {
	if settings == nil {
		return
	}
	settings.DefaultImageAPIProfileID = ""
	settings.AgentModels.Default = AgentModelOverride{}
	settings.AgentModels.General = AgentModelOverride{}
	settings.AgentModels.IDE = AgentModelOverride{}
	settings.AgentModels.InteractiveStory = AgentModelOverride{}
	settings.AgentModels.VersionSummary = AgentModelOverride{}
	settings.AgentModels.ToolAgent = AgentModelOverride{}
	settings.AgentModels.Image = AgentModelOverride{}
	settings.AgentTools.Default = nil
	settings.AgentTools.General = nil
	settings.AgentTools.IDE = nil
	settings.AgentTools.InteractiveStory = nil
	settings.AgentTools.VersionSummary = nil
	settings.AgentTools.ToolAgent = nil
	settings.AgentTools.Image = nil
	settings.AgentPrompts.Default = AgentPromptOverride{}
	settings.AgentPrompts.General = AgentPromptOverride{}
	settings.AgentPrompts.IDE = AgentPromptOverride{}
	settings.AgentPrompts.InteractiveStory = AgentPromptOverride{}
	settings.AgentPrompts.VersionSummary = AgentPromptOverride{}
	settings.AgentPrompts.ToolAgent = AgentPromptOverride{}
	settings.AgentPrompts.Image = AgentPromptOverride{}
	settings.AgentSkills.Default = nil
	settings.AgentSkills.General = nil
	settings.AgentSkills.IDE = nil
	settings.AgentSkills.InteractiveStory = nil
	settings.AgentSkills.VersionSummary = nil
	settings.AgentSkills.ToolAgent = nil
	settings.AgentSkills.Image = nil
	settings.AgentContexts.Default = AgentContextOverride{}
	settings.AgentContexts.General = AgentContextOverride{}
	settings.AgentContexts.IDE = AgentContextOverride{}
	settings.AgentContexts.InteractiveStory = AgentContextOverride{}
	settings.AgentContexts.VersionSummary = AgentContextOverride{}
	settings.AgentContexts.ToolAgent = AgentContextOverride{}
	settings.AgentContexts.Image = AgentContextOverride{}
	settings.GeneralSubAgents.Default = nil
	settings.GeneralSubAgents.General = nil
	settings.GeneralSubAgents.IDE = nil
	settings.GeneralSubAgents.InteractiveStory = nil
	settings.SubAgents = nil
	settings.CustomAgents = nil
	settings.DefaultImageAgentID = nil
	settings.AgentToolParallelism = nil
	settings.AgentSubAgentParallelism = nil
}

func mergeAgentProfileLayer(base, profiles Settings) Settings {
	clearAgentProfileSettings(&base)
	profile := agentProfileSettings(profiles)
	base.DefaultImageAPIProfileID = profile.DefaultImageAPIProfileID
	base.AgentModels = MergeAgentModelSettings(base.AgentModels, profile.AgentModels)
	base.AgentTools = MergeAgentToolSettings(base.AgentTools, profile.AgentTools)
	base.AgentPrompts = MergeAgentPromptSettings(base.AgentPrompts, profile.AgentPrompts)
	base.AgentSkills = MergeAgentSkillSettings(base.AgentSkills, profile.AgentSkills)
	base.AgentContexts = MergeAgentContextSettings(base.AgentContexts, profile.AgentContexts)
	base.GeneralSubAgents = MergeAgentGeneralSubAgentSettings(base.GeneralSubAgents, profile.GeneralSubAgents)
	base.SubAgents = profile.SubAgents
	base.CustomAgents = profile.CustomAgents
	base.DefaultImageAgentID = profile.DefaultImageAgentID
	base.AgentToolParallelism = profile.AgentToolParallelism
	base.AgentSubAgentParallelism = profile.AgentSubAgentParallelism
	return base
}
