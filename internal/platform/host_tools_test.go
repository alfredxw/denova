package platform

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"denova/config"
	productagents "denova/internal/agents"
	"denova/internal/agents/delegation"
	"denova/internal/agents/prompts"
	agent "github.com/alfredxw/denova/agent"
)

type pluginCallingModel struct{}

func (pluginCallingModel) Generate(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == agent.ToolRole {
			return agent.AssistantMessage("Used plugin: "+messages[i].Content, nil), nil
		}
	}
	for _, tool := range agent.GetCommonOptions(nil, options...).Tools {
		if strings.HasPrefix(tool.Name, "plugin_") {
			return agent.AssistantMessage("", []agent.ToolCall{{ID: "count-text", Type: "function", Function: agent.FunctionCall{Name: tool.Name, Arguments: `{"text":"A🌷中"}`}}}), nil
		}
	}
	return nil, fmt.Errorf("model did not receive the selected plugin tool")
}
func (m pluginCallingModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	return agent.StreamReaderFromArray([]*agent.Message{message}), err
}

func pluginTestConfig(t *testing.T, m *Manager, projectID string) *config.Config {
	t.Helper()
	_, layout, err := m.registry.Resolve(projectID, true)
	if err != nil {
		t.Fatal(err)
	}
	disabled := config.AgentToolOverride{}
	for _, item := range config.AgentToolCapabilityCatalogForGOOS("") {
		disabled[item.Capability] = false
	}
	return &config.Config{ProjectID: projectID, Workspace: layout.ContentRoot, ProjectStoreDir: layout.StoreRoot, DenovaDir: m.root, OpenAIModel: "test-model", OpenAIBaseURL: "https://example.invalid", AgentApprovalMode: config.AgentApprovalAsk, AgentPluginScope: config.AgentPluginScope{SessionID: "writing"}, AgentTools: config.AgentToolSettings{Default: disabled}}
}

func TestPluginToolThroughWritingWorkbenchAndGameAgents(t *testing.T) {
	m, projectID := testManager(t)
	testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.shared-tools", Plugin))
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral, config.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			cfg := pluginTestConfig(t, m, projectID)
			if kind == config.AgentKindInteractiveStory {
				cfg.AgentPluginScope = config.AgentPluginScope{StoryID: "story", BranchID: "main"}
			}
			plugins, err := m.HostAgentTools(cfg, kind)
			if err != nil {
				t.Fatal(err)
			}
			host := productagents.AgentHostCapabilities{PluginTools: plugins}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var definition agent.Definition
			switch kind {
			case config.AgentKindIDE:
				definition, _, err = productagents.BuildDefinitionWithCompositionForHost(ctx, cfg, nil, prompts.IDEStoryTeller{}, host)
			case config.AgentKindGeneral:
				definition, _, err = productagents.BuildGeneralDefinitionWithCompositionForHost(ctx, cfg, nil, host)
			case config.AgentKindInteractiveStory:
				definition, _, err = productagents.BuildInteractiveStoryDefinitionWithCompositionForHost(ctx, cfg, nil, prompts.InteractiveStorySystemInstructionInput{}, host)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := definition.Tools.PrepareTools(ctx, agent.ToolRequest{}); err != nil {
				t.Fatal(err)
			}
			if len(m.RuntimeSnapshots()) != 0 {
				t.Fatal("schema inspection started plugin code")
			}
			definition.Model, definition.ModelIdentity = pluginCallingModel{}, agent.CapabilityIdentity{Kind: "test.plugin_model", Version: 1}
			owner, err := agent.New(ctx, definition)
			if err != nil {
				t.Fatal(err)
			}
			sess, err := owner.Session(ctx, agent.SessionKey{Namespace: "test.plugins", ID: kind})
			if err != nil {
				t.Fatal(err)
			}
			run, err := sess.Run(ctx, agent.Input{Text: "Use the selected tool to count the characters."})
			if err != nil {
				t.Fatal(err)
			}
			wait, stop := context.WithTimeout(ctx, 2*time.Second)
			defer stop()
			result, err := run.Wait(wait)
			snapshot, snapshotErr := sess.Snapshot(wait)
			if err != nil || snapshotErr != nil || result.Status != agent.ResultCompleted || len(snapshot.RecentRuns) == 0 || !strings.Contains(snapshot.RecentRuns[0].Output, `"value":3`) {
				t.Fatalf("Agent did not use plugin: %#v %v", result, err)
			}
			deadline := time.Now().Add(time.Second)
			for len(m.RuntimeSnapshots()) != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if len(m.RuntimeSnapshots()) != 0 {
				t.Fatal("plugin process outlived its Agent run")
			}
			if err := owner.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEnabledPluginsAvailableWithoutConversationSelection(t *testing.T) {
	m, projectID := testManager(t)
	testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.global-tools", Plugin))
	cfg := &config.Config{ProjectID: projectID, AgentPluginScope: config.AgentPluginScope{SessionID: "existing-session"}}
	set, err := m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil || set == nil {
		t.Fatalf("enabled plugins must be available without a conversation selection: %v", err)
	}
	definitions, err := set.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("expected one public tool without duplicating its toolset: %d %v", len(definitions), err)
	}
}

func TestPluginToolsReachDelegatedAgents(t *testing.T) {
	m, projectID := testManager(t)
	testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.shared-tools", Plugin))
	cfg := pluginTestConfig(t, m, projectID)
	cfg.AgentTools.Default[config.AgentToolDelegation] = true
	cfg.SubAgents = []config.SubAgentConfig{{ID: "researcher", Name: "Researcher", Description: "Research", Parents: []string{config.AgentKindIDE}, SystemPrompt: "Return findings."}}
	plugins, err := m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	definition, _, err := productagents.BuildDefinitionWithCompositionForHost(context.Background(), cfg, nil, prompts.IDEStoryTeller{}, productagents.AgentHostCapabilities{PluginTools: plugins})
	if err != nil {
		t.Fatal(err)
	}
	catalog, ok := delegation.AsCatalog(definition.Tools)
	if !ok || len(catalog.Children()) == 0 {
		t.Fatal("missing delegated Agents")
	}
	for _, child := range catalog.Children() {
		tools, err := child.Definition.Tools.PrepareTools(context.Background(), agent.ToolRequest{})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, tool := range tools {
			info, _ := tool.Tool.Info(context.Background())
			if strings.HasPrefix(info.Name, "plugin_") {
				found = true
			}
		}
		if !found {
			t.Fatalf("child %s lost plugin tools", child.Name)
		}
		definition := child.Definition
		definition.Model, definition.ModelIdentity = pluginCallingModel{}, agent.CapabilityIdentity{Kind: "test.plugin_model", Version: 1}
		owner, err := agent.New(context.Background(), definition)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = owner.Close(context.Background()) })
		sess, err := owner.Session(context.Background(), agent.SessionKey{Namespace: "test.child", ID: child.Name})
		if err != nil {
			t.Fatal(err)
		}
		run, err := sess.Run(context.Background(), agent.Input{Text: "Use the inherited tool."})
		if err != nil {
			t.Fatal(err)
		}
		wait, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		result, err := run.Wait(wait)
		snapshot, readErr := sess.Snapshot(wait)
		cancel()
		if err != nil || readErr != nil || result.Status != agent.ResultCompleted || len(snapshot.RecentRuns) == 0 || !strings.Contains(snapshot.RecentRuns[0].Output, `"value":3`) {
			t.Fatalf("child %s did not use plugin: %#v %v %v", child.Name, result, err, readErr)
		}
	}
}

func TestSharedPluginSettingsAndScopeIsolation(t *testing.T) {
	m, projectID := testManager(t)
	release := testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.shared-tools", Plugin))
	cfg := pluginTestConfig(t, m, projectID)
	// Existing conversations use current preferences when preparing new work.
	if err := writeBytes(m.settingsPath(release.Ref), []byte("enabled = true\n")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, scope := range []config.AgentPluginScope{{SessionID: "one"}, {SessionID: "two"}, {StoryID: "story", BranchID: "a"}, {StoryID: "story", BranchID: "b"}} {
		cfg.AgentPluginScope = scope
		set, err := m.HostAgentTools(cfg, config.AgentKindIDE)
		if err != nil {
			t.Fatal(err)
		}
		tools, err := set.PrepareTools(ctx, agent.ToolRequest{})
		if err != nil {
			t.Fatal(err)
		}
		result, err := tools[0].Tool.Run(ctx, `{"text":"A B"}`)
		if err != nil || !strings.Contains(fmt.Sprint(result), `"value":2`) {
			t.Fatalf("new execution did not use shared settings: %#v %v", result, err)
		}
	}
	dirs := map[string]bool{}
	for _, runtime := range m.runtimes {
		if dirs[runtime.owner.dataDir] {
			t.Fatal("plugin data crossed conversation or branch")
		}
		dirs[runtime.owner.dataDir] = true
	}
	if len(dirs) != 4 {
		t.Fatalf("scope count: %d", len(dirs))
	}
}

func TestSharedPluginChangesApplyToNewExecutions(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "tool", "test.shared-tools", Plugin)
	release := testInstall(t, m, candidate)
	cfg := pluginTestConfig(t, m, projectID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old, err := m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	oldTools, err := old.PrepareTools(ctx, agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeBytes(m.settingsPath(release.Ref), []byte("enabled = true\n")); err != nil {
		t.Fatal(err)
	}
	current, err := m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	if old.Identity() == current.Identity() {
		t.Fatal("changed settings must invalidate paused execution behavior")
	}
	currentTools, err := current.PrepareTools(ctx, agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		tool     agent.ToolDefinition
		expected string
	}{{oldTools[0], `"value":3`}, {currentTools[0], `"value":2`}} {
		result, err := check.tool.Tool.Run(ctx, `{"text":"A B"}`)
		if err != nil || !strings.Contains(fmt.Sprint(result), check.expected) {
			t.Fatalf("unexpected configuration: %v %v", result, err)
		}
	}
	files := maps.Clone(candidate.files)
	files["update.txt"] = []byte("A new installed release with the same version")
	updated, err := m.freeze(Plugin, files)
	if err != nil {
		t.Fatal(err)
	}
	testInstall(t, m, updated)
	next, err := m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	if next.Identity() == current.Identity() {
		t.Fatal("new execution ignored the installed update")
	}
	if err := m.SetAvailability(ctx, Plugin, release.Manifest.ID, PackageAvailability{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	disabled, err := m.HostAgentTools(cfg, config.AgentKindIDE)
	if err != nil || disabled != nil {
		t.Fatalf("disabled plugin is still available: %v", err)
	}
	// Disabling admits no new work, but an already prepared task may finish.
	if _, err := currentTools[0].Tool.Run(ctx, `{"text":"A B"}`); err != nil {
		t.Fatal(err)
	}
	if err := m.SetAvailability(ctx, Plugin, release.Manifest.ID, PackageAvailability{Removed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := currentTools[0].Tool.Run(ctx, `{"text":"A B"}`); err == nil {
		t.Fatal("removed plugin retained execution authority")
	}
}
