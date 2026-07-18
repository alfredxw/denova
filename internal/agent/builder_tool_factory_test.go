package agent

import (
	"testing"

	"denova/config"
)

func TestLoreToolsFactoryOmitsDisabledLoreSchemas(t *testing.T) {
	factory := loreToolsFactory(&config.Config{Workspace: t.TempDir()}, false)

	tools, err := factory(config.ResolvedAgentToolSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("disabled lore capabilities should expose no lore tool schemas, got %d", len(tools))
	}
}

func TestLoreToolsFactoryHonorsResolvedWriteCapability(t *testing.T) {
	factory := loreToolsFactory(&config.Config{Workspace: t.TempDir()}, false)

	readOnlyTools, err := factory(config.ResolvedAgentToolSettings{LoreRead: true})
	if err != nil {
		t.Fatal(err)
	}
	readOnlyNames := configManagerToolNameSet(t, readOnlyTools)
	if readOnlyNames["write_lore_items"] {
		t.Fatalf("read-only lore capability should not expose write schemas: %v", readOnlyNames)
	}

	writableTools, err := factory(config.ResolvedAgentToolSettings{LoreRead: true, LoreWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	writableNames := configManagerToolNameSet(t, writableTools)
	if !writableNames["write_lore_items"] {
		t.Fatalf("lore write capability should expose write schemas: %v", writableNames)
	}
}
