package config

import (
	"slices"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentKindRegistryDefinesUniqueKindsAndConfigAccessors(t *testing.T) {
	definitions := AgentKindDefinitions()
	if len(definitions) == 0 {
		t.Fatal("agent registry should not be empty")
	}
	seen := map[string]bool{}
	for _, definition := range definitions {
		if definition.Kind == "" {
			t.Fatal("agent registry contains empty kind")
		}
		if seen[definition.Kind] {
			t.Fatalf("duplicate agent kind registered: %s", definition.Kind)
		}
		seen[definition.Kind] = true
		if definition.ModelOverride == nil || definition.ToolOverride == nil || definition.PromptOverride == nil || definition.ContextOverride == nil {
			t.Fatalf("agent %s should declare model/tool/prompt/context accessors", definition.Kind)
		}
		seenCapabilities := make(map[string]bool, len(definition.ToolCapabilities))
		for _, capability := range definition.ToolCapabilities {
			if _, found := lookupAgentToolCapability(capability); !found {
				t.Fatalf("agent %s ceiling contains unknown capability %q", definition.Kind, capability)
			}
			if seenCapabilities[capability] {
				t.Fatalf("agent %s ceiling contains duplicate capability %q", definition.Kind, capability)
			}
			seenCapabilities[capability] = true
		}
	}

	models := AgentModelSettings{
		General:             AgentModelOverride{ProfileID: AgentKindGeneral},
		IDE:                 AgentModelOverride{ProfileID: AgentKindIDE},
		InteractiveStory:    AgentModelOverride{ProfileID: AgentKindInteractiveStory},
		Image:               AgentModelOverride{ProfileID: AgentKindImage},
		ConfigManager:       AgentModelOverride{ProfileID: AgentKindConfigManager},
		InteractiveDirector: AgentModelOverride{ProfileID: AgentKindInteractiveDirector},
		VersionSummary:      AgentModelOverride{ProfileID: AgentKindVersionSummary},
		ToolAgent:           AgentModelOverride{ProfileID: AgentKindToolAgent},
		Automation:          AgentModelOverride{ProfileID: AgentKindAutomation},
		ContextCompaction:   AgentModelOverride{ProfileID: AgentKindContextCompaction},
	}
	prompts := AgentPromptSettings{
		General:             AgentPromptOverride{SystemPrompt: AgentKindGeneral},
		IDE:                 AgentPromptOverride{SystemPrompt: AgentKindIDE},
		InteractiveStory:    AgentPromptOverride{SystemPrompt: AgentKindInteractiveStory},
		Image:               AgentPromptOverride{SystemPrompt: AgentKindImage},
		ConfigManager:       AgentPromptOverride{SystemPrompt: AgentKindConfigManager},
		InteractiveDirector: AgentPromptOverride{SystemPrompt: AgentKindInteractiveDirector},
		VersionSummary:      AgentPromptOverride{SystemPrompt: AgentKindVersionSummary},
		ToolAgent:           AgentPromptOverride{SystemPrompt: AgentKindToolAgent},
		Automation:          AgentPromptOverride{SystemPrompt: AgentKindAutomation},
		ContextCompaction:   AgentPromptOverride{SystemPrompt: AgentKindContextCompaction},
	}
	tools := AgentToolSettings{
		General:             AgentToolOverride{AgentToolWorkspaceRead: true},
		IDE:                 AgentToolOverride{AgentToolWorkspaceRead: true},
		InteractiveStory:    AgentToolOverride{AgentToolWorkspaceWrite: true},
		Image:               AgentToolOverride{AgentToolImageGeneration: true},
		ConfigManager:       AgentToolOverride{AgentToolShell: true},
		InteractiveDirector: AgentToolOverride{AgentToolLoreWrite: true},
		VersionSummary:      AgentToolOverride{AgentToolTodo: true},
		ToolAgent:           AgentToolOverride{AgentToolWebSearch: true},
		Automation:          AgentToolOverride{AgentToolWorkspaceRead: true, AgentToolWebSearch: true},
		ContextCompaction:   AgentToolOverride{AgentToolSkills: true},
	}
	thresholds := map[string]*float64{}
	for _, definition := range definitions {
		value := 0.50 + float64(len(thresholds))*0.01
		thresholds[definition.Kind] = &value
	}
	contexts := AgentContextSettings{
		General:             AgentContextOverride{CompactionThreshold: thresholds[AgentKindGeneral]},
		IDE:                 AgentContextOverride{CompactionThreshold: thresholds[AgentKindIDE]},
		InteractiveStory:    AgentContextOverride{CompactionThreshold: thresholds[AgentKindInteractiveStory]},
		Image:               AgentContextOverride{CompactionThreshold: thresholds[AgentKindImage]},
		ConfigManager:       AgentContextOverride{CompactionThreshold: thresholds[AgentKindConfigManager]},
		InteractiveDirector: AgentContextOverride{CompactionThreshold: thresholds[AgentKindInteractiveDirector]},
		VersionSummary:      AgentContextOverride{CompactionThreshold: thresholds[AgentKindVersionSummary]},
		ToolAgent:           AgentContextOverride{CompactionThreshold: thresholds[AgentKindToolAgent]},
		Automation:          AgentContextOverride{CompactionThreshold: thresholds[AgentKindAutomation]},
		ContextCompaction:   AgentContextOverride{CompactionThreshold: thresholds[AgentKindContextCompaction]},
	}

	for _, definition := range definitions {
		if got := definition.ModelOverride(models).ProfileID; got != definition.Kind {
			t.Fatalf("model accessor for %s returned %q", definition.Kind, got)
		}
		if got := definition.PromptOverride(prompts).SystemPrompt; got != definition.Kind {
			t.Fatalf("prompt accessor for %s returned %q", definition.Kind, got)
		}
		if got := definition.ToolOverride(tools); len(got) == 0 {
			t.Fatalf("tool accessor for %s returned zero override", definition.Kind)
		}
		if got := definition.ContextOverride(contexts).CompactionThreshold; got == nil || *got != *thresholds[definition.Kind] {
			t.Fatalf("context accessor for %s returned %#v", definition.Kind, got)
		}
	}
}

func TestRestrictedAgentKindCapabilityCeilings(t *testing.T) {
	tests := []struct {
		kind string
		want []string
	}{
		{
			kind: AgentKindGeneral,
			want: []string{
				AgentToolWorkspaceRead, AgentToolWorkspaceWrite, AgentToolShell,
				AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
				AgentToolAsk, AgentToolTodo, AgentToolSkills, AgentToolDelegation,
				AgentToolContextRewind,
			},
		},
		{
			kind: AgentKindInteractiveStory,
			want: []string{
				AgentToolWorkspaceRead, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
				AgentToolSkills, AgentToolDelegation, AgentToolLoreRead,
			},
		},
		{
			kind: AgentKindInteractiveDirector,
			want: []string{AgentToolEventRead, AgentToolLoreRead},
		},
	}
	for _, test := range tests {
		definition, found := LookupAgentKind(test.kind)
		if !found {
			t.Fatalf("agent %s is not registered", test.kind)
		}
		if !slices.Equal(definition.ToolCapabilities, test.want) {
			t.Fatalf("agent %s capability ceiling = %#v, want %#v", test.kind, definition.ToolCapabilities, test.want)
		}
	}
}

func TestResolveAgentToolManifestUsesCapabilityRegistryOrder(t *testing.T) {
	settings := ResolvedAgentToolSettings{
		AgentToolWorkspaceRead: true,
		AgentToolLoreRead:      true,
		AgentToolWebSearch:     true,
		AgentToolConfigRead:    true,
	}
	manifest := ResolveAgentToolManifest(settings)
	capabilities := AgentToolCapabilities()
	if len(manifest) != len(capabilities) {
		t.Fatalf("manifest length = %d, want %d", len(manifest), len(capabilities))
	}
	for i, capability := range capabilities {
		if manifest[i].Capability != capability.Source {
			t.Fatalf("manifest[%d].capability = %q, want %q", i, manifest[i].Capability, capability.Source)
		}
	}
	allowed := map[string]bool{}
	for _, item := range manifest {
		allowed[item.Capability] = item.Allowed
	}
	for _, source := range []string{AgentToolWorkspaceRead, AgentToolLoreRead, AgentToolWebSearch, AgentToolConfigRead} {
		if !allowed[source] {
			t.Fatalf("expected %s to be allowed: %#v", source, manifest)
		}
	}
	for _, source := range []string{AgentToolWorkspaceWrite, AgentToolShell, AgentToolSkills, AgentToolLoreWrite, AgentToolTodo, AgentToolImageGeneration, AgentToolConfigApply} {
		if allowed[source] {
			t.Fatalf("unexpected allowed capability %s: %#v", source, manifest)
		}
	}
}

func TestAgentToolCapabilityCatalogResolvesPlatformNamesAndDescriptors(t *testing.T) {
	linux := AgentToolCapabilityCatalogForGOOS("linux")
	windows := AgentToolCapabilityCatalogForGOOS("windows")
	if len(linux) != len(agentToolCapabilities) || len(windows) != len(agentToolCapabilities) {
		t.Fatalf("catalog lengths = linux:%d windows:%d want:%d", len(linux), len(windows), len(agentToolCapabilities))
	}
	seen := map[string]bool{}
	for index, entry := range linux {
		if entry.Capability == "" || entry.TitleKey == "" || entry.DescriptionKey == "" {
			t.Fatalf("catalog[%d] has incomplete identity: %#v", index, entry)
		}
		if seen[entry.Capability] {
			t.Fatalf("duplicate catalog capability %q", entry.Capability)
		}
		seen[entry.Capability] = true
		if len(entry.ToolNames) == 0 {
			t.Fatalf("catalog capability %q has no concrete tool names", entry.Capability)
		}
		if entry.Descriptor.Execution == "" || entry.Descriptor.MutationScope == "" ||
			entry.Descriptor.PostCheck == "" || entry.Descriptor.Recovery == "" ||
			entry.Descriptor.ResultProjection == "" || entry.Descriptor.ResultRetention == "" ||
			entry.Descriptor.Steering == "" {
			t.Fatalf("catalog capability %q has incomplete descriptor: %#v", entry.Capability, entry.Descriptor)
		}
	}
	if got := catalogToolNames(linux, AgentToolShell); len(got) != 1 || got[0] != "bash" {
		t.Fatalf("linux shell names = %#v, want [bash]", got)
	}
	if got := catalogToolNames(windows, AgentToolShell); len(got) != 1 || got[0] != "pwsh" {
		t.Fatalf("windows shell names = %#v, want [pwsh]", got)
	}
}

func TestResolveAgentToolManifestIsAgentSpecificAndHonestAboutAvailability(t *testing.T) {
	settings := ResolvedAgentToolSettings{
		AgentToolWorkspaceRead: true,
		AgentToolShell:         true,
		AgentToolBrowser:       true,
		AgentToolSkills:        true,
		AgentToolWebSearch:     false,
	}
	manifest := ResolveAgentToolManifestForGOOS(settings, AgentKindIDE, "windows")
	definition, ok := LookupAgentKind(AgentKindIDE)
	if !ok {
		t.Fatal("IDE Agent definition is missing")
	}
	if len(manifest) != len(definition.ToolCapabilities) {
		t.Fatalf("IDE manifest length = %d, want %d", len(manifest), len(definition.ToolCapabilities))
	}
	for index, capability := range definition.ToolCapabilities {
		if manifest[index].Capability != capability {
			t.Fatalf("IDE manifest[%d] = %q, want %q", index, manifest[index].Capability, capability)
		}
	}
	if _, found := resolvedManifestCapability(manifest, AgentToolConfigRead); found {
		t.Fatal("IDE manifest must not expose config-manager-only tools")
	}
	browser, found := resolvedManifestCapability(manifest, AgentToolBrowser)
	if !found || !browser.Allowed || browser.Availability != AgentToolAvailabilityRuntimeCheck || browser.UnavailableReasonKey != "" {
		t.Fatalf("browser manifest should defer host discovery: %#v", browser)
	}
	webSearch, found := resolvedManifestCapability(manifest, AgentToolWebSearch)
	if !found || webSearch.Allowed || webSearch.Availability != AgentToolAvailabilityUnavailable ||
		webSearch.UnavailableReasonKey != AgentToolUnavailableDisabledByPolicy {
		t.Fatalf("disabled web search manifest = %#v", webSearch)
	}
	shell, found := resolvedManifestCapability(manifest, AgentToolShell)
	if !found || len(shell.ToolNames) != 1 || shell.ToolNames[0] != "pwsh" ||
		!shell.Allowed || shell.Availability != AgentToolAvailabilityRuntimeCheck || shell.UnavailableReasonKey != "" ||
		shell.Descriptor.MutationScope != agent.ToolMutationExternal ||
		shell.Descriptor.PostCheck != agent.ToolPostCheckExternalReceipt {
		t.Fatalf("Windows IDE shell manifest = %#v", shell)
	}
}

func TestResolveAgentToolManifestsDeclareEmptyModelOnlyAgents(t *testing.T) {
	manifests := ResolveAgentToolManifestsForGOOS(&Config{}, "linux")
	for _, kind := range []string{AgentKindVersionSummary, AgentKindToolAgent, AgentKindContextCompaction} {
		manifest, found := manifests[kind]
		if !found || manifest == nil || len(manifest) != 0 {
			t.Fatalf("model-only Agent %q manifest = %#v, found=%v", kind, manifest, found)
		}
	}
	configManager := manifests[AgentKindConfigManager]
	if _, found := resolvedManifestCapability(configManager, AgentToolConfigRead); !found {
		t.Fatal("config manager manifest should expose config_read")
	}
	if todo, found := resolvedManifestCapability(manifests[AgentKindIDE], AgentToolTodo); !found || todo.AvailableToSubAgents {
		t.Fatalf("todo should remain top-level only: %#v", todo)
	}
	directorEventRead, found := resolvedManifestCapability(manifests[AgentKindInteractiveDirector], AgentToolEventRead)
	if !found || !directorEventRead.Allowed || directorEventRead.Availability != AgentToolAvailabilityRuntimeCheck ||
		directorEventRead.AvailableToSubAgents || len(directorEventRead.ToolNames) != 1 || directorEventRead.ToolNames[0] != "read" {
		t.Fatalf("interactive Director event_read manifest = %#v, found=%v", directorEventRead, found)
	}
	if workspaceRead, found := resolvedManifestCapability(manifests[AgentKindInteractiveDirector], AgentToolWorkspaceRead); found {
		t.Fatalf("interactive Director ceiling exposed workspace_read: %#v", workspaceRead)
	}
	for kind, manifest := range manifests {
		if kind == AgentKindInteractiveDirector {
			continue
		}
		if eventRead, exposed := resolvedManifestCapability(manifest, AgentToolEventRead); exposed {
			t.Fatalf("Agent %q exposed Director-only event_read: %#v", kind, eventRead)
		}
	}
}

func catalogToolNames(catalog []AgentToolCapabilityCatalogEntry, capability string) []string {
	for _, entry := range catalog {
		if entry.Capability == capability {
			return entry.ToolNames
		}
	}
	return nil
}

func resolvedManifestCapability(manifest []ResolvedAgentToolCapability, capability string) (ResolvedAgentToolCapability, bool) {
	for _, entry := range manifest {
		if entry.Capability == capability {
			return entry, true
		}
	}
	return ResolvedAgentToolCapability{}, false
}
