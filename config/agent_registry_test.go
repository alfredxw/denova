package config

import "testing"

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
	}

	models := AgentModelSettings{
		IDE:                   AgentModelOverride{ProfileID: AgentKindIDE},
		InteractiveStory:      AgentModelOverride{ProfileID: AgentKindInteractiveStory},
		ConfigManager:         AgentModelOverride{ProfileID: AgentKindConfigManager},
		InteractiveState:      AgentModelOverride{ProfileID: AgentKindInteractiveState},
		InteractiveHotChoices: AgentModelOverride{ProfileID: AgentKindInteractiveHotChoices},
		VersionSummary:        AgentModelOverride{ProfileID: AgentKindVersionSummary},
		ToolAgent:             AgentModelOverride{ProfileID: AgentKindToolAgent},
		Automation:            AgentModelOverride{ProfileID: AgentKindAutomation},
		ContextCompaction:     AgentModelOverride{ProfileID: AgentKindContextCompaction},
	}
	prompts := AgentPromptSettings{
		IDE:                   AgentPromptOverride{SystemPrompt: AgentKindIDE},
		InteractiveStory:      AgentPromptOverride{SystemPrompt: AgentKindInteractiveStory},
		ConfigManager:         AgentPromptOverride{SystemPrompt: AgentKindConfigManager},
		InteractiveState:      AgentPromptOverride{SystemPrompt: AgentKindInteractiveState},
		InteractiveHotChoices: AgentPromptOverride{SystemPrompt: AgentKindInteractiveHotChoices},
		VersionSummary:        AgentPromptOverride{SystemPrompt: AgentKindVersionSummary},
		ToolAgent:             AgentPromptOverride{SystemPrompt: AgentKindToolAgent},
		Automation:            AgentPromptOverride{SystemPrompt: AgentKindAutomation},
		ContextCompaction:     AgentPromptOverride{SystemPrompt: AgentKindContextCompaction},
	}
	on := true
	tools := AgentToolSettings{
		IDE:                   AgentToolOverride{FileRead: &on},
		InteractiveStory:      AgentToolOverride{FileWrite: &on},
		ConfigManager:         AgentToolOverride{ShellExecute: &on},
		InteractiveState:      AgentToolOverride{LoreRead: &on},
		InteractiveHotChoices: AgentToolOverride{LoreWrite: &on},
		VersionSummary:        AgentToolOverride{Todo: &on},
		ToolAgent:             AgentToolOverride{WebSearch: &on},
		Automation:            AgentToolOverride{FileRead: &on, WebSearch: &on},
		ContextCompaction:     AgentToolOverride{Skills: &on},
	}
	thresholds := map[string]*float64{}
	for _, definition := range definitions {
		value := 0.50 + float64(len(thresholds))*0.01
		thresholds[definition.Kind] = &value
	}
	contexts := AgentContextSettings{
		IDE:                   AgentContextOverride{CompactionThreshold: thresholds[AgentKindIDE]},
		InteractiveStory:      AgentContextOverride{CompactionThreshold: thresholds[AgentKindInteractiveStory]},
		ConfigManager:         AgentContextOverride{CompactionThreshold: thresholds[AgentKindConfigManager]},
		InteractiveState:      AgentContextOverride{CompactionThreshold: thresholds[AgentKindInteractiveState]},
		InteractiveHotChoices: AgentContextOverride{CompactionThreshold: thresholds[AgentKindInteractiveHotChoices]},
		VersionSummary:        AgentContextOverride{CompactionThreshold: thresholds[AgentKindVersionSummary]},
		ToolAgent:             AgentContextOverride{CompactionThreshold: thresholds[AgentKindToolAgent]},
		Automation:            AgentContextOverride{CompactionThreshold: thresholds[AgentKindAutomation]},
		ContextCompaction:     AgentContextOverride{CompactionThreshold: thresholds[AgentKindContextCompaction]},
	}

	for _, definition := range definitions {
		if got := definition.ModelOverride(models).ProfileID; got != definition.Kind {
			t.Fatalf("model accessor for %s returned %q", definition.Kind, got)
		}
		if got := definition.PromptOverride(prompts).SystemPrompt; got != definition.Kind {
			t.Fatalf("prompt accessor for %s returned %q", definition.Kind, got)
		}
		if got := definition.ToolOverride(tools); got == (AgentToolOverride{}) {
			t.Fatalf("tool accessor for %s returned zero override", definition.Kind)
		}
		if got := definition.ContextOverride(contexts).CompactionThreshold; got == nil || *got != *thresholds[definition.Kind] {
			t.Fatalf("context accessor for %s returned %#v", definition.Kind, got)
		}
	}
}

func TestResolveAgentToolManifestUsesCapabilityRegistryOrder(t *testing.T) {
	settings := ResolvedAgentToolSettings{
		FileRead:  true,
		LoreRead:  true,
		WebSearch: true,
	}
	manifest := ResolveAgentToolManifest(settings)
	capabilities := AgentToolCapabilities()
	if len(manifest) != len(capabilities) {
		t.Fatalf("manifest length = %d, want %d", len(manifest), len(capabilities))
	}
	for i, capability := range capabilities {
		if manifest[i].Source != capability.Source {
			t.Fatalf("manifest[%d].source = %q, want %q", i, manifest[i].Source, capability.Source)
		}
	}
	if !manifest[0].Allowed || !manifest[4].Allowed || !manifest[7].Allowed {
		t.Fatalf("expected file_read/lore_read/web_search to be allowed: %#v", manifest)
	}
	if manifest[1].Allowed || manifest[2].Allowed || manifest[3].Allowed || manifest[5].Allowed || manifest[6].Allowed {
		t.Fatalf("unexpected allowed capability: %#v", manifest)
	}
}
