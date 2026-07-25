package agents

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestLoreToolsFactoryOmitsDisabledLoreSchemas(t *testing.T) {
	factory := newToolCatalog(&config.Config{Workspace: t.TempDir()}).Lore(false)

	tools, err := factory(config.ResolvedAgentToolSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("disabled lore capabilities should expose no lore tool schemas, got %d", len(tools))
	}
}

func TestLoreToolsFactoryHonorsResolvedWriteCapability(t *testing.T) {
	factory := newToolCatalog(&config.Config{Workspace: t.TempDir()}).Lore(false)

	readOnlyTools, err := factory(config.ResolvedAgentToolSettings{LoreRead: true})
	if err != nil {
		t.Fatal(err)
	}
	readOnlyNames := toolNameSet(t, readOnlyTools)
	if readOnlyNames["write_lore_items"] {
		t.Fatalf("read-only lore capability should not expose write schemas: %v", readOnlyNames)
	}

	writableTools, err := factory(config.ResolvedAgentToolSettings{LoreRead: true, LoreWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	writableNames := toolNameSet(t, writableTools)
	if !writableNames["write_lore_items"] {
		t.Fatalf("lore write capability should expose write schemas: %v", writableNames)
	}
}

func toolNameSet(t *testing.T, concrete []agent.ToolDefinition) map[string]bool {
	t.Helper()
	names := make(map[string]bool, len(concrete))
	for _, item := range concrete {
		info, err := item.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	return names
}
