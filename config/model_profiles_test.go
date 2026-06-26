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

func TestResolveAgentModelUsesModelNameAsProfileID(t *testing.T) {
	cfg := &Config{
		ModelProfiles: []ModelProfileSettings{
			{OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "gpt-4.1"},
		},
		AgentModels: AgentModelSettings{
			IDE: AgentModelOverride{ProfileID: "gpt-4.1"},
		},
	}
	resolved := ResolveAgentModel(cfg, AgentKindIDE)
	if resolved.ProfileID != "gpt-4.1" {
		t.Fatalf("profile id = %q, want model name", resolved.ProfileID)
	}
	if resolved.OpenAIBaseURL != "https://api.openai.com/v1" || resolved.OpenAIModel != "gpt-4.1" {
		t.Fatalf("resolved model mismatch: %#v", resolved)
	}
}

func TestResolveAgentModelAllowsDefaultProfileOverride(t *testing.T) {
	contextWindow := 1000000
	cfg := &Config{
		OpenAIBaseURL:             "https://legacy.example/v1",
		OpenAIModel:               "legacy-model",
		OpenAIContextWindowTokens: DefaultContextWindowTokens,
		ModelProfiles: []ModelProfileSettings{
			{
				ID:                  "default",
				Name:                "Writing default",
				OpenAIBaseURL:       "https://api.openai.com/v1",
				OpenAIModel:         "gpt-4.1",
				ContextWindowTokens: &contextWindow,
			},
		},
	}
	resolved := ResolveAgentModel(cfg, AgentKindIDE)
	if resolved.ProfileID != "default" {
		t.Fatalf("profile id = %q, want default", resolved.ProfileID)
	}
	if resolved.OpenAIBaseURL != "https://api.openai.com/v1" || resolved.OpenAIModel != "gpt-4.1" {
		t.Fatalf("default profile should override legacy fields: %#v", resolved)
	}
	if resolved.ContextWindowTokens != contextWindow {
		t.Fatalf("context window = %d, want %d", resolved.ContextWindowTokens, contextWindow)
	}
}

func TestResolveAgentModelClearsInheritedDefaultProfileAlias(t *testing.T) {
	profiles := mergeModelProfiles(
		[]ModelProfileSettings{{ID: "default", Name: "DeepSeek 写作", OpenAIModel: "deepseek-v4-pro"}},
		[]ModelProfileSettings{{ID: "default", OpenAIModel: "deepseek-v4-pro"}},
	)
	if len(profiles) != 1 || profiles[0].Name != "" {
		t.Fatalf("default profile alias should be cleared: %#v", profiles)
	}

	resolved := ResolveAgentModel(&Config{ModelProfiles: profiles}, AgentKindIDE)
	if resolved.ProfileID != "default" || resolved.OpenAIModel != "deepseek-v4-pro" {
		t.Fatalf("default profile should still resolve after alias is cleared: %#v", resolved)
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

func TestSanitizeModelProfilesDerivesIDFromModelName(t *testing.T) {
	settings := sanitizeEditableSettings(Settings{
		ModelProfiles: []ModelProfileSettings{
			{OpenAIModel: " gpt-4.1 ", Name: " Fast model "},
			{ID: " legacy "},
		},
	})
	if settings.ModelProfiles[0].ID != "gpt-4.1" || settings.ModelProfiles[0].OpenAIModel != "gpt-4.1" {
		t.Fatalf("model-name profile not normalized: %#v", settings.ModelProfiles[0])
	}
	if settings.ModelProfiles[0].Name != "Fast model" {
		t.Fatalf("model alias not normalized: %#v", settings.ModelProfiles[0])
	}
	if settings.ModelProfiles[1].ID != "legacy" || settings.ModelProfiles[1].OpenAIModel != "legacy" {
		t.Fatalf("legacy id profile should keep working: %#v", settings.ModelProfiles[1])
	}
}

func TestSanitizeDefaultModelProfileCanInheritModelFields(t *testing.T) {
	settings := sanitizeEditableSettings(Settings{
		ModelProfiles: []ModelProfileSettings{
			{ID: "default", Name: "Main"},
		},
	})
	if len(settings.ModelProfiles) != 1 {
		t.Fatalf("sanitized model profiles length = %d, want 1", len(settings.ModelProfiles))
	}
	if settings.ModelProfiles[0].OpenAIModel != "" {
		t.Fatalf("default profile without model should keep inheriting model fields: %#v", settings.ModelProfiles[0])
	}
}

func TestSanitizeSettingsClearsLegacyModelFieldsWhenDefaultProfileExists(t *testing.T) {
	contextWindow := 1000000
	settings := sanitizeEditableSettings(Settings{
		OpenAIAPIKey:              "legacy-key",
		OpenAIBaseURL:             "https://legacy.example/v1",
		OpenAIModel:               "legacy-model",
		OpenAIContextWindowTokens: &contextWindow,
		ModelProfiles: []ModelProfileSettings{
			{
				ID:                  "default",
				Name:                "Main",
				OpenAIAPIKey:        "profile-key",
				OpenAIBaseURL:       "https://api.openai.com/v1",
				OpenAIModel:         "gpt-4.1",
				ContextWindowTokens: &contextWindow,
			},
		},
	})
	if settings.OpenAIAPIKey != "" || settings.OpenAIBaseURL != "" || settings.OpenAIModel != "" || settings.OpenAIContextWindowTokens != nil {
		t.Fatalf("legacy model fields should be cleared when default profile exists: %#v", settings)
	}
	if len(settings.ModelProfiles) != 1 || settings.ModelProfiles[0].ID != "default" || settings.ModelProfiles[0].Name != "Main" {
		t.Fatalf("default profile should be preserved: %#v", settings.ModelProfiles)
	}
}

func TestSanitizeSettingsKeepsLegacyModelFieldsForAliasOnlyDefaultProfile(t *testing.T) {
	contextWindow := 1000000
	settings := sanitizeEditableSettings(Settings{
		OpenAIAPIKey:              "legacy-key",
		OpenAIBaseURL:             "https://legacy.example/v1",
		OpenAIModel:               "legacy-model",
		OpenAIContextWindowTokens: &contextWindow,
		ModelProfiles: []ModelProfileSettings{
			{ID: "default", Name: "Main"},
		},
	})
	if settings.OpenAIAPIKey != "legacy-key" || settings.OpenAIBaseURL != "https://legacy.example/v1" || settings.OpenAIModel != "legacy-model" || settings.OpenAIContextWindowTokens == nil {
		t.Fatalf("alias-only default profile should keep legacy model fields: %#v", settings)
	}
}
