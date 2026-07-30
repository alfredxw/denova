package agents

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestAskRegistrationRequiresInteractiveTopLevelHost(t *testing.T) {
	var captured agent.AgentConfig
	previous := newNativeAgent
	newNativeAgent = func(_ context.Context, cfg agent.AgentConfig) (agent.Runnable, error) {
		captured = cfg
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newNativeAgent = previous })

	build := func(kind string, interactive bool) map[string]bool {
		t.Helper()
		captured = agent.AgentConfig{}
		cfg := askOnlyAgentConfig(t, kind)
		if _, err := buildAgent(context.Background(), cfg, agentBuildSpec{
			Kind: kind, Name: "ask-registration-test", Description: "test",
			Instruction: "test", InteractiveHost: interactive, DisableWriteTodos: true,
		}); err != nil {
			t.Fatal(err)
		}
		return toolNamesForTest(t, captured.Tools)
	}

	if build(config.AgentKindIDE, false)["ask"] {
		t.Fatal("headless IDE Agent registered ask")
	}
	if !build(config.AgentKindIDE, true)["ask"] {
		t.Fatal("interactive IDE Agent did not register ask")
	}
	if !build(config.AgentKindGeneral, true)["ask"] {
		t.Fatal("interactive General Agent did not register ask")
	}
	if !build(config.AgentKindConfigManager, true)["ask"] {
		t.Fatal("interactive Config Manager Agent did not register ask")
	}
	if build(config.AgentKindAutomation, true)["ask"] {
		t.Fatal("background Automation Agent registered ask despite a synthetic interactive flag")
	}
}

func askOnlyAgentConfig(t *testing.T, kind string) *config.Config {
	t.Helper()
	allOff := config.AgentToolOverride{
		config.AgentToolWorkspaceRead: false, config.AgentToolWorkspaceWrite: false,
		config.AgentToolShell: false, config.AgentToolWebSearch: false,
		config.AgentToolWebFetch: false, config.AgentToolBrowser: false,
		config.AgentToolAsk: false, config.AgentToolTodo: false,
		config.AgentToolSkills: false, config.AgentToolDelegation: false,
		config.AgentToolConfigRead: false, config.AgentToolConfigApply: false,
		config.AgentToolLoreRead: false, config.AgentToolLoreWrite: false,
		config.AgentToolImageGeneration: false, config.AgentToolContextRewind: false,
	}
	override := config.AgentToolOverride{config.AgentToolAsk: true, config.AgentToolImageGeneration: false}
	settings := config.AgentToolSettings{Default: allOff}
	switch kind {
	case config.AgentKindGeneral:
		settings.General = override
	case config.AgentKindIDE:
		settings.IDE = override
	case config.AgentKindConfigManager:
		settings.ConfigManager = override
	case config.AgentKindAutomation:
		settings.Automation = override
	}
	return &config.Config{
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "test-model", Workspace: t.TempDir(),
		AgentTools: settings,
	}
}
