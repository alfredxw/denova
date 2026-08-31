package config

import (
	"runtime"

	agent "github.com/alfredxw/denova/agent"
)

const (
	AgentKindIDE              = "ide"
	AgentKindGeneral          = "general"
	AgentKindInteractiveStory = "interactive_story"
	// AgentKindConfigManager is retained only to decode Beta settings and
	// journals. Configuration work now runs in General or Writing Project Agents.
	AgentKindConfigManager  = "config_manager"
	AgentKindVersionSummary = "version_summary"
	AgentKindToolAgent      = "tool_agent"
	AgentKindImage          = "image"
	// AgentKindAutomation is retained only to decode Beta runtime journals.
	// New automation turns always run as their owning Project Agent.
	AgentKindAutomation = "automation"
)

// AgentKindDefinition is the registry entry for one runtime Agent kind.
// Config accessors keep the persisted TOML/JSON shape stable while avoiding
// scattered agent-kind switches for model, tool, prompt and session behavior.
type AgentKindDefinition struct {
	Kind      string
	SessionID string
	// ToolCapabilities is the authoritative capability ceiling for this
	// Agent kind, as well as the stable display order of its tool manifest.
	ToolCapabilities   []string
	ModelOverride      func(AgentModelSettings) AgentModelOverride
	SetModelOverride   func(*AgentModelSettings, AgentModelOverride)
	ToolOverride       func(AgentToolSettings) AgentToolOverride
	SetToolOverride    func(*AgentToolSettings, AgentToolOverride)
	PromptOverride     func(AgentPromptSettings) AgentPromptOverride
	SetPromptOverride  func(*AgentPromptSettings, AgentPromptOverride)
	SkillOverride      func(AgentSkillSettings) AgentSkillOverride
	SetSkillOverride   func(*AgentSkillSettings, AgentSkillOverride)
	ContextOverride    func(AgentContextSettings) AgentContextOverride
	SetContextOverride func(*AgentContextSettings, AgentContextOverride)
}

var agentKindRegistry = []AgentKindDefinition{
	{
		Kind: AgentKindGeneral,
		ToolCapabilities: []string{
			AgentToolFilesystemRead, AgentToolWorkspaceWrite, AgentToolShell,
			AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolAsk, AgentToolTodo, AgentToolSkills, AgentToolDelegation,
			AgentToolScript, AgentToolTrajectory,
			AgentToolConfigRead, AgentToolConfigApply,
		},
		ModelOverride:      func(settings AgentModelSettings) AgentModelOverride { return settings.General },
		SetModelOverride:   func(settings *AgentModelSettings, override AgentModelOverride) { settings.General = override },
		ToolOverride:       func(settings AgentToolSettings) AgentToolOverride { return settings.General },
		SetToolOverride:    func(settings *AgentToolSettings, override AgentToolOverride) { settings.General = override },
		PromptOverride:     func(settings AgentPromptSettings) AgentPromptOverride { return settings.General },
		SetPromptOverride:  func(settings *AgentPromptSettings, override AgentPromptOverride) { settings.General = override },
		SkillOverride:      func(settings AgentSkillSettings) AgentSkillOverride { return settings.General },
		SetSkillOverride:   func(settings *AgentSkillSettings, override AgentSkillOverride) { settings.General = override },
		ContextOverride:    func(settings AgentContextSettings) AgentContextOverride { return settings.General },
		SetContextOverride: func(settings *AgentContextSettings, override AgentContextOverride) { settings.General = override },
	},
	{
		Kind: AgentKindIDE,
		ToolCapabilities: []string{
			AgentToolFilesystemRead, AgentToolWorkspaceWrite, AgentToolShell,
			AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolAsk, AgentToolTodo, AgentToolSkills, AgentToolDelegation,
			AgentToolScript,
			AgentToolConfigRead, AgentToolConfigApply,
			AgentToolLoreRead, AgentToolLoreWrite, AgentToolImageGeneration,
		},
		ModelOverride:      func(settings AgentModelSettings) AgentModelOverride { return settings.IDE },
		SetModelOverride:   func(settings *AgentModelSettings, override AgentModelOverride) { settings.IDE = override },
		ToolOverride:       func(settings AgentToolSettings) AgentToolOverride { return settings.IDE },
		SetToolOverride:    func(settings *AgentToolSettings, override AgentToolOverride) { settings.IDE = override },
		PromptOverride:     func(settings AgentPromptSettings) AgentPromptOverride { return settings.IDE },
		SetPromptOverride:  func(settings *AgentPromptSettings, override AgentPromptOverride) { settings.IDE = override },
		SkillOverride:      func(settings AgentSkillSettings) AgentSkillOverride { return settings.IDE },
		SetSkillOverride:   func(settings *AgentSkillSettings, override AgentSkillOverride) { settings.IDE = override },
		ContextOverride:    func(settings AgentContextSettings) AgentContextOverride { return settings.IDE },
		SetContextOverride: func(settings *AgentContextSettings, override AgentContextOverride) { settings.IDE = override },
	},
	{
		Kind: AgentKindInteractiveStory,
		ToolCapabilities: []string{
			AgentToolFilesystemRead, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolSkills, AgentToolDelegation, AgentToolLoreRead,
			AgentToolScript,
		},
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.InteractiveStory },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.InteractiveStory = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.InteractiveStory },
		SetToolOverride:  func(settings *AgentToolSettings, override AgentToolOverride) { settings.InteractiveStory = override },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.InteractiveStory },
		SetPromptOverride: func(settings *AgentPromptSettings, override AgentPromptOverride) {
			settings.InteractiveStory = override
		},
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.InteractiveStory },
		SetSkillOverride: func(settings *AgentSkillSettings, override AgentSkillOverride) { settings.InteractiveStory = override },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.InteractiveStory },
		SetContextOverride: func(settings *AgentContextSettings, override AgentContextOverride) {
			settings.InteractiveStory = override
		},
	},
	{
		Kind:             AgentKindVersionSummary,
		SessionID:        "version-summary-agent",
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.VersionSummary },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.VersionSummary = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.VersionSummary },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.VersionSummary },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.VersionSummary },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.VersionSummary },
	},
	{
		Kind:             AgentKindToolAgent,
		SessionID:        "tool-agent",
		ModelOverride:    func(settings AgentModelSettings) AgentModelOverride { return settings.ToolAgent },
		SetModelOverride: func(settings *AgentModelSettings, override AgentModelOverride) { settings.ToolAgent = override },
		ToolOverride:     func(settings AgentToolSettings) AgentToolOverride { return settings.ToolAgent },
		PromptOverride:   func(settings AgentPromptSettings) AgentPromptOverride { return settings.ToolAgent },
		SkillOverride:    func(settings AgentSkillSettings) AgentSkillOverride { return settings.ToolAgent },
		ContextOverride:  func(settings AgentContextSettings) AgentContextOverride { return settings.ToolAgent },
	},
	{
		Kind:      AgentKindImage,
		SessionID: "image-agent",
		ToolCapabilities: []string{
			AgentToolFilesystemRead, AgentToolWorkspaceWrite, AgentToolShell,
			AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
			AgentToolSkills, AgentToolImageGeneration,
		},
		ModelOverride:      func(settings AgentModelSettings) AgentModelOverride { return settings.Image },
		SetModelOverride:   func(settings *AgentModelSettings, override AgentModelOverride) { settings.Image = override },
		ToolOverride:       func(settings AgentToolSettings) AgentToolOverride { return settings.Image },
		SetToolOverride:    func(settings *AgentToolSettings, override AgentToolOverride) { settings.Image = override },
		PromptOverride:     func(settings AgentPromptSettings) AgentPromptOverride { return settings.Image },
		SetPromptOverride:  func(settings *AgentPromptSettings, override AgentPromptOverride) { settings.Image = override },
		SkillOverride:      func(settings AgentSkillSettings) AgentSkillOverride { return settings.Image },
		SetSkillOverride:   func(settings *AgentSkillSettings, override AgentSkillOverride) { settings.Image = override },
		ContextOverride:    func(settings AgentContextSettings) AgentContextOverride { return settings.Image },
		SetContextOverride: func(settings *AgentContextSettings, override AgentContextOverride) { settings.Image = override },
	},
}

// AgentKindDefinitions returns all registered Agent kinds in stable UI/runtime order.
func AgentKindDefinitions() []AgentKindDefinition {
	out := make([]AgentKindDefinition, len(agentKindRegistry))
	for index, definition := range agentKindRegistry {
		out[index] = cloneAgentKindDefinition(definition)
	}
	return out
}

func LookupAgentKind(kind string) (AgentKindDefinition, bool) {
	for _, definition := range agentKindRegistry {
		if definition.Kind == kind {
			return cloneAgentKindDefinition(definition), true
		}
	}
	return AgentKindDefinition{}, false
}

func cloneAgentKindDefinition(definition AgentKindDefinition) AgentKindDefinition {
	definition.ToolCapabilities = append([]string(nil), definition.ToolCapabilities...)
	return definition
}

// AgentToolCapability describes one configurable model-callable tool family.
type AgentToolCapability struct {
	Source         string
	TitleKey       string
	DescriptionKey string
	Descriptor     AgentToolDescriptorSummary

	toolNames           []string
	toolDescriptors     map[string]agent.ToolDescriptor
	windowsToolNames    []string
	runtimeAvailability bool
	runtimeResultLimits map[string]struct{}
	subAgentUnavailable bool
}

var agentToolCapabilities = []AgentToolCapability{
	withRuntimeResultLimit(capabilityDefinitionWithToolDescriptors(AgentToolFilesystemRead, "agents.tool.filesystemRead.title", "agents.tool.filesystemRead.subtitle", []string{"read", "glob", "grep"},
		descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationGeneric, agent.ToolResultRecoveryRead), agent.ToolSourceRead),
		map[string]agent.ToolDescriptor{
			"glob": descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationGeneric, agent.ToolResultRecoveryRerun), agent.ToolSourceRead),
			"grep": descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationSearch, agent.ToolResultRecoveryRerun), agent.ToolSourceRead),
		})),
	withRuntimeResultLimit(capabilityDefinition(AgentToolWorkspaceWrite, "agents.tool.workspaceWrite.title", "agents.tool.workspaceWrite.subtitle", []string{"write", "edit"}, descriptorWithSource(workspaceWriteDescriptor(agent.ToolRecoveryReconcilable, agent.ToolPresentationFile), agent.ToolSourceWrite))),
	withRuntimeResultLimit(runtimePlatformCapabilityDefinition(AgentToolShell, "agents.tool.shell.title", "agents.tool.shell.subtitle", []string{"bash"}, []string{"pwsh"}, descriptorWithSource(descriptorSummary(
		agent.ToolExecutionWorkspaceExclusive, agent.ToolMutationExternal, agent.ToolPostCheckExternalReceipt,
		agent.ToolRecoveryNonIdempotent, agent.SteeringFinishCurrent, agent.ToolPresentationTerminal,
	), agent.ToolSourceShell))),
	capabilityDefinition(AgentToolWebSearch, "agents.tool.webSearch.title", "agents.tool.webSearch.subtitle", []string{"web_search"}, descriptorWithSource(interruptibleReadDescriptor(agent.ToolPresentationSearch, agent.ToolResultRecoveryRerun, agent.ToolResultDeferred), agent.ToolSourceWeb)),
	capabilityDefinition(AgentToolWebFetch, "agents.tool.webFetch.title", "agents.tool.webFetch.subtitle", []string{"web_fetch"}, descriptorWithSource(interruptibleReadDescriptor(agent.ToolPresentationWeb, agent.ToolResultRecoveryRefetch, agent.ToolResultEagerCandidate), agent.ToolSourceWeb)),
	withRuntimeResultLimit(runtimeCapabilityDefinition(AgentToolBrowser, "agents.tool.browser.title", "agents.tool.browser.subtitle", []string{"browser"}, descriptorWithSource(descriptorSummary(
		agent.ToolExecutionSessionExclusive, agent.ToolMutationExternal, agent.ToolPostCheckExternalReceipt,
		agent.ToolRecoveryNonIdempotent, agent.SteeringFinishCurrent, agent.ToolPresentationBrowser,
	), agent.ToolSourceWeb))),
	runtimeSubAgentUnavailableCapabilityDefinition(AgentToolAsk, "agents.tool.ask.title", "agents.tool.ask.subtitle", []string{"ask"}, descriptorWithMaxResultBytes(descriptorWithRetention(descriptorSummary(
		agent.ToolExecutionInteractiveWait, agent.ToolMutationNone, agent.ToolPostCheckNone,
		agent.ToolRecoveryReadOnly, agent.SteeringInterruptibleWait, agent.ToolPresentationInteraction,
	), agent.ToolResultProtected), 256<<10)),
	subAgentUnavailableCapabilityDefinition(AgentToolTodo, "agents.tool.todo.title", "agents.tool.todo.subtitle", []string{"todo"}, descriptorWithSource(descriptorSummary(
		agent.ToolExecutionSessionExclusive, agent.ToolMutationSession, agent.ToolPostCheckSessionState,
		agent.ToolRecoveryIdempotent, agent.SteeringFinishCurrent, agent.ToolPresentationTodo,
	), agent.ToolSourceWrite)),
	withRuntimeResultLimit(runtimeCapabilityDefinitionWithToolDescriptors(AgentToolSkills, "agents.tool.skills.title", "agents.tool.skills.subtitle", []string{"skill", "read"}, readOnlyDescriptor(agent.ToolPresentationGeneric, agent.ToolResultRecoveryRerun), map[string]agent.ToolDescriptor{
		"read": descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationGeneric, agent.ToolResultRecoveryRead), agent.ToolSourceRead),
	}), "read"),
	withRuntimeResultLimit(runtimeSubAgentUnavailableCapabilityDefinitionWithToolDescriptors(
		AgentToolDelegation, "agents.tool.delegation.title", "agents.tool.delegation.subtitle", []string{"task", "task_wait"},
		descriptorWithRetention(descriptorSummary(
			agent.ToolExecutionChild, agent.ToolMutationNone, agent.ToolPostCheckNone,
			agent.ToolRecoveryReconcilable, agent.SteeringFinishCurrent, agent.ToolPresentationDelegation,
		), agent.ToolResultProtected),
		map[string]agent.ToolDescriptor{
			"task_wait": descriptorWithRetention(descriptorSummary(
				agent.ToolExecutionInteractiveWait, agent.ToolMutationNone, agent.ToolPostCheckNone,
				agent.ToolRecoveryReadOnly, agent.SteeringInterruptibleWait, agent.ToolPresentationDelegation,
			), agent.ToolResultProtected),
		},
	)),
	withRuntimeResultLimit(capabilityDefinition(AgentToolScript, "agents.tool.script.title", "agents.tool.script.subtitle", []string{"script"}, descriptorWithRetention(descriptorSummary(
		agent.ToolExecutionChild, agent.ToolMutationNone, agent.ToolPostCheckNone,
		agent.ToolRecoveryNonIdempotent, agent.SteeringFinishCurrent, agent.ToolPresentationScript,
	), agent.ToolResultProtected))),
	withRuntimeResultLimit(runtimeSubAgentUnavailableCapabilityDefinitionWithToolDescriptors(
		AgentToolTrajectory, "agents.tool.trajectory.title", "agents.tool.trajectory.subtitle",
		[]string{"read"},
		descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationFile, agent.ToolResultRecoveryRead), agent.ToolSourceRead),
		map[string]agent.ToolDescriptor{
			"read": descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationFile, agent.ToolResultRecoveryRead), agent.ToolSourceRead),
		},
	)),
	withRuntimeResultLimit(capabilityDefinition(AgentToolConfigRead, "agents.tool.configRead.title", "agents.tool.configRead.subtitle", []string{"config_read"}, descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationGeneric, agent.ToolResultRecoveryRerun), agent.ToolSourceRead))),
	withRuntimeResultLimit(capabilityDefinition(AgentToolConfigApply, "agents.tool.configApply.title", "agents.tool.configApply.subtitle", []string{"config_apply"}, descriptorWithSource(descriptorSummary(
		agent.ToolExecutionConfigExclusive, agent.ToolMutationConfig, agent.ToolPostCheckConfigRevision,
		agent.ToolRecoveryReconcilable, agent.SteeringFinishCurrent, agent.ToolPresentationGeneric,
	), agent.ToolSourceWrite))),
	withRuntimeResultLimit(runtimeSubAgentUnavailableCapabilityDefinition(AgentToolEventRead, "agents.tool.eventRead.title", "agents.tool.eventRead.subtitle", []string{"read"}, descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationGeneric, agent.ToolResultRecoveryRead), agent.ToolSourceRead))),
	capabilityDefinition(AgentToolLoreRead, "agents.tool.loreRead.title", "agents.tool.loreRead.subtitle", []string{"list_lore_items", "read_lore_items"}, descriptorWithSource(readOnlyDescriptor(agent.ToolPresentationGeneric, agent.ToolResultRecoveryRerun), agent.ToolSource("denova.lore"))),
	capabilityDefinition(AgentToolLoreWrite, "agents.tool.loreWrite.title", "agents.tool.loreWrite.subtitle", []string{"write_lore_items"}, descriptorWithSource(workspaceWriteDescriptor(agent.ToolRecoveryReconcilable, agent.ToolPresentationFile), agent.ToolSource("denova.lore"))),
	capabilityDefinition(AgentToolImageGeneration, "agents.tool.imageGeneration.title", "agents.tool.imageGeneration.subtitle", []string{"generate_image"}, descriptorWithSource(workspaceWriteDescriptor(agent.ToolRecoveryNonIdempotent, agent.ToolPresentationImage), agent.ToolSourceImage)),
}

func AgentToolCapabilities() []AgentToolCapability {
	out := make([]AgentToolCapability, len(agentToolCapabilities))
	for index, capability := range agentToolCapabilities {
		out[index] = cloneAgentToolCapability(capability)
	}
	return out
}

// AgentToolDescriptorSummary is the complete stable execution, recovery, and
// presentation contract for one concrete model-visible tool.
type AgentToolDescriptorSummary struct {
	Source             agent.ToolSource              `json:"source"`
	Execution          agent.ToolExecutionClass      `json:"execution"`
	MutationScope      agent.ToolMutationScope       `json:"mutation_scope"`
	PostCheck          agent.ToolPostCheckPolicy     `json:"post_check"`
	Recovery           agent.ToolRecoveryClass       `json:"recovery"`
	ResultRecoveryKind agent.ToolResultRecoveryKind  `json:"result_recovery_kind,omitempty"`
	ResultProjection   agent.ToolResultProjection    `json:"result_projection"`
	ResultRetention    agent.ToolResultRetentionMode `json:"result_retention"`
	Steering           agent.SteeringPolicy          `json:"steering"`
	MaxResultBytes     int                           `json:"max_result_bytes"`
	CallPresentation   agent.ToolPresentationKind    `json:"call_presentation"`
	ResultPresentation agent.ToolPresentationKind    `json:"result_presentation"`
}

// AgentToolCapabilityCatalogEntry is one platform-resolved capability. Its
// order comes from the registry and its concrete tool names match the target
// GOOS (bash on Unix-like hosts, pwsh on Windows).
type AgentToolCapabilityCatalogEntry struct {
	Capability           string                                `json:"capability"`
	TitleKey             string                                `json:"title_key"`
	DescriptionKey       string                                `json:"description_key"`
	ToolNames            []string                              `json:"tool_names"`
	Descriptor           AgentToolDescriptorSummary            `json:"descriptor"`
	ToolDescriptors      map[string]AgentToolDescriptorSummary `json:"tool_descriptors"`
	AvailableToSubAgents bool                                  `json:"available_to_subagents"`
}

type AgentToolAvailability string

const (
	AgentToolAvailabilityAvailable    AgentToolAvailability = "available"
	AgentToolAvailabilityRuntimeCheck AgentToolAvailability = "runtime_check"
	AgentToolAvailabilityUnavailable  AgentToolAvailability = "unavailable"

	AgentToolUnavailableDisabledByPolicy = "agents.tool.unavailable.disabledByPolicy"
)

type ResolvedAgentToolCapability struct {
	AgentToolCapabilityCatalogEntry
	Allowed              bool                  `json:"allowed"`
	Availability         AgentToolAvailability `json:"availability"`
	UnavailableReasonKey string                `json:"unavailable_reason_key,omitempty"`
}

// AgentToolCapabilityCatalogForGOOS returns the complete capability catalog
// in stable display order. It is metadata only; use an Agent manifest for the
// capabilities that a concrete Agent can actually assemble.
func AgentToolCapabilityCatalogForGOOS(goos string) []AgentToolCapabilityCatalogEntry {
	capabilities := AgentToolCapabilities()
	result := make([]AgentToolCapabilityCatalogEntry, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, catalogEntryForCapability(capability, goos, 0))
	}
	return result
}

// ResolveAgentToolManifest retains the complete catalog view for callers that
// do not select an Agent kind. Product settings should use the per-Agent form.
func ResolveAgentToolManifest(settings ResolvedAgentToolSettings) []ResolvedAgentToolCapability {
	return resolveAgentToolCapabilities(settings, AgentToolCapabilities(), runtime.GOOS, 0)
}

// ResolveAgentToolManifestForGOOS resolves only capabilities supported by the
// selected Agent assembly. Dynamic host dependencies remain runtime_check
// instead of being guessed by the config package.
func ResolveAgentToolManifestForGOOS(settings ResolvedAgentToolSettings, agentKind, goos string, maxResultBytes ...int) []ResolvedAgentToolCapability {
	definition, ok := LookupAgentKind(agentKind)
	if !ok {
		return []ResolvedAgentToolCapability{}
	}
	capabilities := make([]AgentToolCapability, 0, len(definition.ToolCapabilities))
	for _, source := range definition.ToolCapabilities {
		if capability, found := lookupAgentToolCapability(source); found {
			capabilities = append(capabilities, capability)
		}
	}
	return resolveAgentToolCapabilities(settings, capabilities, goos, firstPositiveInt(maxResultBytes...))
}

// ResolveAgentToolManifestsForGOOS builds the settings API projection for all
// registered Agent kinds, including explicit empty manifests for model-only
// Agents.
func ResolveAgentToolManifestsForGOOS(cfg *Config, goos string) map[string][]ResolvedAgentToolCapability {
	result := make(map[string][]ResolvedAgentToolCapability, len(agentKindRegistry))
	for _, definition := range AgentKindDefinitions() {
		settings := ResolveAgentTools(cfg, definition.Kind)
		result[definition.Kind] = ResolveAgentToolManifestForGOOS(settings, definition.Kind, goos, agentToolResultLimitBytes(cfg))
	}
	return result
}

func AgentToolAllowed(settings ResolvedAgentToolSettings, source string) bool {
	for _, capability := range agentToolCapabilities {
		if capability.Source == source {
			return settings[source]
		}
	}
	return false
}

func resolveAgentToolCapabilities(settings ResolvedAgentToolSettings, capabilities []AgentToolCapability, goos string, maxResultBytes int) []ResolvedAgentToolCapability {
	result := make([]ResolvedAgentToolCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		allowed := AgentToolAllowed(settings, capability.Source)
		availability := AgentToolAvailabilityAvailable
		reasonKey := ""
		switch {
		case !allowed:
			availability = AgentToolAvailabilityUnavailable
			reasonKey = AgentToolUnavailableDisabledByPolicy
		case capability.runtimeAvailability:
			availability = AgentToolAvailabilityRuntimeCheck
		}
		result = append(result, ResolvedAgentToolCapability{
			AgentToolCapabilityCatalogEntry: catalogEntryForCapability(capability, goos, maxResultBytes),
			Allowed:                         allowed,
			Availability:                    availability,
			UnavailableReasonKey:            reasonKey,
		})
	}
	return result
}

func lookupAgentToolCapability(source string) (AgentToolCapability, bool) {
	for _, capability := range agentToolCapabilities {
		if capability.Source == source {
			return cloneAgentToolCapability(capability), true
		}
	}
	return AgentToolCapability{}, false
}

func catalogEntryForCapability(capability AgentToolCapability, goos string, maxResultBytes int) AgentToolCapabilityCatalogEntry {
	names := capability.toolNames
	if normalizedGOOS(goos) == "windows" && len(capability.windowsToolNames) != 0 {
		names = capability.windowsToolNames
	}
	descriptor := capability.Descriptor
	toolDescriptors := summarizeToolDescriptors(capability.toolDescriptors, names)
	if maxResultBytes > 0 {
		for name, summary := range toolDescriptors {
			if _, dynamic := capability.runtimeResultLimits[name]; !dynamic {
				continue
			}
			summary.MaxResultBytes = maxResultBytes
			toolDescriptors[name] = summary
		}
		if len(capability.runtimeResultLimits) == len(capability.toolDescriptors) {
			descriptor.MaxResultBytes = maxResultBytes
		}
	}
	return AgentToolCapabilityCatalogEntry{
		Capability:           capability.Source,
		TitleKey:             capability.TitleKey,
		DescriptionKey:       capability.DescriptionKey,
		ToolNames:            append([]string{}, names...),
		Descriptor:           descriptor,
		ToolDescriptors:      toolDescriptors,
		AvailableToSubAgents: !capability.subAgentUnavailable,
	}
}

func agentToolResultLimitBytes(cfg *Config) int {
	if cfg == nil || cfg.AgentToolResultLimitKB <= 0 {
		return DefaultAgentToolResultLimitKB * 1024
	}
	return cfg.AgentToolResultLimitKB * 1024
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func normalizedGOOS(goos string) string {
	if goos == "" {
		return runtime.GOOS
	}
	return goos
}

func cloneAgentToolCapability(capability AgentToolCapability) AgentToolCapability {
	capability.toolNames = append([]string(nil), capability.toolNames...)
	capability.toolDescriptors = cloneToolDescriptors(capability.toolDescriptors)
	capability.windowsToolNames = append([]string(nil), capability.windowsToolNames...)
	capability.runtimeResultLimits = cloneStringSet(capability.runtimeResultLimits)
	return capability
}

func capabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor agent.ToolDescriptor) AgentToolCapability {
	toolDescriptors := make(map[string]agent.ToolDescriptor, len(toolNames))
	for _, name := range toolNames {
		toolDescriptors[name] = descriptor
	}
	return AgentToolCapability{
		Source: source, TitleKey: titleKey, DescriptionKey: descriptionKey,
		toolNames: append([]string(nil), toolNames...), Descriptor: mustSummarizeAgentToolDescriptor(descriptor),
		toolDescriptors: toolDescriptors,
	}
}

func capabilityDefinitionWithToolDescriptors(source, titleKey, descriptionKey string, toolNames []string, descriptor agent.ToolDescriptor, overrides map[string]agent.ToolDescriptor) AgentToolCapability {
	definition := capabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	for name, override := range overrides {
		definition.toolDescriptors[name] = override
	}
	return definition
}

func platformCapabilityDefinition(source, titleKey, descriptionKey string, toolNames, windowsToolNames []string, descriptor agent.ToolDescriptor) AgentToolCapability {
	definition := capabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.windowsToolNames = append([]string(nil), windowsToolNames...)
	for _, name := range windowsToolNames {
		definition.toolDescriptors[name] = descriptor
	}
	return definition
}

func runtimePlatformCapabilityDefinition(source, titleKey, descriptionKey string, toolNames, windowsToolNames []string, descriptor agent.ToolDescriptor) AgentToolCapability {
	definition := platformCapabilityDefinition(source, titleKey, descriptionKey, toolNames, windowsToolNames, descriptor)
	definition.runtimeAvailability = true
	return definition
}

func runtimeCapabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor agent.ToolDescriptor) AgentToolCapability {
	definition := capabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.runtimeAvailability = true
	return definition
}

func runtimeCapabilityDefinitionWithToolDescriptors(source, titleKey, descriptionKey string, toolNames []string, descriptor agent.ToolDescriptor, overrides map[string]agent.ToolDescriptor) AgentToolCapability {
	definition := capabilityDefinitionWithToolDescriptors(source, titleKey, descriptionKey, toolNames, descriptor, overrides)
	definition.runtimeAvailability = true
	return definition
}

func subAgentUnavailableCapabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor agent.ToolDescriptor) AgentToolCapability {
	definition := capabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.subAgentUnavailable = true
	return definition
}

func runtimeSubAgentUnavailableCapabilityDefinition(source, titleKey, descriptionKey string, toolNames []string, descriptor agent.ToolDescriptor) AgentToolCapability {
	definition := runtimeCapabilityDefinition(source, titleKey, descriptionKey, toolNames, descriptor)
	definition.subAgentUnavailable = true
	return definition
}

func runtimeSubAgentUnavailableCapabilityDefinitionWithToolDescriptors(
	source, titleKey, descriptionKey string,
	toolNames []string,
	descriptor agent.ToolDescriptor,
	overrides map[string]agent.ToolDescriptor,
) AgentToolCapability {
	definition := runtimeCapabilityDefinitionWithToolDescriptors(source, titleKey, descriptionKey, toolNames, descriptor, overrides)
	definition.subAgentUnavailable = true
	return definition
}

func withRuntimeResultLimit(definition AgentToolCapability, toolNames ...string) AgentToolCapability {
	if len(toolNames) == 0 {
		toolNames = append(append([]string(nil), definition.toolNames...), definition.windowsToolNames...)
	}
	definition.runtimeResultLimits = make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		definition.runtimeResultLimits[name] = struct{}{}
	}
	return definition
}

func descriptorSummary(execution agent.ToolExecutionClass, mutation agent.ToolMutationScope, postCheck agent.ToolPostCheckPolicy, recovery agent.ToolRecoveryClass, steering agent.SteeringPolicy, presentation agent.ToolPresentationKind) agent.ToolDescriptor {
	retention := agent.ToolResultDeferred
	if mutation != agent.ToolMutationNone || recovery == agent.ToolRecoveryNonIdempotent {
		retention = agent.ToolResultProtected
	}
	return agent.ToolDescriptor{
		Source:    agent.ToolSourceOther,
		Execution: execution, MutationScope: mutation, PostCheck: postCheck,
		Recovery: recovery, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: retention, Steering: steering, MaxResultBytes: 128 << 10,
		Presentation: agent.UniformToolPresentation(presentation),
	}
}

func descriptorWithRetention(descriptor agent.ToolDescriptor, retention agent.ToolResultRetentionMode) agent.ToolDescriptor {
	descriptor.ResultRetention = retention
	return descriptor
}

func descriptorWithSource(descriptor agent.ToolDescriptor, source agent.ToolSource) agent.ToolDescriptor {
	descriptor.Source = source
	return descriptor
}

func descriptorWithMaxResultBytes(descriptor agent.ToolDescriptor, maxResultBytes int) agent.ToolDescriptor {
	descriptor.MaxResultBytes = maxResultBytes
	return descriptor
}

func readOnlyDescriptor(presentation agent.ToolPresentationKind, recoveryKind agent.ToolResultRecoveryKind) agent.ToolDescriptor {
	descriptor := descriptorSummary(
		agent.ToolExecutionParallelRead, agent.ToolMutationNone, agent.ToolPostCheckNone,
		agent.ToolRecoveryReadOnly, agent.SteeringFinishCurrent, presentation,
	)
	descriptor.ResultRecoveryKind = recoveryKind
	return descriptor
}

func interruptibleReadDescriptor(presentation agent.ToolPresentationKind, recoveryKind agent.ToolResultRecoveryKind, retention agent.ToolResultRetentionMode) agent.ToolDescriptor {
	descriptor := readOnlyDescriptor(presentation, recoveryKind)
	descriptor.Steering = agent.SteeringInterruptibleWait
	descriptor.ResultRetention = retention
	return descriptor
}

func workspaceWriteDescriptor(recovery agent.ToolRecoveryClass, presentation agent.ToolPresentationKind) agent.ToolDescriptor {
	return descriptorSummary(
		agent.ToolExecutionWorkspaceExclusive, agent.ToolMutationWorkspace, agent.ToolPostCheckWorkspaceChange,
		recovery, agent.SteeringFinishCurrent, presentation,
	)
}

// SummarizeAgentToolDescriptor derives the settings/catalog projection from
// the same runtime descriptor shape used by Agent execution. Display-only
// presentation is normalized here but remains excluded from model identity.
func SummarizeAgentToolDescriptor(descriptor agent.ToolDescriptor) (AgentToolDescriptorSummary, error) {
	if err := descriptor.Validate(); err != nil {
		return AgentToolDescriptorSummary{}, err
	}
	presentation, err := descriptor.Presentation.Normalize()
	if err != nil {
		return AgentToolDescriptorSummary{}, err
	}
	return AgentToolDescriptorSummary{
		Source:             descriptor.Source,
		Execution:          descriptor.Execution,
		MutationScope:      descriptor.MutationScope,
		PostCheck:          descriptor.PostCheck,
		Recovery:           descriptor.Recovery,
		ResultRecoveryKind: descriptor.ResultRecoveryKind,
		ResultProjection:   descriptor.ResultProjection,
		ResultRetention:    descriptor.ResultRetention,
		Steering:           descriptor.Steering,
		MaxResultBytes:     descriptor.MaxResultBytes,
		CallPresentation:   presentation.Call,
		ResultPresentation: presentation.Result,
	}, nil
}

func summarizeToolDescriptors(descriptors map[string]agent.ToolDescriptor, names []string) map[string]AgentToolDescriptorSummary {
	result := make(map[string]AgentToolDescriptorSummary, len(names))
	for _, name := range names {
		result[name] = mustSummarizeAgentToolDescriptor(descriptors[name])
	}
	return result
}

func cloneStringSet(values map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for value := range values {
		result[value] = struct{}{}
	}
	return result
}

func cloneToolDescriptors(descriptors map[string]agent.ToolDescriptor) map[string]agent.ToolDescriptor {
	result := make(map[string]agent.ToolDescriptor, len(descriptors))
	for name, descriptor := range descriptors {
		result[name] = descriptor
	}
	return result
}

func mustSummarizeAgentToolDescriptor(descriptor agent.ToolDescriptor) AgentToolDescriptorSummary {
	summary, err := SummarizeAgentToolDescriptor(descriptor)
	if err != nil {
		panic(err)
	}
	return summary
}
