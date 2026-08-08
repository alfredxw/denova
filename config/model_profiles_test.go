package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
)

func TestReadSettingsFileMigratesLegacyModelProfileFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	legacy := `[[model_profiles]]
id = "doubao"
name = "Doubao"
openai_api_key = "legacy-key"
openai_base_url = "https://ark.cn-beijing.volces.com/api/v3"
openai_model = "doubao-seed"
max_output_tokens = 2048
`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	settings, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.ModelProfiles) != 1 {
		t.Fatalf("model profile count = %d, want 1", len(settings.ModelProfiles))
	}
	profile := settings.ModelProfiles[0]
	if profile.APIKey != "legacy-key" || profile.BaseURL != "https://ark.cn-beijing.volces.com/api/v3" || profile.Model != "doubao-seed" {
		t.Fatalf("legacy model profile was not migrated: %#v", profile)
	}
	if profile.Provider != string(providers.ProviderVolcengine) || profile.Protocol != string(providers.ProtocolOpenAIChatCompletions) {
		t.Fatalf("legacy route = provider %q protocol %q", profile.Provider, profile.Protocol)
	}
	if profile.LegacyMaxOutputTokens != nil {
		t.Fatalf("legacy max output tokens must be dropped: %#v", profile.LegacyMaxOutputTokens)
	}

	if err := WriteSettingsFile(path, settings); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(persisted)
	if strings.Contains(text, "openai_api_key") || strings.Contains(text, "openai_base_url") || strings.Contains(text, "openai_model") || strings.Contains(text, "max_output_tokens") {
		t.Fatalf("legacy model profile fields must not be written back:\n%s", text)
	}
	for _, field := range []string{`api_key = 'legacy-key'`, `base_url = 'https://ark.cn-beijing.volces.com/api/v3'`, `model = 'doubao-seed'`} {
		if !strings.Contains(text, field) {
			t.Fatalf("canonical model profile field %q missing from:\n%s", field, text)
		}
	}
}

func TestReadSettingsFilePrefersCanonicalModelProfileFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	mixed := `[[model_profiles]]
id = "mixed"
api_key = "canonical-key"
base_url = "https://canonical.example/v1"
model = "canonical-model"
openai_api_key = "legacy-key"
openai_base_url = "https://legacy.example/v1"
openai_model = "legacy-model"
`
	if err := os.WriteFile(path, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}

	settings, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := settings.ModelProfiles[0]
	if profile.APIKey != "canonical-key" || profile.BaseURL != "https://canonical.example/v1" || profile.Model != "canonical-model" {
		t.Fatalf("canonical fields must win over legacy aliases: %#v", profile)
	}
}

func TestResolveAgentModelProviderProtocolRouting(t *testing.T) {
	tests := []struct {
		name         string
		config       *Config
		wantProvider providers.ProviderID
		wantProtocol providers.ProtocolID
	}{
		{
			name:         "legacy OpenAI endpoint remains on Chat Completions",
			config:       &Config{OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "gpt-4.1"},
			wantProvider: providers.ProviderOpenAI,
			wantProtocol: providers.ProtocolOpenAIChatCompletions,
		},
		{
			name: "explicit OpenAI provider delegates its default protocol to the registry",
			config: &Config{ModelProfiles: []ModelProfileSettings{{
				ID: "default", Provider: string(providers.ProviderOpenAI), Model: "gpt-5",
			}}},
			wantProvider: providers.ProviderOpenAI,
			wantProtocol: "",
		},
		{
			name: "explicit OpenAI provider can retain Chat Completions",
			config: &Config{ModelProfiles: []ModelProfileSettings{{
				ID: "default", Provider: string(providers.ProviderOpenAI), Protocol: string(providers.ProtocolOpenAIChatCompletions), Model: "gpt-4.1",
			}}},
			wantProvider: providers.ProviderOpenAI,
			wantProtocol: providers.ProtocolOpenAIChatCompletions,
		},
		{
			name:         "DeepSeek endpoint defaults to Chat Completions",
			config:       &Config{OpenAIBaseURL: "https://api.deepseek.com", OpenAIModel: "deepseek-chat"},
			wantProvider: providers.ProviderDeepSeek,
			wantProtocol: providers.ProtocolOpenAIChatCompletions,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := ResolveAgentModel(test.config, AgentKindIDE)
			if resolved.Provider != string(test.wantProvider) || resolved.Protocol != string(test.wantProtocol) {
				t.Fatalf("routing = provider %q protocol %q, want %q/%q", resolved.Provider, resolved.Protocol, test.wantProvider, test.wantProtocol)
			}
		})
	}
}

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

func TestResolveAgentModelUsesUnifiedThinkingLevel(t *testing.T) {
	tests := []struct {
		name   string
		models AgentModelSettings
		want   providers.ThinkingLevel
	}{
		{
			name: "unset resolves to model default",
			want: providers.ThinkingLevelDefault,
		},
		{
			name:   "agent inherits default level",
			models: AgentModelSettings{Default: AgentModelOverride{ThinkingLevel: "xhigh"}},
			want:   providers.ThinkingLevelXHigh,
		},
		{
			name: "explicit model default overrides parent",
			models: AgentModelSettings{
				Default: AgentModelOverride{ThinkingLevel: "high"},
				IDE:     AgentModelOverride{ThinkingLevel: "default"},
			},
			want: providers.ThinkingLevelDefault,
		},
		{
			name:   "OpenAI none alias normalizes to off",
			models: AgentModelSettings{IDE: AgentModelOverride{ThinkingLevel: "none"}},
			want:   providers.ThinkingLevelOff,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := ResolveAgentModel(&Config{AgentModels: test.models}, AgentKindIDE)
			if resolved.ThinkingLevel != string(test.want) {
				t.Fatalf("thinking level = %q, want %q", resolved.ThinkingLevel, test.want)
			}
		})
	}
}

func TestResolveAgentModelUsesModelNameAsProfileID(t *testing.T) {
	cfg := &Config{
		ModelProfiles: []ModelProfileSettings{
			{BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1"},
		},
		AgentModels: AgentModelSettings{
			IDE: AgentModelOverride{ProfileID: "gpt-4.1"},
		},
	}
	resolved := ResolveAgentModel(cfg, AgentKindIDE)
	if resolved.ProfileID != "gpt-4.1" {
		t.Fatalf("profile id = %q, want model name", resolved.ProfileID)
	}
	if resolved.BaseURL != "https://api.openai.com/v1" || resolved.Model != "gpt-4.1" {
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
				BaseURL:             "https://api.openai.com/v1",
				Model:               "gpt-4.1",
				ContextWindowTokens: &contextWindow,
			},
		},
	}
	resolved := ResolveAgentModel(cfg, AgentKindIDE)
	if resolved.ProfileID != "default" {
		t.Fatalf("profile id = %q, want default", resolved.ProfileID)
	}
	if resolved.BaseURL != "https://api.openai.com/v1" || resolved.Model != "gpt-4.1" {
		t.Fatalf("default profile should override legacy fields: %#v", resolved)
	}
	if resolved.ContextWindowTokens != contextWindow {
		t.Fatalf("context window = %d, want %d", resolved.ContextWindowTokens, contextWindow)
	}
}

func TestResolveAgentModelInheritsBlankFieldsFromDefaultProfile(t *testing.T) {
	contextWindow := 1000000
	cfg := &Config{
		ModelProfiles: []ModelProfileSettings{
			{
				ID:                  "default",
				APIKey:              "default-key",
				BaseURL:             "https://api.default.example/v1",
				Model:               "default-model",
				ContextWindowTokens: &contextWindow,
			},
			{
				ID:    "fast",
				Model: "fast-model",
			},
		},
		AgentModels: AgentModelSettings{
			IDE: AgentModelOverride{ProfileID: "fast"},
		},
	}
	resolved := ResolveAgentModel(cfg, AgentKindIDE)
	if resolved.ProfileID != "fast" {
		t.Fatalf("profile id = %q, want fast", resolved.ProfileID)
	}
	if resolved.APIKey != "default-key" || resolved.BaseURL != "https://api.default.example/v1" || resolved.Model != "fast-model" {
		t.Fatalf("blank profile fields should inherit from default profile: %#v", resolved)
	}
	if resolved.ContextWindowTokens != contextWindow {
		t.Fatalf("context window = %d, want inherited %d", resolved.ContextWindowTokens, contextWindow)
	}
}

func TestResolveAgentModelInheritsProviderAndProtocolFromDefaultProfile(t *testing.T) {
	cfg := &Config{
		ModelProfiles: []ModelProfileSettings{
			{
				ID:       "default",
				Provider: string(providers.ProviderOpenAI),
				Protocol: string(providers.ProtocolOpenAIResponses),
				Model:    "gpt-5",
			},
			{
				ID:    "fast",
				Model: "gpt-5-mini",
			},
		},
		AgentModels: AgentModelSettings{
			IDE: AgentModelOverride{ProfileID: "fast"},
		},
	}

	resolved := ResolveAgentModel(cfg, AgentKindIDE)
	if resolved.Provider != string(providers.ProviderOpenAI) || resolved.Protocol != string(providers.ProtocolOpenAIResponses) {
		t.Fatalf("inherited routing = %q/%q, want OpenAI Responses", resolved.Provider, resolved.Protocol)
	}
	if resolved.BaseURL != "" {
		t.Fatalf("inherited base URL = %q", resolved.BaseURL)
	}
}

func TestResolveAgentModelScopesInheritedAPIKeyToEndpointOrigin(t *testing.T) {
	cfg := &Config{
		ModelProfiles: []ModelProfileSettings{
			{
				ID:       "default",
				Provider: string(providers.ProviderDeepSeek),
				APIKey:   "deepseek-secret",
				BaseURL:  "https://api.deepseek.com",
				Model:    "deepseek-v4-pro",
			},
			{
				ID:       "same-origin",
				Provider: string(providers.ProviderDeepSeek),
				Protocol: string(providers.ProtocolAnthropicMessages),
				BaseURL:  "https://api.deepseek.com/anthropic",
				Model:    "deepseek-v4-pro",
			},
			{
				ID:       "other-origin",
				Provider: string(providers.ProviderDeepSeek),
				BaseURL:  "https://gateway.example.test/v1",
				Model:    "deepseek-v4-pro",
			},
			{
				ID:       "other-provider",
				Provider: "google",
				Model:    "gemini-2.5-pro",
			},
		},
	}

	cfg.AgentModels.IDE.ProfileID = "same-origin"
	if got := ResolveAgentModel(cfg, AgentKindIDE).APIKey; got != "deepseek-secret" {
		t.Fatalf("same-origin route API key = %q, want inherited secret", got)
	}
	cfg.AgentModels.IDE.ProfileID = "other-origin"
	if got := ResolveAgentModel(cfg, AgentKindIDE).APIKey; got != "" {
		t.Fatalf("other-origin route inherited API key %q", got)
	}
	cfg.AgentModels.IDE.ProfileID = "other-provider"
	if got := ResolveAgentModel(cfg, AgentKindIDE).APIKey; got != "" {
		t.Fatalf("other-provider route inherited API key %q", got)
	}
}

func TestMergeModelProfilesScopesRouteOwnedSettings(t *testing.T) {
	parent := ModelProfileSettings{
		ID:              "default",
		Provider:        string(providers.ProviderDeepSeek),
		Protocol:        string(providers.ProtocolOpenAIChatCompletions),
		APIKey:          "secret",
		BaseURL:         "https://api.deepseek.com",
		Headers:         map[string]string{"X-Tenant": "tenant"},
		ProtocolOptions: map[string]any{"thinking_toggle": "nested"},
		SessionKeyMapping: &providers.SessionKeyMapping{
			Location: providers.SessionKeyLocationHeader, Name: "X-Session-Id",
		},
	}

	otherOrigin := mergeModelProfiles([]ModelProfileSettings{parent}, []ModelProfileSettings{{
		ID:      "default",
		BaseURL: "https://gateway.example.test/v1",
	}})[0]
	if otherOrigin.APIKey != "" || otherOrigin.Headers != nil || otherOrigin.ProtocolOptions != nil || otherOrigin.SessionKeyMapping != nil {
		t.Fatalf("endpoint change retained route-owned settings: %#v", otherOrigin)
	}

	sameOriginNewProtocol := mergeModelProfiles([]ModelProfileSettings{parent}, []ModelProfileSettings{{
		ID:       "default",
		Protocol: string(providers.ProtocolAnthropicMessages),
		BaseURL:  "https://api.deepseek.com/anthropic",
	}})[0]
	if sameOriginNewProtocol.APIKey != "secret" || sameOriginNewProtocol.Headers["X-Tenant"] != "tenant" {
		t.Fatalf("same-origin protocol change lost credentials: %#v", sameOriginNewProtocol)
	}
	if sameOriginNewProtocol.ProtocolOptions != nil {
		t.Fatalf("protocol change retained incompatible options: %#v", sameOriginNewProtocol.ProtocolOptions)
	}
	if sameOriginNewProtocol.SessionKeyMapping != nil {
		t.Fatalf("protocol change retained incompatible session key mapping: %#v", sameOriginNewProtocol.SessionKeyMapping)
	}

	explicitMapping := mergeModelProfiles([]ModelProfileSettings{parent}, []ModelProfileSettings{{
		ID:       "default",
		Protocol: string(providers.ProtocolOpenAIResponses),
		SessionKeyMapping: &providers.SessionKeyMapping{
			Location: providers.SessionKeyLocationBody, Name: "session_id",
		},
	}})[0]
	if explicitMapping.SessionKeyMapping == nil || explicitMapping.SessionKeyMapping.Location != providers.SessionKeyLocationBody || explicitMapping.SessionKeyMapping.Name != "session_id" {
		t.Fatalf("explicit session key mapping = %#v", explicitMapping.SessionKeyMapping)
	}
}

func TestMergeModelProfilesResetsInheritedRoutingDefaultsWhenProviderChanges(t *testing.T) {
	for _, parent := range []ModelProfileSettings{
		{
			ID: "default", BaseURL: "https://api.deepseek.com", Model: "deepseek-chat",
		},
		{
			ID: "default", Provider: string(providers.ProviderDeepSeek), Protocol: string(providers.ProtocolOpenAIChatCompletions),
			BaseURL: "https://api.deepseek.com", Model: "deepseek-chat",
		},
	} {
		profiles := mergeModelProfiles(
			[]ModelProfileSettings{parent},
			[]ModelProfileSettings{{ID: "default", Provider: string(providers.ProviderOpenAI), Model: "gpt-5"}},
		)
		resolved := ResolveAgentModel(&Config{ModelProfiles: profiles}, AgentKindIDE)
		if resolved.Provider != string(providers.ProviderOpenAI) || resolved.Protocol != "" {
			t.Fatalf("merged routing = %q/%q", resolved.Provider, resolved.Protocol)
		}
		if resolved.BaseURL != "" {
			t.Fatalf("provider change retained inherited endpoint %q", resolved.BaseURL)
		}
	}
}

func TestResolveAgentModelDoesNotReuseLegacyEndpointForExplicitCompatibleProvider(t *testing.T) {
	resolved := ResolveAgentModel(&Config{
		OpenAIAPIKey:  "legacy-secret",
		OpenAIBaseURL: "https://api.deepseek.com",
		OpenAIModel:   "deepseek-chat",
		ModelProfiles: []ModelProfileSettings{{
			ID:       "default",
			Provider: string(providers.ProviderOpenAICompatible),
			Model:    "custom-model",
		}},
	}, AgentKindIDE)
	if resolved.Provider != string(providers.ProviderOpenAICompatible) ||
		resolved.Protocol != "" {
		t.Fatalf("routing = %q/%q", resolved.Provider, resolved.Protocol)
	}
	if resolved.BaseURL != "" {
		t.Fatalf("compatible provider inherited legacy endpoint %q", resolved.BaseURL)
	}
	if resolved.APIKey != "" {
		t.Fatalf("compatible provider inherited legacy API key %q", resolved.APIKey)
	}
}

func TestResolveAgentModelClearsInheritedDefaultProfileAlias(t *testing.T) {
	profiles := mergeModelProfiles(
		[]ModelProfileSettings{{ID: "default", Name: "DeepSeek 写作", Model: "deepseek-v4-pro"}},
		[]ModelProfileSettings{{ID: "default", Model: "deepseek-v4-pro"}},
	)
	if len(profiles) != 1 || profiles[0].Name != "" {
		t.Fatalf("default profile alias should be cleared: %#v", profiles)
	}

	resolved := ResolveAgentModel(&Config{ModelProfiles: profiles}, AgentKindIDE)
	if resolved.ProfileID != "default" || resolved.Model != "deepseek-v4-pro" {
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
			{Model: " gpt-4.1 ", Name: " Fast model "},
			{ID: " legacy "},
		},
	})
	if settings.ModelProfiles[0].ID != "gpt-4.1" || settings.ModelProfiles[0].Model != "gpt-4.1" {
		t.Fatalf("model-name profile not normalized: %#v", settings.ModelProfiles[0])
	}
	if settings.ModelProfiles[0].Name != "Fast model" {
		t.Fatalf("model alias not normalized: %#v", settings.ModelProfiles[0])
	}
	if settings.ModelProfiles[1].ID != "legacy" || settings.ModelProfiles[1].Model != "legacy" {
		t.Fatalf("legacy id profile should keep working: %#v", settings.ModelProfiles[1])
	}
}

func TestSanitizeModelProfilesKeepsIncompleteDraft(t *testing.T) {
	contextWindow := DefaultContextWindowTokens
	settings := sanitizeEditableSettings(Settings{
		ModelProfiles: []ModelProfileSettings{
			{
				Name:                "  Draft provider  ",
				APIKey:              "draft-key",
				BaseURL:             " https://api.example.com/v1 ",
				ContextWindowTokens: &contextWindow,
			},
		},
	})

	if len(settings.ModelProfiles) != 1 {
		t.Fatalf("sanitized model profiles length = %d, want 1", len(settings.ModelProfiles))
	}
	draft := settings.ModelProfiles[0]
	if draft.ID != "" || draft.Model != "" {
		t.Fatalf("incomplete draft must stay ineligible for model resolution: %#v", draft)
	}
	if draft.Name != "Draft provider" || draft.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("incomplete draft fields were not normalized: %#v", draft)
	}
	if draft.ContextWindowTokens == nil || *draft.ContextWindowTokens != DefaultContextWindowTokens {
		t.Fatalf("incomplete draft context window was not retained: %#v", draft)
	}
}

func TestWriteSettingsFileKeepsIncompleteModelDraft(t *testing.T) {
	contextWindow := DefaultContextWindowTokens
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteSettingsFile(path, Settings{
		ModelProfiles: []ModelProfileSettings{{
			Name:                "Draft provider",
			APIKey:              "draft-key",
			BaseURL:             "https://api.example.com/v1",
			ContextWindowTokens: &contextWindow,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	saved, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.ModelProfiles) != 1 {
		t.Fatalf("saved model profiles length = %d, want 1", len(saved.ModelProfiles))
	}
	draft := saved.ModelProfiles[0]
	if draft.ID != "" || draft.Model != "" {
		t.Fatalf("incomplete draft must remain ineligible after a write/read round trip: %#v", draft)
	}
	if draft.Name != "Draft provider" || draft.APIKey != "draft-key" || draft.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("incomplete draft was not preserved after a write/read round trip: %#v", draft)
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
	if settings.ModelProfiles[0].Model != "" {
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
				APIKey:              "profile-key",
				BaseURL:             "https://api.openai.com/v1",
				Model:               "gpt-4.1",
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
