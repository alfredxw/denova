package config

import "testing"

func TestResolveAgentModelContextWindowDefaultsAndOverrides(t *testing.T) {
	defaultModel := ResolveAgentModel(&Config{}, AgentKindIDE)
	if defaultModel.ContextWindowTokens != DefaultContextWindowTokens {
		t.Fatalf("default context window = %d, want %d", defaultModel.ContextWindowTokens, DefaultContextWindowTokens)
	}

	mainContextWindow := 600000
	mainModel := ResolveAgentModel(&Config{OpenAIContextWindowTokens: mainContextWindow}, AgentKindIDE)
	if mainModel.ContextWindowTokens != mainContextWindow {
		t.Fatalf("main model context window = %d, want %d", mainModel.ContextWindowTokens, mainContextWindow)
	}

	contextWindow := 1000000
	cfg := &Config{
		ModelProfiles: []ModelProfileSettings{
			{ID: "large", ContextWindowTokens: &contextWindow},
		},
		AgentModels: AgentModelSettings{
			IDE: AgentModelOverride{ProfileID: "large"},
		},
	}
	resolved := ResolveAgentModel(cfg, AgentKindIDE)
	if resolved.ContextWindowTokens != contextWindow {
		t.Fatalf("profile context window = %d, want %d", resolved.ContextWindowTokens, contextWindow)
	}

	cfg = &Config{
		OpenAIContextWindowTokens: mainContextWindow,
		ModelProfiles: []ModelProfileSettings{
			{ID: "inherits-main"},
		},
		AgentModels: AgentModelSettings{
			InteractiveStory: AgentModelOverride{ProfileID: "inherits-main"},
		},
	}
	resolved = ResolveAgentModel(cfg, AgentKindInteractiveStory)
	if resolved.ContextWindowTokens != mainContextWindow {
		t.Fatalf("profile inherited context window = %d, want %d", resolved.ContextWindowTokens, mainContextWindow)
	}
}

func TestSanitizeModelProfilesCapsContextWindow(t *testing.T) {
	tooLarge := 3000000
	invalid := -1
	settings := sanitizeEditableSettings(Settings{
		OpenAIContextWindowTokens: &tooLarge,
		ModelProfiles: []ModelProfileSettings{
			{ID: "large", ContextWindowTokens: &tooLarge},
			{ID: "bad", ContextWindowTokens: &invalid},
			{ID: "  "},
		},
	})
	if len(settings.ModelProfiles) != 2 {
		t.Fatalf("sanitized model profiles length = %d, want 2", len(settings.ModelProfiles))
	}
	if got := *settings.OpenAIContextWindowTokens; got != MaxContextWindowTokens {
		t.Fatalf("main context window = %d, want %d", got, MaxContextWindowTokens)
	}
	if got := *settings.ModelProfiles[0].ContextWindowTokens; got != MaxContextWindowTokens {
		t.Fatalf("large profile context window = %d, want %d", got, MaxContextWindowTokens)
	}
	if settings.ModelProfiles[1].ContextWindowTokens != nil {
		t.Fatalf("invalid context window should be cleared: %#v", settings.ModelProfiles[1])
	}
}
