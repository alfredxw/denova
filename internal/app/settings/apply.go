// Package settings applies persisted settings to process-local runtime
// configuration. It is shared by every app service that prepares an Agent
// runtime, keeping precedence and bounds identical across writing and game
// modes.
package settings

import (
	"os"
	"strings"

	"denova/config"
)

// RefreshProject reloads the user and project layers for one immutable runtime
// snapshot. Callers retain their project identity while receiving current
// model, tool, prompt, locale, and limit policy.
func RefreshProject(runtimeCfg config.Config, workspace, stateRoot string) (config.Config, error) {
	workspace = strings.TrimSpace(workspace)
	stateRoot = strings.TrimSpace(stateRoot)
	layered, err := config.LoadLayeredWithStartupConfigAt(
		runtimeCfg.DataDir(), workspace, config.ProjectConfigPath(stateRoot),
	)
	if err != nil {
		return config.Config{}, err
	}
	ApplyLayered(&runtimeCfg, layered)
	runtimeCfg.Workspace = workspace
	runtimeCfg.ProjectStoreDir = stateRoot
	return runtimeCfg, nil
}

// ApplyLayered applies user and workspace layers, followed by the canonical
// effective settings that require explicit defaults or replacement semantics.
func ApplyLayered(cfg *config.Config, layered config.LayeredSettings) {
	if cfg == nil {
		return
	}
	ApplyLayer(cfg, layered.User)
	ApplyLayer(cfg, layered.Workspace)

	effective := layered.Effective
	if cfg.OpenAIBaseURL == "" && effective.OpenAIBaseURL != "" {
		cfg.OpenAIBaseURL = effective.OpenAIBaseURL
	}
	if cfg.OpenAIModel == "" && effective.OpenAIModel != "" {
		cfg.OpenAIModel = effective.OpenAIModel
	}
	if effective.OpenAIContextWindowTokens != nil {
		cfg.OpenAIContextWindowTokens = positiveInt(effective.OpenAIContextWindowTokens, config.DefaultContextWindowTokens)
	}
	if len(effective.ModelProfiles) > 0 {
		cfg.ModelProfiles = effective.ModelProfiles
	}
	if len(effective.ModelEndpoints) > 0 {
		cfg.ModelEndpoints = effective.ModelEndpoints
	}
	if effective.DefaultImageAPIProfileID != "" {
		cfg.DefaultImageAPIProfileID = effective.DefaultImageAPIProfileID
	}
	cfg.ImageAPIEndpoints = append([]config.ImageAPIEndpointSettings(nil), effective.ImageAPIEndpoints...)
	cfg.ImageAPIProfiles = append([]config.ImageAPIProfileSettings(nil), effective.ImageAPIProfiles...)
	config.ApplyModelEnvironment(cfg)
	config.ApplyImageAPIEnvironment(cfg)
	cfg.AgentModels = effective.AgentModels
	cfg.AgentTools = effective.AgentTools
	cfg.AgentPrompts = effective.AgentPrompts
	cfg.AgentSkills = effective.AgentSkills
	cfg.AgentContexts = effective.AgentContexts
	cfg.GeneralSubAgents = effective.GeneralSubAgents
	cfg.SubAgents = effective.SubAgents
	cfg.CustomAgents = effective.CustomAgents
	if effective.DefaultImageAgentID != nil {
		cfg.DefaultImageAgentID = config.NormalizeCustomAgentID(*effective.DefaultImageAgentID)
	}
	cfg.WebAccess = config.ResolveWebAccessSettings(effective.WebAccess)
	cfg.Labs = config.ResolveLabs(effective.Labs)
	if cfg.SkillsDir == "" && effective.SkillsDir != "" {
		cfg.SkillsDir = effective.SkillsDir
	}
	if cfg.DataDir() == "" && layered.Paths.DenovaDir != "" {
		cfg.SetDataDir(layered.Paths.DenovaDir)
	}
	if effective.BackendPort != nil {
		cfg.BackendPort = positiveInt(effective.BackendPort, 8080)
	}
	if effective.FrontendPort != nil {
		cfg.FrontendPort = positiveInt(effective.FrontendPort, 5173)
	}
	if effective.AllowLANAccess != nil {
		cfg.AllowLANAccess = *effective.AllowLANAccess
	}
	cfg.RemoteAccessUsername = effective.RemoteAccessUsername
	cfg.RemoteAccessPasswordHash = effective.RemoteAccessPasswordHash
	if effective.Language != "" {
		cfg.Language = effective.Language
	}
	if cfg.IDEStoryTellerID == "" && effective.IDEStoryTellerID != "" {
		cfg.IDEStoryTellerID = effective.IDEStoryTellerID
	}
	if cfg.InteractiveStoryTellerID == "" && effective.InteractiveStoryTellerID != "" {
		cfg.InteractiveStoryTellerID = effective.InteractiveStoryTellerID
	}
	if effective.IDEImagePresetID != "" {
		cfg.IDEImagePresetID = effective.IDEImagePresetID
	}
	cfg.WritingSkillDefault = effective.WritingSkillDefault
	cfg.MaxIteration = positiveInt(effective.MaxIteration, 0)
	if effective.ModelMaxRetries != nil {
		cfg.ModelMaxRetries = positiveInt(effective.ModelMaxRetries, 5)
	}
	if effective.AgentIdleTimeoutSeconds != nil {
		cfg.AgentIdleTimeoutSeconds = idleTimeoutSeconds(effective.AgentIdleTimeoutSeconds)
	}
	if effective.AgentToolResultLimitKB != nil {
		cfg.AgentToolResultLimitKB = toolResultLimitKB(effective.AgentToolResultLimitKB)
	}
	if effective.AgentToolParallelism != nil {
		cfg.AgentToolParallelism = toolParallelism(effective.AgentToolParallelism)
	}
	if effective.AgentSubAgentParallelism != nil {
		cfg.AgentSubAgentParallelism = subAgentParallelism(effective.AgentSubAgentParallelism)
	}
	cfg.AgentApprovalMode = config.NormalizeAgentApprovalMode(effective.AgentApprovalMode)
	cfg.AgentApprovalRules = config.NormalizeAgentApprovalRules(effective.AgentApprovalRules)
	cfg.ShellEnvironmentMode = effective.ShellEnvironmentMode
	cfg.ShellEnvironmentShell = effective.ShellEnvironmentShell
	cfg.AgentBashPath = effective.AgentBashPath
	if effective.LLMInputLogEnabled != nil {
		cfg.LLMInputLogEnabled = *effective.LLMInputLogEnabled
	}
	if effective.TraceCaptureLevel != "" {
		cfg.TraceCaptureLevel = effective.TraceCaptureLevel
	}
	if effective.TraceExporter != "" {
		cfg.TraceExporter = effective.TraceExporter
	}
	if effective.TraceRetentionRuns != nil {
		cfg.TraceRetentionRuns = positiveInt(effective.TraceRetentionRuns, config.DefaultTraceRetentionRuns)
	}
	if effective.ChapterFilenameFormat != "" {
		cfg.ChapterFilenameFormat = effective.ChapterFilenameFormat
	}
	if effective.VolumeDirFormat != "" {
		cfg.VolumeDirFormat = effective.VolumeDirFormat
	}
	if effective.ChapterGroupMin != nil {
		cfg.ChapterGroupMin = positiveInt(effective.ChapterGroupMin, 3)
	}
	if effective.ChapterGroupMax != nil {
		cfg.ChapterGroupMax = positiveInt(effective.ChapterGroupMax, 8)
	}
	if effective.VersionTimedEnabled != nil {
		cfg.VersionTimedEnabled = *effective.VersionTimedEnabled
	}
	if effective.VersionTimedIntervalMinutes != nil {
		cfg.VersionTimedIntervalMinutes = positiveInt(effective.VersionTimedIntervalMinutes, 10)
	}
	if effective.TerminalEnabled != nil {
		cfg.TerminalEnabled = *effective.TerminalEnabled
	}
	cfg.TerminalShell = effective.TerminalShell
	if effective.TerminalCommands == nil {
		cfg.TerminalCommands = nil
	} else {
		cfg.TerminalCommands = make([]config.TerminalCommandSettings, len(effective.TerminalCommands))
		copy(cfg.TerminalCommands, effective.TerminalCommands)
	}
	if effective.TerminalMaxSessions != nil {
		cfg.TerminalMaxSessions = positiveInt(effective.TerminalMaxSessions, config.DefaultTerminalMaxSessions)
	}
	if effective.TerminalScrollbackKB != nil {
		cfg.TerminalScrollbackKB = positiveInt(effective.TerminalScrollbackKB, config.DefaultTerminalScrollbackKB)
	}
}

// ApplyLayer merges one persisted layer while preserving environment-variable
// precedence for secrets and process-level connection settings.
func ApplyLayer(cfg *config.Config, settings config.Settings) {
	if cfg == nil {
		return
	}
	if settings.OpenAIAPIKey != "" && os.Getenv("OPENAI_API_KEY") == "" {
		cfg.OpenAIAPIKey = settings.OpenAIAPIKey
	}
	if settings.OpenAIBaseURL != "" && os.Getenv("OPENAI_BASE_URL") == "" {
		cfg.OpenAIBaseURL = settings.OpenAIBaseURL
	}
	if settings.OpenAIModel != "" && os.Getenv("OPENAI_MODEL") == "" {
		cfg.OpenAIModel = settings.OpenAIModel
	}
	if len(settings.ModelProfiles) > 0 {
		cfg.ModelProfiles = config.Merge(config.Settings{ModelProfiles: cfg.ModelProfiles}, config.Settings{ModelProfiles: settings.ModelProfiles}).ModelProfiles
	}
	if len(settings.ModelEndpoints) > 0 {
		cfg.ModelEndpoints = config.Merge(config.Settings{ModelEndpoints: cfg.ModelEndpoints}, config.Settings{ModelEndpoints: settings.ModelEndpoints}).ModelEndpoints
	}
	if settings.DefaultImageAPIProfileID != "" {
		cfg.DefaultImageAPIProfileID = settings.DefaultImageAPIProfileID
	}
	if len(settings.ImageAPIProfiles) > 0 {
		cfg.ImageAPIProfiles = config.Merge(config.Settings{ImageAPIProfiles: cfg.ImageAPIProfiles}, config.Settings{ImageAPIProfiles: settings.ImageAPIProfiles}).ImageAPIProfiles
	}
	if len(settings.ImageAPIEndpoints) > 0 {
		cfg.ImageAPIEndpoints = config.Merge(config.Settings{ImageAPIEndpoints: cfg.ImageAPIEndpoints}, config.Settings{ImageAPIEndpoints: settings.ImageAPIEndpoints}).ImageAPIEndpoints
	}
	config.ApplyImageAPIEnvironment(cfg)
	cfg.AgentModels = config.MergeAgentModelSettings(cfg.AgentModels, settings.AgentModels)
	cfg.AgentTools = config.MergeAgentToolSettings(cfg.AgentTools, settings.AgentTools)
	cfg.AgentPrompts = config.MergeAgentPromptSettings(cfg.AgentPrompts, settings.AgentPrompts)
	cfg.AgentSkills = config.MergeAgentSkillSettings(cfg.AgentSkills, settings.AgentSkills)
	cfg.AgentContexts = config.MergeAgentContextSettings(cfg.AgentContexts, settings.AgentContexts)
	cfg.GeneralSubAgents = config.MergeAgentGeneralSubAgentSettings(cfg.GeneralSubAgents, settings.GeneralSubAgents)
	cfg.SubAgents = config.MergeSubAgents(cfg.SubAgents, settings.SubAgents)
	cfg.CustomAgents = config.MergeCustomAgents(cfg.CustomAgents, settings.CustomAgents)
	if settings.DefaultImageAgentID != nil {
		cfg.DefaultImageAgentID = config.NormalizeCustomAgentID(*settings.DefaultImageAgentID)
	}
	if settings.SkillsDir != "" && os.Getenv("DENOVA_SKILLS_DIR") == "" && os.Getenv("NOVA_SKILLS_DIR") == "" {
		cfg.SkillsDir = settings.SkillsDir
	}
	if settings.AllowLANAccess != nil {
		cfg.AllowLANAccess = *settings.AllowLANAccess
	}
	if settings.RemoteAccessUsername != "" {
		cfg.RemoteAccessUsername = settings.RemoteAccessUsername
	}
	if settings.RemoteAccessPasswordHash != "" {
		cfg.RemoteAccessPasswordHash = settings.RemoteAccessPasswordHash
	}
	if settings.Language != "" {
		cfg.Language = settings.Language
	}
	if settings.IDEStoryTellerID != "" {
		cfg.IDEStoryTellerID = settings.IDEStoryTellerID
	}
	if settings.InteractiveStoryTellerID != "" {
		cfg.InteractiveStoryTellerID = settings.InteractiveStoryTellerID
	}
	if settings.IDEImagePresetID != "" {
		cfg.IDEImagePresetID = settings.IDEImagePresetID
	}
	if settings.WritingSkillDefault != "" {
		cfg.WritingSkillDefault = settings.WritingSkillDefault
	}
	if settings.MaxIteration != nil {
		cfg.MaxIteration = positiveInt(settings.MaxIteration, 0)
	}
	if settings.ModelMaxRetries != nil {
		cfg.ModelMaxRetries = positiveInt(settings.ModelMaxRetries, 5)
	}
	if settings.AgentIdleTimeoutSeconds != nil {
		cfg.AgentIdleTimeoutSeconds = idleTimeoutSeconds(settings.AgentIdleTimeoutSeconds)
	}
	if settings.AgentToolResultLimitKB != nil {
		cfg.AgentToolResultLimitKB = toolResultLimitKB(settings.AgentToolResultLimitKB)
	}
	if settings.AgentToolParallelism != nil {
		cfg.AgentToolParallelism = toolParallelism(settings.AgentToolParallelism)
	}
	if settings.AgentSubAgentParallelism != nil {
		cfg.AgentSubAgentParallelism = subAgentParallelism(settings.AgentSubAgentParallelism)
	}
	if settings.AgentApprovalMode != "" {
		cfg.AgentApprovalMode = config.NormalizeAgentApprovalMode(settings.AgentApprovalMode)
	}
	if settings.AgentApprovalRules != nil {
		cfg.AgentApprovalRules = config.NormalizeAgentApprovalRules(settings.AgentApprovalRules)
	}
	if settings.ShellEnvironmentMode != "" {
		cfg.ShellEnvironmentMode = settings.ShellEnvironmentMode
	}
	if settings.ShellEnvironmentShell != "" {
		cfg.ShellEnvironmentShell = settings.ShellEnvironmentShell
	}
	if settings.AgentBashPath != "" {
		cfg.AgentBashPath = settings.AgentBashPath
	}
	if settings.LLMInputLogEnabled != nil {
		cfg.LLMInputLogEnabled = *settings.LLMInputLogEnabled
	}
	if settings.TraceCaptureLevel != "" {
		cfg.TraceCaptureLevel = settings.TraceCaptureLevel
	}
	if settings.TraceExporter != "" {
		cfg.TraceExporter = settings.TraceExporter
	}
	if settings.TraceRetentionRuns != nil {
		cfg.TraceRetentionRuns = positiveInt(settings.TraceRetentionRuns, config.DefaultTraceRetentionRuns)
	}
	if settings.ChapterFilenameFormat != "" {
		cfg.ChapterFilenameFormat = settings.ChapterFilenameFormat
	}
	if settings.VolumeDirFormat != "" {
		cfg.VolumeDirFormat = settings.VolumeDirFormat
	}
	if settings.ChapterGroupMin != nil {
		cfg.ChapterGroupMin = positiveInt(settings.ChapterGroupMin, 3)
	}
	if settings.ChapterGroupMax != nil {
		cfg.ChapterGroupMax = positiveInt(settings.ChapterGroupMax, 8)
	}
	if settings.VersionTimedEnabled != nil {
		cfg.VersionTimedEnabled = *settings.VersionTimedEnabled
	}
	if settings.VersionTimedIntervalMinutes != nil {
		cfg.VersionTimedIntervalMinutes = positiveInt(settings.VersionTimedIntervalMinutes, 10)
	}
}

// ApplyLocale accepts only the product's supported request locales.
func ApplyLocale(cfg *config.Config, locale string) {
	if cfg == nil {
		return
	}
	switch locale {
	case "zh-CN", "en-US":
		cfg.Language = locale
	}
}

func positiveInt(value *int, fallback int) int {
	if value == nil || *value <= 0 {
		return fallback
	}
	return *value
}

func idleTimeoutSeconds(value *int) int {
	if value == nil || *value < 0 {
		return config.DefaultAgentIdleTimeoutSeconds
	}
	return *value
}

func toolResultLimitKB(value *int) int {
	if value == nil || *value <= 0 {
		return config.DefaultAgentToolResultLimitKB
	}
	return *value
}

func toolParallelism(value *int) int {
	if value == nil || *value <= 0 {
		return config.DefaultAgentToolParallelism
	}
	if *value > config.MaxAgentToolParallelism {
		return config.MaxAgentToolParallelism
	}
	return *value
}

func subAgentParallelism(value *int) int {
	if value == nil || *value <= 0 {
		return config.DefaultAgentSubAgentParallelism
	}
	if *value > config.MaxAgentSubAgentParallelism {
		return config.MaxAgentSubAgentParallelism
	}
	return *value
}
