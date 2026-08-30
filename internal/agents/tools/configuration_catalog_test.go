package tools

import (
	"context"
	"testing"

	"denova/config"
)

func TestProjectAgentFactoriesExposeConfigurationTools(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	settings := config.ResolvedAgentToolSettings{
		config.AgentToolConfigRead:  true,
		config.AgentToolConfigApply: true,
	}
	catalog := NewCatalog(cfg, nil, RuntimeExecutables{})

	for name, factory := range map[string]Factory{
		"general": catalog.Configuration(),
		"ide":     catalog.IDE(),
	} {
		t.Run(name, func(t *testing.T) {
			definitions, err := factory(settings)
			if err != nil {
				t.Fatal(err)
			}
			names := make(map[string]bool, len(definitions))
			for _, definition := range definitions {
				info, infoErr := definition.Tool.Info(context.Background())
				if infoErr != nil {
					t.Fatal(infoErr)
				}
				names[info.Name] = true
			}
			if len(names) != 2 || !names["config_read"] || !names["config_apply"] {
				t.Fatalf("Project Agent configuration tools = %v, want config_read and config_apply", names)
			}
		})
	}
}
