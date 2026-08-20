package agents

import (
	"context"
	"slices"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/agents/trajectory"
)

func TestHarnessOptimizerExposesOnlyHarnessReadSurface(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DenovaDir: dataDir, SkillsDir: t.TempDir(), Workspace: t.TempDir(),
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "test-model",
		AgentTools: config.AgentToolSettings{Default: config.AgentToolOverride{
			config.AgentToolFilesystemRead: true,
			config.AgentToolSkills:         true,
			config.AgentToolWorkspaceWrite: false,
			config.AgentToolShell:          false,
			config.AgentToolWebSearch:      false,
			config.AgentToolWebFetch:       false,
			config.AgentToolBrowser:        false,
			config.AgentToolAsk:            false,
			config.AgentToolTodo:           false,
			config.AgentToolDelegation:     false,
			config.AgentToolLoreRead:       false,
			config.AgentToolLoreWrite:      false,
		}},
	}
	trajectoryAdapter, err := trajectory.NewReadAdapter(trajectory.Catalog{
		Sources: func(context.Context) ([]trajectory.Source, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := producttools.NewReadAdapterBinding(config.AgentToolHarnessState, trajectoryAdapter)
	if err != nil {
		t.Fatal(err)
	}
	definition, _, err := BuildHarnessOptimizerDefinitionWithCompositionForHost(context.Background(), cfg, AgentHostCapabilities{
		ReadAdapters: []producttools.ReadAdapterBinding{binding},
	})
	if err != nil {
		t.Fatalf("build Harness Optimizer with trajectory adapter: %v", err)
	}
	prepared, err := definition.Tools.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(prepared))
	for _, tool := range prepared {
		info, infoErr := tool.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		names = append(names, info.Name)
	}
	if !slices.Equal(names, []string{"read"}) {
		t.Fatalf("Harness Optimizer tools = %v, want only the composed read surface", names)
	}
}
