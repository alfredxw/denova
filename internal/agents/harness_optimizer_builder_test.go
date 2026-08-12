package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/agents/trajectory"
)

func TestHarnessOptimizerBuildsWithSkillsAndTrajectoryReadAdapters(t *testing.T) {
	dataDir := t.TempDir()
	skillsDir := filepath.Join(dataDir, "builtin-skills")
	skillDir := filepath.Join(skillsDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test-skill\ndescription: Test shared read adapter composition.\n---\n\n# Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		DenovaDir: dataDir, SkillsDir: skillsDir, Workspace: t.TempDir(),
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "test-model",
		AgentTools: config.AgentToolSettings{Default: config.AgentToolOverride{
			config.AgentToolWorkspaceRead:  true,
			config.AgentToolSkills:         true,
			config.AgentToolWorkspaceWrite: false,
			config.AgentToolShell:          false,
			config.AgentToolWebSearch:      false,
			config.AgentToolWebFetch:       false,
			config.AgentToolBrowser:        false,
			config.AgentToolAsk:            false,
			config.AgentToolTodo:           false,
			config.AgentToolGoal:           false,
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
	binding, err := producttools.NewReadAdapterBinding(config.AgentToolWorkspaceRead, trajectoryAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildHarnessOptimizerDefinitionWithCompositionForHost(context.Background(), cfg, AgentHostCapabilities{
		ReadAdapters: []producttools.ReadAdapterBinding{binding},
	}); err != nil {
		t.Fatalf("build Harness Optimizer with Skill and trajectory adapters: %v", err)
	}
}
