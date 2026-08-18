package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentToolDescriptorSummaryMatchesFrontendContract(t *testing.T) {
	frontendTypes, err := os.ReadFile(filepath.Join("..", "web", "src", "features", "settings", "types.ts"))
	if err != nil {
		t.Fatal(err)
	}
	interfaceMatch := regexp.MustCompile(`(?s)export interface AgentToolDescriptorSummary\s*\{([^}]*)\}`).FindSubmatch(frontendTypes)
	if len(interfaceMatch) != 2 {
		t.Fatal("frontend AgentToolDescriptorSummary interface is missing")
	}
	fieldMatches := regexp.MustCompile(`(?m)^\s*([a-z][a-z0-9_]*)\??:\s*(?:string|number|ToolPresentationKind)\s*$`).FindAllSubmatch(interfaceMatch[1], -1)
	frontendFields := make([]string, 0, len(fieldMatches))
	for _, match := range fieldMatches {
		frontendFields = append(frontendFields, string(match[1]))
	}

	backendType := reflect.TypeOf(AgentToolDescriptorSummary{})
	backendFields := make([]string, 0, backendType.NumField())
	for index := range backendType.NumField() {
		name := strings.Split(backendType.Field(index).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			t.Fatalf("backend descriptor field %s has no JSON contract name", backendType.Field(index).Name)
		}
		backendFields = append(backendFields, name)
	}
	slices.Sort(frontendFields)
	slices.Sort(backendFields)
	if !slices.Equal(frontendFields, backendFields) {
		t.Fatalf("frontend descriptor fields = %v, backend JSON fields = %v", frontendFields, backendFields)
	}
}

func TestGoalCapabilityUsesStandardInjectedToolContract(t *testing.T) {
	manifest := ResolveAgentToolManifestForGOOS(
		ResolvedAgentToolSettings{AgentToolGoal: true}, AgentKindIDE, "linux",
	)
	var goal *ResolvedAgentToolCapability
	for index := range manifest {
		if manifest[index].Capability == AgentToolGoal {
			goal = &manifest[index]
			break
		}
	}
	if goal == nil {
		t.Fatal("IDE manifest has no Goal capability")
	}
	want, err := SummarizeAgentToolDescriptor(agent.StandardGoalToolDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(goal.ToolNames, []string{agent.StandardGoalToolName}) {
		t.Fatalf("Goal tool names = %v", goal.ToolNames)
	}
	if descriptor := agent.StandardGoalToolDescriptor(); descriptor.Capability != AgentToolGoal {
		t.Fatalf("Goal descriptor capability = %q, want %q", descriptor.Capability, AgentToolGoal)
	}
	if got := goal.ToolDescriptors[agent.StandardGoalToolName]; got != want {
		t.Fatalf("Goal descriptor = %+v, want %+v", got, want)
	}
}

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
	if seen["context_compaction"] {
		t.Fatal("context_compaction is an internal event/protocol type, not an Agent kind")
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
	}
	tools := AgentToolSettings{
		General:             AgentToolOverride{AgentToolFilesystemRead: true},
		IDE:                 AgentToolOverride{AgentToolFilesystemRead: true},
		InteractiveStory:    AgentToolOverride{AgentToolWorkspaceWrite: true},
		Image:               AgentToolOverride{AgentToolImageGeneration: true},
		ConfigManager:       AgentToolOverride{AgentToolShell: true},
		InteractiveDirector: AgentToolOverride{AgentToolLoreWrite: true},
		VersionSummary:      AgentToolOverride{AgentToolTodo: true},
		ToolAgent:           AgentToolOverride{AgentToolWebSearch: true},
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
	}

	for _, definition := range definitions {
		expectedKind := definition.Kind
		if definition.Kind == AgentKindHarnessOptimizer {
			expectedKind = AgentKindGeneral
		}
		if got := definition.ModelOverride(models).ProfileID; got != expectedKind {
			t.Fatalf("model accessor for %s returned %q", definition.Kind, got)
		}
		if got := definition.PromptOverride(prompts).SystemPrompt; got != expectedKind {
			t.Fatalf("prompt accessor for %s returned %q", definition.Kind, got)
		}
		if got := definition.ToolOverride(tools); len(got) == 0 {
			t.Fatalf("tool accessor for %s returned zero override", definition.Kind)
		}
		if got := definition.ContextOverride(contexts).CompactionThreshold; got == nil || *got != *thresholds[expectedKind] {
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
				AgentToolFilesystemRead, AgentToolWorkspaceWrite, AgentToolShell,
				AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
				AgentToolAsk, AgentToolTodo, AgentToolGoal, AgentToolSkills, AgentToolDelegation,
				AgentToolScript, AgentToolHarnessState,
			},
		},
		{
			kind: AgentKindInteractiveStory,
			want: []string{
				AgentToolFilesystemRead, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser,
				AgentToolSkills, AgentToolDelegation, AgentToolLoreRead,
				AgentToolScript, AgentToolHarnessState,
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
		AgentToolFilesystemRead: true,
		AgentToolLoreRead:       true,
		AgentToolWebSearch:      true,
		AgentToolConfigRead:     true,
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
	for _, source := range []string{AgentToolFilesystemRead, AgentToolLoreRead, AgentToolWebSearch, AgentToolConfigRead} {
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
		if entry.Descriptor.Source == "" || entry.Descriptor.Execution == "" || entry.Descriptor.MutationScope == "" ||
			entry.Descriptor.PostCheck == "" || entry.Descriptor.Recovery == "" ||
			entry.Descriptor.ResultProjection == "" || entry.Descriptor.ResultRetention == "" ||
			entry.Descriptor.Steering == "" || entry.Descriptor.MaxResultBytes <= 0 || entry.Descriptor.CallPresentation == "" ||
			entry.Descriptor.ResultPresentation == "" {
			t.Fatalf("catalog capability %q has incomplete descriptor: %#v", entry.Capability, entry.Descriptor)
		}
		if len(entry.ToolDescriptors) != len(entry.ToolNames) {
			t.Fatalf("catalog capability %q descriptor count = %d, names = %v", entry.Capability, len(entry.ToolDescriptors), entry.ToolNames)
		}
		for _, name := range entry.ToolNames {
			if _, present := entry.ToolDescriptors[name]; !present {
				t.Fatalf("catalog capability %q has no descriptor for %q", entry.Capability, name)
			}
		}
	}
	if got := catalogToolNames(linux, AgentToolShell); len(got) != 1 || got[0] != "bash" {
		t.Fatalf("linux shell names = %#v, want [bash]", got)
	}
	if got := catalogToolNames(windows, AgentToolShell); len(got) != 1 || got[0] != "pwsh" {
		t.Fatalf("windows shell names = %#v, want [pwsh]", got)
	}
}

func TestResolvedManifestProjectsRuntimeResultLimitPerConcreteTool(t *testing.T) {
	const runtimeLimit = 64 << 10
	manifest := ResolveAgentToolManifestForGOOS(ResolvedAgentToolSettings{
		AgentToolFilesystemRead: true,
		AgentToolSkills:         true,
		AgentToolWebSearch:      true,
	}, AgentKindIDE, "linux", runtimeLimit)
	entries := make(map[string]ResolvedAgentToolCapability, len(manifest))
	for _, entry := range manifest {
		entries[entry.Capability] = entry
	}
	for _, name := range []string{"read", "glob", "grep"} {
		if got := entries[AgentToolFilesystemRead].ToolDescriptors[name].MaxResultBytes; got != runtimeLimit {
			t.Fatalf("workspace %s result limit = %d, want %d", name, got, runtimeLimit)
		}
	}
	if got := entries[AgentToolFilesystemRead].ToolDescriptors["grep"].CallPresentation; got != agent.ToolPresentationSearch {
		t.Fatalf("workspace grep presentation = %q, want search", got)
	}
	if got := entries[AgentToolSkills].ToolDescriptors["read"].MaxResultBytes; got != runtimeLimit {
		t.Fatalf("Skill reference read result limit = %d, want %d", got, runtimeLimit)
	}
	if got := entries[AgentToolSkills].ToolDescriptors["skill"].MaxResultBytes; got != DefaultAgentToolResultLimitKB*1024 {
		t.Fatalf("Skill loader result limit = %d, want stable default", got)
	}
	if got := entries[AgentToolWebSearch].ToolDescriptors["web_search"].MaxResultBytes; got != DefaultAgentToolResultLimitKB*1024 {
		t.Fatalf("web_search result limit = %d, want stable default", got)
	}
}

func TestResolveAgentToolManifestIsAgentSpecificAndHonestAboutAvailability(t *testing.T) {
	settings := ResolvedAgentToolSettings{
		AgentToolFilesystemRead: true,
		AgentToolShell:          true,
		AgentToolBrowser:        true,
		AgentToolSkills:         true,
		AgentToolWebSearch:      false,
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
	for _, kind := range []string{AgentKindVersionSummary, AgentKindToolAgent} {
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
	if filesystemRead, found := resolvedManifestCapability(manifests[AgentKindInteractiveDirector], AgentToolFilesystemRead); found {
		t.Fatalf("interactive Director ceiling exposed filesystem_read: %#v", filesystemRead)
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
