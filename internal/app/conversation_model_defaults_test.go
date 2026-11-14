package app

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/session"
	appsettings "denova/internal/app/settings"
	"denova/internal/interactive"
)

func TestManualModelSelectionPersistsAcrossRestartAndProjects(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	root := application.cfg.DataDir()
	workspace := application.workspace
	projectID := application.ProjectID()
	ctx := context.Background()
	if _, err := application.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser, []byte(`{
		"model_profiles": [{"id":"alternate","model":"alternate-model"}],
		"agent_models": {"ide":{"thinking_level":"medium"},"interactive_story":{"thinking_level":"low"}}
	}`), ""); err != nil {
		t.Fatal(err)
	}
	writing := ConversationConfigBinding{Mode: ConversationModeWriting, ProjectID: projectID, SessionID: application.session.ID}
	initial, err := application.ConversationConfig(ctx, writing)
	if err != nil {
		t.Fatal(err)
	}
	profile, thinking := "alternate", "high"
	saved, err := application.PatchConversationConfig(ctx, writing, ConversationConfigPatch{ProfileID: &profile, ThinkingLevel: &thinking}, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Old game", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	game := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: projectID, StoryID: story.ID, BranchID: "main"}
	gameInitial, err := application.ConversationConfig(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	gameThinking := "max"
	if _, err := application.PatchConversationConfig(ctx, game, ConversationConfigPatch{ThinkingLevel: &gameThinking}, gameInitial.Revision); err != nil {
		t.Fatal(err)
	}
	// Revisiting and changing permissions in an older conversation must not
	// replace either remembered model preference.
	approval := config.AgentApprovalFullAccess
	if _, err := application.PatchConversationConfig(ctx, writing, ConversationConfigPatch{ApprovalMode: &approval}, saved.Revision); err != nil {
		t.Fatal(err)
	}
	application.Close()
	restartConfig, _, err := config.LoadWithProject(root, workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := New(ctx, restartConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	layered, err := restarted.SettingsService().Snapshot(appsettings.Global())
	if err != nil {
		t.Fatal(err)
	}
	if got := layered.User.AgentModels.IDE; got.ProfileID != profile || got.ThinkingLevel != thinking {
		t.Fatalf("remembered writing model after restart = %#v", got)
	}
	if got := layered.User.AgentModels.InteractiveStory; got.ProfileID != "" || got.ThinkingLevel != gameThinking {
		t.Fatalf("remembered game model after restart = %#v", got)
	}
	// A fresh Project store has no recent conversation to infer a model from.
	otherStore, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherStore.Close() })
	_, fresh, err := agentconversation.GetOrCreateSession(otherStore, "new-project", restarted.cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ProfileID != profile || fresh.ThinkingLevel != thinking {
		t.Fatalf("new Project did not inherit remembered writing model: %#v", fresh)
	}
	oldGame, err := restarted.ConversationConfig(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if oldGame.ProfileID != gameInitial.ProfileID || oldGame.ThinkingLevel != gameThinking {
		t.Fatalf("existing game changed after restart: %#v", oldGame)
	}
}

func TestGameDraftModelSelectionPersistsWithoutCreatingStory(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID()}
	thinking := "high"
	if _, err := application.PatchConversationConfig(context.Background(), binding, ConversationConfigPatch{ThinkingLevel: &thinking}, 0); err != nil {
		t.Fatal(err)
	}
	preview, err := application.ConversationConfig(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Revision != 0 || preview.ThinkingLevel != thinking {
		t.Fatalf("draft did not read saved default: %#v", preview)
	}
	stories, err := application.InteractiveStories()
	if err != nil {
		t.Fatal(err)
	}
	if len(stories.Stories) != 0 {
		t.Fatalf("selecting a draft model created a story: %#v", stories)
	}
}

func TestWritingDraftRemembersSelectionAcrossBookProjects(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	ctx := context.Background()
	firstBook, err := application.CreateBook(ctx, t.TempDir(), "First", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	secondBook, err := application.CreateBook(ctx, t.TempDir(), "Second", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	first := ConversationConfigBinding{Mode: ConversationModeAgentChat, ProjectID: firstBook.ProjectID, SessionID: "first-draft"}
	second := ConversationConfigBinding{Mode: ConversationModeAgentChat, ProjectID: secondBook.ProjectID, SessionID: "second-draft"}
	// Load both Project runtimes before saving, so this also checks cache refresh.
	if _, err := application.ConversationConfig(ctx, second); err != nil {
		t.Fatal(err)
	}
	thinking := "high"
	saved, err := application.PatchConversationConfig(ctx, first, ConversationConfigPatch{ThinkingLevel: &thinking}, 0)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := application.ConversationConfig(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ThinkingLevel != thinking || preview.Revision != 0 {
		t.Fatalf("other Book draft did not use saved model preference: %#v", preview)
	}
	// A rejected stale edit must not change the remembered preference.
	rejectedThinking := "max"
	_, err = application.PatchConversationConfig(ctx, first, ConversationConfigPatch{ThinkingLevel: &rejectedThinking}, 0)
	if !IsConversationConfigRevisionConflict(err) {
		t.Fatalf("stale update = %v", err)
	}
	invalidProfile := "missing-model"
	_, err = application.PatchConversationConfig(ctx, first, ConversationConfigPatch{ProfileID: &invalidProfile}, saved.Revision)
	if err == nil || errors.Is(err, ErrConversationDefaultsNotSaved) {
		t.Fatalf("invalid model must fail before persistence: %v", err)
	}
	settings, err := application.SettingsService().Snapshot(appsettings.Global())
	if err != nil {
		t.Fatal(err)
	}
	if got := settings.User.AgentModels.IDE; got.ThinkingLevel != thinking || got.ProfileID == invalidProfile {
		t.Fatalf("rejected selection changed model defaults: %#v", got)
	}
}

func TestGameRuntimeSwitchRemembersCompleteExternalSelection(t *testing.T) {
	engines := map[config.RuntimeID]*config.RuntimeSelection{
		config.RuntimeGrok:        {Kind: config.RuntimeGrok, Grok: &config.GrokRuntimeSettings{Model: "runtime-model", Effort: "high", ContextWindow: 48000}},
		config.RuntimeAntigravity: {Kind: config.RuntimeAntigravity, Antigravity: &config.AntigravityRuntimeSettings{Model: "runtime-model", Effort: "high"}},
	}
	for kind, selection := range engines {
		t.Run(string(kind), func(t *testing.T) {
			application := newExecutionProfileTestApp(t)
			binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID()}
			saved, err := application.PatchConversationConfig(t.Context(), binding, ConversationConfigPatch{Runtime: selection}, 0)
			if err != nil {
				t.Fatalf("switch to %s: %v", kind, err)
			}
			if saved.Engine().Kind != kind {
				t.Fatalf("switch engine = %q, want %q", saved.Engine().Kind, kind)
			}
			layered, err := application.SettingsService().Snapshot(appsettings.Global())
			if err != nil {
				t.Fatal(err)
			}
			preferences := layered.User.AgentRuntimes.ForAgent(config.AgentKindInteractiveStory)
			if preferences.Selected != kind {
				t.Fatalf("remembered selection = %q, want %q", preferences.Selected, kind)
			}
			switch kind {
			case config.RuntimeGrok:
				if preferences.Grok == nil || preferences.Grok.Model != "runtime-model" || preferences.Grok.Effort != "high" || preferences.Grok.ContextWindow != 48000 {
					t.Fatalf("remembered Grok settings = %#v", preferences.Grok)
				}
			case config.RuntimeAntigravity:
				if preferences.Antigravity == nil || preferences.Antigravity.Model != "runtime-model" || preferences.Antigravity.Effort != "high" {
					t.Fatalf("remembered Antigravity settings = %#v", preferences.Antigravity)
				}
			}
			// A new Game draft must resolve the remembered engine without error.
			draft, err := application.ConversationConfig(t.Context(), binding)
			if err != nil {
				t.Fatalf("new Game draft failed to parse remembered default: %v", err)
			}
			if draft.Engine().Kind != kind {
				t.Fatalf("new draft engine = %q, want %q", draft.Engine().Kind, kind)
			}
			// Creating another story must also resolve the remembered engine.
			second, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Second story", StoryTellerID: "classic"})
			if err != nil {
				t.Fatal(err)
			}
			nextStory, err := application.ConversationConfig(t.Context(), ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID(), StoryID: second.ID, BranchID: "main"})
			if err != nil {
				t.Fatalf("new story failed to parse remembered default: %v", err)
			}
			if nextStory.Engine().Kind != kind {
				t.Fatalf("new story engine = %q, want %q", nextStory.Engine().Kind, kind)
			}
		})
	}
}

func TestRememberCustomAgentRuntimeCompletesNewEngineAndKeepsInactive(t *testing.T) {
	engines := map[config.RuntimeID]*config.RuntimeSelection{
		config.RuntimeGrok:        {Kind: config.RuntimeGrok, Grok: &config.GrokRuntimeSettings{Model: "runtime-model", Effort: "high", ContextWindow: 48000}},
		config.RuntimeAntigravity: {Kind: config.RuntimeAntigravity, Antigravity: &config.AntigravityRuntimeSettings{Model: "runtime-model", Effort: "high"}},
	}
	for kind, selection := range engines {
		t.Run(string(kind), func(t *testing.T) {
			application := newExecutionProfileTestApp(t)
			if _, err := application.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser, []byte(`{
				"custom_agents": [{"id":"agent-1","name":"Agent One","contract":"project.general.v1","runtime":{"selected":"claude","claude":{"model":"keep-claude"}}}]
			}`), ""); err != nil {
				t.Fatal(err)
			}
			snapshot := conversationconfig.Snapshot{Config: conversationconfig.Config{
				AgentKind: config.AgentKindGeneral, CustomAgentID: "agent-1", Runtime: selection,
			}}
			binding := ConversationConfigBinding{Mode: ConversationModeWriting, ProjectID: application.ProjectID()}
			if err := application.rememberConversationDefaults(binding, snapshot, conversationconfig.Patch{Runtime: selection}); err != nil {
				t.Fatalf("remember switch to %s: %v", kind, err)
			}
			persisted, err := application.SettingsService().Snapshot(appsettings.Global())
			if err != nil {
				t.Fatal(err)
			}
			var saved *config.RuntimePreferences
			for index := range persisted.User.CustomAgents {
				if persisted.User.CustomAgents[index].ID == "agent-1" {
					saved = persisted.User.CustomAgents[index].Runtime
					break
				}
			}
			if saved == nil || saved.Selected != kind {
				t.Fatalf("remembered custom agent runtime = %#v, want selected %q", saved, kind)
			}
			if saved.Claude == nil || saved.Claude.Model != "keep-claude" {
				t.Fatalf("inactive Claude settings were dropped: %#v", saved.Claude)
			}
			switch kind {
			case config.RuntimeGrok:
				if saved.Grok == nil || saved.Grok.Model != "runtime-model" || saved.Grok.Effort != "high" || saved.Grok.ContextWindow != 48000 {
					t.Fatalf("remembered custom agent Grok settings = %#v", saved.Grok)
				}
			case config.RuntimeAntigravity:
				if saved.Antigravity == nil || saved.Antigravity.Model != "runtime-model" || saved.Antigravity.Effort != "high" {
					t.Fatalf("remembered custom agent Antigravity settings = %#v", saved.Antigravity)
				}
			}
		})
	}
}

func TestProjectRuntimeSwitchRemembersCompleteExternalEngine(t *testing.T) {
	engines := map[config.RuntimeID]*config.RuntimeSelection{
		config.RuntimeGrok:        {Kind: config.RuntimeGrok, Grok: &config.GrokRuntimeSettings{Model: "runtime-model", Effort: "high", ContextWindow: 48000}},
		config.RuntimeAntigravity: {Kind: config.RuntimeAntigravity, Antigravity: &config.AntigravityRuntimeSettings{Model: "runtime-model", Effort: "high"}},
	}
	for kind, selection := range engines {
		t.Run(string(kind), func(t *testing.T) {
			application := newExecutionProfileTestApp(t)
			projectID := application.ProjectID()
			// An explicit Project selector already resolves another engine, so the
			// switch must update that scope instead of the user default.
			if _, err := application.SettingsService().Patch(appsettings.Project(projectID), config.SettingsLayerWorkspace, []byte(`{
				"agent_runtimes": {"interactive_story": {"selected":"codex","codex":{"model":"project-base"}}}
			}`), ""); err != nil {
				t.Fatal(err)
			}
			binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: projectID}
			if _, err := application.PatchConversationConfig(t.Context(), binding, ConversationConfigPatch{Runtime: selection}, 0); err != nil {
				t.Fatalf("switch to %s: %v", kind, err)
			}
			project, err := application.SettingsService().Snapshot(appsettings.Project(projectID))
			if err != nil {
				t.Fatal(err)
			}
			preferences := project.Workspace.AgentRuntimes.ForAgent(config.AgentKindInteractiveStory)
			if preferences.Selected != kind {
				t.Fatalf("project remembered selection = %q, want %q", preferences.Selected, kind)
			}
			if preferences.Codex == nil || preferences.Codex.Model != "project-base" {
				t.Fatalf("project switch dropped the inactive Codex branch: %#v", preferences.Codex)
			}
			switch kind {
			case config.RuntimeGrok:
				if preferences.Grok == nil || preferences.Grok.Model != "runtime-model" || preferences.Grok.Effort != "high" || preferences.Grok.ContextWindow != 48000 {
					t.Fatalf("project remembered Grok settings = %#v", preferences.Grok)
				}
			case config.RuntimeAntigravity:
				if preferences.Antigravity == nil || preferences.Antigravity.Model != "runtime-model" || preferences.Antigravity.Effort != "high" {
					t.Fatalf("project remembered Antigravity settings = %#v", preferences.Antigravity)
				}
			}
			user, err := application.SettingsService().Snapshot(appsettings.Global())
			if err != nil {
				t.Fatal(err)
			}
			if got := user.User.AgentRuntimes.ForAgent(config.AgentKindInteractiveStory); got.Selected != "" {
				t.Fatalf("project switch leaked into user layer: %#v", got)
			}
		})
	}
}
