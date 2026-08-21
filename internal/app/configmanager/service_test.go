package configmanager

import (
	"testing"

	"denova/config"
)

func TestConfigureScopedToolsLimitsImageGenerationToLoreOrigin(t *testing.T) {
	nonLore := config.Config{}
	configureScopedTools(&nonLore, "settings")
	if nonLore.ConfigManagerOrigin != "settings" || nonLore.AgentTools.ConfigManager != nil {
		t.Fatalf("non-lore scoped tools = %#v", nonLore)
	}

	lore := config.Config{}
	configureScopedTools(&lore, " lore ")
	if lore.ConfigManagerOrigin != "lore" ||
		!lore.AgentTools.ConfigManager[config.AgentToolLoreRead] ||
		!lore.AgentTools.ConfigManager[config.AgentToolImageGeneration] {
		t.Fatalf("lore scoped tools = %#v", lore.AgentTools.ConfigManager)
	}
}
