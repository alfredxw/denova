package agentruntime

import (
	"testing"

	"denova/config"
)

func TestConfigurationProjectionKeepsCustomEnginesIndependentAndSharedBudgetEditable(t *testing.T) {
	settings := config.Settings{
		AgentRuntimes: config.AgentRuntimeSettings{IDE: &config.RuntimePreferences{Selected: config.RuntimeCodex}},
		CustomAgents: []config.CustomAgentConfig{
			{ID: "custom-writing", Contract: "writing.primary.v1"},
			{ID: "custom-general", Contract: "project.general.v1", Runtime: &config.RuntimePreferences{Selected: config.RuntimeCodex}},
		},
	}
	projection := ProjectAgentConfiguration(settings)
	for id, selected := range map[string]config.RuntimeID{"ide": config.RuntimeCodex, "general": config.RuntimeNative, "custom-writing": config.RuntimeNative, "custom-general": config.RuntimeCodex} {
		item := projection[id]
		if item.Selected != selected {
			t.Fatalf("%s selected %s, want %s", id, item.Selected, selected)
		}
		seen := map[ConfigurationSectionID]bool{}
		for _, section := range item.Sections {
			if seen[section.ID] {
				t.Fatalf("duplicate section %s", section.ID)
			}
			seen[section.ID] = true
			if section.Owner == "shared" && section.State != "editable" {
				t.Fatalf("shared section became inactive: %+v", section)
			}
			if section.Owner != "shared" && section.Owner != string(selected) && (section.State != "inactive" || section.ReasonKey == "") {
				t.Fatalf("inactive section lost ownership: %+v", section)
			}
		}
		if !seen[SectionInputBudget] || !seen[SectionNativeContext] || !seen[SectionNativeSubagents] {
			t.Fatalf("missing separately owned sections: %+v", item)
		}
	}
	if _, exists := projection[config.AgentKindInteractiveStory]; !exists {
		t.Fatal("game runtime configuration is missing")
	}
}
