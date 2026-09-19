package config

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
)

func TestRuntimeAPIProfileResolution(t *testing.T) {
	for _, kind := range []RuntimeID{RuntimeCodex, RuntimeClaude} {
		t.Run(string(kind), func(t *testing.T) {
			provider := "openai"
			selection := RuntimeSelection{Kind: kind, Codex: &CodexRuntimeSettings{ProfileID: "custom"}}
			if kind == RuntimeClaude {
				provider = "anthropic"
				selection.Codex = nil
				selection.Claude = &ClaudeRuntimeSettings{ProfileID: "custom"}
			}
			cfg := Config{ModelProfiles: []ModelProfileSettings{{ID: "custom", EndpointID: "route", Model: "custom-model"}}, ModelEndpoints: []ModelEndpointSettings{{ID: "route", Provider: provider, APIKey: "private-test-key", Headers: map[string]string{"X-Route": "tenant"}}}}
			if err := selection.Validate(AgentKindIDE); err != nil {
				t.Fatal(err)
			}
			got, err := ResolveRuntimeModel(&cfg, selection)
			if err != nil || got.Model != "custom-model" || got.APIKey != "private-test-key" || got.BaseURL == "" || got.Headers["X-Route"] != "tenant" {
				t.Fatalf("profile was not resolved: model=%q error=%v", got.Model, err)
			}
			encoded, _ := json.Marshal(selection)
			if strings.Contains(string(encoded), "private-test-key") || strings.Contains(string(encoded), "base_url") {
				t.Fatal("selection persisted credentials")
			}
			got.Headers["X-Route"] = "changed"
			if cfg.ModelEndpoints[0].Headers["X-Route"] != "tenant" {
				t.Fatal("resolved routing aliases saved settings")
			}
			cfg.ModelEndpoints[0].Protocol = "openai-chat-completions"
			if _, err := ResolveRuntimeModel(&cfg, selection); !errors.Is(err, ErrRuntimeModelProfile) {
				t.Fatal("accepted incompatible protocol")
			}
			cfg.ModelEndpoints[0].Protocol = ""
			cfg.ModelEndpoints[0].ProtocolOptions = map[string]any{"unsupported": true}
			if _, err := ResolveRuntimeModel(&cfg, selection); !errors.Is(err, ErrRuntimeModelProfile) {
				t.Fatal("discarded protocol options")
			}
			cfg.ModelEndpoints[0].ProtocolOptions = nil
			cfg.ModelEndpoints[0].SessionKeyMapping = &providers.SessionKeyMapping{Location: providers.SessionKeyLocationBody, Name: "session"}
			if _, err := ResolveRuntimeModel(&cfg, selection); !errors.Is(err, ErrRuntimeModelProfile) {
				t.Fatal("discarded session mapping")
			}
			cfg.ModelEndpoints = nil
			if _, err := ResolveRuntimeModel(&cfg, selection); !errors.Is(err, ErrRuntimeModelProfile) {
				t.Fatal("missing endpoint fell back")
			}
			cfg.ModelProfiles = nil
			if _, err := ResolveRuntimeModel(&cfg, selection); !errors.Is(err, ErrRuntimeModelProfile) {
				t.Fatal("missing profile fell back")
			}
		})
	}
}

func TestRuntimeModelSourcesAreExclusive(t *testing.T) {
	for _, settings := range []CodexRuntimeSettings{{ProfileID: " "}, {ProfileID: "p", Model: "m"}, {ProfileID: "p", Effort: "high"}} {
		if settings.validate() == nil {
			t.Fatalf("accepted conflicting selection: %#v", settings)
		}
	}
}
